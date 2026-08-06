---
id: pj-nmas
status: backlog
order: "aI"
created: "2026-08-06T20:55:39+10:00"
summary: Investigate knownTags contract and whether a scope tags CLI is warranted; decision only
---

# Investigate knownTags and a tags command

## Goal

Decide whether pj should grow a tags-related CLI surface (especially under `pj scope`), and what the long-term contract for `knownTags` should be: vocabulary hygiene only, or something agents and humans can list and maintain without hand-editing CUE. Land a written recommendation and, if work follows, one or more clearly bounded follow-on projects — do not implement a tags command in this project.

## Scope

In scope:

- Document the current `knownTags` contract (optional list in scope `pj.cue`, soft `schema_warn:` only, free-form tags remain legal)
- Map every code path that reads or checks `knownTags` (scopeconfig, lens, doctor diagnose, reconcile/schema cache)
- Compare product options for discovery and maintenance of tag vocabulary and in-use tags
- Recommend command shape if any (namespace, subcommands, read-only vs mutators), or recommend no new command with rationale
- Call out consistency pressure on other `pj.cue` keys (`statuses`, `fields`, `autoCommit`) if mutators are proposed
- Note durability implications (repo-driven host commit vs pj-driven self-commit/sync) for any schema write path
- Produce a decision section in this document and spawn follow-on project(s) only if implementation is warranted

Out of scope:

- Implementing any tags command, flags, or CUE rewrite helpers
- Changing doctor token wording or lens behaviour except as a recommendation for later work
- Redesigning project frontmatter `tags` multi-value semantics or `meta add|rm tags`
- Agent skill install/list surfaces, board TSV format work (`pj-cshw`), or sync/doctor extraction (`pj-bwgv`)
- Editing live consumer scopes' `pj.cue` (e.g. adding host-local lens tags like machine names) as part of this project

## Current State

Scope config may declare an optional controlled tag vocabulary:

```cue
knownTags?: [...string]
```

Design intent (archive design.md, still accurate vs code): free-form tags stay allowed; doctor warns on values not in the list (typos), it does not reject them. Empty or omitted `knownTags` means no vocabulary checks.

Code paths today:

| Area | Behaviour |
|---|---|
| `internal/scopeconfig` | Loads `KnownTags` into `ScopeSchema` |
| `internal/cli/lens.go` | `pj lens` warns `schema_warn:` when a lens tag is outside knownTags; lens still applies |
| `internal/cli/doctor_diagnose.go` | Project tags not in knownTags → `schema_warn:` (likely typo); integrity stays ok for soft warns alone |
| Project tags | `pj meta add|rm <id> tags <value>` — free-form; no closed set |
| Board filter | `pj list --tag T`, machine-local `pj lens` |

There is no `pj tags` and no `pj scope tags`. Scope verbs are registry lifecycle and rename only:

```text
pj scope init | import | rebind | forget | list | rename
```

After init, optional schema (`knownTags`, custom statuses, custom fields) is maintained by editing `pj.cue` by hand. `scope rename` is the only post-init rewrite of `pj.cue`, and only for the scope name.

Discovery today without a tags command:

- Declared vocabulary: open `pj.cue`
- Per-project tags: `pj meta get <id>`
- Typos / undeclared: `pj doctor`
- In-use inventory: ad-hoc `pj query` (schema unstable; debug only)

Motivation observed in product use: operators set a machine-local lens to a tag that is not in `knownTags` (e.g. host or person tags) and see `schema_warn:`; they ask how to extend the vocabulary and whether a list command should exist. That is a UX/discoverability gap, not a broken filter.

## Requirements

1. Produce a written Decision in this project file covering:
   - Keep hand-edited `knownTags` only vs add a CLI surface
   - If CLI: recommended namespace (`pj scope tags` vs top-level `pj tags` vs other) and why
   - Read-only list only vs mutators (`add`/`rm`) vs usage inventory (known vs in-use)
   - Whether soft-warn semantics stay; any recommended change to lens vs project-tag warning policy
   - Consistency: if mutators, how statuses/fields/autoCommit stay out of scope or get a stated general rule
   - Explicit non-goals for any follow-on implementation

2. The Decision must be implementable by a fresh-session agent without conversation context: enough to open one or more follow-on projects with clear Goal/Scope, or to close with "no CLI change".

3. Inventory findings must cite concrete packages/files and the soft-warn token contract; prefer code and skill over archive design prose when they disagree.

4. If the recommendation is "implement X", create one or more backlog/todo follow-on projects (separate files via `pj create`) that point at the principled long-term solution — not a smallest-diff patch. This investigation project itself remains investigation-only.

5. If the recommendation is "no command", document how agents/humans should discover and extend vocabulary (doctor, `pj.cue`, skill one-liner if any), and whether a skill-only clarification is enough.

## Constraints

- Investigation and writing only in this project; no production code or skill behaviour changes required to mark this done
- Do not invent behaviour that contradicts closed contracts already in code (token catalogue, free-form tags, soft schema_warn)
- Archive design prose is not live authority
- Any recommended mutator path must acknowledge CUE fidelity (comments, formatting, multi-file schema) and pj-driven vs repo-driven durability — do not hand-wave "just rewrite the file"
- Prefer not growing the skill with essay-length schema docs; one-liners only if a follow-on lands
- Pure Go / no cgo and path-centric stdout philosophy remain defaults for any future CLI shape recommended here

## Implementation Plan

1. Re-read live sources: `scopeconfig` schema fields, lens warn path, doctor diagnose knownTags loop, skill Board/Manage scopes text, AGENTS.md module layout. Note any drift from archive design wording.
2. Survey how real scopes use `knownTags` (this repo's projects scope if any; at least one multi-tag consumer scope if available on the machine) — declared list vs tags actually on projects vs lens tags.
3. Evaluate options against discoverability, typo prevention, CLI surface growth, CUE mutation risk, and consistency with other schema keys. Prefer the principled long-term contract over a one-off `tags add`.
4. Write the Decision into this document (chosen option, command shape or none, mutator policy, follow-on plan).
5. If implementation is warranted, `pj create` follow-on project(s) with Goal/Scope/Requirements that reference this Decision; leave them backlog or todo as appropriate. Do not start coding them under this id.
6. Optional: minimal skill or AGENTS note only if the Decision says discovery docs are the sole deliverable and the text belongs in-repo now — prefer documenting the recommendation here and landing skill edits in a follow-on when behaviour changes.

## Implementation Guidance

- Default bias: scope config stays CUE; CLI is for board and registry unless a clear, repeated operator pain justifies schema mutators
- `pj scope tags` is a better namespace than top-level `pj tags` if a command exists, because vocabulary is scope schema not board data
- Read-only list is a smaller commitment than add/rm; usage inventory may already be half-covered by doctor schema_warn lines
- Host-local or personal lens tags may deliberately stay outside shared `knownTags` — the Decision should say whether that is supported (warn forever) or discouraged (add to vocabulary)
- Do not treat "agents should open pj.cue" as failure if the Decision keeps hand-edit; do treat silent vocabulary with no discoverable list as a real agent friction if evidence supports it

## Acceptance Criteria

1. This project file contains a Decision section with a clear recommend/do-not-recommend and the points required under Requirements item 1.
2. Current State (or Decision) cites the real code paths and soft-warn semantics accurately against the tree at investigation time.
3. No tags command or `knownTags` mutator is implemented under this project id (investigation only).
4. If follow-on work is recommended, at least one new pj project exists that an implementer can pick up without reading this conversation; if no follow-on, the Decision states that explicitly.
5. Out-of-scope items above remain untouched (no drive-by doctor/lens/meta changes).
