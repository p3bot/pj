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
2. Behaviour is a pure rename of today's `status` mutator: same argument rules, known-status check, archive boundary move, stdout path hand-off, mid-rebase refuse, quarantine/duplicate refuse, self-commit message shape updated to use `mark` (e.g. `pj: <id> -> done`), and repo-driven `uncommitted:` signalling.
3. Remove the top-level `status` mutator entry point. Do not leave a hidden alias.
4. Place `mark` in the Work help group in the slot `status` occupied (Work: `create`, `get`, `edit`, `mark`, `reorder`, `next`).
5. Update every in-tree user-facing string, test, and skill reference that documents `pj status <id> <status>` as the mutator.

### Status (dashboard)

6. Command: `pj status [--scope S]` with no positional args. Pure read: reconcile as other board reads do; never run git for mutation; may run cheap git status porcelain only when needed for the `uncommitted` field (same allowlist idea as write-path / doctor repo-driven health).
7. Stdout is parse-stable `key\tvalue` lines, one key per line, in the locked order below. No header row. Empty values are still emitted as `key\t` (key present, value empty) so agents can rely on key presence.
8. Locked keys and meanings (value forms):

   Identity / resolution:
   - `scope` — resolved scope name
   - `dir` — absolute scope directory
   - `resolved` — how the scope was chosen: `cwd` | `flag` | `env` (or the resolve package's existing labels if they already form a closed set — use that set, document it in help)
   - `mode` — `pj-driven` | `repo-driven` | `plain-files` (autoCommit true + git-root → pj-driven; autoCommit false + git-root → repo-driven; no git-root → plain-files)
   - `lens` — active lens tags for the scope, space-separated; empty if none

   Counts (non-negative integers; default active inventory unless noted):
   - `total` — non-quarantined projects in the default active set (same visibility idea as bare `list`, not `--all`)
   - `todo`, `in-progress`, `blocked`, `draft`, `backlog`, `done` — counts for those built-in statuses. `done` counts terminal done projects even when they live under `archive/` (agents need the done tally). Custom statuses are not separate keys in v1; they fold into `total` when active but do not get their own lines.
   - Include `cancelled` only if it is cheap and consistent with built-ins; if included, place it after `done`. Prefer the user's listed keys first; add `cancelled` when it avoids a blind spot for terminal work.

   Pointers / health:
   - `next` — full id of the first next-eligible project (same rules as `pj next` without claim); empty if none
   - `claimed` — space-separated full ids with status `in-progress`, sorted (order, id) like list
   - `blocked` — space-separated full ids with status `blocked`, same sort (note: key name reuses `blocked` for the id list; the count key is also `blocked` — **disambiguate**: use `blocked` for the count and `blocked_ids` for the id list, and `claimed` for in-progress ids. Do not emit two lines with the same key.)
   - `dangling` — count of same-scope depends edges whose target is missing (or the number of projects carrying a depends_dangling condition — pick one definition, implement consistently, document in Long help)
   - `integrity` — `ok` when the scope has no open integrity findings that doctor would report as problems for this scope on a bare diagnose; otherwise `issues` (or a short non-ok token). Do not dump full doctor output here.
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

13. Rename coverage: existing status-mutator tests retarget to `mark`; archive move, usage on unknown status, self-commit message.
14. Dashboard coverage: key order stable; counts match a fixture board; `next` / `claimed` / `blocked_ids` correct; empty next; lens field; mode labels; `uncommitted` 0 on pj-driven; no stdout token leakage.

## Constraints

- Pure Go, no cgo; follow AGENTS.md and existing CLI patterns.
- No breaking-change shim required, but do not leave dead `status` mutator symbols that tests still invoke under the old name.
- Dashboard is pure orientation: no flock mutation, no file writes, no push.
- Reuse next selection and depends-gate logic; do not fork eligibility rules.
- Exit codes unchanged in class: usage → 2; resolution failures → non-zero.
- Short-ids letter-first; dashboard pointer fields print full ids only.
- List/TSV work stays untouched.

## Implementation Plan

1. Rename mutator implementation and Cobra command to `mark`; update root group membership and all tests/messages.
2. Add dashboard `status` command module; wire Board group and `--scope`.
3. Implement field collectors reusing ambient resolve, reconcile, list visibility, next selection, depends gate, and repo-driven dirty count helpers where they already exist.
4. Lock stdout emitter to the key order in Requirement 9.
5. Update skill contract and help Long strings.
6. Run `go test ./...` and a manual `pj status` / `pj mark` smoke on this repo's scope.

## Implementation Guidance

- Keep mark's body as the current status mutator with command name and commit-message strings updated — avoid a behaviour rewrite while renaming.
- For `integrity`, prefer a boolean derived from the same finding classes doctor surfaces for one scope rather than shelling doctor. A coarse `ok` / `issues` is enough; do not re-encode every token into stdout.
- Disambiguation in Requirement 8 is mandatory: never emit duplicate keys. Counts use short status names; id lists use `claimed` and `blocked_ids`.
- If resolve already exposes a closed "how chosen" label, use it for `resolved` instead of inventing a parallel vocabulary.

## Acceptance Criteria

1. `pj mark <id> blocked` (and other known statuses) matches prior `pj status` mutator behaviour, including archive moves and path stdout.
2. `pj status <id> blocked` is not the mutator (usage or unknown-subcommand style failure — positionals are not accepted for dashboard status).
3. `pj status` with no args prints exactly the locked keys in order as `key\tvalue` lines for the ambient scope.
4. `next` matches `pj next`'s chosen id (or empty together).
5. `claimed` lists in-progress ids; `blocked_ids` lists blocked ids; counts align with those sets.
6. Work help shows `mark` and not a status mutator; Board help shows dashboard `status`.
7. Skill documents both verbs correctly.
8. Existing write-path tests pass under the `mark` name; new dashboard tests cover key order and a multi-status fixture.
