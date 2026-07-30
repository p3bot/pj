---
id: pj-p68k
status: todo
order: "aB"
created: "2026-07-30T22:00:01+10:00"
summary: Add pj meta set/add/rm so agents can mutate frontmatter without opening $EDITOR
---
# Meta write verbs (set, add, rm)

## Goal

Give agents a complete-state CLI for mutating project frontmatter fields (`summary`, `depends`, `related`, `tags`, `links`, and declared custom fields) without shelling `$EDITOR`. Today `pj meta` is read-only; the only mutators are status, reorder, claim, and free-form edit. Closing that gap is the authoring hot path agents actually need for edge and metadata work.

## Scope

In scope:
- Promote `pj meta` to a command family: keep the existing read (`pj meta <id>`), add `set`, `add`, and `rm` subcommands.
- Mutate allowed frontmatter keys with the same write durability as `status` / `reorder` (scope flock, mid-rebase refuse, index write-through, auto-commit self-commit / repo-driven `uncommitted:`).
- Stdin value form: a trailing `-` value means read the value from stdin (for long summaries and agent pipes).
- Same-scope `depends` integrity at write time: refuse an add of a target that does not exist in the scope.
- Tests and skill-contract updates for the new verbs.

Out of scope:
- Renaming `status` to `mark`, or a new scope-dashboard `status` (sibling work).
- Changing `pj list` columns or TSV/CSV shape (sibling work).
- Opening a full free-form YAML editor or multi-key batch API in one invocation.
- Changing merge/rebase rules for frontmatter fields (`internal/fmmerge` stays as-is).
- New frontmatter keys beyond the closed built-in set and existing CUE `fields` customs.
- `pj edit` behaviour, or making `create` accept initial meta flags.

## Current State

- `pj meta <id>` (`internal/cli/meta.go`) is pure read: preamble (`id`, `title`, `path`), blank line, raw frontmatter interior via `frontmatter.Split`. No subcommands.
- Built-in frontmatter keys (closed): `id`, `status`, `order`, `depends`, `related`, `tags`, `created`, `links`, `summary`, plus merge-only `status_conflict` (`internal/frontmatter`).
- Multi-value built-ins: `depends`, `related`, `tags`, `links`. Scalar built-in of interest: `summary`.
- Custom fields come from scope `pj.cue` `fields` with types `string|int|bool|strings` (`internal/scopeconfig`).
- Complete-state write pattern already exists for `status` and `reorder`: resolve id → scope flock → reconcile → refuse unusable/quarantined/duplicate → `readProjectFile` → mutate model → `writeProjectFile` → index write-through → self-commit or `uncommitted:` (`internal/cli/status.go`, `reorder.go`, `writeutil.go`).
- Depends edges are materialised into the index on reconcile; read path emits `depends_dangling:`, `depends_self:`, etc. There is no write-time gate yet that prevents creating a dangling same-scope edge.
- `pj status` owns status changes (and archive moves). `pj reorder` owns `order`. `id` and `created` are identity/history. Agents must not rewrite those via meta.
- No external product consumer of the skill body yet for these verbs; the skill file may be thin. Still update whatever skill surface documents board/meta commands so the contract stays the sole agent hand-off.

## Requirements

1. Command shape:
   - `pj meta <id> [--scope S]` — unchanged read behaviour and exit rules.
   - `pj meta set <id> <key> <value> [--scope S]`
   - `pj meta add <id> <key> <value> [--scope S]`
   - `pj meta rm <id> <key> <value> [--scope S]`
   Keep meta under the Board help group. Subcommand help must make the scalar vs multi-value split obvious.

2. Value from stdin: when `<value>` is exactly `-`, read the entire stdin as the value (no trailing-newline stripping beyond a single optional final newline, matching common CLI practice for piped text). Legal for `set` and for `add`/`rm` where a value is required. Empty stdin is an empty value (for `set` this clears a scalar when clearing is allowed).

3. Key classes and legal operations:
   - Scalar settable: `summary`; custom fields of type `string`, `int`, `bool`.
   - Multi-value add/rm: `depends`, `related`, `tags`, `links`; custom fields of type `strings`.
   - `meta set` on a multi-value key is a usage error (direct the agent to `add`/`rm`).
   - `meta add` / `meta rm` on a scalar key is a usage error.
   - Immutable via meta (refuse, no write): `id`, `status`, `order`, `created`, `status_conflict`. Point agents at the dedicated verbs where they exist (`status`, `reorder`).
   - Unknown key (not built-in multi/scalar above and not a declared custom field): usage error.
   - Custom field values must parse as the declared type; string/strings enums from `fields.*.values` must be enforced when present.

4. Multi-value semantics:
   - `add`: append `value` if not already present (idempotent success when already present; no duplicate entries).
   - `rm`: remove the matching entry if present (idempotent success when absent).
   - Comparison is exact string equality on the stored form.
   - Removing the last element omits the key on serialize (same as other empty optional slices today).

