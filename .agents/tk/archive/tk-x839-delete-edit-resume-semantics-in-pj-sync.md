---
id: tk-x839
status: done
order: "aA"
created: "2026-07-29T21:00:02+10:00"
---
# Delete/edit resume semantics in pj sync

Source: pre-commit review on 2026-07-25
Severity: medium
Category: Correctness
Location: `internal/cli/sync_integrate.go` — `resolveResumeStop`

## Goal

Stop `pj sync` from silently discarding one machine's deletion when a human re-runs it after a
delete/edit handoff without acting. A resumed stop currently cannot tell a delete/edit file from
one the human already resolved — driver output for a project `.md`, a marker-free file for a
config file — so it stages the surviving side and continues, choosing a winner with no record
that a choice was made.

## Scope

In scope:
- The resumed-stop procedure's treatment of a conflicted project `.md` that is a delete/edit
  rather than driver output.
- The same treatment for the two allowlisted config files, `pj.cue` and `.gitignore`. Their pass
  runs on marker-freeness alone and has the identical defect, in the branch immediately above the
  one this project changes.
- The signal a human uses to express each of the three outcomes (deletion wins, edit wins,
  keep as-is), and the report emitted when none has been given.
- The wording of every delete/edit handoff line at both pauses. Project `.md` renders through the
  one driver-outcome emitter (requirement 8). Config files have no driver, so they use one shared
  config delete/edit reporter called from both the first-pause procedure and the resume procedure
  (requirement 9) — the same one-emitter discipline, not a second formatter per entry point.
- Tests covering an unactioned re-run and each resolution path.

Out of scope:
- The first-encounter handoff's non-wording behaviour. The rebase driver's delete/edit
  classification and the decision to leave the path unstaged are correct and stay as they are.
  This project changes what happens on the next invocation; the first pause changes in wording
  alone (and, for config, only by routing the same report through the shared emitter rather than
  inventing a second one).
- Body-conflict resume. Requirement 7 of P6b fixes that body markers are never scanned before
  staging, because an unresolved body pushes visible markers and is the human's say-so. That
  rule is deliberate and must not be weakened by this work.
- The status-dispute resume. A `status_conflict` key already blocks `rebase --continue`
  correctly.
- Any new git plumbing. Everything needed is already exported by `internal/git`.
- Further design-note edits. The amendments that lock this resolution have already landed
  (see "Design silence and the resolution"); this project implements to them and re-decides
  nothing. The config-file case needs none either — the locked gate on `rebase --continue` is
  stated for any unactioned delete/edit path, not for project `.md` alone.
- The preflight's refusal of a root whose scope has no readable schema. It is what makes a
  removed `pj.cue` unreachable at the resumed stop; requirement 9 reports around it rather than
  changing it.

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

A pass ahead of that one handles the two allowlisted config files, `pj.cue` and `.gitignore`, and
asks the same question of the whole file: markers present pauses, markers absent means the human
resolved it in place, so it is staged. A config delete/edit has no markers either — git leaves the
surviving side's file untouched — so it too is staged and the deletion overruled. The routes
differ only in how the deletion reaches the other machine. A hand-deleted `.gitignore` is
allowlisted, so the snapshot commits the deletion and pushes it like any other change; a
hand-deleted `pj.cue` cannot be pushed by pj at all, because the per-git-root preflight refuses a
root whose schema will not evaluate, so it arrives only by a hand commit. Either way the machine
that receives it is where the survivor is staged, and for `pj.cue` what silently returns is a
scope's schema. The first pause misreports this case as well: it tells the human to resolve
conflict markers in a file that has none.

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

Earlier design notes covered the delete/edit pause and the human restoring or removing the
file before re-running `pj sync`. They did not cover a re-run where they did neither.

`AGENTS.md` requires that silence be flagged rather than guessed. P6b's own requirement 2 shows
the established pattern for this situation: state the resolution with the reasoning that
produced it, so the choice is deliberate and reviewable rather than an accident of
implementation. This document carries both the reasoning and the locked rule; the skill body
and the code that implement it are the live contract.

The resolution is locked at three sites:

- The unactioned re-run rule — the stage-set discriminator, the four outcomes, and why
  staging the survivor is the defect.
- The mid-rebase resume gate — an unactioned delete/edit joins `status_conflict` as a block
  on `rebase --continue`.
- The Conflicts-and-paused-sync skill row — the agent-facing rule that re-running without
  acting re-pauses by design.

