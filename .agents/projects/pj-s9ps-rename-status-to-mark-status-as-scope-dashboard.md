---
id: pj-s9ps
status: todo
order: "aC"
created: "2026-07-30T22:08:28+10:00"
summary: Rename status mutator to mark; repurpose status as a key/value scope pulse for agents
---
# Rename status to mark; status as scope dashboard

## Goal

Rename the status mutator to `pj mark` so the verb matches agent language (`mark blocked`, `mark done`), and repurpose `pj status` as a pure-read scope dashboard that prints a stable key/value pulse (counts, next, claimed, integrity). Agents get a single cheap orientation command instead of stitching `list`, `next`, and doctor tokens by hand.

## Scope

In scope:
- Rename the existing complete-state mutator `pj status <id> <status>` to `pj mark <id> <status>` with identical behaviour (archive boundary move, self-commit, refuses, path hand-off).
- Implement a new pure-read `pj status [--scope S]` that prints one `key\tvalue` line per field for the resolved scope.
- Help-group placement, root help membership, skill/help text, and tests for both verbs.
- Hard rename: no `status` mutator alias and no deprecation shim (no external consumers of the CLI contract yet).

Out of scope:
- `pj meta set|add|rm` (owned by `pj-p68k`).
- `pj list` column or TSV/CSV format changes (follow-up project).
- Changing status labels, categories, archive rules, or claim semantics beyond renaming the mutator command.
- Multi-scope dashboard (`--all`), JSON output, or colour beyond existing TTY policy.
- Repair or doctor behaviour; status reports integrity health, it does not fix it.

## Current State

- `pj status <id> <status>` (`internal/cli/status.go`) is the complete-state mutator: rewrite status, rename across the terminal boundary when needed, print post-write absolute path, self-commit on auto-commit scopes, `uncommitted:` on repo-driven dirty scopes.
- Root help places `status` under Work with `create`, `get`, `edit`, `reorder`, `next` (`internal/cli/root.go`).
- `pj next` / `pj next --claim` already define "next" selection (built-in `todo`, depends satisfied, lens, non-archive, non-duplicate) and claim → `in-progress`.
- Board inventory is `pj list` (TSV rows). Integrity diagnosis is `pj doctor` (token lines). There is no single orientation verb.
- Ambient resolution already yields scope name, dir, and how the scope was chosen (`internal/resolve`); autoCommit plus git-root presence distinguish pj-driven / repo-driven / plain-files modes used elsewhere in help and doctor.
- Product contract is not yet consumed by external agents; a hard rename is acceptable.

## Requirements

### Mark (mutator)

1. Command: `pj mark <id> <status> [--scope S]`.
2. Behaviour is a pure rename of today's `status` mutator: same argument rules, known-status check, archive boundary move, stdout path hand-off, mid-rebase refuse, quarantine/duplicate refuse, self-commit subject unchanged (`pj: <id> -> <status>`, same shape as today's status/claim messages — the verb rename does not rewrite git subjects), and repo-driven `uncommitted:` signalling.
3. Remove the top-level `status` mutator entry point. Do not leave a hidden alias.
4. Place `mark` in the Work help group in the slot `status` occupied (Work: `create`, `get`, `edit`, `mark`, `reorder`, `next`).
5. Update every in-tree user-facing string, test, and skill reference that documents `pj status <id> <status>` as the mutator.

### Status (dashboard)

