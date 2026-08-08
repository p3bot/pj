---
id: tk-p68k
status: done
order: "aB"
created: "2026-07-30T22:00:01+10:00"
summary: Add pj meta get/set/add/rm so agents can read and mutate frontmatter without $EDITOR
---
# Meta command family (get, set, add, rm)

## Goal

Give agents a complete-state CLI for reading and mutating project frontmatter fields (`summary`, `depends`, `related`, `tags`, `links`, and declared custom fields) without shelling `$EDITOR`. Today `pj meta` is a single read-only verb; the only mutators are status, reorder, claim, and free-form edit. Closing that gap is the authoring hot path agents need for edge and metadata work.

## Scope

In scope:
- Promote `pj meta` to a command family: `get`, `set`, `add`, `rm` (with `remove` as an alias of `rm`).
- Full-header get with a revised preamble; optional single-key get for piping.
- Mutate allowed frontmatter keys with the same write durability as `status` / `reorder` (extend existing paths; do not redesign them).
- Stdin value form: a trailing `-` value means read the value from stdin.
- Write-time `depends` integrity on `meta add … depends` (existence and self-edge refuses); related stays soft (normalise short→full, no existence/self hard refuse).
- Tests and skill-contract updates for the new verbs and get output shape.
- In-tree call-site copy and tests that hard-code `pj meta <id>` or the old preamble (`id`/`title`/`path`) — update to `meta get` and title-then-path (e.g. `list` Long help). Not a change to list columns or TSV shape.

Out of scope:
- Renaming `status` to `mark`, or a new scope-dashboard `status` (sibling work).
- Changing `pj list` columns or TSV/CSV shape (sibling work).
- Opening a full free-form YAML editor or multi-key batch API in one invocation.
- Changing merge/rebase rules for frontmatter fields (`internal/fmmerge` stays as-is).
- New built-in frontmatter keys beyond the closed set; undeclared free-form keys via meta.
- `pj edit` behaviour, or making `create` accept initial meta flags.
- A `meta clear` / full-key wipe verb.
- Changing existing depends read diagnostics, durability mechanics, token catalogue policy, or other sibling verb behaviour beyond the extensions needed for these commands.

## Current State

- `pj meta <id>` (`internal/cli/meta.go`) is pure read: preamble (`id`, `title`, `path`), blank line, raw frontmatter interior via `frontmatter.Split`. No subcommands.
- Built-in frontmatter keys (closed): `id`, `status`, `order`, `depends`, `related`, `tags`, `created`, `links`, `summary`, plus merge-only `status_conflict` (`internal/frontmatter`).
- Multi-value built-ins: `depends`, `related`, `tags`, `links`. Scalar built-in of interest: `summary`.
- Custom fields come from scope `pj.cue` `fields` with types `string|int|bool|strings` (`internal/scopeconfig`).
- Complete-state write pattern already exists for `status` and `reorder`: resolve id → scope flock → reconcile → refuse unusable/quarantined/duplicate → `readProjectFile` → mutate model → `writeProjectFile` → index write-through → self-commit or `uncommitted:` (`internal/cli/status.go`, `reorder.go`, `writeutil.go`).
- Depends edges are materialised into the index on reconcile; read path emits `depends_dangling:`, `depends_self:`, etc. There is no write-time gate yet that prevents creating a dangling same-scope edge.
- `pj status` owns status changes (and archive moves). `pj reorder` owns `order`. `id` and `created` are identity/history. Agents must not rewrite those via meta.
- Skill body for board/meta commands will need updating so the contract stays the sole agent hand-off.

## Locked command surface

```
pj meta                              → meta family help (not top-level help)
pj meta get    <id> [key]            [--scope S]
pj meta set    <id> <key> <value>    [--scope S]
pj meta add    <id> <key> <value>    [--scope S]
pj meta rm     <id> <key> <value>    [--scope S]
pj meta remove …                     # alias of rm
```

- Arg order is always `op → id → key → value` (get: optional key, no value).
- No bare `pj meta <id>` (breaking change accepted; package still under development).
- No dual-tier parent RunE that takes an id.
- Keep meta under the Board help group.
- Subcommand help must make the scalar vs multi-value split obvious.

