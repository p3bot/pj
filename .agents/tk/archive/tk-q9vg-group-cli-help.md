---
id: tk-q9vg
status: done
order: "Zz"
created: "2026-07-29T18:51:05+10:00"
summary: Partition root pj help into Work, Board, and Admin Cobra groups
---
# Group CLI Help

## Goal

Partition `pj --help` / `pj help` so top-level commands appear under three titled groups (Work, Board, Admin) instead of one flat list. Improve scanability for agents and humans without changing verb behaviour.

## Scope

In scope:
- Register three Cobra command groups on the root command and assign every first-class top-level verb to exactly one group.
- Stable group titles and membership as locked below.
- Tests that root help shows the group titles and that grouped commands render under them.
- Leave `help` and `completion` ungrouped (Cobra "Additional Commands").

Out of scope:
- Grouping subcommands of `pj scope` or `pj skill` (keep those help lists flat).
- Custom help templates, colour, or rewording of existing Short/Long strings beyond what grouping requires.
- New verbs, flags, skill-contract changes, or reorder of non-help behaviour.
- Changing exit codes, path hand-off, or command names.

## Current State

- Cobra v1.10.2 is already a dependency; it supports `Command.AddGroup`, `Group{ID, Title}`, and per-command `GroupID`.
- `internal/cli/root.go` `newRootCmd` builds the root tree with a single flat `AddCommand` list and no groups.
- Root help today is one "Available Commands" block (~18 entries, alpha-ish by registration).
- Nested parents with subcommands: `pj scope` (six verbs) and `pj skill` (three refuse placeholders). Neither is regrouped in this project.
- Cobra panics at registration if a subcommand sets a `GroupID` that was not `AddGroup`'d on the parent.

## References

- Cobra user guide: command groups (`AddGroup`, `GroupID`, `SetHelpCommandGroupID`, `SetCompletionCommandGroupID`) in the module at the pinned cobra version.
- `start get project/writing` — project document section contract for implementer hand-off.
- `pj skill` — agent work loop that this grouping should reflect (work path vs board vs admin).

## Requirements

1. On the root command, define three groups in this order (AddGroup order is help section order):
   - id `work`, title `Work:`
   - id `board`, title `Board:`
   - id `admin`, title `Admin:`
2. Assign top-level commands as follows (within each group, registration order should match this list so help reads as a mini workflow, not pure alpha):
   - Work: `create`, `get`, `edit`, `status`, `reorder`, `next`
   - Board: `list`, `meta`, `deps`, `search`, `query`, `lens`
   - Admin: `scope`, `sync`, `doctor`, `skill`
3. Do not assign a GroupID to the generated `help` or `completion` commands unless required to avoid mis-placement; default is leave them ungrouped under Additional Commands.
4. Root help (`pj`, `pj --help`, `pj help`) must show the three titled sections with the membership above.
5. No change to command Use lines, Short descriptions, flags, or runtime behaviour of any verb.
6. Add or extend CLI tests so a regression that drops a group title or mis-groups a key command fails the suite.

## Constraints

- Pure Go, no cgo; use stock Cobra group API only (no forked help template unless stock grouping cannot meet the titles/membership).
- Titles are exactly `Work:`, `Board:`, and `Admin:` (including the trailing colon, matching Cobra's usual title style).
- Group IDs are stable internal strings (`work`, `board`, `admin`); they are not user-facing beyond help rendering.
- Do not invent a fourth product group for skill, scope, or ops — those verbs live under Admin.
- Do not group `pj scope` or `pj skill` children in this project.
- Follow existing `internal/cli` construction patterns (`newRootCmd` / per-command constructors); keep the tree free of package-level command vars that leak flag state across tests.
- Prefer shared constants for group IDs/titles so membership cannot drift between AddGroup and GroupID assignment.

## Implementation Plan

1. Introduce root group ID/title constants and call `AddGroup` on the root before or when attaching children.
2. Set each top-level command's `GroupID` at construction (or immediately when added) so every first-class verb is covered.
3. Align `AddCommand` order with the membership lists under Requirements so within-group help order is intentional.
4. Run root help manually and via tests; fix any panic from missing group registration.
5. Extend CLI help/tree tests for titles and a representative command under each group.

## Implementation Guidance

- Prefer setting `GroupID` on the `cobra.Command` literal inside each `new*Cmd` only if that does not force awkward root coupling; setting GroupID in `newRootCmd` after constructing children is fine and keeps group taxonomy in one place.
- If a constructor is shared or reused as a non-root child later, do not hard-wire a root GroupID inside a shared helper that would be wrong on another parent — root assignment in `newRootCmd` is safer.
- Cobra's default template prints each group's Title then its commands; ungrouped available commands appear under "Additional Commands". Trust that unless a test proves otherwise.
- Do not call `SetHelpCommandGroupID` / `SetCompletionCommandGroupID` unless help/completion land in an unwanted section after grouping.

## Acceptance Criteria

1. `pj --help` (and equivalent root help) shows three sections titled exactly `Work:`, `Board:`, and `Admin:` in that order.
2. Under Work: `create`, `get`, `edit`, `status`, `reorder`, `next` appear (and no Board/Admin-only verbs).
3. Under Board: `list`, `meta`, `deps`, `search`, `query`, `lens` appear (and no Work/Admin-only verbs).
4. Under Admin: `scope`, `sync`, `doctor`, `skill` appear (and no Work/Board-only verbs).
5. `help` and `completion` remain available; they are not required to sit under Work/Board/Admin.
6. `pj scope --help` and `pj skill --help` remain a single flat Available Commands list (no new group headers).
7. Automated tests fail if a required group title is missing from root help or if a sample command from each group is absent from that section's block.
8. Existing verb behaviour and non-help CLI tests remain green; this project is help presentation only.
