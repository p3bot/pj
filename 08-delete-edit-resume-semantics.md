# Delete/edit resume semantics in pj sync

Source: pre-commit review on 2026-07-25
Severity: medium
Category: Correctness
Location: `internal/cli/sync_integrate.go` — `resolveResumeStop`

## Goal

Stop `pj sync` from silently discarding one machine's deletion when a human re-runs it after a
delete/edit handoff without acting. A resumed stop currently cannot tell a delete/edit file
apart from driver output, so it stages the surviving side and continues, choosing a winner with
no record that a choice was made.

## Scope

In scope:
- The resumed-stop procedure's treatment of a conflicted project `.md` that is a delete/edit
  rather than driver output.
- The signal a human uses to express each of the three outcomes (deletion wins, edit wins,
  keep as-is), and the report emitted when none has been given.
- Tests covering an unactioned re-run and each resolution path.

Out of scope:
- The first-encounter handoff. The rebase driver's delete/edit classification, the message
  `pj sync` prints when it first pauses, and the decision to leave the path unstaged are all
  correct and stay as they are. This project changes only what happens on the next invocation.
- Body-conflict resume. Requirement 7 of P6b fixes that body markers are never scanned before
  staging, because an unresolved body pushes visible markers and is the human's say-so. That
  rule is deliberate and must not be weakened by this work.
- The status-dispute resume. A `status_conflict` key already blocks `rebase --continue`
  correctly.
- Any new git plumbing. Everything needed is already exported by `internal/git`.
- Further `design.md` changes. The amendments that lock this resolution have already landed
  (see "Design silence and the resolution"); this project implements to them and re-decides
  nothing.

## Current State

`pj sync` is the sole push boundary (P6b, `docs/archive/06b-sync-and-push-boundary.md`). When a
rebase stop leaves files unmerged and pj cannot resolve them, the command reports the handoff
and exits with the rebase paused. The next `pj sync` enters mid-rebase and runs
`resolveResumeStop` in `internal/cli/sync_integrate.go`.

That function sorts each conflicted project `.md` by one question: does its frontmatter still
carry git conflict markers?

- Markers present means the file was never field-merged — the state a `pj.cue` conflict leaves
  behind, because the schema-before-data rule fails closed on that scope's `.md`. It is driven
  through the rebase driver now.
- Markers absent means the file is driver output: the driver field-merged the frontmatter and
  left a body conflict or a status dispute for the human. The human's resolution is staged so
  the rebase can continue over it, unless a `status_conflict` key is still present.

A delete/edit file fits neither description. The driver returns its handoff without writing
anything, so what sits in the worktree is git's copy of the surviving side: ordinary content,
clean frontmatter, no markers. It is indistinguishable from driver output and takes the second
path, so it is staged and the rebase continues.

Reproduction, against two clones of one remote sharing an auto-commit scope:

1. Machine A deletes a project file by hand and runs `pj sync`.
2. Machine B runs `pj status <id> in-progress` on the same project, then `pj sync`.
3. B pauses with the delete/edit handoff, naming the path, the deleting side, and the surviving
   status. Correct so far.
4. Without touching anything, run `pj sync` on B again. It stages the surviving file, continues
   the rebase, and pushes. A's deletion is gone from the remote and nothing on either machine
   records that a decision was taken.

Step 4 is the expected path rather than a corner case: P7's skill contract has agents run
`pj sync` at end of turn, so a paused sync is followed by another `pj sync` as a matter of
course.

The plumbing to distinguish the two shapes already exists. `git.ConflictStages` reports which of
the three stages an unmerged path has, and a delete/edit is exactly the path missing stage 2 or
stage 3. `git.ShowStage` reads a stage's blob. Both landed in P6a and are already used by the
rebase driver.

An explicit keep signal also already exists and needs no new mechanism. `classifyStop` builds
its item list from `git.UnmergedFiles`, which lists paths git reports as unmerged. A human who
runs `git add <path>` resolves the path at the git level, so it never appears in the stop at all
and the rebase continues over it untouched.

