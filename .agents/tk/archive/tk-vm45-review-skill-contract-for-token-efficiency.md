---
id: tk-vm45
status: cancelled
order: "a1"
created: "2026-07-29T19:08:09+10:00"
summary: Interactively review each pj skill section to cut tokens and improve agent transfer
---
# Review skill contract for token efficiency

## Goal

Review the embedded agent skill contract (`internal/skill/skill.md`, printed by `pj skill`) section by section with a human, so each pass reduces token load while improving how well a fresh agent session absorbs the rules. Produce a tighter skill that still preserves locked behaviour and tests.

## Scope

In scope:
- The skill opening (YAML frontmatter + H1 + text before the first `##`) and all eighteen locked `##` sections, reviewed one unit at a time interactively.
- Edits to `internal/skill/skill.md` that compress wording, remove redundancy, and raise knowledge transfer (clearer rules, fewer caveats that do not change decisions).
- Updates to skill structure tests only when a deliberate section rename, merge, split, or closed-token catalogue change is agreed with the human (see Constraints).
- A short running log in this project body (under Implementation Plan or a Review log subsection) recording which units are done and any open hold items.
- Open product decisions that change agent rules in the skill (see Decisions). Resolve each with the human during the units that own the prose; record the choice and apply the skill edit only after agreement.

Out of scope:
- Redesigning the CLI surface, exit codes, or index schema to "make the skill shorter."
- New verbs or merge helpers for frontmatter unless the human expands scope after a Decision.
- Rewriting AGENTS.md or archived design docs as the primary artefact (skill.md is the sole runtime contract).
- Bulk automated rewrite of the whole skill in one shot without per-section human confirmation.
- Changing doctor token strings in `internal/token` except when the human explicitly approves a catalogue change and all emitters/tests follow.

## Current State

- `pj skill` embeds and prints `internal/skill/skill.md` (~500 lines / ~36KB). That is the full agent-facing contract each cold session may ingest.
- Eighteen locked `##` headings are asserted in order by `internal/skill` (`requiredHeadings` + tests). The file opens with Agent Skills frontmatter (`name: pj`, trigger `description`) and H1 `# pj`. Tests also require every closed doctor token from `token.All()`, end-of-turn/conflict needles, no `design.md` dependency, and no placeholder markers.
- Section list (review order):
  1. Preamble (before first `##`)
  2. Core work loop
  3. Capture
  4. Frontmatter mutation
  5. Body conventions
  6. Title, slug, and filename
  7. Ordering
  8. List and filters
  9. Search
  10. Dependencies and impact
  11. Archive
  12. End of turn (by autoCommit mode)
  13. Conflicts and paused sync
  14. Concurrent agents
  15. Cold start and import
  16. Cross-scope work
  17. Waiting and external blockers
  18. Unsupported operations
  19. Doctor and integrity warnings
- Token waste patterns to watch for: repeated restatements of the same rule across sections, long illustrative dumps that do not change agent action, tables that restate prose, hedging, conversation-style asides, and detail better left to `--help` or code when the skill only needs a pointer.
- Observed agent failure (first live use of pj in this scope): after `pj create`, an agent full-file-wrote the project markdown and replaced the create-owned frontmatter fence (including inventing or "fixing" `created`, and rewriting other keys as a side effect of body authoring). The skill already restricts individual keys (`id`/`created` never; `order`/`status` via verbs) but does not state a hard fence-preservation rule for body edits. Capture/Body only say soft phrases like "fill under the H1." Frontmatter mutation still allows direct edit of `summary`, `depends`, `tags`, `links`, and custom fields — so key rules and whole-file clobber are different problems.

## First-use agent feedback

Source: agent reflection after the first real session using this scope (create, body authoring, status, list/meta, skill print, two product projects). Not a user requirement list — input for skill compression priorities and knowledge-transfer checks. Do not copy this section into `skill.md` wholesale; fold only durable rules that survive review.

What already helps agents:
- Path hand-off is the right primitive: `create` / `get` / `status` / `next` print an absolute path; open that file. No inventing filenames.
- One project = one markdown file matches multi-session hand-off: the implementer needs that document, not chat history or a proprietary board as source of truth.
- Status as labels plus archive layout is simple enough to drive from verbs without simulating a workflow engine.
- `pj skill` as the sole agent contract is the right shape for a cold session.
- Repo-driven mode is honest for a normal git repo (`uncommitted:`) instead of forcing `pj sync` ownership.

