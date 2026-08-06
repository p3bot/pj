---
id: pj-bwgv
status: todo
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
- Extract doctor repair orchestration that sits above `internal/repair` and `internal/rewrite` (scope selection preflight, lock span, batch apply, edge_verify reporting) so `cli` only invokes and prints
- Extract the git-root sync flow (target selection inputs excluded if they stay resolve/registry-bound: preflight, lock order, snapshot, fetch/integrate loop, mid-rebase resume, sync-time integrity, push-if-ahead, success/paused reporting payloads) into a package that does not import Cobra
- Thin `internal/cli` adapters: flag parse, open engine, call the new packages, map results to stdout/stderr and exit codes
- Move or rewrite tests that currently live only under `internal/cli` for these flows so package-level behaviour is proven without the full command tree where practical; keep enough CLI integration tests to lock user-visible contracts
- Update AGENTS.md module layout bullets if they list packages and omit the new ones

Out of scope:

- Changing sync or doctor user-facing behaviour (flags, tokens, exit codes, sole push boundary, mid-rebase resume contract, --all isolation)
- Changing `internal/repair` algorithms, `internal/fmmerge`, `internal/rebasedriver` merge semantics, or the closed token catalogue wording (except package ownership of emission helpers if already local)
- Extracting other large CLI areas (`meta`, board reads, scope rename) — separate work if ever needed
- Resolving the pure-merge → repair `KeepBefore` import (architecture finding M2) unless a trivial import fix falls out of the move
- Making `internal/` a public library or introducing a plugin system
- Performance work unrelated to the extraction

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

Existing tests (must remain green; many should move or dual-live):

- `internal/cli/sync_test.go`, `sync_flow_test.go`, `sync_merge_test.go`, `sync_harness_test.go`
- `internal/cli/doctor_test.go`, `doctor_git_test.go`

Lock order invariant (must survive extraction): acquire scope locks first (name order), then git-root commit lock — reverse deadlocks write verbs. Self-commit under a held git-root lock must use `selfcommit.CommitCore`, not `Commit`.

Authority and contracts: packages, tests, and embedded `internal/skill/skill.md` win over `docs/archive/design.md`. Do not invent behaviour that contradicts closed contracts (token catalogue, sole push boundary, exit codes).

## Requirements

1. Introduce one or more new `internal/*` packages that own the domain logic currently in the sync and doctor production files listed above. Package names are the implementer's choice; they must read as engines (policy + orchestration), not as CLI helpers.

2. Those packages must not import `github.com/spf13/cobra` or depend on `cli.App` / unexported `engine`. They may accept concrete deps already used by the engines (`*index.DB`, `*reconcile.Reconciler`, registry/resolve inputs, paths, contexts, schema loaders, stderr/result sinks via interfaces or callbacks only if needed).

3. `internal/cli` retains command registration, flag parsing, `openEngine`, ambient resolution, mapping package outcomes to `stdoutln` / `stderrln` / `ExitError`, and any glue that is inherently process-edge. After extraction, sync and doctor production files in `cli` should be thin adapters, not the home of the state machine.

4. Doctor diagnosis remains a pure report path: no project file mutation on bare diagnose. Repair still goes through `internal/repair` + `internal/rewrite` under the documented lock spans. Sync remains the sole push boundary; no second push path.

5. Preserve lock ordering, mid-rebase refuse/resume behaviour, `--all` per-root failure isolation, and reentrant lock split (acquiring wrappers vs locks-held cores) already present in the tree.

6. Tests: package-level coverage for extracted domain behaviour where isolation is practical; CLI tests still cover user-visible command contracts. Do not drop coverage for merge-resume, integrity, or doctor repair classes already tested.

7. Document the new packages in AGENTS.md's module layout list so agents map names to roles.

## Constraints

- Pure Go, no cgo; existing stack (cobra at the edge only, modernc sqlite, CUE, external git)
- No behaviour change to CLI flags, TSV/path stdout, token prefixes, or exit code classes unless required to fix a bug discovered during the move — prefer behaviour-preserving extraction
- Do not import `internal/cli` from any other package (no inverted dependency)
- Do not break pure-Go / no-cgo; do not introduce mock-heavy abstraction layers that fight hermetic `testgit` style tests already in the tree
- Prefer packages and tests over archive design prose
- Short-id / full-id / order grammars unchanged

## Implementation Plan

1. Map export surface: list types and functions that must leave `cli` (selection inputs, `syncRoot` state machine, integrate stop loop, diagnose line builders, repair batch runner) and which stay (Cobra `RunE`, ambient flags).

2. Extract diagnosis first — highest purity, fewest git edges. Land package + tests; `pj doctor` bare path becomes an adapter.

3. Extract repair orchestration above `internal/repair` (preflight, locks, apply batches, edge_verify). Keep algorithm ownership in `repair`.

4. Extract sync flow: start from `syncRoot` / integrate / snapshot / integrity / push as a coherent engine API; wire `runSync` and selection as thin CLI. Preserve lock order and CommitCore usage.

5. Relocate or dual-cover tests; delete dead code from `cli` once adapters are the only remaining callers.

6. Update AGENTS.md package list; run full `go test ./...` and a manual smoke of `pj doctor` and `pj sync --help` / ambient refuse paths as needed.

## Implementation Guidance

- Prefer return values and structured results over threading `*cobra.Command` for logging. If progress lines must stream, pass an explicit reporter interface or `io.Writer` defined next to the engine, not cobra types.
- Mirror existing package docs: one-paragraph role, lock assumptions, "caller holds X".
- Do not invent a grand "service container". `engine` in cli can keep holding `reg`/`db`/`rec` and pass them into the new packages.
- If a single mega-package would recreate a god-package under a new name, split diagnose vs sync (and optionally repair orchestration) instead.
- Extraction is a behaviour-preserving refactor first; resist drive-by feature work.

## Acceptance Criteria

1. Non-test production logic for the sync state machine and doctor diagnose/repair orchestration lives primarily under new `internal/*` packages, not as large `engine` method bodies in `internal/cli`.
2. Those packages have no import of `github.com/spf13/cobra`.
3. `go test ./...` passes with coverage retained for prior sync merge/resume and doctor repair scenarios (tests may have moved packages).
4. `pj sync` and `pj doctor` (including `--repair`, `--reindex`, `--all` where previously supported) keep the same flags and the same classes of stderr tokens and exit outcomes for the scenarios covered by existing tests.
5. AGENTS.md lists the new package(s) with one-line roles consistent with neighbouring bullets.
6. `internal/cli` sync and doctor files are adapter-scale relative to the pre-extraction bodies (domain algorithms and multi-step flows are not re-implemented in cli).