## Design silence and the resolution

`design.md` covered the delete/edit pause and the human restoring or removing the file before
re-running `pj sync`. It did not cover a re-run where they did neither.

`AGENTS.md` requires that silence be flagged rather than guessed. P6b's own requirement 2 shows
the established pattern for this situation: state the resolution with the reasoning that
produced it, so the choice is deliberate and reviewable rather than an accident of
implementation. That has now happened in both places, and the split matters — `design.md`
carries the rule, this document carries the reasoning that produced it.

The design now locks the resolution at three sites:

- `DECISION (unactioned re-run)`, under the layer-3 delete/edit DECISION in Merge conflict
  handling — the stage-set discriminator, the four outcomes, and why staging the survivor is
  the defect.
- The `Sync resume` row in the mid-rebase command-classes table — an unactioned delete/edit
  joins `status_conflict` as a gate on `rebase --continue`.
- The locked Conflicts-and-paused-sync skill row — the agent-facing rule that re-running
  without acting re-pauses by design.

`design.md` is authoritative: implement to it. If it and this document ever disagree, the design
wins and this document is corrected.

The resolution is that an unactioned re-run must not stage the surviving file. Silence is the
defect: staging picks a winner and leaves nothing behind to show a choice was made, whereas
every other handoff class either blocks (`status_conflict`) or pushes something visibly
unresolved (body markers). A delete/edit that resolves itself pushes an ordinary-looking file,
so nobody ever learns the deletion was overruled.

Pausing again does not strand anyone, because all three outcomes have a natural signal that maps
onto the design's own wording of "restore or remove the file":

- Remove the file. The deletion wins.
- Modify the file. The surviving edit wins, carrying whatever the human wrote.
- Stage the file with `git add`. It is kept exactly as it stands. The path stops being unmerged,
  so the resumed stop never sees it.
- Do none of these. The stop pauses again on the same handoff.

The discriminator is therefore whether the worktree content still matches the surviving side's
stage blob. Matching means nothing was decided. Absent or differing means the human acted.

## References

- `design.md` — Merge conflict handling, for the layer-4 handoffs and the delete/edit pause;
  Sync model, for `pj sync`'s five steps and the resume contract. Authoritative on every
  conflict.
- `docs/archive/06b-sync-and-push-boundary.md` — requirement 7 (the resume contract and the
  marker-freeness discriminator) and requirement 4 (the per-stop procedure). Explains why the
  current discriminator exists and what it was built to separate.
- `docs/archive/06a-frontmatter-merge-and-rebase-driver.md` — the rebase driver's contract,
  including the delete/edit outcome class and the stage plumbing it uses.
- `AGENTS.md` — pure Go, no cgo; external git binary; the rule that design silence is flagged
  rather than guessed.
- Project writing guide — `start get project/writing`.

## Requirements

1. A resumed stop distinguishes a delete/edit conflicted project `.md` from driver output. The
   marker-freeness test alone cannot do this, because both shapes have clean frontmatter; use
   the unmerged path's stage set, which is already exported.
2. A delete/edit whose worktree content still equals the surviving side's stage blob is treated
   as unresolved. It is not staged, the stop stays paused, and the run reports the same handoff
   detail the first pause gave — the path, which side deleted it, and the surviving status — so
   the human sees the same instruction on every re-run rather than a bare paused message.
3. A delete/edit the human removed from the worktree stages as a deletion and lets the rebase
   continue. This is the behaviour the existing test
   `TestSyncDeleteEditPausesThenResumes` covers and it must not regress.
4. A delete/edit the human modified stages as their resolution and lets the rebase continue.
5. A delete/edit the human resolved with `git add` never reaches the resumed stop, because it is
   no longer unmerged. Confirm this holds rather than adding a second path for it.