## Requirements

### 1. `meta get` — full header (no key)

Stdout shape:

```
title: <H1 or empty>
path: <absolute path>

<raw frontmatter interior>
```

- Preamble is `title` then `path` only; drop preamble `id:` (duplicate of the interior).
- Blank line, then the frontmatter interior exactly as stored — key order, quoting, comments, customs, and `status_conflict` preserved, never re-encoded and never the body.
- Extractable frontmatter exits 0 (even under parse_error, riding the token); wholly unparseable frontmatter is non-zero with empty stdout.
- Pure read; never runs git.

### 2. `meta get` — single key

`pj meta get <id> <key>`

- Value only on stdout; no preamble (piping / scripting).
- Single-key get is decoded from the typed frontmatter model (not raw YAML for that key). Full get remains the raw `Split` path.
- When the fence is extractable but the typed model cannot be parsed (including parse_error quarantine with a broken interior): non-zero exit, empty stdout, ride the same parse_error token as sibling reads; do not treat this as “key absent”. Full get is the repair/inspect path in that state.
- Scalar present: exactly one line on stdout (canonical form: `true`/`false`, decimal int, string text as stored). Meta `set` refuses embedded newlines in string scalars, so a successfully set string is always single-line; a hand-edited multiline string is still printed as stored (may span lines) — agents that need strict one-line should repair via `set`.
- Multi-value present: one entry per line, stored order; not CSV, not JSON.
- Key absent or empty multi-value list (model parsed successfully): empty stdout, exit 0.
- Unknown key: usage error (exit 2); message lists the meta known-key catalogue (see §4).
- Immutable keys are readable on get (`id`, `status`, `order`, `created`, `status_conflict`); only writes refuse them.

### 3. Value from stdin

When `<value>` is exactly `-`, read the entire stdin as the value (no trailing-newline stripping beyond a single optional final newline). Legal for `set`, `add`, and `rm`. Empty stdin is an empty value.

### 4. Key classes and legal write operations

- Scalar settable: `summary`; custom fields of type `string`, `int`, `bool`.
- Multi-value add/rm: `depends`, `related`, `tags`, `links`; custom fields of type `strings`.
- `meta set` on a multi-value key is a usage error (direct the agent to `add`/`rm`).
- `meta add` / `meta rm` on a scalar key is a usage error.
- Empty value on `set` for any scalar settable key (`summary` or custom `string`/`int`/`bool`): omit the key on serialize (clear). This is a distinct “absent” write, not a parse of `""` as int/bool. Non-empty values must parse as the declared type; `fields.*.values` enums must be enforced on non-empty `set` (string) and on `add` (strings) when present.
- Embedded newlines: `set` on `summary` or custom `string` refuses a value that contains U+000A (after the single optional final newline strip on stdin `-`); usage exit 2. Same rule for each `add`/`rm` entry on multi-value keys (one entry must not embed a newline). Keeps single-key get and multi-value line protocol pipe-safe for meta-authored data.
- Immutable via meta (no write): `id`, `status`, `order`, `created`, `status_conflict`. Usage error (exit 2) — same class as unknown key and wrong-class ops. Message names the key as immutable and points at the dedicated verbs where they exist (`status`, `reorder`).
- Meta known-key catalogue (one set for every meta verb’s unknown-key usage error): every closed built-in frontmatter key name — `id`, `status`, `order`, `depends`, `related`, `tags`, `created`, `links`, `summary`, `status_conflict` — plus every custom field name declared in the scope’s `pj.cue` `fields`. Order: built-ins in that document order (same as `internal/frontmatter` key constants), then custom names sorted ascending. Wrong-class ops (e.g. `set` on `depends`) and immutable writes are separate usage/refuse errors for *known* keys; they must not be folded into the unknown-key message.
- Unknown key (not in the catalogue): usage error (exit 2); message lists the catalogue so agents can recover.
- Undeclared free-form keys are refused on write; meta does not invent schema. Hand-edited undeclared keys remain preserved by the frontmatter model when present, but cannot be authored through meta.

### 5. Multi-value semantics

