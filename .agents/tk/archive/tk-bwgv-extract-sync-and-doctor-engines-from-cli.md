---
id: tk-bwgv
status: done
order: "aH"
created: "2026-08-06T17:41:27+10:00"
summary: Move sync and doctor domain logic out of internal/cli into packages, matching fmmerge/rebasedriver/repair
---

# Extract sync and doctor engines from cli

## Goal

Move the domain orchestration for `pj sync` and `pj doctor` out of `internal/cli` into dedicated packages so the composition root only wires Cobra, exit codes, and I/O. Match the layering already used for frontmatter merge, the rebase driver, repair planning, and self-commit.

## Scope

In scope:

- Extract doctor diagnosis (token-line integrity report over the index and reconcile result) into a package with a narrow, cobra-free API
- Extract the shared repair orchestration that sits above `internal/repair` and `internal/rewrite` (acquiring preflight/lock span for doctor; locks-held batch apply + edge_verify for both doctor and sync integrity) so `cli` only invokes and prints
- Extract the sole-push-boundary sync engine into a cobra-free package that owns both who and how: selection policy (auto-commit-only filter, unreachable/disabled/config-error reporting, participants grouped by git-root) and the per-root flow (preflight, lock order, snapshot, fetch/integrate loop, mid-rebase resume, sync-time integrity via the shared locks-held repair core, push-if-ahead, success/paused reporting payloads)
- Thin `internal/cli` adapters: flag parse, ambient resolve, open deps, call the new packages, map structured results to stdout/stderr and exit codes. Selection **inputs** stay at the composition root (see Current State: three invocation shapes → two package inputs); selection **policy** does not
- Promote shared helpers that both the new engines and remaining `cli` call sites need into one home outside `cli` (package name(s) implementer choice; split by role if more than one package). Do not duplicate them into each engine. In particular: scope-file allowlist classification and dirty counting (`isAllowlistedScopeFile` / `countAllowlistedDirty`), scope flock acquire (`.pj.lock` / `acquireScopeLock`), and any other non-trivial helpers the extract pulls that write verbs or status still use. Thin schema/git-root wrappers may move with that home or be re-expressed identically at call sites — not forked with divergent logic
- Move or rewrite tests that currently live only under `internal/cli` for these flows so package-level behaviour is proven without the full command tree where practical; keep enough CLI integration tests to lock user-visible contracts
- Update AGENTS.md module layout bullets if they list packages and omit the new ones

Out of scope:

- Changing sync or doctor user-facing behaviour (flags, tokens, exit codes, sole push boundary, mid-rebase resume contract, --all isolation)
- Changing `internal/repair` algorithms, `internal/fmmerge`, `internal/rebasedriver` merge semantics, or the closed token catalogue wording (except package ownership of emission helpers if already local)
- Expanding pure `internal/repair` into lock/commit orchestration (it stays ops-only planning that returns `rewrite.Op`)
- Extracting other large CLI areas (`meta`, board reads, scope rename) — separate work if ever needed
- Resolving the pure-merge → repair `KeepBefore` import (architecture finding M2) unless a trivial import fix falls out of the move
- Making `internal/` a public library or introducing a plugin system
- Performance work unrelated to the extraction
- Inventing a callback/func-injection layer so helpers can stay unexported in `cli` while engines call them — promote the helpers instead

## Current State

`internal/cli` is about half of non-test production code (~7k of ~14k LOC). Lower engines already follow a clean split:

| Package | Role |
|---|---|
| `internal/fmmerge` | Pure 3-way frontmatter merge over stage blobs |
| `internal/rebasedriver` | Per-path conflict resolution; stages, body merge, stage/unstaged |
| `internal/repair` | Deterministic integrity ops returning `rewrite.Op` only |
| `internal/selfcommit` | Self-commit with Commit vs CommitCore lock reentrancy |
| `internal/reconcile` | Git-free index read-through; no project file mutation |

Sync and doctor were never extracted the same way. Production code still on `*engine` with `*cobra.Command` threaded for stderr:

| Cluster | Approx non-test LOC | Files |
|---|---|---|
| Sync | ~1157 | `sync.go`, `sync_root.go`, `sync_snapshot.go`, `sync_integrate.go`, `sync_integrity.go` |
| Doctor | ~1121 | `doctor.go`, `doctor_diagnose.go`, `doctor_repair.go` |