The skill body and the implementing code are the live contract. This document records the
reasoning and the locked rule; if it and the tree disagree, correct this prose.

The two allowlisted config files are covered by that rule, not an extension of it. The layer-3
DECISION is written in merge vocabulary — stage sets, surviving frontmatter, a status to report —
because the frontmatter merge only ever sees project `.md`. The gate it produces is stated
without that restriction: the mid-rebase `Sync resume` row refuses `rebase --continue` "while a
delete/edit path still stands unactioned". A conflicted `pj.cue` or `.gitignore` with one side
absent is such a path, and nothing in the design licenses staging a survivor there. What the
config case cannot borrow is the report, which has no frontmatter and no status behind it; that
part is this document's, below.

The resolution is that an unactioned re-run must not stage the surviving file. Silence is the
defect: staging picks a winner and leaves nothing behind to show a choice was made, whereas
every other handoff class either blocks (`status_conflict`) or pushes something visibly
unresolved (body markers). A delete/edit that resolves itself pushes an ordinary-looking file,
so nobody ever learns the deletion was overruled.

Pausing again does not strand anyone, because every outcome has a signal the human gives in the
file itself. They are the design's "restore or remove the file" spelled out concretely — "remove"
as it stands, and "restore" split into the two things it can actually mean, since the surviving
file is already in the worktree and needs no restoring:

- Remove the file. The deletion wins.
- Modify the file. The surviving edit wins, carrying whatever the human wrote.
- Stage the file with `git add`. It is kept exactly as it stands. The path stops being unmerged,
  so the resumed stop never sees it.
- Do none of these. The stop pauses again on the same handoff.

The discriminator is therefore whether the worktree content still matches the surviving side's
stage blob. Matching means nothing was decided; a file that is gone, or present with different
content, means the human acted.

Gone means gone. The removal signal is an explicit does-not-exist result, never any failure to
read the file. A read that fails for another reason — a permission change, an I/O error, the path
replaced by a directory — is an operational fault, not a decision, and it aborts the integrate
with the path named rather than staging anything. Deletion is the one outcome here that loses
data, so it must rest on a positive signal: reading a fault as a deletion would stage a removal
and push it, destroying the surviving edit on a failure nobody ever saw. The git layer already
fixes this rule one layer down, where a failed `git show` must never be read as an absent stage
because that reclassifies a genuine git fault as a deletion; the resumed stop reads the worktree
for the same purpose and takes the same rule.

## References

- Archived design notes (historical; not live authority) — Merge conflict handling for the
  layer-4 handoffs and the delete/edit pause; Sync model for `pj sync`'s five steps and the
  resume contract.
- `docs/archive/06b-sync-and-push-boundary.md` — requirement 7 (the resume contract and the
  marker-freeness discriminator) and requirement 4 (the per-stop procedure). Explains why the
  current discriminator exists and what it was built to separate.
- `docs/archive/06a-frontmatter-merge-and-rebase-driver.md` — the rebase driver's contract,
  including the delete/edit outcome class and the stage plumbing it uses.
- `AGENTS.md` — pure Go, no cgo; external git binary; prefer packages, tests, and the
  embedded skill over archive prose; flag unclear behaviour rather than guessing.
- Project writing guide — `start get project/writing`.

## Requirements

1. A resumed stop distinguishes a delete/edit conflicted project `.md` from driver output. The
   marker-freeness test alone cannot do this, because both shapes have clean frontmatter; use
   the unmerged path's stage set, which is already exported. The surviving side is whichever of
   stage 2 and stage 3 the set reports present — both layouts are ordinary traffic (the other
   machine deleted while this one edited, and this machine hand-deleted while the other edited),
   so no fixed stage number may be assumed. Reading the absent side would ask git for a stage
   that does not exist, which `internal/git` surfaces as a fault, aborting the integrate and
   leaving the human with no route forward through pj.
2. A delete/edit whose worktree content still equals the surviving side's stage blob is treated
   as unresolved. It is not staged, the stop stays paused, and the run reports the same handoff
   detail the first pause gave — the path, which side deleted it, and the surviving status — so
   the human sees the same instruction on every re-run rather than a bare paused message.
   That detail comes from the rebase driver, not from a second copy of its reasoning in the sync
   loop: the unresolved file is re-driven and its outcome reported exactly as the first pause
   reported it. The surviving status is frontmatter, readable only by parsing the surviving stage
   blob, and the driver already does that and returns the whole triple. Re-deriving it in the sync
   loop would duplicate the parse, need its own answer for a blob whose frontmatter will not
   parse, and let the two pauses classify one file two ways — the merge parses every present stage
   before it branches on the stage shape, so an unparseable survivor is a fail-closed merge at the
   first pause while the stage set alone would call it a delete/edit at the resumed stop.