- `add`: append `value` if not already present (idempotent success when already present; no duplicate entries).
- `rm` / `remove`: remove the matching entry if present (idempotent success when absent). Value is required — this removes one list entry, not the whole key.
- Comparison is exact string equality on the stored form, **after** any key-specific normalisation of the argv value (see §6 for depends/related).
- Removing the last element omits the key on serialize (same as other empty optional slices today).
- No `meta clear` / full-list wipe verb.

### 6. Edge lists: depends integrity and related (soft)

On-disk form for both `depends` and `related` is full project ids only (same as the rest of the tree / archive intent). Short ids remain a CLI ergonomic on argv; meta normalises them before store. Semantics differ:

**`depends` (gating) — write-time integrity on `meta add <id> depends <target>` only:**

- Target must be a legal project id form (short or full per existing id predicates). Malformed target form is usage exit 2 (same class as a bad subject id via `parseIDArg`); do not treat it as a missing-target integrity refuse.
- Self-edge: hard refuse, non-zero exit, no write; stderr/error carries `depends_self:` (existing token).
- Same-scope target (after normalising to full id in the subject's scope): refuse if no non-quarantined project with that id exists in the subject scope after the subject reconcile. Do not write a known-dangling edge. Hard refuse with `depends_dangling:` (quarantined-only rows count as absent for this check).
- Cross-scope full ids: if the target scope is not registered on this machine, hard refuse with `depends_unresolvable:`. If the target scope is registered, reconcile that target scope (in addition to the subject scope already reconciled for the write) before the existence lookup, then hard refuse with `depends_unresolvable:` when the id has no non-quarantined row there or the target scope is unreachable; accept only when a non-quarantined project with that full id is present after that refresh. Do not use full depends-closure reconciliation for every meta write — only the single named target scope when the edge is cross-scope. (Agents can still hand-edit if they truly need an offline placeholder; the CLI path stays strict.)
- Write-time mapping is locked (no new token names): malformed target → exit 2; self → `depends_self:`; same-scope missing/quarantined → `depends_dangling:`; cross-scope unregistered/unreachable/absent after target reconcile → `depends_unresolvable:`. Every integrity refuse above is hard on this verb (non-zero, no write) even where the same token is only an informational hold on the read/doctor path — exit code carries hardness; the token keeps the closed vocabulary. Emit via the same `token.Line` pattern other write refuses use (`parse_error:`, `duplicate_id:`).
- Extend write-time checks only; do not change existing read-path depends diagnostics.

**`related` (soft “see also”, non-gating):**

- No target existence check (missing/unresolvable related is cosmetic; doctor soft-path only).
- Self-related: do not hard-refuse on meta write (archive: soft `schema_warn:` only; no `related_self:` token; mutators not refused). Idempotent add of self is allowed if the agent passes it; prefer not to special-case beyond exact-string de-dupe.
- Short id on argv: normalise to full id in the subject scope when storing (same ergonomic as depends). Full id required on disk so bare shorts never become `schema_error:` edges via the CLI.

**Both `depends` and `related` (add and rm):**

- Normalise short → full in the subject scope on the argv value **before** existence checks (depends add), de-dupe membership, store, and remove. Agents may pass the same short id to `add` and `rm`; both operate on the full-id stored form. Full ids on argv are left unchanged (including cross-scope full ids).
- Do not store or compare bare short ids for these keys via the CLI.
- `tags` and `links` are free-form strings (not project ids); no id normalisation or existence checks; exact string match only.

### 7. Write durability and refuses

Reuse and extend the existing status/reorder complete-state path; do not invent a parallel durability stack or change sibling verb behaviour.

- Scope flock for the write span (subject scope only; do not take the target scope's flock for a depends existence check).
- Subject-scope reconcile under that flock, as status/reorder do. Cross-scope `add depends` additionally reconciles the registered target scope before existence lookup (§6); that extra reconcile is read-through only and does not extend the subject write flock.
- Refuse unusable scope config, mid-rebase on auto-commit git-roots, quarantined parse_error rows, and duplicate-id collisions, with the same tokens/exit classes those verbs already use.
- Atomic file rewrite via existing `writeProjectFile` / serialize path (preserve body; re-encode frontmatter through the typed model like other mutators).
- Index write-through for the touched path so subsequent reads see new edges/tags/summary without a separate reconcile wait beyond the verb's own path.
- Auto-commit scopes: self-commit the touched path with a clear fixed message (e.g. `pj: <id> meta set summary`). Repo-driven: emit `uncommitted:` after the write when dirty under the allowlist. `create`-style "never self-commit" does not apply; these are complete-state mutators.
- Stdout hand-off after successful mutate: absolute project path (meta never moves files). Stderr carries tokens/warnings only.

### 8. Tests

Cover at least: full get preamble shape (title, path, no preamble id); single-key get scalar and multi (newline); absent key empty exit 0; single-key get when typed parse fails (non-zero, empty stdout, parse_error token — not confusable with absent); unknown key usage lists known keys; set summary (argv and stdin `-`); clear summary; refuse set summary with embedded newlines; empty set clears custom int/bool (omit key); non-empty custom type/enum parse; add/rm depends with existing same-scope target; rm depends by short id removes the stored full id; cross-scope depends add when target exists in a registered scope (existence after reconciling the target scope, not a stale index-only hit); refuse missing same-scope depends target with `depends_dangling:`; refuse missing/unregistered/unreachable cross-scope depends target with `depends_unresolvable:`; refuse self depends with `depends_self:`; malformed depends target id is exit 2 (not an integrity token); add related short id stores full id (no existence refuse); add tags idempotent; rm absent tag idempotent; refuse set on `depends`; refuse add on `summary`; refuse immutable keys; unknown write key lists known keys; self-commit / uncommitted behaviour consistent with siblings.

### 9. Discoverability

Update the embedded skill, `pj meta` family Long help, and every other in-tree user-facing string that teaches the old shape (`pj meta <id>`, preamble with `id:`) so agents discover `meta get|set|add|rm|remove`, full vs single-key get (title-then-path preamble), stdin `-`, the depends existence rule, and the write-time depends refuse tokens (`depends_self:`, `depends_dangling:`, `depends_unresolvable:` — hard on this verb via non-zero exit) without reading this project file. Includes at least `list` Long’s pointer to meta. Does not change list columns or TSV.

## Constraints

- Pure Go, no cgo; follow AGENTS.md and existing `internal/cli` patterns.
- Extend existing durability, resolve, index write-through, and depends machinery; do not redesign them for meta alone.
- Do not invent new stderr token names when an existing token already fits; if a new closed token is required, add it to `internal/token` and keep the catalogue the single source. Meta write-time depends integrity reuses `depends_self:`, `depends_dangling:`, and `depends_unresolvable:` only (§6); no parallel write-only token names.
- Do not bypass the typed frontmatter model with raw string splicing for mutators (raw interior remains a full-get concern).
- Do not let meta change status, order, id, created, or status_conflict.
- Exit code classes stay consistent: usage/bad id (including malformed depends/related target form) → exit 2; unknown project → generic non-zero; integrity refuses (including write-time depends self/dangle/unresolvable) → non-zero with the locked token on the error, same pattern as sibling write refuses.
- Short-ids remain letter-first; examples and tests must honour `IsShortID`.
- No push from these verbs; sole push boundary remains `pj sync`.

## Implementation Plan

1. Lift `pj meta` to a parent command family: bare `meta` shows family help; attach `get`, `set`, `add`, `rm` (`remove` alias).
2. Implement `meta get` with the locked full-header preamble and optional single-key path.
3. Classify keys (immutable / scalar / multi-value / custom) against frontmatter builtins and the reconciled scope schema; unknown-key usage errors list known keys.
4. Implement mutate → write → index → self-commit via shared `runMetaMutate` (or similar), reusing writeutil and status/reorder orchestration.
5. Implement depends target resolution and existence checks only on `add depends`: same-scope against the subject reconcile; cross-scope against a fresh reconcile of the single registered target scope before lookup; emit the locked §6 token/exit mapping on each refuse.
6. Wire stdin `-` value loading once for set/add/rm.
7. Extend CLI tests (table-driven) for success and refuse cases; rewrite existing meta read tests for `meta get` and the new preamble; run `go test ./...`.
8. Update skill contract text, meta family help, and in-tree call-site help (e.g. `list` Long) for the locked surface.

## Implementation Guidance

- Prefer one shared mutator path with an operation enum rather than three copy-pasted lock/reconcile blocks.
- Normalise depends/related targets to full ids when the argument is a short id in the subject scope on both add and rm (before compare/store); do not apply depends existence/self hard refuses to related.
- For cross-scope `add depends`, reconcile only the named target scope for existence (reuse reconcile machinery; do not call full `reconcileClosure`). Subject flock stays subject-only; do not nest or require the target scope flock for the check.
- Depends write refuses: put `depends_self:` / `depends_dangling:` / `depends_unresolvable:` on the returned error via `token.Line` (mirroring `duplicateRefusal` / parse_error write refuses); do not invent a second message channel.
- For `tags`, free-form tags remain legal; optional `unknown_tag:` warn-only behaviour may mirror the read path but must not refuse the write.
- When serializing after mutation, keep the same quoting/`order` force-quote behaviour the frontmatter package already guarantees; do not special-case meta.
- Unknown-key usage errors emit the meta known-key catalogue from §4 (membership and order fixed there); do not invent per-op catalogues.

## Acceptance Criteria

1. `pj meta get <id>` prints `title:` then `path:` then a blank line then raw frontmatter; preamble has no `id:` line.
2. `pj meta get <id> summary` prints only the summary value (or empty exit 0 if absent); `pj meta get <id> depends` prints one id per line.
3. `pj meta get <id> <key>` when the typed model cannot be parsed exits non-zero with empty stdout and a parse_error token; full get still works on extractable raw interior.
4. Unknown key on get or mutate is exit 2 and the error lists the meta known-key catalogue (all closed built-ins including immutable, plus declared customs; built-in document order then customs sorted).
5. `pj meta set <id> summary 'one line'` rewrites summary and prints the absolute project path; full get shows the new summary in the raw interior.
6. `pj meta set <id> summary -` with piped stdin stores that text (single optional trailing newline stripped); embedded newlines in the value are usage exit 2.
7. `pj meta add <id> depends <other>` succeeds when `<other>` exists in the same scope; repeating the add does not duplicate.
8. `pj meta add <id> depends <other-scope-full-id>` succeeds when the target scope is registered and, after reconciling that target scope, a non-quarantined project with that full id exists; edge stored as that full id.
9. `pj meta add <id> depends <missing>` refuses with non-zero exit, no file change, and `depends_dangling:` when the target is same-scope and absent (or only quarantined); refuses with `depends_unresolvable:` when a cross-scope full id’s scope is unregistered, unreachable, or the id is absent after that scope is reconciled. Malformed target id form is usage exit 2 with no integrity token.
10. `pj meta add <id> depends <id>` (self) refuses with no write and `depends_self:`.
11. `pj meta rm <id> depends <other>` (and `remove`) removes the edge; rm when absent is success; rm by short id removes the stored full id for that short in the subject scope.
12. `pj meta add <id> related <short>` stores the subject-scope full id; no existence check on related; self-related is not a hard refuse; rm related by short matches that full id.
13. `pj meta set <id> depends x` and `pj meta add <id> summary x` are usage errors (exit 2).
14. `pj meta set <id> status todo` (and id/order/created/status_conflict) is usage exit 2 with no write; message marks the key immutable and points at `status` / `reorder` where relevant.
15. Declared custom fields work by type; empty `set` omits scalar customs (string/int/bool); undeclared keys refuse on write.
16. Auto-commit scope self-commits a meta mutation; repo-driven scope emits `uncommitted:` rather than committing.
17. Skill and `pj meta --help` / subcommand help document get/set/add/rm/remove, full vs single-key get, stdin `-`, the depends existence rule, and the three write-time depends refuse tokens (`depends_self:`, `depends_dangling:`, `depends_unresolvable:`).
18. In-tree help/tests that referenced `pj meta <id>` or preamble `id:` are updated; `list` Long points at `pj meta get`.
19. Existing status/reorder/deps/read behaviour is unchanged except where intentionally extended for these verbs.
