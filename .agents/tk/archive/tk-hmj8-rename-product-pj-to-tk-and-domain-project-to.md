---
id: tk-hmj8
status: done
order: "aK"
created: "2026-08-08T13:17:26+10:00"
summary: Hard-cut product rename pj→tk and domain project→ticket; hybrid wire until cutover
---

# Rename product pj to tk and domain project to ticket

## Goal

Rename the product from `pj` to `tk` (tickets) and the domain unit from project to ticket so the short name matches the vocabulary agents and humans actually use. After cutover, shipping surfaces must not retain product-era `pj` naming or domain project/projects vocabulary for the markdown work item — including Go identifiers, SQL names, comments, and prose. Residual `pj` is limited to historical board ticket bodies, git history, `docs/archive/design.md` (pre-shipping design history; not live authority), and the README Upgrading section (the only live surface that must name the old product so operators can migrate).

## Scope

In scope:

- Product identity in this repository: Go module path, `cmd/` entry, Cobra command name, help text, embedded skill id and install path, XDG config/state dir names, scope lock filename, scope config filename, ambient scope env override (`PJ_SCOPE` → `TK_SCOPE`), durability mode token (`pj-driven` → `tk-driven`), self-commit message prefixes, stderr/help prose, package comments, and tests
- Full domain rename for the markdown work item: project/projects → ticket/tickets across live product surfaces and code — user-facing strings, skill, README, AGENTS.md, comments, Go types/functions/variables/tests that mean the work item (for example `Project` → `Ticket`, `LooksLikeProject` → an idiomatic ticket equivalent), and SQLite index table/query identifiers (`projects` → `tickets`) with `SchemaVersion` bump
- Intelligent English: rephrase for natural ticket vocabulary; do not mechanical word-swap. Prefer correct sentences ("when this ticket is done") over awkward substitutes that only change the noun
- Cross-repository shipping surfaces that still point at `pj`: sibling `homebrew-tap` formula shape and docs (see Homebrew formula), sibling `org` workspace `AGENTS.md` / `README.md` rows for this tool
- Documented one-shot machine cutover notes in the product README (or equivalent short upgrade section): install `tk`, use a fresh XDG state dir under `tk` (do not copy `index.db` from the old `pj` state dir), rename on-disk wire files, switch shell env from `PJ_SCOPE` to `TK_SCOPE`, re-import or rebind scopes so the new index is built from files, reinstall skill, retire `pj` binary/formula/skill

Out of scope:

- Machine-local registry surgery on developer machines (owner handles `scope forget` / import / rebind)
- Renaming other scopes' on-disk directories outside this repo (for example other trees still using `.agents/projects`); convention may move to tickets-oriented paths over time, not as a forced global rewrite in this ticket
- Dual-read or forever-compat shims that accept both `pj.cue` and `tk.cue`, both XDG roots, or both binary names
- Renaming library contexts/tasks under `library/contexts/project/` or `library/tasks/project/` (separate vocabulary migration outside this product)
- Changing CLI verb names, status catalogue, order grammar, id short-id grammar, or board behaviour beyond naming
- Git history rewrite; GitHub repo rename (already `https://github.com/p3bot/tk`)
- Creating the first post-cutover GitHub release tag, and filling the Homebrew formula stable `url` / `sha256` for that tag (operator after cutover lands; see Homebrew formula)
- Rewriting board ticket files under the scope directory (active or archive) to scrub historical `pj` or project wording — those bodies are historical work records, not a shipping surface
- Rewriting `docs/archive/design.md` to scrub product-era `pj` or domain project wording — pre-shipping design history, not live authority; allowed residual like board ticket bodies (optional follow-up outside this ticket)
- Carrying an old `pj` XDG `index.db` into the new `tk` state dir, dual-name DROP lists to support that path, or any row migrator for the index — cutover is fresh state dir plus re-import (see Index cutover)

## Current State

### Product tree

- Repository: `https://github.com/p3bot/tk` (GitHub about: Ticket management for AI agents). Local checkout under the p3bot org workspace as `tk/`
- Module path and binary are still `github.com/p3bot/pj` and `pj` (`cmd/pj`, Cobra `Use: "pj"`)
- Wire names hardcoded for the current binary: scope config `pj.cue`, lock `.pj.lock`, XDG config and state directories named `pj`, ambient scope env `PJ_SCOPE`, skill id/install dir `pj`, durability mode label `pj-driven`
- Domain language still says project for the markdown work item in skill, README, AGENTS.md, index schema (`projects` table, `Project` type, `LooksLikeProject`, and related names), comments, and much of the tree
- `docs/archive/design.md` is large pre-shipping design history with dense `pj` and project wording; it is not live authority and is left as historical residual in this ticket (not rewritten)
- Homebrew formula still ships as `p3bot/tap/pj` from `homebrew-tap/Formula/pj.rb` pointing at the old module/paths
- Org workspace docs still list `pj/` and `github.com/p3bot/pj` in tables

