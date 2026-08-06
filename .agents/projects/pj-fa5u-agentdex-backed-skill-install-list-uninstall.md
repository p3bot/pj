---
id: pj-fa5u
status: todo
order: "aE"
created: "2026-08-06T10:58:36+10:00"
summary: Implement pj skill install/list/uninstall via agentdex skills paths (primary vs native-else-shared, S3 uninstall, no ledger).
---

# Agentdex-backed skill install list uninstall

## Goal

Replace the hard-refuse `pj skill install`, `pj skill list`, and `pj skill uninstall` placeholders with a real, agentdex-backed implementation that installs the embedded agent skill contract into agent skills directories, lists those installs, and removes them safely. Path lookup for each agent's skills roots must come from the agentdex library catalog — never from hardcoded Claude/Grok/OpenCode paths.

## Scope

In scope:

- Depend on `github.com/p3bot/agentdex` (published module; pin a released tag such as v0.0.2 or newer compatible).
- Implement `pj skill install [agents...] [--local]`.
- Implement `pj skill list [--local]`.
- Implement `pj skill uninstall [agents...] [--local]`.
- Wire path resolution through agentdex (`Open`, agent list/get, skills matrix, EnrichNone).
- Update the embedded skill contract (`internal/skill/skill.md`) and refuse message so install/list/uninstall document real behaviour.
- Tests that inject agentdex boundaries (working dir, look path, catalog dir or fixtures) without relying on the host machine's installed agents.

Out of scope:

- Changing the content or structure of the skill contract beyond install/list/uninstall command docs and any discovery wording that still says hard-refuse.
- Auto-install on `pj scope init` or any automatic tree write.
- Installing into alternatives role paths except when selected as primary.
- Per-agent isolation on shared `~/.agents/skills` beyond the path and blocker rules below.
- models.dev enrichment, agentdex CLI, or catalog publishing.
- XDG install ledger or tracking state for skill copies.
- Skill install for products other than the single embedded `pj` skill.
- Windows native support beyond whatever agentdex already targets (Linux, macOS, WSL).

## Current State

- `pj skill` prints the embedded contract from `internal/skill/skill.md` (sole runtime source).
- `pj skill install|list|uninstall` hard-refuse with a shared message naming agentdex as the planned backend (`internal/cli/skill.go`).
- P1–P7 are landed; skill discovery is print-only plus refuse placeholders (project `pj-r748`).
- agentdex is a pure-Go library at `github.com/p3bot/agentdex` (also sibling tree `../agentdex`). Published Go tags include v0.0.1 and v0.0.2. Catalog is a separate CUE module fetched at runtime.
- agentdex reports per-agent skills as classified roots per scope (global/local): agents (shared `.agents`), native (product tree), alternatives; primary is derived agents → native → alternatives[0]. Paths resolve whether or not the binary is found; `Found` only gates binary/version.
- Layout convention: `<skills_root>/<name>/SKILL.md` with frontmatter `name:`. Existing manual installs use `pj/SKILL.md` and `name: pj`.

## References

- agentdex Go module: `github.com/p3bot/agentdex` (tag v0.0.2 at design time); package docs and README describe Index, Agents.List/Get, Detection.Skills, EnrichNone, errors.
- Sibling checkout (optional local read): `/home/grant/Projects/p3bot/agentdex` — same module; AGENTS.md and `docs/agents-skills-path-matrix.md` document skills roles and catalogued agents.
- CUE catalog module: `github.com/p3bot/agentdex/catalog@v1` (runtime fetch; not a pj Go import).
- `internal/cli/skill.go` — current refuse placeholders.
- `internal/skill/skill.md` — embedded contract to update for the new verbs.
- Archived design notes (`docs/archive/design.md`) — historical discovery intent only; this project document and the code override archive prose.
- Project writing guide: `start get project/writing`.

## Requirements

### Skill identity