3. A delete/edit the human removed from the worktree stages as a deletion and lets the rebase
   continue. Removed is an explicit does-not-exist result on the path; every other read failure
   is an operational fault that aborts the integrate with the path named, staging nothing and
   leaving the rebase where it stands.
   This is the behaviour the existing test `TestSyncDeleteEditPausesThenResumes` covers and it
   must not regress.
4. A delete/edit the human modified stages as their resolution and lets the rebase continue.
5. A delete/edit the human resolved with `git add` never reaches the resumed stop, because it is
   no longer unmerged. Confirm this holds rather than adding a second path for it.
6. Body conflicts and status disputes are unaffected. A driver-output file with clean
   frontmatter and no `status_conflict` still stages whatever body the human left, with its body
   never scanned for markers.
7. The report for an unresolved delete/edit names the three ways out — remove the file, edit it,
   or `git add` it — so doing nothing is not the only obvious option. It replaces the current
   "restore or remove" wording, whose "restore" names no action at all: the surviving file is
   already in the worktree, so a reader who follows it does nothing and re-runs into the same
   pause. Whether the report also carries a closed-catalogue token is a decision for the
   implementer against the catalogue in `internal/token`; do not invent a new token.
8. The handoff line has exactly one emitter — the delete/edit branch of the driver-outcome
   reporter — and both pauses reach it, the first by driving the file and the resumed stop by
   re-driving an unresolved one, so no second formatter is introduced and neither message can
   drift from the other. Its wording is corrected in place: it names the deleting side in the
   reader's terms rather than the merge's stage vocabulary.
   `ours` is stage 2 — the incoming upstream tip, which during a rebase is the *other* machine's
   work — and `theirs` is stage 3, this machine's own commit being replayed, so printing those
   two words tells a human the opposite of what happened. Name each side for what it is: the
   incoming side fetched from the remote, or this machine's replayed commit. `fmmerge.Side` and
   the driver's `DeleteEdit` keep their stage-derived values unchanged; only the rendering moves.
9. A conflicted `pj.cue` or `.gitignore` that is a delete/edit takes the same rule as a project
   `.md`: the survivor is never staged on an unactioned re-run. The config pass runs before the
   project-file pass and tests the whole file for markers, so the same stage-set discrimination
   goes in ahead of that marker test, and the same read rule holds — only a does-not-exist result
   stages a removal. Unactioned re-pauses, a modified file stages as the human's resolution, and a
   `git add`-ed path never reaches the stop at all.
   Removal is where the two file kinds part. A removed `.gitignore` stages as a deletion and the
   rebase continues, exactly as a project `.md` does. A removed `pj.cue` never reaches the resumed
   stop: the per-git-root preflight refuses the whole root before the resume when a registered
   scope's schema will not evaluate, and a scope whose `pj.cue` is gone is that case. That refusal
   is correct — a scope with no readable schema cannot be merged — and stays as it is.
   The report is this project's own, because a config file has no frontmatter and no driver behind
   it, and it is written per file kind so that it never names a route the next run refuses. Both
   forms name the path and the deleting side in the same reader's terms as requirement 8, and
   neither uses the current "resolve the conflict markers" line, which points at markers a
   delete/edit does not have. The `.gitignore` form names all three ways out. The `pj.cue` form
   names editing the file or `git add`-ing it, and says that making the deletion win takes the
   scope out of pj's hands first: while a registered scope has no schema, sync refuses the root
   rather than merging it.
   That report has exactly one emitter — the same discipline requirement 8 imposes on project
   `.md`, applied where config cannot reuse the driver. Extend the existing config reporter (or
   a helper it calls) so it can emit either the marker-carrying form or the delete/edit form;
   both the first-pause procedure (`driveStop`) and the resume procedure (`resolveResumeStop`)
   classify the path with the stage set and call that one function. Do not format the delete/edit
   line in only one of the two procedures: a first pause that still says "resolve the conflict
   markers" while the re-run says the three ways out is the dual-formatter drift this project is
   closing for project files.
   A conflicted `pj.cue` keeps failing its scope's project `.md` closed while it stands
   unresolved, delete/edit or not; the fail-closed gate reads "this scope's schema is not
   trustworthy yet", which an unresolved deletion satisfies as surely as a marker does. On
   resume that gate is the `schemaConflicted` map the config pass already fills for a
   marker-carrying `pj.cue`: an unactioned `kindSchema` delete/edit must set the same entry
   before the project-file pass runs. Leaving the survivor unstaged and re-reporting is not
   enough — without the map entry the project pass treats the schema as clear and field-merges
   under the worktree survivor while the deletion of that schema is still open. A `pj.cue` the
   human resolved (modified content staged, or `git add`-ed so the path is no longer unmerged)
   does not set the entry, exactly as today after a marker resolution.
   A config file the human genuinely resolved in place still stages as it does today. Requirement
   6's rule is untouched: only the stage set moves a path onto the delete/edit route.