Rough method counts: ~26 `engine` methods for sync, ~34 for doctor (including `diagnoser` methods). Domain structure already exists inside those files (selection → lock → snapshot → integrate → integrity → push; diagnose token lines; repair batches). The missing boundary is a package API and cobra-free core.

Sync selection today (`selectSyncTargets` / `ambientSelection` / `allSelection` / `autoCommitParticipants` in `sync.go`) is sole-push-boundary product policy, not Cobra glue. After extraction, auto-commit filter, root grouping, and selection-time refusals live in the sync engine with the per-root state machine. The package must not read `PJ_SCOPE`, cwd, or Cobra; it enforces auto-commit-only at the package boundary so a second caller cannot invent a different target set.

Composition root maps three invocation shapes onto two package inputs (only these two; no third package mode):

| Invocation | How it arises today | Package input |
|---|---|---|
| Single ambient | successful ambient/`--scope` resolve | one ambient scope (name + dir already resolved) |
| All-registered | `--all` | all-registered mode |
| All-registered | ambient resolve returns `resolve.ErrNoScope` (no `--scope`, no ambient code-root) | same all-registered mode as `--all` |

Other ambient resolve errors still fail at the composition root before the package runs. Bare `pj sync` with no ambient scope therefore fans out like `--all`; that is product behaviour to preserve, not an error path.

Shared helpers outside the sync/doctor file list (must not be forked on extract):

| Helper (today) | Defined in | Used by extract targets and also by |
|---|---|---|
| `isAllowlistedScopeFile` / `countAllowlistedDirty` | `commit.go` | sync snapshot/integrate, doctor diagnose; write durability, `status` |
| `acquireScopeLock` (`.pj.lock`) | `commit.go` | doctor repair, sync locks; every write verb |
| `gitRootFor` | `commit.go` | sync, doctor; write verbs, status, board |
| `schemaAutoCommit` / `schemaCustom` | `writeutil.go` / `board.go` | sync integrity, doctor; write verbs, board |

Allowlist classification is one product rule (project files vs residue). Snapshot, doctor `uncommitted:` / `non_allowlist:`, write-side dirty counts, and status `uncommitted` must stay on that single definition after extraction.

Shared repair core (must survive extraction as one API, not two copies):

- Doctor mutating path: `repairScope` acquires the scope flock then the git-root commit lock, then calls `runRepairBatches`
- Sync integrity: after integrate completes, `syncIntegrity` already holds both locks from `acquireSyncLocks` and calls the same `runRepairBatches` with repair-only flags (not re-space-order)
- Apply under a held git-root lock uses `selfcommit.CommitPathsCore` (re-acquiring with `Commit` / `CommitPaths` deadlocks) — same reentrancy split as `selfcommit.Commit` vs `CommitCore`
- `reportEdgeVerify` is shared: doctor collision repair emits it after renames; sync drains add/add collided ids through the same reporter (stdout `edge_verify:` lines)

Existing tests (must remain green; many should move or dual-live):

- `internal/cli/sync_test.go`, `sync_flow_test.go`, `sync_merge_test.go`, `sync_harness_test.go`
- `internal/cli/doctor_test.go`, `doctor_git_test.go`

Lock order invariant (must survive extraction): acquire scope locks first (name order), then git-root commit lock — reverse deadlocks write verbs. Self-commit under a held git-root lock must use `selfcommit.CommitCore`, not `Commit`.

Authority and contracts: packages, tests, and embedded `internal/skill/skill.md` win over `docs/archive/design.md`. Do not invent behaviour that contradicts closed contracts (token catalogue, sole push boundary, exit codes).

## Requirements

1. Introduce one or more new `internal/*` packages that own the domain logic currently in the sync and doctor production files listed above. Package names are the implementer's choice; they must read as engines (policy + orchestration), not as CLI helpers.

2. Those packages must not import `github.com/spf13/cobra` or depend on `cli.App` / unexported `engine`. They may accept concrete deps already used by the engines (`*index.DB`, `*reconcile.Reconciler`, registry/resolve inputs, paths, contexts, schema loaders, stderr/result sinks via interfaces or callbacks only if needed).