Friction that showed up immediately:
- Frontmatter is easy to destroy via whole-file body writes (see Current State and Decision D1).
- Skill size (~36KB / ~500 lines) is high token cost every time a session leans on `pj skill`. High value, high cost — primary motivation for this project.
- Discovery vs doing: a flat root help wall and a large skill make "which verb?" slower than the hot path once known (`create` → body under H1 → `status` → `next --claim`). Root help grouping is a separate project (`pj-q9vg`); skill review should still make the hot path scannable early (Core work loop / Capture).
- Two durability stories (repo-driven host commit vs pj-driven `sync`) are correct product design; agents will cargo-cult `sync` unless end-of-turn prose stays short and mode-explicit.
- Board features (depends, related, search, next eligibility) matter more as the portfolio grows; skill must not bury the minimal loop under rare concurrent/cross-scope detail.

When pj is handy for agents:
- Durable project documents with stable ids, queue claim, status, and machine-local index — especially when a later session or another agent must pick up without chat history.

When it is the wrong tool:
- Ephemeral chat todos, tiny one-shot edits, or heavy PM (sprints, assignees, estimates) that should not become project files.

Net for this review: keep the ledger model; prioritise a shorter skill and inviolable frontmatter fence so the loop feels natural. Prefer cuts and reorder that surface path hand-off, body-under-H1, status/next rules, and mode-appropriate end-of-turn early; push rare multi-clone/cross-scope/doctor catalogue detail later or compress hard without dropping decision rules.

## Decisions

Resolve with the human before or during the named units. Do not encode a bar into `skill.md` until chosen. Record the decision here when made.

### D1 — Agent edits to project frontmatter

Status: open

Problem: Agents must not destroy create/verb-owned frontmatter when writing the body. Unclear how far the ban goes: preserve the fence only, or forbid all agent frontmatter edits.

Options (pick one or a hybrid; rewrite skill prose to match):

1. Fence preservation (body path). When authoring or editing the markdown body, never replace the YAML frontmatter block. Edit only after the closing fence (under the H1). If the tool only supports full-file write: read first, copy the existing fence through unchanged, change body only. Never invent or repair `id`, `created`, or `order` from memory. Deliberate direct FM edits for keys the skill already allows (`summary`, `depends`, `related`, `tags`, `links`, customs) remain legal as a separate, intentional edit — not as a side effect of body write.
2. Agents never edit frontmatter. All FM changes go through verbs only. Requires either accepting that `summary`/edges/tags cannot be set until new verbs exist, or adding those verbs (out of scope unless expanded). Stronger safety; larger product gap today.
3. Hybrid. Fence preservation always (option 1 body rule) plus a tighter allowlist or "human direction only" for direct FM keys; document which keys agents may touch without asking.

Own units: Capture, Frontmatter mutation, Body conventions (and a one-line pointer in Core work loop if the early loop still says only "file tools on that path"). Cross-check Unsupported operations if a new refuse line is warranted.

Chosen: (unset)

Skill change notes: (unset)

## References

- Live contract: `internal/skill/skill.md` (edit target); print with `pj skill`.
- Structure lock: `internal/skill/skill.go` (`requiredHeadings`), `internal/skill/skill_test.go`.
- Closed tokens: `internal/token` (skill must keep every token string present unless catalogue changes with human approval).
- Project writing guide: `start get project/writing` (knowledge-transfer bar for agent-facing prose applies to the skill too: explicit, self-contained, no conversation references).
- Archived P7 design context: `pj-r748` / `projects/archive/pj-r748-p7-skill-contract-and-discovery.md` (history only; must not override the embedded skill).

## Requirements

1. Work unit-by-unit in the Current State list. For each unit: show the current text (or a tight summary plus path/line range), propose concrete edits aimed at fewer tokens and better transfer, wait for human decision, apply only agreed edits to `skill.md`, then mark the unit done in this project document.
2. Do not advance to the next unit until the human accepts or explicitly defers the current one.
3. Prefer cuts and merges that preserve decision-relevant rules: absolute path hand-off, status/next eligibility, autoCommit end-of-turn modes, sole push boundary, doctor token actions, refuse-list, and other locked behaviour.
4. After any edit batch that should stay green, run the skill package tests (`go test ./internal/skill/...`) and fix fallout before claiming the unit complete.
5. If a change needs a heading rename, section merge/split, or token catalogue change, stop and get explicit human approval; then update `requiredHeadings` and tests in the same change set.
6. Keep this project document as the session hand-off: a late agent must see which units are done, what was deferred, and what remains.
7. Before finishing Capture / Frontmatter mutation / Body conventions, resolve Decision D1 (or record an explicit deferral with reason). Apply the matching skill wording in those units only after the human chooses a bar. While D1 is open, agents working this scope should still avoid whole-file frontmatter clobber in practice (treat option 1 body rule as interim hygiene, not as the locked skill text).