### Index cutover (closed decision — do not re-open)

Operator path after the binary hard-cuts to XDG dir name `tk`:

1. Build and run `tk` — config/state resolve under the new `tk` XDG names, not `pj`
2. The new state directory has no `index.db`; the first open creates a fresh file with the post-rename schema (`tickets`, new `SchemaVersion`)
3. Re-import or rebind scopes as needed; reconcile fills the index from markdown on disk

Do not copy or rename `index.db` from the old `pj` state dir into `tk`. Do not design dual-name DROP lists, stranded-table recovery, or row migration for a carried file. A pure identifier rename `projects` → `tickets` in CREATE/DROP/query/`SchemaText` plus a `SchemaVersion` bump is enough: the intended machines never open a pre-rename database under the new product. The old `pj` state dir may be deleted at operator convenience; it is not an input to `tk`.

### Homebrew formula (closed decision — do not re-open)

Existing release tags (`v0.0.1`, `v0.0.2`) still ship `module github.com/p3bot/pj` and `cmd/pj`. A correct stable bottle URL for `tk` only exists after the operator tags a post-cutover commit. This ticket does not create that tag.

In this ticket, shape the formula only:

- `git mv` (or equivalent) `Formula/pj.rb` → `Formula/tk.rb`; class, `desc`, homepage, `go build` path (`./cmd/tk`), and test assertions name `tk`
- Tap README / AGENTS tables list `tk`, not `pj` as the live tool
- Remove or supersede the live `pj` formula name so the tap does not keep advertising `pj`

Leave stable `url` and `sha256` for the operator to set when the first post-cutover tag exists (placeholder, comment, or temporary values are fine; do not invent a tag or point at a pre-cutover tarball under the new name as if it were `tk`). This ticket does not require `brew install` to succeed against a stable URL before that tag. `go install` / build-from-source remain the immediate install paths.

### Hybrid board already in place (do not undo)

This scope already runs under the installed `pj` binary with board identity `tk`:

- Scope directory: `.agents/tk/` (not `projects/`)
- On-disk wire files still use product-era names the current binary requires: `pj.cue`, `.pj.lock` in gitignore
- `pj.cue` `name` field is `"tk"`; ticket full ids and filenames are `tk-<short>-…`
- Ambient workflow until cutover: invoke `pj …`

Layer rule that removes the chicken/egg trap:

- Board identity (scope name, ticket ids, scope directory path) may already be `tk`
- Product wire names track the installed binary and stay `pj` until the code cutover lands
- A half-renamed binary (install path renamed without code changes) is forbidden

Tracked scope wires in this product tree (`.agents/tk/pj.cue`, `.agents/tk/.gitignore` lock line) are shipping tree content, not machine-local only. Mid-implementation they stay as `pj.*` so the installed `pj` binary can still drive the board. In the same cutover change set that hard-cuts the code to `tk.cue` / `.tk.lock`, `git mv` those tracked paths to the new names so a post-cutover clone builds `tk` and resolves this scope without a second local rename. Operator Upgrading notes still cover wire renames on other machines' scopes that are not part of this tree.

### Durability

- This scope is repo-driven (`autoCommit: false`): host git commits ticket document edits; no `pj sync` for board files

## References

- Product skill contract (runtime source of truth for agent CLI usage): `internal/skill/skill.md` (also printed by `pj skill` until cutover)
- Agent guidance for this repo: `AGENTS.md`
- User-facing overview: `README.md`
- Archived design history (residual; not rewritten here): `docs/archive/design.md`
- Org workspace map: sibling org `AGENTS.md` and `README.md` when this checkout sits beside the org workspace
- Homebrew tap: sibling `homebrew-tap/Formula/pj.rb` and tap README/AGENTS tables
- GitHub repository: https://github.com/p3bot/tk
- Ticket writing guide (library context; its path still says project): p3bot library `contexts/project/writing` — structure and principles only; this document is a ticket

## Requirements