6. Body conflicts and status disputes are unaffected. A driver-output file with clean
   frontmatter and no `status_conflict` still stages whatever body the human left, with its body
   never scanned for markers.
7. The report for an unresolved delete/edit names the three ways out — remove the file, edit it,
   or `git add` it — so doing nothing is not the only obvious option. Whether it also carries a
   closed-catalogue token is a decision for the implementer against the catalogue in
   `internal/token`; do not invent a new token.

## Constraints

- Pure Go, no cgo. Every git invocation goes through `internal/git`. Do not add git plumbing —
  `ConflictStages` and `ShowStage` already cover this.
- `design.md` overrides the Go CLI design guide, and overrides the resolution stated above if it
  turns out to speak to this case.
- Non-interactive. The command never prompts; the human resolves in-file and re-runs.
- Do not weaken the body-marker rule. A body conflict's markers are never scanned before
  staging.
- The resumed stop must not re-drive a file the driver already field-merged. Re-reading its
  still-present stages would overwrite a body the human just resolved.
- No state persisted across invocations. The current discriminator deliberately needs none, and
  the stage set is available from git at the moment it is needed.

## Implementation Plan

1. Read `resolveResumeStop` and the surrounding per-stop procedures in
   `internal/cli/sync_integrate.go`, then `git.ConflictStages` and `git.ShowStage` in
   `internal/git/integrate.go`, so the new discrimination fits the existing shape rather than
   duplicating it.
2. Add the delete/edit discrimination ahead of the existing clean-frontmatter staging branch,
   leaving the marker-carrying branch untouched.
3. Wire the three outcomes: unresolved pauses and re-reports, removed stages the deletion,
   modified stages the resolution.
4. Extend the report so an unresolved delete/edit repeats the full handoff detail and names all
   three ways out.
5. Add tests. The critical one is the unactioned re-run: after the first pause, run `pj sync`
   again with nothing touched and assert the rebase is still paused, the handoff is reported
   again, and the remote still carries the deletion rather than the resurrected file. Cover the
   removed and modified resolutions too, and assert a body conflict still resumes as before.
6. Verify each new test fails against the current behaviour before the change, so it is a real
   regression test rather than a passing assertion.

## Implementation Guidance

- The sync loop's job is ordering, the continue-or-pause decision, and reporting. Reading the
  stage set to classify a path is within that job; re-implementing any part of the driver's
  merge is not.
- `git.ConflictStages` returns a `Stages` struct whose fields are named `Base`, `Ours`, and
  `Theirs` for stages 1, 2, and 3. During a rebase the everyday reading of those two side
  labels is inverted — `git.RebaseSides` documents this and warns callers to pair each rev with
  its stage number, never with a side label. Decide which stage is the surviving side from the
  stage number, and confirm it against how the driver maps them.
- The existing tests in `internal/cli/sync_flow_test.go` and the harness in
  `internal/cli/sync_harness_test.go` already build two clones of a shared remote with an
  auto-commit scope. Reuse `twoMachines` rather than building new fixtures.
- `TestSyncDeleteEditPausesThenResumes` is the closest existing test. It resolves by removing
  the file, so it should keep passing unchanged; the new unactioned-re-run test is its missing
  sibling.

## Acceptance Criteria

- A delete/edit handoff followed by a `pj sync` with nothing touched leaves the rebase paused,
  reports the same handoff detail naming the path, the deleting side, and the surviving status,
  and exits non-zero. The remote still carries the deletion.
- Removing the file and re-running completes the rebase, records the deletion, and pushes.
- Modifying the file and re-running completes the rebase, keeps the human's content, and pushes.
- Running `git add` on the file and re-running completes the rebase with that exact content and
  pushes.
- A body-only conflict still pauses with clean field-merged frontmatter and markers in the body,
  and the next `pj sync` after in-place resolution continues and pushes without its body being
  scanned.
- Every new test fails against the pre-change behaviour.