## Constraints

- Pure Go, no cgo. Every git invocation goes through `internal/git`. Do not add git plumbing —
  `ConflictStages` and `ShowStage` already cover this.
- This document's requirements and the implemented contracts override the Go CLI design
  guide. Do not invent behaviour from archive design prose.
- Non-interactive. The command never prompts; the human resolves in-file and re-runs.
- Do not weaken the body-marker rule. A body conflict's markers are never scanned before
  staging.
- The resumed stop must not re-drive a file the driver already field-merged. Re-reading its
  still-present stages would overwrite a body the human just resolved. An unresolved delete/edit
  is re-driven and is no exception to the rule behind that constraint: nothing was field-merged,
  and the merge's two writing branches — the composed field-merge and the add/add rename — are
  both reachable only with two present sides, so a stage set with one side absent cannot write at
  all. The stage-set test is itself the guarantee that re-driving is read-only.
- A failed read is never a deletion signal. Only a does-not-exist result stages a removal; any
  other error reading the worktree file or the surviving stage blob aborts the integrate.
- No state persisted across invocations. The current discriminator deliberately needs none, and
  the stage set is available from git at the moment it is needed.

## Implementation Plan

1. Read `resolveResumeStop` and the surrounding per-stop procedures in
   `internal/cli/sync_integrate.go`, then `git.ConflictStages` and `git.ShowStage` in
   `internal/git/integrate.go`, so the new discrimination fits the existing shape rather than
   duplicating it.
2. Add the delete/edit discrimination ahead of the existing clean-frontmatter staging branch,
   leaving the marker-carrying branch untouched.
   Add the same stage-set discrimination on the config side in both procedures that touch
   conflicted config files: the first-pause pass in `driveStop` (today always reports and
   pauses) and the resume pass in `resolveResumeStop` (today stages on marker-freeness). One
   shared stage-set test feeds both; the project and config paths then diverge only in what they
   do next — the project path re-drives for its report, the config path calls the shared config
   reporter.
   On resume, every still-unresolved `pj.cue` must set `schemaConflicted` for its scope dir —
   marker-carrying (already does) and unactioned delete/edit (must after this change). Do not
   treat "left unstaged" as a substitute for that map entry; the project-file pass keys only on
   the map. First pause already sets the entry for every conflicted `pj.cue` and keeps doing so.
3. Wire the three outcomes: unresolved re-drives the file and reports the driver's outcome through
   the existing outcome reporter, which already leaves it unstaged and pauses; removed stages the
   deletion; modified stages the resolution. On the config resume path the same three outcomes
   apply (with the `pj.cue` removal caveat in requirement 9), using the shared reporter for the
   unactioned case rather than a resume-only string. An unactioned `pj.cue` outcome also records
   `schemaConflicted` before the project-file loop; a staged resolution does not.
4. Correct the project `.md` delete/edit handoff wording where it already lives, in the outcome
   reporter's delete/edit branch (`applyDriverOutcome`) — the side naming and the three ways out.
   Extend the config reporter the same way so both `driveStop` and `resolveResumeStop` reach one
   function for the delete/edit form (and still one function for the marker-carrying form). No
   procedure-local format strings for either file kind.