1. Product short name is `tk` everywhere that currently identifies the CLI as `pj` (binary name, Cobra use, go install path, skill id, help, docs).
2. Go module path is `github.com/p3bot/tk`; entry is `cmd/tk`; imports and `go.mod` match.
3. Wire filenames and machine paths after cutover are `tk.cue`, `.tk.lock`, XDG config/state under `tk` (not `pj`). Ambient scope env override is `TK_SCOPE` only (not `PJ_SCOPE`). No dual-read of the old names. Tracked scope wires in this product tree (`.agents/tk/pj.cue` → `tk.cue`, and the scope `.gitignore` lock line `.pj.lock` → `.tk.lock`) rename in the same cutover change set as the code hard cut — not as a post-merge local-only operator step.
4. Durability mode token and all user-visible labels that said `pj-driven` say `tk-driven`. `repo-driven` and `plain-files` stay.
5. Domain noun for the markdown work item is ticket/tickets everywhere on the shipping surface: skill, README, AGENTS.md, help, stderr, comments, Go identifiers (types, functions, variables, test names) that mean the work item, and SQLite index identifiers. Keep scope, board, lens, status, mark, and other non-domain terms. Intelligent English only — rephrase so sentences read naturally under ticket vocabulary; never leave clumsy mechanical swaps.
6. Glossary for agents and docs:
   - ticket — one markdown work item (frontmatter + single ATX H1 + body)
   - scope — directory of tickets plus scope config file
   - board — the set of tickets in a scope as presented by list/status/next
   - tk — the CLI product
   - tk-driven — auto-commit durability mode for scopes
7. Product-identity sweep (zero shipping `pj`): after cutover, a word-boundary search over the defined sweep surface finds no product-identity `pj` (module paths, `pj.cue`, `.pj.lock`, `PJ_SCOPE`, `pj-driven`, skill id `pj`, `cmd/pj`, binary/help names, self-commit message prefixes). Sweep surface: Go sources and tests, `go.mod`/`go.sum`, `cmd/`, embedded skill, package comments, live `README.md` and `AGENTS.md` outside the named Upgrading section. Not on the sweep surface (allowed residual): git history; board ticket files under the scope directory (active and archive), including this ticket — historical work records; `docs/archive/design.md` — pre-shipping design history, not live authority; the README Upgrading section (and equivalent short upgrade notes) that must name the old product so operators can migrate; true false positives after judgement (for example a substring that is not the product). Prefer zero hits on the sweep surface. Outside those residuals, `pj` must not appear.
8. Domain sweep (zero shipping domain-project): after cutover, shipping surfaces do not use project/projects for the markdown work item. Go identifiers that meant the work item use ticket vocabulary (`Ticket`, ticket table names, idiomatic renames of helpers such as `LooksLikeProject`). SQL table/indexes that were `projects` are `tickets`. Prose and comments use natural ticket English. Allowed residual: board ticket bodies (historical); `docs/archive/design.md` (same residual class as board bodies); true non-domain English where "project" does not mean the markdown work item (rare — judge, do not force); git history. Prefer zero domain-project hits on the same sweep surface as requirement 7 (plus Go identifier and SQL name checks).
9. `docs/archive/design.md` is not rewritten in this ticket. Leave product-era `pj` and domain project wording in place as historical residual. Do not delete the archive as a shortcut. Optional polish is a separate follow-up if ever wanted; it does not block this cutover.
10. Embedded skill and `tk skill install` publish under skill id `tk` (install path `…/tk/SKILL.md`). Old `pj` skill installs are retired by uninstall guidance or cutover notes, not left as the documented install.
11. Sibling `homebrew-tap` gains a code-shaped `tk` formula and docs (Homebrew formula): module/homepage/`./cmd/tk`/class/filename target `tk`; `pj` is removed or superseded as the live tool name. Stable `url`/`sha256` for a post-cutover tag are operator-owned and may remain unset or placeholder in this ticket — not a fail.
12. Sibling org workspace docs that describe this tool list `tk` and `github.com/p3bot/tk`, not `pj`.
13. README includes a short Upgrading (from `pj`) section for machine operators (install `tk`, wire file renames, fresh XDG state under `tk` with no copied `index.db`, re-import/rebind scopes, optional `tk doctor --reindex`, `PJ_SCOPE`→`TK_SCOPE` in shell profiles, skill, brew when the operator has filled the formula URL after tagging, retire `pj`). That section is the only live shipping residual that may name the old product (requirement 7). Registry operations may be described as operator steps; do not implement a special registry migrator unless it falls out naturally from existing scope admin verbs.
14. SQLite/index identifiers that say `projects` for the ticket table are renamed to `tickets` in this ticket (not deferred): bump `SchemaVersion`, update CREATE/DROP/query/`SchemaText` with a pure name rename (DROP lists the new table name `tickets` as for any other schema object), and keep the existing mismatch-rebuild behaviour — no hand-written row migration, no dual-name DROP list for the pre-rename `projects` table. Cutover does not open a pre-rename database: operators use a fresh `tk` XDG state dir and re-import scopes (Index cutover). Do not add code or acceptance criteria aimed at carrying `index.db` across the product rename.