3. `internal/cli` retains command registration, flag parsing, `openEngine`, ambient resolution, mapping package outcomes to `stdoutln` / `stderrln` / `ExitError`, and any glue that is inherently process-edge. After extraction, sync and doctor production files in `cli` should be thin adapters, not the home of the state machine or of sync selection policy. For sync, the adapter maps the three invocation shapes in Current State onto the two package inputs (ambient success → ambient; `--all` or `resolve.ErrNoScope` → all-registered; other resolve errors fail at the edge) and does not re-implement auto-commit filter, root grouping, or selection-time refusals.

4. Doctor diagnosis remains a pure report path: no project file mutation on bare diagnose. Repair still goes through `internal/repair` + `internal/rewrite` under the documented lock spans. Sync remains the sole push boundary; no second push path. The sync package owns selection policy (auto-commit filter, root grouping, selection-time refusals and skip/disabled lines as structured results) and the per-root flow; `cli` does not re-implement that policy.

5. Preserve lock ordering, mid-rebase refuse/resume behaviour, `--all` per-root failure isolation, and reentrant lock split (acquiring wrappers vs locks-held cores) already present in the tree.

6. Exactly one shared repair-orchestration API after extraction, shaped like `selfcommit` (acquiring entry for doctor; locks-held core for both doctor and sync integrity). Both call sites must use that API — no second apply path, no second `CommitPathsCore` wrapper, no parallel edge_verify reporter. Sync integrity continues to request repair-only batches (not re-space-order). Package home is implementer choice subject to import direction: pure `internal/repair` stays ops-only; the shared core must not live only under a doctor-named surface that forces an awkward sync→doctor identity, nor only under the sync/push package that forces doctor to import the sole push boundary. Preferred shapes: a dedicated orchestration package both engines import, or co-location with diagnose under a neutral engine name.

7. Tests: package-level coverage for extracted domain behaviour where isolation is practical; CLI tests still cover user-visible command contracts. Do not drop coverage for merge-resume, integrity, or doctor repair classes already tested.

8. Document the new packages in AGENTS.md's module layout list so agents map names to roles.

9. Shared helpers used by both extracted engines and remaining `cli` (allowlist classification/dirty count, scope flock acquire, and any other non-trivial helpers the move depends on) have exactly one definition outside `cli` after extraction. New engines and write/status paths import that home — no second copy of allowlist or lock-path logic under a sync-only or doctor-only package, and no inverted dependency on `cli`.

## Constraints

- Pure Go, no cgo; existing stack (cobra at the edge only, modernc sqlite, CUE, external git)
- No behaviour change to CLI flags, TSV/path stdout, token prefixes, or exit code classes unless required to fix a bug discovered during the move — prefer behaviour-preserving extraction
- Do not import `internal/cli` from any other package (no inverted dependency)
- Do not break pure-Go / no-cgo; do not introduce mock-heavy abstraction layers that fight hermetic `testgit` style tests already in the tree
- Prefer packages and tests over archive design prose
- Short-id / full-id / order grammars unchanged
- Pure `internal/repair` remains planning-only (`rewrite.Op` out); do not pull lock acquisition, rewrite apply, or self-commit into it (that also keeps the fmmerge→repair import edge from growing heavier)
- Do not fork the scope-file allowlist predicate or the scope flock path: one home, many importers

## Implementation Plan

1. Map export surface: list types and functions that must leave `cli` (sync selection policy and its result types, `syncRoot` state machine, integrate stop loop, diagnose line builders, shared repair batch runner + edge_verify, **and** the shared helpers table above) and which stay (Cobra `RunE`, flag parse, ambient resolve mapped to the two package inputs, I/O and exit mapping). Treat the dual call site (`repairScope` / `syncIntegrity` → same locks-held core) as a first-class export, not two independent movers.

2. Promote shared helpers (allowlist, scope lock, and any other non-trivial cross-callers from step 1) into their outside-`cli` home; retarget existing `cli` call sites (write verbs, status, board) so behaviour is unchanged before or as the first extract lands. Prefer this before diagnosis/repair/sync moves so engines never compile against a private `cli` copy.

3. Extract diagnosis first — highest purity, fewest git edges. Land package + tests; `pj doctor` bare path becomes an adapter.

4. Extract the shared repair orchestration above `internal/repair` (acquiring wrapper for doctor; locks-held batch apply + edge_verify for both callers). Keep algorithm ownership in `repair`. Wire the existing doctor mutating path to the new API while sync still lives in `cli` if needed, so there is never a window with two apply implementations. Scope lock acquire comes from the promoted helper home, not a private copy.