## Constraints

- Skill remains the sole runtime agent contract: no new dependency on design docs.
- Preserve closed token prefixes in the skill unless the human approves a catalogue change end-to-end.
- Do not invent CLI behaviour in the skill that code does not implement; if prose and code disagree, flag it — do not "fix" by writing fiction into the skill.
- Australian English is fine if already used; do not churn spelling for its own sake.
- Token efficiency is secondary to correctness: never drop a rule that changes agent action solely to shorten the file.
- Interactive gate is mandatory: no silent multi-section rewrite.

## Implementation Plan

1. Claim this project when starting a review session; open this file and `internal/skill/skill.md`.
2. Review units in list order (preamble first, then the eighteen `##` sections). For each:
   - Read the unit.
   - Draft a proposal: keep / cut / rephrase / move (with rationale tied to tokens or transfer).
   - Human decides.
   - Apply agreed text; run skill tests when the file changed.
   - Tick the unit in the Review log below.
3. When reaching Capture, Frontmatter mutation, and Body conventions: surface Decision D1, get a human choice, record it under Decisions, then propose the concrete skill diff for those units.
4. After all units are done or explicitly deferred, do a final full-file read for cross-section duplication that unit review missed; one optional consolidation pass with human approval.
5. Run `go test ./internal/skill/...` (and broader CLI skill tests if headings or `pj skill` output shape changed).
6. Mark this project done when the human accepts the review as complete (including any recorded deferrals). Open Decisions must be chosen, deferred with reason, or explicitly accepted as out of scope before done.

### Review log

Track progress here (update as units finish):

- [x] Preamble — Agent Skills frontmatter (`name: pj` + trigger description), H1 `# pj`, path hand-off blurb tightened
- [ ] Core work loop
- [ ] Capture (includes D1 skill wording if chosen)
- [ ] Frontmatter mutation (includes D1)
- [ ] Body conventions (includes D1)
- [ ] Title, slug, and filename
- [ ] Ordering
- [ ] List and filters
- [ ] Search
- [ ] Dependencies and impact
- [ ] Archive
- [ ] End of turn (by autoCommit mode)
- [ ] Conflicts and paused sync
- [ ] Concurrent agents
- [ ] Cold start and import
- [ ] Cross-scope work
- [ ] Waiting and external blockers
- [ ] Unsupported operations
- [ ] Doctor and integrity warnings
- [ ] Final cross-section pass (optional)
- [ ] Decision D1 resolved (chosen / deferred with reason)

Deferred / notes:
- D1 agent frontmatter policy open — resolve during Capture / Frontmatter / Body (see Decisions).

## Implementation Guidance

- Knowledge transfer beats cryptic density: a slightly longer single rule that an agent will follow is better than a short line that is easy to misread.
- When two sections repeat the same rule, keep the canonical statement in the section that owns the decision and replace the other with a one-line pointer to that section title.
- Doctor section is large by necessity (closed token table). Compress surrounding prose and duplicate action text; do not casually delete token rows.
- Tables are fine when they compress many rows of parallel facts; convert tables to prose only when the table is sparse or narrative.
- Measure roughly with line/byte count of `skill.md` before and after the full review if useful; do not optimise to a fixed percentage target.
- Use First-use agent feedback when prioritising: hot path and durability mode clarity beat polishing rare sections first; do not delete multi-agent/cross-scope rules solely because the first session did not need them — compress and defer in reading order instead.
- While reviewing, ask: would a cold agent still choose path hand-off, body-under-H1 only, correct next/claim, and the right end-of-turn boundary after this cut?

## Acceptance Criteria

1. Every unit in the Review log is either checked done or explicitly deferred with a note in this document.
2. Agreed edits are in `internal/skill/skill.md` (and tests/headings/tokens only when approved).
3. `go test ./internal/skill/...` passes after the final applied batch.
4. The skill still opens with Agent Skills frontmatter (`name: pj` + description) and H1 `# pj`, still contains the required heading set (or the updated locked set if the human approved a structural change), and still includes every closed token string unless the catalogue was deliberately changed.
5. A fresh agent reading only `pj skill` can still run the core work loop and end-of-turn durability rules without consulting this project document.
6. This project body records enough review state that a different session can resume mid-list without redoing finished units.
7. Decision D1 is either encoded in the skill per the chosen option, or explicitly deferred/out-of-scope in this document with a reason the human accepted.
