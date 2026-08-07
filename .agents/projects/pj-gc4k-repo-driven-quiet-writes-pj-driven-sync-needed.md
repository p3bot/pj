---
id: pj-gc4k
status: in-progress
order: "aF"
created: "2026-08-06T16:57:51+10:00"
summary: Silence repo-driven write-side uncommitted; signal pj-driven scopes to run pj sync
---
# Repo-driven quiet writes; pj-driven sync-needed signal

## Goal

Stop nagging repo-driven scopes about dirty allowlisted files after every write — durability there is host git, not pj. Give pj-driven (auto-commit) scopes a clear write-side `sync_needed:` signal whenever durability still requires `pj sync` (residual allowlisted dirt, unpushed self-commits, or a recorded push failure). The token names the condition only; the skill teaches that the response is `pj sync`.

## Scope

In scope:

- Repo-driven write path: remove the post-write `uncommitted:` stderr ride
- Repo-driven `pj status` `uncommitted` pulse and bare `pj doctor` `uncommitted:` stay as today (opt-in visibility; write spam only is removed)
- Pj-driven write path: new closed token `sync_needed:` with emission rules and short reason body below
- Retire the freeform failed-push `note:` on the write path in favour of the same token
- Embedded skill durability/recovery text (repo-driven quiet writes; status/doctor still surface host dirty; pj-driven `sync_needed:` → `pj sync`)
- Tests that encode today's repo-driven write/doctor expectations, plus pj-driven emission coverage
- Token catalogue / closed stderr prefixes for `sync_needed:`

Out of scope:

- Changing who commits or pushes (repo-driven stays host-owned; pj-driven still self-commits mutators; `pj sync` remains sole push boundary)
- Making `create` self-commit (create remains non-self-committing; sync snapshot still covers it)
- Throttling the existing repo-driven token as a half-measure
- Board list TSV / skill length / related-edge work (separate findings)
- Auto-enabling autoCommit on existing scopes
- Putting "run pj sync" (or host push/commit instructions) in the token message body

## Current State

Mode split (from `pj.cue` `autoCommit` + git-root):

- autoCommit true → pj-driven (even without git-root)
- autoCommit false + git-root → repo-driven
- autoCommit false + no git → plain-files

Write durability lives mainly in `internal/cli/commit.go`:

- `completeStateDurability` — if not autoCommit, calls `repoDirtyHealth`; if autoCommit, self-commits when a git-root exists (or `sync_disabled:` when not); on successful self-commit may print freeform `note: … failed push … — run pj sync` when last-push-error is set
- `createDurability` — create never self-commits; on non-autoCommit calls `repoDirtyHealth`; terminal create gets a separate archive note
- `repoDirtyHealth` — counts allowlisted dirty paths under the scope dir via `git status --porcelain` and emits `uncommitted: N allowlisted path(s) under <dir> uncommitted — commit with the host repo`

Stable token: `internal/token/token.go` `Uncommitted = "uncommitted:"` (comment: host-owned dirty after a write). No `sync_needed:` today.

Doctor: `internal/integrity/diagnose.go` can emit the same `uncommitted:` class for repo-driven dirty allowlist on bare diagnose; pj-driven doctor uses `sync_disabled:` / `last_push_error:` etc.

Status pulse: `internal/cli/status.go` sets `uncommitted` to the allowlisted dirty count only in repo-driven mode (non-zero only there); help text says so. Reads never ride the stderr token (covered by tests).

Skill (`internal/skill/skill.md` Durability / Recovery): documents repo-driven `uncommitted:` on stderr as dirty board → host commit, not `pj sync`. Pj-driven path documents self-commit + `pj sync` as sole push/integrate, and that create never self-commits — no write-side `sync_needed:` contract yet.

Tests to expect change:

- `internal/cli/cli_write_test.go` — `TestRepoDrivenUncommitted` expects write-side `uncommitted:`
- `internal/cli/meta_test.go` — `TestMetaSelfCommitAndUncommitted` expects write-side `uncommitted:` on repo-driven meta
- `internal/cli/doctor_git_test.go` — repo-driven doctor expects `uncommitted:`; pj-driven paths expect none
- Skill structure / token catalogue tests when the new prefix and durability prose land

Problem with the current split:

- Repo-driven multi-step agent turns re-emit the same host-commit nag after every mutator; the host already owns `git status` / commit
- Pj-driven agents who need a durability action are not guided by a matchable write-side token; create leaves dirty allowlist until `pj sync` (snapshot); successful self-commits leave unpushed work silent; failed-push uses freeform `note:` rather than a catalogue prefix

## Requirements

1. After a complete-state write or create on a **repo-driven** scope, do not emit `uncommitted:` (or any equivalent "commit with the host repo" token) on stderr solely because allowlisted paths are dirty.