1. On-disk skill id is `pj`. Install writes `<skills_root>/pj/SKILL.md` with content exactly equal to `skill.Text()` (embedded contract, including frontmatter `name: pj`).
2. Uninstall recognises a skill directory as owned when the directory is named `pj`, contains only `SKILL.md`, and that file's frontmatter `name` is `pj`. Body need not equal the current embedded text (hand-edited copies still uninstall).

### agentdex integration

3. Open an agentdex Index with options suitable for CLI and tests (at least working directory for local roots; injectable look path / env for tests). Use EnrichNone for all skill path operations (no models.dev requirement).
4. Agent paths come only from agentdex detection/catalog expansion. Do not hardcode per-product skills directories.
5. An agent has a "skills concept" when agentdex reports non-empty skills path data for it (skills not omitted in the catalog).

### Agent sets

6. Default agent set (install with no agent args, uninstall with no agent args, list): every agent with Detection.Found and a skills concept.
7. Explicit agent ids (install and uninstall only): space-separated positionals. May include not-installed (!Found) agents; paths still resolve from the catalog. Unknown id fails. Id with no skills concept fails. Id with no writable path under the path rule fails.
8. Empty default set on install or uninstall fails (clear error). Empty default set on list succeeds with empty inventory (exit 0).
9. `pj skill list` takes no agent positionals — only optional `--local`.

### Location scope

10. Default location is global skills roots. `--local` selects project-local roots only (working directory base as agentdex uses). No flag writes both in one invocation; both scopes requires two commands.

### Path rules — install

11. No agent args: for each agent in the default set, target path is Primary (agentdex derivation: agents → native → alternatives[0]) at the selected location scope. De-dupe by absolute path; write once per path.
12. One or more agent args: for each id, target path is Native if non-empty, else Shared (catalog agents role) at the selected location scope. De-dupe by absolute path; write once per path.
13. Create parent directories as needed (`MkdirAll` on `…/pj`). Overwrite existing `SKILL.md` on that path. Hard fail if the skills root path exists and is not a directory, or create/write fails.

### Path rules — candidates (uninstall and list)

14. For agent `a` at a location scope, candidates(a) is the unique non-empty absolute paths among Primary(a), Native(a), and Shared(a) (agents role). Do not add the full alternatives list; primary already covers alternatives[0] when that is primary.

### Uninstall (S3)

15. Let S be the uninstall set (default set or explicit ids). Let R be installed agents with a skills concept that are not in S.
16. For each absolute path P in the union of candidates(a) for a in S:
    - If some b in R has P in candidates(b), keep any skill install at P and report that it was kept (list blockers). Not an error.
    - Otherwise apply the remove rule for directory D = P/pj.
17. Remove rule for D = P/pj:
    - D missing → treat as absent (not an error).
    - D not a directory → hard fail for that path.
    - D must contain only SKILL.md; any other entry → keep and report (do not delete).
    - SKILL.md frontmatter name must be `pj`; otherwise keep and report.
    - If checks pass, remove the entire directory D (not only the file). Do not remove the skills root P.
18. Report per path: removed, kept (with blockers or reason), or absent as appropriate. Exit 0 when every path was handled without hard failure.

### List

19. `pj skill list [--local]` inventories existing installs for the default agent set only (installed + skills concept). No agent args.
20. Emit one row per unique path where candidates for that set contain a present `…/pj/SKILL.md`. Prefer full path to SKILL.md. Optionally include which agents in the set claim that path. Sort by path. No rows for absent candidates. Orphans for not-installed agents are not listed.

### Errors and recovery

21. Catalog unavailable or invalid: fail closed; do not invent paths. Stderr must instruct the user to run `pj skill` and install the skill manually into their agent's skills directory. Map agentdex sentinels clearly (token style consistent with pj where a closed token is appropriate).
22. Usage-class failures (unknown agent, no-skills agent, empty install/uninstall default set, no writable path for explicit id): non-zero exit consistent with similar pj usage errors (exit 2 where that is the project convention for bad input).
23. Catalog and I/O failures: ordinary failure exit (not silent success).

### Contract and CLI surface