5. Add tests. The critical one is the unactioned re-run: after the first pause, run `pj sync`
   again with nothing touched and assert the rebase is still paused, the handoff is reported
   again, and the remote still carries the deletion rather than the resurrected file. Cover the
   removed and modified resolutions too, and assert a body conflict still resumes as before.
   Cover the mirrored delete direction as well: this machine hand-deletes the project (the
   snapshot commits it as `pj: remove <id>`) while the other machine edits it, so the surviving
   side is stage 2 rather than stage 3. Its unactioned re-run must pause and re-report exactly
   as the other direction does, never abort on a stage read.
   Cover the config route with `.gitignore`, which reaches the other machine through pj alone: one
   machine hand-deletes it (the snapshot commits the deletion), the other edits it. Assert the
   first pause and the unactioned re-run print the same delete/edit line (path, deleting side,
   three ways out) — not the markers line on the first pause and the delete/edit line on the
   second. The unactioned re-run must leave the remote carrying the deletion; removing the file
   must complete the rebase with the deletion recorded.
   Cover the schema fail-closed gate: a stop where `pj.cue` is an unactioned delete/edit and a
   project `.md` under that scope is still unmerged must, on the next `pj sync`, leave the
   project file unmerged and report that it was not merged because the scope's `pj.cue` is
   conflicted — not field-merge it under the worktree survivor.
   `TestSyncGitignoreConflictDoesNotBlockProjectMerges` covers the marker-carrying config
   conflict and must keep passing unchanged.
6. Verify each new test fails against the current behaviour before the change, so it is a real
   regression test rather than a passing assertion.

## Implementation Guidance

- The sync loop's job is ordering, the continue-or-pause decision, and reporting. Reading the
  stage set to classify a path and comparing the worktree against the surviving stage blob are
  within that job. For a project `.md`, deciding what the handoff says stays with the driver,
  which already owns the classification and the surviving status. For a config file there is no
  driver: the shared config reporter owns both the marker-carrying and delete/edit forms, and
  both procedures call it after the same stage-set classification.
- Project `.md` and config share the stage-set and worktree-vs-survivor rules; they do not share
  the schema fail-closed map. Only `kindSchema` writes `schemaConflicted`. When wiring the
  config resume outcomes, copy the marker branch's map update onto the unactioned delete/edit
  branch for `pj.cue` — the "same rule as project `.md`" framing stops at staging and reporting.
- Re-driving an unresolved delete/edit adds no new abort path. The driver evaluates the scope
  schema before merging, but the per-git-root preflight already refuses the whole root ahead of
  the lock span when a participating scope's `pj.cue` does not evaluate, so that load cannot fail
  here for a reason the run has not already stopped on.
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
- The first pause and every re-run print the same handoff line, naming the deleting side as the
  incoming side or as this machine's replayed commit — never as `ours` or `theirs` — and listing
  all three ways out.
- A delete/edit whose surviving side carries unparseable frontmatter reports the identical
  fail-closed handoff at the first pause and at every unactioned re-run, naming the offending key
  — never a delete/edit line carrying an empty status.
- The same holds with the sides mirrored: this machine hand-deleted the project and the other
  edited it, so the surviving side is stage 2. The unactioned re-run pauses and re-reports rather
  than failing on a stage read.
- Removing the file and re-running completes the rebase, records the deletion, and pushes.
- Modifying the file and re-running completes the rebase, keeps the human's content, and pushes.
- Running `git add` on the file and re-running completes the rebase with that exact content and
  pushes.
- A hand-deleted `.gitignore` meeting a concurrent edit behaves the same way: the first pause and
  every unactioned re-run print the same delete/edit line — path, deleting side, three ways out —
  never the markers line on either invocation; the remote still carries the deletion; removing
  the file and re-running completes the rebase with the deletion recorded.
- An unactioned `pj.cue` delete/edit at a stop that also has an unmerged project `.md` under that
  scope keeps the project file unmerged on the next `pj sync` and reports the existing
  fail-closed line (scope's `pj.cue` is conflicted / not merged) — it does not field-merge under
  the worktree survivor.
- A body-only conflict still pauses with clean field-merged frontmatter and markers in the body,
  and the next `pj sync` after in-place resolution continues and pushes without its body being
  scanned.
- A config file carrying real conflict markers still pauses, and one the human resolved in place
  still stages and continues — the marker route is unchanged.
- No handoff names a route the next run refuses: the `pj.cue` delete/edit report never offers
  removal as a way out, since removing it refuses the root at the preflight with the existing
  `config_unparseable` line and leaves the rebase paused.
- Every new test fails against the pre-change behaviour.