5. Extract sync as one engine: selection policy (from ambient or all-registered package inputs) plus `syncRoot` / integrate / snapshot / integrity / push. CLI `runSync` binds flags and ambient resolve, maps the three invocation shapes onto those two inputs (`ErrNoScope` → all-registered, same as `--all`), calls the package, and maps structured selection/run results to streams and exit codes — no residual `ambientSelection` / `allSelection` policy body in `cli`. Integrity must call the shared locks-held repair core from step 4 — do not re-implement batches inside the sync package. Snapshot/integrate/doctor allowlist checks use the promoted allowlist home. Preserve lock order and CommitCore usage.

6. Relocate or dual-cover tests; delete dead code from `cli` once adapters are the only remaining callers (including any leftover local wrappers that only forwarded to the promoted helpers).

7. Update AGENTS.md package list (engines and any new helper package(s)); run full `go test ./...` and a manual smoke of `pj doctor` and `pj sync --help` / ambient refuse paths as needed.

## Implementation Guidance

- Prefer return values and structured results over threading `*cobra.Command` for logging. If progress lines must stream, pass an explicit reporter interface (or dual out/err writers) defined next to the engine, not cobra types — repair progress and `edge_verify` stay on their current streams. Selection-time unreachable/disabled/config lines and per-root success/paused lines follow the same pattern (structured from the package; CLI prints).
- Sync engine API sketch: composition root supplies either one ambient scope (name + dir already resolved) or all-registered mode, plus registry/reconcile/index/state deps; package performs selection policy and runs roots; it does not read env, cwd, or Cobra. All-registered is selected by `--all` or by ambient resolve `ErrNoScope` — same package input; the package does not distinguish how all-registered was chosen. Names are implementer choice.
- Shared helper home(s): prefer small role-shaped packages (e.g. allowlist/scope-file policy next to path concerns; flock helper next to existing `flock` usage) over a kitchen-sink `internal/cliutil`. Do not park allowlist or scope-lock only under the sync package (forces doctor/writers to import the push boundary) or only under a doctor-named package.
- Mirror existing package docs: one-paragraph role, lock assumptions, "caller holds X".
- Do not invent a grand "service container". `engine` in cli can keep holding `reg`/`db`/`rec` and pass them into the new packages.
- If a single mega-package would recreate a god-package under a new name, split diagnose vs sync and keep the shared repair orchestration on the selfcommit-shaped acquiring/locks-held API (dedicated package or neutral co-location with diagnose — not pure `repair`, not only under the push-boundary package).
- Extraction is a behaviour-preserving refactor first; resist drive-by feature work.

## Acceptance Criteria

1. Non-test production logic for the sync state machine (including selection policy) and doctor diagnose/repair orchestration lives primarily under new `internal/*` packages, not as large `engine` method bodies in `internal/cli`.
2. Those packages have no import of `github.com/spf13/cobra`.
3. One shared locks-held repair + edge_verify core is used by both the doctor mutating path and sync integrity; no duplicate apply/`CommitPathsCore` path remains in either cluster.
4. `go test ./...` passes with coverage retained for prior sync merge/resume and doctor repair scenarios (tests may have moved packages).
5. `pj sync` and `pj doctor` (including `--repair`, `--reindex`, `--all` where previously supported) keep the same flags and the same classes of stderr tokens and exit outcomes for the scenarios covered by existing tests.
6. Bare `pj sync` with no ambient scope (no `--scope`, cwd not under a registered code-root) uses the same all-registered selection as `pj sync --all` — not a no-scope error. Other ambient resolve failures still fail at the composition root before the package runs.
7. AGENTS.md lists the new package(s) with one-line roles consistent with neighbouring bullets.
8. `internal/cli` sync and doctor files are adapter-scale relative to the pre-extraction bodies: domain algorithms, multi-step flows, and sync selection policy are not re-implemented in `cli` (flag parse, ambient resolve mapped to the two package inputs, package call, stream and exit mapping only).
9. Scope-file allowlist classification and scope flock acquire each have a single definition outside `cli`; sync snapshot/integrate, doctor diagnose/repair, and remaining write/status call sites use that definition — no parallel copies under engine packages.