24. Replace hard-refuse implementations with the real verbs. Update skill.md Commands (and any refuse wording elsewhere in the contract) to describe install/list/uninstall accurately.
25. Install remains user-initiated only; never automatic on scope init or other mutators.
26. Discovery command family remains ambient-scope free (no registered scope required), matching `pj skill` today.

## Constraints

- Pure Go, no cgo (agentdex is pure Go; do not introduce cgo).
- Module path `github.com/p3bot/pj`; match repo Go version.
- Follow existing CLI patterns: Cobra under `internal/cli`, path-centric stdout where appropriate, tokens/warnings on stderr, ExitError / exit codes as used by peer commands.
- agentdex is path authority for agent skills roots; shared string constants for product paths are forbidden.
- MPL-2.0 agentdex dependency is acceptable as a library dependency; do not copy agentdex source into the tree.
- Do not load design.md at runtime; skill.md remains the sole embedded skill source.
- Scoped commits if the implementer commits; repo mode is repo-driven (host commit).

## Implementation Plan

1. Add the agentdex module dependency and a thin internal adapter (or direct use) that opens an Index, resolves agent sets, and returns absolute skill roots for Primary / Native / Shared at global or local scope. Keep detection enrichment at EnrichNone.
2. Implement shared skill-path helpers: join `pj/SKILL.md`, parse frontmatter name for ownership check, directory purity check, write install, remove directory under uninstall rules.
3. Implement install, list, uninstall command logic per Requirements; replace refuse stubs in `internal/cli/skill.go`.
4. Update `internal/skill/skill.md` command rows and any hard-refuse language for install/list/uninstall.
5. Tests: fixture catalog or agentdex test doubles; temp dirs for skills roots; cover default vs named path rules, `--local`, de-dupe, multi-tenant keep, ownership/purity refuse, empty list, catalog failure message, explicit !Found path install/uninstall.
6. Run format, vet, package tests for touched packages; fix regressions in existing skill tests that expect hard refuse.

## Implementation Guidance

- Prefer testing with `agentdex.WithCatalogDir` pointing at a minimal valid catalog module and `WithWorkingDir` / `WithLookPath` / `WithEnvLookup` so host PATH and home do not leak. The agentdex repo's testdata and catalogtest packages may inform fixtures; do not require the agentdex CLI binary at test time.
- Shared primaries (many agents → one `~/.agents/skills`) make path de-dupe and the R-blocker rule essential; implement those as first-class behaviour, not afterthoughts.
- Frontmatter name check for uninstall can reuse or mirror existing frontmatter parsing patterns in the tree if they fit; a minimal YAML name extract is enough if full FM parse is heavier than needed.
- list output should stay simple and path-centric (TSV or one path per line). Match surrounding CLI style rather than inventing a JSON envelope.
- Token names for catalog failures should fit `internal/token` conventions if the project adds closed tokens; otherwise clear plain stderr consistent with the current skill refuse style is acceptable if that matches peer discovery commands.

## Acceptance Criteria

1. `pj skill install` with no args writes `pj/SKILL.md` (content = `skill.Text()`) to each unique Primary of installed agents that have skills, at global scope; creates directories as needed.
2. `pj skill install <id>…` writes Native if set else Shared for each id; supports !Found agents when the catalog has skills paths.
3. `pj skill install --local` (and uninstall/list `--local`) use project-local roots only.
4. `pj skill list` shows existing `…/pj/SKILL.md` paths for the default installed set only; no agent positionals; empty set yields empty success.
5. `pj skill uninstall` with no args removes owned pure `pj/` dirs under candidates of the default set when unblocked; with args, same for S; multi-tenant paths kept with a non-error report when R still claims the path.
6. Uninstall does not delete `pj/` when extra files exist or frontmatter name is not `pj`; does not require body to match embedded text; removes the whole `pj/` directory when checks pass.
7. Catalog unavailable/invalid fails without writing; message tells the user to use `pj skill` for manual install.
8. Embedded skill contract documents the real install/list/uninstall behaviour; hard-refuse placeholders are gone.
9. Tests cover path rules, blockers, ownership checks, and catalog failure without depending on the developer's personal agent installs.
