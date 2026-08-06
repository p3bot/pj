---
id: pj-gc4k
status: todo
order: "aF"
created: "2026-08-06T16:57:51+10:00"
summary: Silence repo-driven write-side uncommitted; signal pj-driven scopes to run pj sync
---
# Repo-driven quiet writes; pj-driven sync-needed signal

## Goal

Stop nagging repo-driven scopes about dirty allowlisted files after every write — durability there is host git, not pj. Give pj-driven (auto-commit) scopes a clear write-side signal when the agent should run `pj sync`, especially after create and other non-self-committed allowlisted dirt, and when push/integrate is still required.

## Scope

In scope:

- Repo-driven write path: remove the post-write `uncommitted:` stderr ride
- Decision on bare `pj doctor` and `pj status` for the same dirty count (see Requirements)
- Pj-driven write path: introduce an explicit sync-needed stderr signal (new or repurposed token) with agent-actionable wording that points at `pj sync`, not host commit
- Embedded skill durability/recovery text that currently documents repo-driven `uncommitted:` behaviour
- Tests that encode today's repo-driven write/doctor expectations
- Token catalogue / closed stderr prefixes if a new token is added or `uncommitted:` meaning changes

Out of scope:

- Changing who commits or pushes (repo-driven stays host-owned; pj-driven still self-commits mutators; `pj sync` remains sole push boundary)
- Making `create` self-commit (create remains non-self-committing; sync snapshot still covers it)
- Throttling the existing repo-driven token as a half-measure
- Board list TSV / skill length / related-edge work (separate findings)
- Auto-enabling autoCommit on existing scopes

## Current State

Mode split (from `pj.cue` `autoCommit` + git-root):

- autoCommit true → pj-driven (even without git-root)
- autoCommit false + git-root → repo-driven
- autoCommit false + no git → plain-files

Write durability lives mainly in `internal/cli/commit.go`:

- `completeStateDurability` — if not autoCommit, calls `repoDirtyHealth`; if autoCommit, self-commits when a git-root exists (or `sync_disabled:` when not)
- `createDurability` — create never self-commits; on non-autoCommit calls `repoDirtyHealth`; terminal create gets a separate archive note
- `repoDirtyHealth` — counts allowlisted dirty paths under the scope dir via `git status --porcelain` and emits `uncommitted: N allowlisted path(s) under <dir> uncommitted — commit with the host repo`

Stable token: `internal/token/token.go` `Uncommitted = "uncommitted:"` (comment: host-owned dirty after a write).

Doctor: `internal/cli/doctor_diagnose.go` can emit the same `uncommitted:` class for repo-driven dirty allowlist on bare diagnose.

Status pulse: `internal/cli/status.go` sets `uncommitted` to the allowlisted dirty count only in repo-driven mode (non-zero only there); help text says so. Reads never ride the stderr token (covered by tests).

Skill (`internal/skill/skill.md` Durability / Recovery): documents repo-driven `uncommitted:` on stderr as dirty board → host commit, not `pj sync`. Pj-driven path documents self-commit + `pj sync` as sole push/integrate, and that create never self-commits.

Tests to expect change:

- `internal/cli/cli_write_test.go` — `TestRepoDrivenUncommitted` expects write-side `uncommitted:`
- `internal/cli/doctor_git_test.go` — repo-driven doctor expects `uncommitted:`; pj-driven paths expect none
- Skill structure / token catalogue tests if they assert the token or durability prose

Problem with the current split:

- Repo-driven multi-step agent turns re-emit the same host-commit nag after every mutator; the host already owns `git status` / commit
- Pj-driven agents who need a durability action are not guided by `uncommitted:`; create leaves dirty allowlist until `pj sync` (snapshot), and unpushed / failed-push cases already have partial notes but no unified write-side "run pj sync" contract

## Requirements

1. After a complete-state write or create on a **repo-driven** scope, do not emit `uncommitted:` (or any equivalent "commit with the host repo" token) on stderr solely because allowlisted paths are dirty.

2. After a complete-state write or create on a **pj-driven** scope, when the agent still needs `pj sync` for durability (at minimum: allowlisted dirty remaining after the write path, e.g. create never self-commits; and align with existing failed-push / needs-integrate cases so the agent is not silent), emit a stable stderr token whose message explicitly tells the agent to run `pj sync`. Do not tell them to host-commit or invent a second push path.

