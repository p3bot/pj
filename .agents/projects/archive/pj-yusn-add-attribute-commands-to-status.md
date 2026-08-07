---
id: pj-yusn
status: done
order: "aJ"
created: "2026-08-07T13:25:57+10:00"
summary: Optional pj status [key] prints one locked pulse field bare value; full pulse unchanged
---
# Add attribute commands to status

## Goal

Extend `pj status` so an agent can request one locked pulse field by name and receive only that field’s bare value on stdout. Today the only way to learn scope mode, next, integrity, or any other pulse key is to print and parse the full dashboard. A single-key form (`pj status mode`) matches the agent scripting pattern already established by `pj meta get <id> <key>` and cuts token cost for the common one-field read.

## Scope

In scope:

- Optional single positional key on `pj status`: `pj status [key] [--scope S]`
- Bare-value stdout for the one-key path; full padded pulse unchanged when no key is given
- Closed key catalogue identical to today’s locked `statusKeys` wire names
- Usage refuse for unknown keys (exit 2) with a message that lists known keys
- Long help, skill Orient contract, and tests for both paths
- Keep pure-read behaviour: same reconcile/selection/integrity construction as full status

Out of scope:

- Multi-key selection in one invocation (agents re-invoke or a later project)
- Changing full-pulse layout, padding, key order, or field meanings
- Optimising the one-key path to skip next selection, reconcile, or other pulse work
- Subcommand tree (`pj status get …`)
- Board TSV delimiter work (`pj-cshw`), tags vocabulary (`pj-nmas`), or skill token-efficiency rewrites beyond the one-line Orient update this feature needs
- New pulse keys or aliases for wire names (`in-progress`, `blocked_ids` stay exact)

## Current State

- `pj status [--scope S]` (`internal/cli/status.go`) is pure-read orientation: fixed key order, left-padded keys to `statusKeyWidth`, single tab, value; empty values still emit `key\t`. Exit 0 when ambient resolve succeeds, including empty `next`.
- Locked keys (order fixed): `scope`, `dir`, `resolved`, `mode`, `lens`, `total`, `todo`, `review`, `in-progress`, `blocked`, `draft`, `backlog`, `done`, `cancelled`, `next`, `claimed`, `blocked_ids`, `dangling`, `integrity`, `uncommitted`.
- Command currently uses `cobra.NoArgs`; unexpected positionals are usage exit 2.
- Pulse map is built in one place after reconcile/closure, next selection, counts, and health; stdout loops `statusKeys`.
- `pj meta get <id> [key]` already defines the agent pattern for attribute reads: no key → rich multi-line form; one known key → bare value only; unknown key → exit 2 listing known keys; absent/empty value → empty stdout, exit 0.
- Skill Orient path points agents at `pj status` as the cheap scope pulse; it does not document a single-key form.

## Requirements

1. Command surface: `pj status [key] [--scope S]`. Zero positionals keep today’s full pulse contract byte-for-byte (same keys, padding, order, empty-value lines, exit 0 rules, stderr tokens). Exactly one positional is the attribute path. Two or more positionals are usage exit 2.

2. Attribute path stdout: print only the decoded bare value for that key, followed by a newline when the value is non-empty. Do not print the key name, padding, or tab. Match meta single-key spirit: value only for piping (`mode=$(pj status mode)`).

3. Empty values: when the selected key’s value is empty (`next` with no eligible project, empty `lens`, empty `claimed` / `blocked_ids`, etc.), write empty stdout and exit 0. Do not invent a placeholder, do not fall back to the full pulse.

4. Known keys: the attribute catalogue is exactly the locked full-pulse key set (same strings as stdout keys today, including hyphens and underscores). No aliases in this project.

5. Unknown key: usage exit 2, empty stdout, message names the bad key and lists the known catalogue (same class of refuse as `pj meta get` unknown key). Do not treat unknown as empty success.

6. Shared computation: the attribute path must use the same pulse construction as the full dashboard (same reconcile/closure, next selection, counts, integrity, uncommitted rules, stderr token behaviour). Select one entry from the finished map; do not fork a lighter path that can drift from full `status` for the same key under the same scope.

7. Scope flag and ambient resolution behave as today for both paths. Attribute selection does not change when resolve fails: non-zero exit and empty stdout as other pure reads.

8. Help: `Use` and Long text document optional `[key]`, state that one key prints the bare value, list or clearly point at the locked key set, and keep the existing full-pulse documentation for the no-key path. Short description may stay dashboard-oriented; clarify single-key in Long.

9. Skill: update the agent skill Orient line (or equivalent one-liner) so agents know `pj status [key]` exists for bare-value reads. Do not grow an essay; do not rewrite unrelated skill sections under this project.

10. Tests:
    - Full pulse with no args still matches locked key order and padding (existing coverage remains valid).
    - Each (or a representative set of) locked keys returns the same value the full pulse would show for that key under the same fixture.
    - Empty-value key (e.g. empty `next`) → empty stdout, exit 0.
    - Unknown key → exit 2, empty stdout.
    - Extra positionals → exit 2.
    - Attribute path does not emit full pulse lines or key names on stdout.

## Constraints

- Pure Go, no cgo; follow AGENTS.md and existing CLI patterns.
- Pure read: no flock mutation, no file writes, no push, no self-commit.
- Do not change full-pulse field meanings, key order, or padding width rules except as required to share the map with the attribute path.
- Do not introduce multi-key output formats in this project.
- Archive design prose is not live authority; code and skill win on conflict.
- Repo-driven durability for this scope: host commit when the work is done; do not `pj sync`.

## Implementation Plan

1. Read `internal/cli/status.go` and `status_test.go`; note `statusKeys`, pulse build, and `NoArgs` gate. Read meta get’s unknown-key usage pattern for refuse shape consistency.
2. Allow zero or one positional; keep full-pulse stdout path when args are empty; when one arg is present, resolve it against the locked catalogue, build the same pulse map, print bare value or empty success / usage refuse.
3. Update Long help and skill Orient one-liner for the optional key.
4. Add table-driven or fixture tests covering known key, empty value, unknown key, and multi-arg refuse; keep existing full-pulse assertions green.
5. Run package tests for `internal/cli` and `internal/skill` (and any other packages touched).

## Implementation Guidance

- Prefer extracting or reusing one pulse-build function that both paths call, then either emit all keys or look up one — over duplicating reconcile/count logic.
- Mirror meta get’s exit and empty-stdout conventions where they apply; status remains a scope pulse, not a project-id verb.
- Multi-key support is deliberately deferred: if agents need two fields, two invocations are acceptable. Do not half-implement multi by printing two bare lines (empty fields make that unparseable).

## Acceptance Criteria

1. `pj status` with no key produces the same full-pulse contract as before this project (locked keys, padding, empty lines present).
2. `pj status <key>` for each locked key prints only that key’s bare value (or empty stdout when the value is empty) and exits 0 when ambient resolve succeeds.
3. `pj status mode` on a repo-driven scope with a usable schema prints `repo-driven` (or the correct mode label for the fixture) with no key name or tab on that line.
4. Unknown key and two-or-more positionals are usage exit 2 with empty stdout.
5. Attribute path values match the corresponding full-pulse values under the same scope and fixture (no forked semantics).
6. Skill Orient (or equivalent) documents optional `[key]` for bare-value reads without an essay-length status section.
7. Long help documents optional key, bare-value behaviour, and the locked catalogue.