2. After a complete-state write or create on a **pj-driven** scope with a git-root, emit exactly one `sync_needed:` stderr line when any of the following holds after that write path finishes (evaluate in a fixed priority order so a single short reason is chosen):
   - allowlisted paths under the scope dir remain dirty (create never self-commits; any other residual allowlisted dirt)
   - the write performed a successful self-commit and the branch is still ahead of its upstream (unpushed)
   - a last-push-error marker is present for that git-root (replace today's freeform failed-push `note:` with this token; do not dual-emit)
   If none hold, stay silent. Do not emit `sync_needed:` on pure reads, on repo-driven/plain-files writes, or as a substitute for `sync_disabled:` when there is no git-root.

3. Token wire contract:
   - New closed catalogue prefix: `sync_needed:`
   - Line shape: `sync_needed: <extremely-short-reason>`
   - The reason names the condition only (examples of acceptable brevity: `dirty`, `unpushed`, `push failed`). Exact short strings are implementer choice; keep them stable once chosen and pin them in tests
   - Do **not** include "run pj sync", host-commit wording, or host `git push` as a substitute — the skill maps `sync_needed:` → `pj sync`
   - Never overload `uncommitted:` to mean pj-driven sync. Retained uses (status pulse, doctor) remain host-commit / repo-driven dirty only

4. **Read/diagnose surfaces (locked):** write silence only — do not strip host-dirty visibility from opt-in commands.
   - `pj status` `uncommitted` field: keep current behaviour (allowlisted dirty count, non-zero only in repo-driven mode); help text stays accurate
   - bare `pj doctor`: keep emitting `uncommitted:` for repo-driven allowlisted dirty (same class as today)
   - Skill: repo-driven write path no longer rides `uncommitted:`; status/doctor may still show host dirty → host commit, not `pj sync`

5. Update `internal/skill/skill.md` Durability and Recovery so:
   - repo-driven: no write-side `uncommitted:` expectation; host commit/PR remains the durability path; optional `pj status` / `pj doctor` still surface dirty allowlist when useful
   - pj-driven: write-side `sync_needed:` means run `pj sync` (skill states the action; token states the reason); create still never self-commits; never host push around sync

6. Update or replace tests that currently require write-side `uncommitted:` on repo-driven mutators (`cli_write_test`, `meta_test`, and any siblings); keep status/doctor tests that expect repo-driven dirty visibility; add coverage that pj-driven create (dirty), a successful self-commit with unpushed commits, and the last-push-error path each surface `sync_needed:`; keep the invariant that pure reads never emit durability dirty/sync tokens.

7. Keep the closed token catalogue consistent (`internal/token`, doctor/skill tests that list stable prefixes).

## Constraints

- Pure Go, no cgo; follow existing CLI patterns (stdout purity, tokens on stderr)
- Do not invent a push or commit path around `pj sync` for auto-commit roots
- Do not change the sole push boundary contract (P6b)
- Do not make create self-commit as a side effect of this project unless required to make the signal honest — default is keep create non-self-committing and signal sync
- Short-ids and existing exit-code / token grammar rules still apply
- Prefer packages, tests, and embedded skill over archive design prose when they disagree; archive `docs/archive/design.md` is history only

## Implementation Plan

1. Map every call site of `repoDirtyHealth`, `Uncommitted`, freeform failed-push note, and status `uncommitted` pulse; list write vs doctor vs status.
2. Implement repo-driven silence on the write path (`completeStateDurability` / `createDurability` non-autoCommit branch).
3. Add `sync_needed:` to the token catalogue; implement pj-driven write-side emission for dirty / unpushed / push-failed (Requirement 2) with one short reason per line (Requirement 3); remove the freeform failed-push `note:`.
4. Leave status `uncommitted` pulse and doctor `uncommitted:` behaviour unchanged (Requirement 4); only remove write-path emission.
5. Retarget write-path tests; keep status/doctor dirty expectations; add pj-driven create / unpushed self-commit / last-push-error signal tests; catalogue completeness.
6. Rewrite skill Durability/Recovery: `sync_needed:` → run `pj sync`; repo-driven write path quiet; status/doctor may still report host dirty; keep skill tests green.
7. Manual smoke: repo-driven write quiet on dirty board while `pj status`/`pj doctor` still show dirty; pj-driven create and a self-committing mutator with unpushed each print `sync_needed:`; `pj sync` remains the integrate/push verb.

## Implementation Guidance

- Root fix is ownership: emit durability nags only for the owner of the next action (host git vs `pj sync`), not "any dirty tree"
- Do not implement "flip repoDirtyHealth on for auto-commit" with the host-commit message — wrong action and wrong lifecycle (dirty create vs committed-but-unpushed)
- Reuse existing allowlist dirty counting helpers where they already match sync snapshot bounds; do not scan whole-repo dirty
- Unpushed detection should reuse existing git helpers (`UnpushedCount` / upstream checks) already used by sync; if upstream is missing, do not invent a second class — existing `sync_disabled:` / doctor paths cover no-upstream
- Priority when multiple conditions hold: pick one reason (suggested order: push failed, then dirty, then unpushed) so agents always see a single line
- Token-efficient skill edits: change the contracts, do not grow Workflows

## Acceptance Criteria

1. On a repo-driven scope with a git-root, after `pj mark` / `meta` / `create` leaves allowlisted files dirty, command stderr does not contain `uncommitted:` (or a host-commit equivalent) from that write path.
2. On that same dirty repo-driven board, `pj status` still reports a non-zero `uncommitted` pulse and bare `pj doctor` still emits `uncommitted:` (write silence only).
3. On a pj-driven scope with a git-root, after `pj create` (allowlisted dirty, no self-commit), stderr contains a line matching `sync_needed: <short-reason>` (dirty class).
4. On a pj-driven scope with a git-root, after a successful self-committing mutator that leaves the branch ahead of upstream, stderr contains `sync_needed:` with an unpushed-class short reason (unless a higher-priority reason applies).
5. When a last-push-error marker is present after a pj-driven write path that would previously have printed the freeform failed-push note, stderr carries `sync_needed:` (push-failed class) and does not carry that freeform `note:`.
6. `sync_needed:` lines never say "run pj sync", never say "commit with the host repo", and never recommend host `git push` as a substitute for `pj sync`.
7. Pure reads (`list`, `get`, `next` without `--claim`, etc.) still emit no durability dirty/sync tokens (`pj status` pulse field is stdout, not a durability token ride).
8. Skill Durability/Recovery maps `sync_needed:` to `pj sync` and matches the above; structure/hot-path or token tests that pin these strings still pass.
9. `pj sync` remains the only push path for auto-commit roots; no new commit/push CLI is introduced.