## Constraints

- Hard cut at the current early version line: no compatibility layer that accepts both old and new wire names in one binary
- Pure Go, no cgo; keep existing platform support (macOS and Linux only) and existing external `git` shell-out model
- Do not commit, push, tag, or publish without explicit operator instruction
- Mid-implementation: keep this scope's live wire files as `pj.cue` / `.pj.lock` so the installed `pj` binary still drives the board; do not half-rename wires while code still expects `pj.*`
- Cutover change set: hard-cut code and tests to `tk.cue` / `.tk.lock` and, in the same change set, rename this repo's tracked scope wires (`.agents/tk/pj.cue` → `tk.cue`, `.gitignore` lock entry → `.tk.lock`) via `git mv` / edit so a clone after cutover works with `tk` without a second local rename
- Other machines' scopes (not tracked in this tree): operator renames wires at install time per README Upgrading; no dual-read in the binary
- Do not hand-edit ticket frontmatter keys with ad hoc tools when `pj`/`tk` mutators exist for the workflow; this ticket document's frontmatter stays tool-owned
- Agent-facing markdown in skill and ticket-oriented docs: no bold, italic, emojis, horizontal rules, or heading depth beyond `###`
- Cross-repo edits stay limited to homebrew-tap and org docs that name this product; do not expand into unrelated p3bot tools
- Prefer one coherent cutover over a long branch that leaves the tree half `pj` and half `tk` on `main`

## Implementation Plan

1. Inventory every product-identity and domain-noun surface in this repo (module path, cmd, constants for cue/lock/XDG/skill/mode, ambient scope env `PJ_SCOPE`, self-commit prefixes, skill text, README, AGENTS.md, tokens, Go domain identifiers, SQL names, tests) and the two sibling doc/formula surfaces. Skip `docs/archive/design.md` and board ticket bodies (residual). Use the inventory to drive the rename, not drive-by edits.
2. Rename the Go module and command tree to `github.com/p3bot/tk` / `cmd/tk`. Update all imports. Set Cobra and user-facing command name to `tk`.
3. Change wire constants and authors: scope config filename `tk.cue`, lock `.tk.lock`, XDG dir `tk`, ambient scope env `TK_SCOPE` (replace every `PJ_SCOPE` read and help string), skill id/path `tk`, mode `tk-driven`, self-commit message prefixes `tk:`. Update tests and fixtures that write or assert the old names.
4. In the same cutover change set as step 3: rename this repo's tracked scope wires — `git mv` `.agents/tk/pj.cue` → `.agents/tk/tk.cue`, and change `.agents/tk/.gitignore` from `.pj.lock` to `.tk.lock`. Leave board ticket bodies alone.
5. Full domain rename project → ticket: Go types/functions/variables/tests that mean the work item (for example `Project` → `Ticket`, helpers like `LooksLikeProject` → an idiomatic ticket name, `queryProjects` / `projectColumns` and peers), SQL `projects` → `tickets` with `SchemaVersion` bump and pure DROP/CREATE/query/`SchemaText` renames (Index cutover — no dual-name DROP), plus skill, help, README, AGENTS.md, and comments. Keep scope/board/lens vocabulary. Apply intelligent English in every prose edit. Leave `docs/archive/design.md` untouched.
6. Sweep product-identity (requirement 7) and domain-project (requirement 8) until the defined sweep surface is clean. Do not scrub board ticket bodies or `docs/archive/design.md`. Fix stragglers on the sweep surface only.
7. Update sibling homebrew-tap per Homebrew formula: shape `tk` formula and docs; remove or stop publishing `pj` as the live tool name; do not invent a post-cutover tag or a fake stable `sha256`.
8. Update sibling org AGENTS.md and README product rows to `tk`.
9. Add README Upgrading notes for operators upgrading from `pj` (wire files on other scopes, fresh `tk` XDG state with no copied `index.db`, re-import/rebind, env, skill, brew after operator tags and fills formula URL).
10. Run full test and lint suite for this module; fix fallout from renames.
11. Stop. Operator owns registry, first post-cutover GitHub release tag, Homebrew formula `url`/`sha256`, per-machine install order (fresh state dir, import scopes), and wire renames on scopes not tracked in this tree. Do not force registry edits as part of the code change.

## Implementation Guidance

