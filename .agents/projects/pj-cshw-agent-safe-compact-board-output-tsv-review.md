---
id: pj-cshw
status: backlog
order: "aG"
created: "2026-08-06T17:09:35+10:00"
summary: Review and decide compact board stdout that agents can still field-split when tabs are stripped
---
# Agent-safe compact board output (TSV review)

## Goal

Review whether pj's tab-separated board (and related) stdout is acceptable for agent consumers, and decide a contract that stays compact (low token cost) while remaining field-separable when agent tools strip or collapse tab characters. Land a written decision; implement only what that decision requires.

## Scope

In scope:

- `pj list` headerless TSV contract (full-id, status, title, waiting-on)
- Other tab-separated stdout that agents parse the same way (at least `pj search`, `pj scope list`, and `pj deps` neighbour lines) — decide whether one policy covers all or only list first
- Tradeoff between token/line density and reliability under agent capture paths that drop `\t`
- Skill text that documents the chosen list/search (and any format flag) contract
- Tests and closed output contracts if the decision changes bytes on stdout

Out of scope:

- Human-only pretty printers that inflate lines with padding as the sole default (unless the decision explicitly chooses dual format)
- JSON-envelope-first redesign of all pj stdout (path-centric model stays unless the decision says otherwise)
- Fixing agent read tools themselves (outside this repo)
- Durability `uncommitted:` / sync-needed work (`pj-gc4k`)
- Skill length reduction as a general edit pass
- related one-way write docs (already clarified in skill)

## Current State

Skill Board section documents list as headerless TSV:

```text
full-id \t status \t title \t waiting-on
```

Bare list = default active set; status args union-filter; `--all` expands; `--tag` is OR. Search TSV includes absolute path. Scope list is TSV (`name`, `dir`, `root`, `mode`). Deps neighbour lines use `id \t status \t label`.

Design intent: parse-stable machine lines, empty fields still delimited, no quoting hell for free-text titles, colour avoided on stdout for path/TSV stability. Token cost is already low: one short delimiter, no JSON keys, no headers.

Observed failure: some agent tool/capture paths strip or normalise tabs. Real tabs (`cat -A` shows `^I`) become gone, so a line such as:

```text
pj-fa5u\ttodo\tAgentdex-backed skill install list uninstall\t
```

is seen as:

```text
pj-fa5utodoAgentdex-backed skill install list uninstall
```

Exit code stays 0; the agent silently loses field structure and may invent status/title boundaries. This is a consumer-path problem, not pj concatenating fields.

Token-relevant alternatives discussed (not decided):

| Shape | Token density | Survives tab-stripping | Field-parseable with free-text titles |
|---|---|---|---|
| Tab TSV (today) | Best | Often no | Yes if tabs survive |
| Other single-char / short delimiter (e.g. `\|`) | Same order as TSV | Usually yes | Yes if delimiter policy for titles |
| Space-padded columns | Worse (padding) | Better for eyes; runs of spaces may still collapse | No (titles contain spaces) |
| JSON / key=value | Worst | Yes | Yes |
| Skill-only "preserve tabs" | No wire change | No | N/A |

Space padding was considered and rejected as a token-preserving sole format: free-text titles make whitespace split unsafe, and padding increases tokens without a reliable parse.

Repo output philosophy (AGENTS.md): path-centric with TSV/stdout hand-off; diagnostics on stderr. Any change must keep that spirit unless the decision explicitly supersedes it.

## Requirements

1. Produce an explicit decision recorded in this project (or a short Decision section update when implementing) choosing one of:
   - Keep tab TSV only (accept agent risk; optional minimal skill note)
   - Change default delimiter for list (and optionally search/scope list/deps) to a surviving compact form
   - Dual format: keep TSV default for scripts; add opt-in compact non-tab form; skill points agents at the opt-in
   - Another compact option that meets the same density and parse goals — only if it is clearly better than the above

2. The decision must state:
   - Which commands are covered
   - Default vs opt-in behaviour
   - How free-text titles (and empty waiting-on) remain unambiguous
   - Compatibility impact on existing tab-split consumers and tests
   - Whether skill text changes and how (prefer not growing skill length)

3. If the decision requires code changes, implement them under the same project (or spin a follow-on only if the decision alone is the deliverable and implementation is large — prefer one project through implementation when the change is a delimiter/flag-sized surface).

4. Prefer the principled long-term contract over a skill-only workaround that leaves silent mis-parse as the normal agent path.

5. Do not optimise for human column alignment at the expense of token density unless dual-format explicitly separates "table" from "wire".

## Constraints

- Pure Go, no cgo; closed, testable stdout contracts
- Title and waiting-on may contain spaces; any space-separated or padded-only scheme is not a field-parse contract
- Keep lines compact; do not adopt JSON as the default board format without a strong recorded reason
- Stdout purity: paths and table data on stdout; tokens on stderr
- Skill remains the agent contract; avoid duplicating a long format essay in Workflows
- Archive design prose is not live authority

## Implementation Plan

1. Inventory every TSV/tab stdout emitter agents commonly use (list, search, scope list, deps at minimum); note field counts and whether titles/labels can contain candidate delimiters.
2. Weigh options against token density, agent survival, title safety, and breakages; write the Decision into this document (or the commit message body is not enough — the project file must carry it).
3. If keeping tabs only: minimal skill clarification if useful; close with acceptance that agents may need non-tab capture or per-id `get`/`meta`.
4. If changing delimiter or adding a format: implement shared formatting helper, flags or defaults as decided, retarget tests, update skill one-liners, run package tests for CLI/skill.
5. Smoke with a tab-stripping simulation (e.g. `tr -d '\t'`) to confirm the chosen agent path still yields separable fields.

## Implementation Guidance

- Token budget is a first-class goal: same four (or N) fields, minimal delimiter, no padding-as-default
- A surviving ASCII delimiter at nearly the same cost as tab is the main alternative if silence under strip is unacceptable
- Dual format is the compatibility-preserving path if existing tab consumers must not break
- Do not treat "agents should use cat -A" as the primary fix unless the decision is explicitly keep-TSV
- Prefer one shared formatter over per-command one-off strings if multiple verbs share the policy

## Acceptance Criteria

1. This project file contains a clear Decision (chosen option, covered commands, default vs opt-in, title/empty-field rule, compatibility note).
2. If code changes: `pj list` (and any other covered commands) match that Decision; tests pin the bytes.
3. If skill changes: Board (and any other affected) lines match the Decision without an essay-length format section.
4. A simulated tab-delete of agent-facing output either still allows correct field recovery under the new contract, or the Decision explicitly accepts that failure mode for keep-TSV.
5. Default board output remains compact (no padded table or JSON default unless Decision records an exception with rationale).