3. Choose a token design that keeps classes distinct:
   - Prefer a dedicated sync-needed token (new closed prefix) for pj-driven action, rather than overloading `uncommitted:` to mean "host commit" in one mode and "pj sync" in another
   - If `uncommitted:` is retained at all, its meaning must remain host-commit / repo-driven dirty only — never "run pj sync"

4. Decide and document behaviour for **read/diagnose surfaces** under the same ownership model:
   - `pj status` `uncommitted` field: either keep as an optional pulse for repo-driven dirty count (no write spam) or zero/omit if the project treats host dirty as fully outside pj — pick one and make skill + help match
   - bare `pj doctor`: either stop listing repo-driven allowlisted dirty as `uncommitted:`, or keep doctor-only visibility without write rides — pick one consistent with "repo-driven durability is host git"

5. Update `internal/skill/skill.md` Durability and Recovery so:
   - repo-driven: no write-side `uncommitted:` expectation; host commit/PR is the durability path without pj nagging mid-turn
   - pj-driven: write-side sync-needed signal → `pj sync`; create still never self-commits; never host push around sync

6. Update or replace tests that currently require write-side `uncommitted:` on repo-driven mutators; add coverage that pj-driven create (and any other intentional non-self-commit dirty path) surfaces the sync-needed signal; keep the invariant that pure reads never emit durability tokens.

7. Keep the closed token catalogue consistent (`internal/token`, doctor/skill tests that list stable prefixes).

## Constraints

- Pure Go, no cgo; follow existing CLI patterns (stdout purity, tokens on stderr)
- Do not invent a push or commit path around `pj sync` for auto-commit roots
- Do not change the sole push boundary contract (P6b)
- Do not make create self-commit as a side effect of this project unless required to make the signal honest — default is keep create non-self-committing and signal sync
- Short-ids and existing exit-code / token grammar rules still apply
- Prefer packages, tests, and embedded skill over archive design prose when they disagree; archive `docs/archive/design.md` is history only

## Implementation Plan

1. Map every call site of `repoDirtyHealth`, `Uncommitted`, and status `uncommitted` pulse; list write vs doctor vs status.
2. Implement repo-driven silence on the write path (`completeStateDurability` / `createDurability` non-autoCommit branch).
3. Implement pj-driven sync-needed emission for the cases in Requirements (create dirty, any post-write allowlisted dirty under auto-commit, and consistent handling with last-push-error notes so agents get one clear action class).
4. Apply the chosen doctor/status policy from Requirement 4.
5. Retarget tests; add pj-driven create/signal tests; adjust token catalogue if a new prefix is introduced.
6. Rewrite skill Durability/Recovery lines to match shipped behaviour; keep skill tests green.
7. Manual smoke: repo-driven write stays quiet on stderr for dirty board; pj-driven create prints sync-needed; `pj sync` remains the integrate/push verb.

## Implementation Guidance

- Root fix is ownership: emit durability nags only for the owner of the next action (host git vs `pj sync`), not "any dirty tree"
- Do not implement "flip repoDirtyHealth on for auto-commit" with the same host-commit message — wrong action and wrong lifecycle (dirty create vs committed-but-unpushed)
- Reuse existing allowlist dirty counting helpers where they already match sync snapshot bounds; do not scan whole-repo dirty
- Failed-push note after self-commit already points at `pj sync`; fold or align wording so agents see one conceptual class of "pj-managed durability next step"
- Token-efficient skill edits: change the contracts, do not grow Workflows

## Acceptance Criteria

1. On a repo-driven scope with a git-root, after `pj mark` / `meta` / `create` leaves allowlisted files dirty, command stderr does not contain `uncommitted:` (or a host-commit equivalent) from that write path.
2. On a pj-driven scope with a git-root, after `pj create` (allowlisted dirty, no self-commit), stderr contains a stable token whose message instructs running `pj sync`.
3. That pj-driven signal never says "commit with the host repo" and never recommends host `git push` as a substitute for `pj sync`.
4. Pure reads (`list`, `get`, `next` without `--claim`, etc.) still emit no durability dirty/sync tokens.
5. Skill Durability/Recovery text matches the above; structure/hot-path or token tests that pin these strings still pass.
6. `pj sync` remains the only push path for auto-commit roots; no new commit/push CLI is introduced.