6. Command: `pj status [--scope S]` with no positional args. Pure read: never run git for mutation; may run cheap git status porcelain only when needed for the `uncommitted` field (same allowlist idea as write-path / doctor repo-driven health). Reconcile path is not ambient-only list: the dashboard must use the same `reconcileClosure` (ambient scope plus transitive depended-on scopes) + `buildGate` path as `pj next`, so depends gates and the `next` field see fresh cross-scope targets. Other fields may share that single closure result; do not ambient-reconcile then re-select next with a forked gate.
7. Stdout is parse-stable `key\tvalue` lines, one key per line, in the locked order below. No header row. Empty values are still emitted as `key\t` (key present, value empty) so agents can rely on key presence. Exit contract: when ambient resolve succeeds, always emit the full locked key block and exit 0 — including when `next` is empty (no eligible project, empty under lens, or every todo held by deps). An empty queue is a normal pulse value (`next\t`), not a failure. Do not return `pj next`'s empty-queue plain diagnostic (`nothing ready…`) as the status command result; reuse next *selection* only. Usage (e.g. unexpected positionals) → exit 2; resolution failures → non-zero, as other pure reads.
8. Locked keys and meanings (value forms):

   Identity / resolution:
   - `scope` — resolved scope name
   - `dir` — absolute scope directory
   - `resolved` — how the scope was chosen: `cwd` | `flag` | `env` (or the resolve package's existing labels if they already form a closed set — use that set, document it in help)
   - `mode` — `pj-driven` | `repo-driven` | `plain-files` only (no fourth `unknown`). Derive from *known* autoCommit plus git-root: known true + git-root → `pj-driven`; known false + git-root → `repo-driven`; otherwise → `plain-files` (no git-root, or autoCommit unknown because schema is missing/unparseable). Never treat unknown autoCommit as repo-driven (matches doctor). `uncommitted` is non-zero only when mode is exactly `repo-driven`. Intentional divergence from `pj scope list`, which prints `unknown` when autoCommit is not knowable: on the dashboard that case is `plain-files` and the broken-config signal is stderr `config_unparseable:` (document both in Long help so agents do not read plain-files as “healthy host files” without checking stderr).
   - `lens` — active lens tags for the scope, space-separated; empty if none

   Lens rule (whole dashboard): the active lens filters the *working board* the same way bare `list` and `pj next` do. No `--no-lens` in v1. Identity keys, health keys, and terminal tallies stay full-scope (see each field).

   Working-board membership (base set for non-terminal status counts, `claimed`, `blocked_ids`, and as the base for `total`): non-quarantined, not under `archive/`, and passes the active lens. Same archive rule as bare `list` (not `--all`). Layout drift under `archive/` is an `integrity` signal, not a working-board tally.

   Default-list slice (used only by `total`): working-board membership further restricted by `status.InDefaultList` — the same visibility as bare `list` (no status filter, not `--all`, lens on). Built-in `backlog` is not default-list; built-in `done`/`cancelled` are not either (and terminals are not on the working board when archived). Custom statuses follow their category: active yes, backlog/done no.

   Counts (non-negative integers):
   - `total` — count of projects in the default-list slice (locked: same rows bare `list` would print under the active lens). Does **not** include `backlog`; does include lens-passing custom *active* projects at dir root (they have no separate key).
   - Working-board built-in counts: `todo`, `review`, `in-progress`, `blocked`, `draft`, `backlog` — each is the count of projects with that status under working-board membership only (no extra `InDefaultList` filter). `backlog` is therefore counted here even though it is excluded from `total` and from bare `list`.
   - Terminal tallies, full-scope (ignore lens and include `archive/`): `done`, `cancelled`
   - Custom statuses are not separate keys in v1; only custom *active* projects contribute, and only via `total`

   Pointers / health:
   - `next` — full id of the first next-eligible project (same rules as `pj next` without claim, including lens): same `reconcileClosure`, shared depends gate, next-candidate filter, sort, and lens walk; empty if none. Do not compute `next` from ambient-only reconcile (list’s path); that diverges when eligibility depends on another scope’s terminal state. Empty does not change the command exit code (see Requirement 7).
   - `claimed` — space-separated full ids with status `in-progress` under working-board membership, sorted (order, id)
   - `blocked_ids` — space-separated full ids with status `blocked` under working-board membership, same sort (count key remains `blocked`; never emit two lines with the same key)
   - `dangling` — non-negative integer: number of same-scope depends edges whose target id has no project in this scope (edge-count, not distinct-project count). Aligns one-for-one with doctor’s per-edge `depends_dangling` findings. Cross-scope unresolvable depends are not dangling. Full-scope, not lens-filtered. Document this definition in Long help.
   - `integrity` — `ok` or `issues` only. Ambient scope only (not the depends closure). Flip to `issues` when the resolved ambient scope has any of the post-reconcile integrity classes pure reads already emit for that scope: quarantined `parse_error` rows present, `duplicate_id`, `equal_order`, `archive_non_terminal`, `archive_terminal_at_root`. Evaluate against ambient-scope rows/aggregates only — a depended-on scope's collision or parse_error must not flip this field (those may still ride stderr from closure reconcile). Do not flip on soft doctor-only classes (`schema_warn`, `stale_in_progress`, related/cross-scope unresolvable, depends advisories beyond the reconcile set), hard doctor-only edge classes, or repo-health (`uncommitted`, `non_allowlist`, `sync_disabled` — dirt is the `uncommitted` key). Do not dump doctor output into stdout; agents that need the full catalogue run `pj doctor`.
   - `uncommitted` — integer count of allowlisted dirty paths under the scope dir when mode is repo-driven; `0` when clean or when mode is not repo-driven (pj-driven/plain-files do not surface host dirt here)

9. Final locked key order for stdout (implement exactly):

   ```
   scope
   dir
   resolved
   mode
   lens
   total
   todo
   review
   in-progress
   blocked
   draft
   backlog
   done
   cancelled
   next
   claimed
   blocked_ids
   dangling
   integrity
   uncommitted
   ```

10. Stderr may carry existing reconcile/lens/integrity warning tokens the same way other pure reads do; those tokens must not be mixed into the key/value stdout block.
11. Place `status` in the Board help group (orientation next to `list` / `meta` / `next` consumption), e.g. Board: `list`, `status`, `meta`, `deps`, `search`, `query`, `lens` — or immediately after `list`. Work no longer lists a `status` mutator.
12. Skill and `--help` text describe: `mark` changes status; `status` is the scope pulse; show an example line shape `key\tvalue`.

### Tests

13. Rename coverage: existing status-mutator tests retarget to `mark`; archive move, usage on unknown status, self-commit subject still `pj: <id> -> <status>`.
14. Dashboard coverage: key order stable (including `review`); counts match a fixture board with every built-in status represented — assert `total` equals bare-list membership under the lens (excludes `backlog`) while the `backlog` key still counts backlog rows; `next` / `claimed` / `blocked_ids` correct under an active lens (and empty next with exit 0 and full key block); `next` matches `pj next` on a fixture where eligibility depends on a cross-scope terminal (closure required); `dangling` edge-count fixture; `integrity` ok vs issues under an ambient duplicate_id fixture (soft schema_warn alone stays `ok`; a depended-on scope's integrity class alone leaves ambient `integrity` as `ok`); lens field; mode labels; `uncommitted` 0 on pj-driven; no stdout token leakage.

## Constraints

- Pure Go, no cgo; follow AGENTS.md and existing CLI patterns.
- No breaking-change shim required, but do not leave dead `status` mutator symbols that tests still invoke under the old name.
- Dashboard is pure orientation: no flock mutation, no file writes, no push.
- Reuse next selection and depends-gate logic via the same `reconcileClosure` + `buildGate` path as `pj next`; do not fork eligibility rules or ambient-only reconcile for `next`. Do not wire status’s return path through `emptyQueueError` / next’s non-zero empty diagnostics.
- Exit codes: usage → 2; resolution failures → non-zero; successful pulse (full key block) → 0 even when `next` is empty.
- Short-ids letter-first; dashboard pointer fields print full ids only.
- List/TSV work stays untouched.

## Implementation Plan

1. Rename mutator implementation and Cobra command to `mark`; update root group membership and all tests/messages.
2. Add dashboard `status` command module; wire Board group and `--scope`.
3. Implement field collectors: ambient resolve once; `reconcileClosure` + `buildGate` as `pj next` does; list visibility for counts; next selection from that gate (not a second ambient reconcile); dangling via same-scope edge count; integrity from ambient-scope post-reconcile aggregates only (not whole-closure warnings); repo-driven dirty count helpers where they already exist.
4. Lock stdout emitter to the key order in Requirement 9.
5. Update skill contract and help Long strings.
6. Run `go test ./...` and a manual `pj status` / `pj mark` smoke on this repo's scope.

## Implementation Guidance

- Keep mark's body as the current status mutator with the command name and user-facing strings updated — avoid a behaviour rewrite while renaming. Leave the self-commit subject as `pj: <id> -> <status>` (do not invent a mark-prefixed git message).
- For `next`, call the same selection path as unclaimed `pj next` (`reconcileClosure`, `buildGate`, next-candidates, depends gate, lens). Sharing one closure result across the whole dashboard is preferred; ambient-only list reconcile is not sufficient for this field. When no candidate wins, leave the `next` value empty and continue emitting the remaining keys — never surface next’s empty-queue diagnostic as status’s error.
- For `integrity`, derive `ok`/`issues` from ambient-scope integrity only (parse_error presence, duplicate_id, equal_order, archive layout drift for that scope). Do not shell `pj doctor`, do not fold soft or repo-health classes into the flag, and do not flip on depended-on scopes present only because of `reconcileClosure`. Query aggregates with the ambient scope name alone (or filter rows/tokens to it); never treat the whole-closure warning list as the field.
- Disambiguation in Requirement 8 is mandatory: never emit duplicate keys. Counts use short status names; id lists use `claimed` and `blocked_ids`.
- Lens and archive: reuse the same tag-match helper as `list` / `next`. Working-board membership (non-quarantined, not archived, lens) feeds non-terminal status counts, `claimed`, and `blocked_ids`. `total` is the default-list slice of that set (`InDefaultList` / bare-list visibility) — do not put `backlog` into `total`. `next` already excludes archive via next-eligibility. Identity, terminal tallies (`done`/`cancelled`, including archive), and health (`dangling`, `integrity`, `uncommitted`) do not apply the lens. No `--no-lens` in v1.
- Mode: do not use `schemaAutoCommit`'s nil→false trap for labelling; require a known schema autoCommit to claim `pj-driven` or `repo-driven`. Unknown autoCommit + git-root → `plain-files` and `uncommitted` 0; `config_unparseable:` still rides stderr from reconcile. Do not emit `unknown` (that label is `pj scope list` only); Long help must state the three-label set and the stderr signal for unparseable config.
- If resolve already exposes a closed "how chosen" label, use it for `resolved` instead of inventing a parallel vocabulary.

## Acceptance Criteria

1. `pj mark <id> blocked` (and other known statuses) matches prior `pj status` mutator behaviour, including archive moves and path stdout.
2. `pj status <id> blocked` is not the mutator (usage or unknown-subcommand style failure — positionals are not accepted for dashboard status).
3. `pj status` with no args prints exactly the locked keys in order as `key\tvalue` lines for the ambient scope and exits 0 when resolve succeeds (including empty `next`).
4. `next` matches `pj next`'s chosen id (or empty together), including when eligibility depends on a cross-scope target refreshed only by `reconcileClosure`; empty-queue cases still exit 0 with a full key block (unlike bare `pj next`).
5. `claimed` lists in-progress ids; `blocked_ids` lists blocked ids; counts align with those sets.
6. Work help shows `mark` and not a status mutator; Board help shows dashboard `status`.
7. Skill documents both verbs correctly.
8. Existing write-path tests pass under the `mark` name; new dashboard tests cover key order and a multi-status fixture.