- Replace order matters: longest and most specific strings first (`github.com/p3bot/pj`, `pj-driven`, `PJ_SCOPE`, `pj.cue`, `.pj.lock`, `cmd/pj`), then remaining product-identity `pj`, then domain project/projects — identifiers first where mechanical rename is safe, then prose with intelligent English
- Prefer `git mv` for `cmd/pj` → `cmd/tk`, formula file renames, and this repo's tracked `.agents/tk/pj.cue` → `tk.cue` so history follows the path
- The hybrid board state is intentional until cutover: mid-work keeps `pj.*` wires for the installed binary; after cutover, tests, docs, and this repo's tracked wires describe `tk` only — do not add code paths that preserve hybrid wire names
- Intelligent English (live surfaces only): rewrite the sentence so it sounds native under ticket vocabulary. Do not mechanical-replace the word project and leave broken grammar or stiff phrasing. Examples of intent: "when this project has finished" → "when this ticket is done"; "project management CLI" → "ticket management CLI" or "CLI for markdown tickets"; "one project per file" → "one ticket per file". Keep non-domain English only when "project" clearly means something else (for example a software/git project in the abstract) and ticket would be wrong
- Go domain identifiers: rename types, funcs, vars, and test names that mean the work item to ticket vocabulary with idiomatic Go names (`Ticket`, `LooksLikeTicket` or better if a clearer name fits, and peers). Update all call sites and tests in the same pass
- Do not edit `docs/archive/design.md` for this rename. It stays pre-`tk` design history and is not live authority relative to packages and the embedded skill
- Product-identity sweep follows requirement 7; domain sweep follows requirement 8. Board ticket bodies and `docs/archive/design.md` are historical residual. The Upgrading section must name the old product; that is the only live shipping residual for `pj`
- Homebrew class name, formula filename, build path, and test assertions must match the new binary name. Stable `url`/`sha256` stay operator-owned until a post-cutover tag exists (Homebrew formula) — do not treat pre-cutover tags as a finished `tk` bottle
- Index is a derived cache under the product XDG state dir. Cutover path is fixed (Index cutover): new `tk` state dir, no carried `index.db`, re-import scopes, reconcile builds `tickets`. Pure `projects`→`tickets` rename plus `SchemaVersion` bump; no row migrator; no dual-name DROP for a file that is not reused
- When in doubt whether a project string or identifier is domain vocabulary, treat it as the work item and rename; leave it only when ticket would be factually wrong English

## Acceptance Criteria

1. `go.mod` module path is `github.com/p3bot/tk`; `cmd/tk` is the only main entry; `go build` produces a `tk` binary whose root help identifies the product as `tk`.
2. Fresh scope init (or documented authoring path) writes `tk.cue` and ignores/uses `.tk.lock`; nothing in the shipping code looks for `pj.cue` or `.pj.lock`. This repo's tracked scope wires are `tk.cue` and `.tk.lock` in `.gitignore` after cutover so a clone builds and resolves the ambient scope with `tk` without a local wire rename.
3. XDG helpers resolve config/state under a `tk` directory name, not `pj`. Ambient resolve and help read `TK_SCOPE` only (not `PJ_SCOPE`).
4. Skill id is `tk`; `tk skill` text describes tickets and `tk` commands in natural English; install layout is `…/tk/SKILL.md`.
5. Mode labelling and skill durability text use `tk-driven` (not `pj-driven`). Self-commit messages use a `tk:` product prefix (not `pj:`).
6. Product-identity sweep (requirement 7) shows no remaining shipping `pj` outside allowed residuals (Upgrading, board ticket bodies, `docs/archive/design.md`). Domain sweep (requirement 8) shows ticket vocabulary for the work item in prose, comments, Go identifiers, and SQL names on the sweep surface — no residual domain-project there.
7. `docs/archive/design.md` is unchanged by this ticket (historical residual; not a fail under requirements 7–9).
8. `homebrew-tap` formula and docs target `tk` / `github.com/p3bot/tk` / `./cmd/tk` and no longer ship `pj` as the live name; stable `url`/`sha256` may still be operator placeholders (Homebrew formula). Org docs name `tk` / `github.com/p3bot/tk`.
9. README Upgrading section states operator steps from `pj` to `tk` (including `PJ_SCOPE`→`TK_SCOPE`, fresh `tk` XDG state with no copied `index.db`, and re-import/rebind) without requiring dual-read support in the binary (sole live shipping residual under requirement 7).
10. Index schema naming says `tickets` not `projects`, with `SchemaVersion` bumped and CREATE/DROP/query/`SchemaText` updated via pure rename (Index cutover; no dual-name DROP requirement).