5. Depends integrity on `meta add <id> depends <target>`:
   - Target must be a legal project id form (short or full per existing id predicates).
   - Self-edge: refuse.
   - Same-scope target (after normalising to full id in the subject's scope): refuse if no non-quarantined project with that id exists in the scope. Do not write a known-dangling edge.
   - Cross-scope full ids: if the target scope is registered on this machine and the id is absent there, refuse; if the target scope is not registered, refuse as unresolvable rather than writing a blind edge. (Agents can still hand-edit if they truly need an offline placeholder; the CLI path stays strict.)
   - `related`, `tags`, and `links` do not require target existence.

6. Write durability and refuses (match status/reorder):
   - Scope flock for the write span.
   - Refuse unusable scope config, mid-rebase on auto-commit git-roots, quarantined parse_error rows, and duplicate-id collisions, with the same tokens/exit classes those verbs already use.
   - Atomic file rewrite via existing `writeProjectFile` / serialize path (preserve body; re-encode frontmatter through the typed model like other mutators).
   - Index write-through for the touched path so subsequent reads see new edges/tags/summary without a separate reconcile wait beyond the verb's own path.
   - Auto-commit scopes: self-commit the touched path with a clear fixed message (e.g. `pj: <id> meta set summary`). Repo-driven: emit `uncommitted:` after the write when dirty under the allowlist. `create`-style "never self-commit" does not apply; these are complete-state mutators.

7. Stdout hand-off: print the project file's absolute path (post-write path; meta never moves files) so agents can chain like other mutators. Stderr carries tokens/warnings only.

8. Tests covering: set summary (argv and stdin `-`); clear summary; add/rm depends with existing target; refuse missing same-scope depends target; refuse self depends; add tags idempotent; rm absent tag idempotent; refuse set on `depends`; refuse add on `summary`; refuse immutable keys; custom field type/enum; self-commit / uncommitted behaviour consistent with siblings; read `pj meta <id>` still works after mutations.

9. Update the embedded skill (and any CLI Long help) so agents discover `meta set|add|rm`, the stdin `-` form, and the depends existence rule without reading this project file.

## Constraints

- Pure Go, no cgo; follow AGENTS.md and existing `internal/cli` patterns.
- Do not invent new stderr token names when an existing token already fits; if a new closed token is required, add it to `internal/token` and keep the catalogue the single source.
- Do not bypass the typed frontmatter model with raw string splicing for these verbs (raw interior remains a read-path concern for `pj meta` display).
- Do not let meta change status, order, id, created, or status_conflict.
- Exit code classes stay consistent: usage/bad id → exit 2; unknown project → generic non-zero; integrity refuses → non-zero with token lines on stderr where siblings already emit them.
- Short-ids remain letter-first; examples and tests must honour `IsShortID`.
- No push from these verbs; sole push boundary remains `pj sync`.

## Implementation Plan

1. Lift `pj meta` to a parent command: default/read path stays `meta <id>`; attach `set`, `add`, `rm` subcommands with shared flag/resolve helpers.
2. Classify keys (immutable / scalar / multi-value / custom) against frontmatter builtins and the reconciled scope schema.
3. Implement mutate → write → index → self-commit path by reusing writeutil and the status/reorder orchestration, not a parallel durability stack.
4. Implement depends target resolution and existence checks only on `add depends`.
5. Wire stdin `-` value loading once for all three mutators.
6. Extend CLI tests (table-driven) for success and refuse cases; run `go test ./...`.
7. Update skill contract text and help Long strings for discoverability.

## Implementation Guidance

- Prefer one shared `runMetaMutate` (or similar) that takes an operation enum rather than three copy-pasted lock/reconcile blocks.
- Normalise depends/related targets to full ids when the argument is a short id in the ambient/subject scope, so stored edges stay full-id as the index already expects.
- For `tags`, free-form tags remain legal; optional `unknown_tag:` warn-only behaviour may mirror the read path but must not refuse the write.
- When serializing after mutation, keep the same quoting/`order` force-quote behaviour the frontmatter package already guarantees; do not special-case meta.

## Acceptance Criteria

1. `pj meta set <id> summary 'one line'` rewrites summary and prints the absolute project path; `pj meta <id>` shows the new summary in the raw interior.
2. `pj meta set <id> summary -` with piped stdin stores that text (single optional trailing newline stripped).
3. `pj meta add <id> depends <other>` succeeds when `<other>` exists in the same scope and appears in frontmatter/index edges; repeating the add does not duplicate.
4. `pj meta add <id> depends <missing>` refuses with non-zero exit and no file change when the target is same-scope and absent.
5. `pj meta add <id> depends <id>` (self) refuses with no write.
6. `pj meta rm <id> depends <other>` removes the edge; rm when absent is success with no spurious error.
7. `pj meta set <id> depends x` and `pj meta add <id> summary x` are usage errors (exit 2).
8. `pj meta set <id> status todo` (and id/order/created/status_conflict) refuse with no write.
9. Auto-commit scope self-commits a meta mutation; repo-driven scope emits `uncommitted:` rather than committing.
10. Skill and `pj meta --help` / subcommand help document set/add/rm, stdin `-`, and the depends existence rule.
