# pj — Agent Project Management CLI

Guidance for AI agents working in this repository. `pj` is a single-purpose CLI
that tracks feature work as plain markdown files, one project per file, edited in
place. The running code and the embedded skill contract (`pj skill` /
`internal/skill/skill.md`) are the source of truth. Archived design prose is not
live authority and must not override the tree.

## Project status

P1 through P7 have landed. `pj` runs as a Cobra CLI with the machine-local CUE
registry, scope `pj.cue` evaluation, ambient resolution, and the full `pj scope`
verb set (`init`, `import`, `rebind`, `forget`, `list`, `rename`); the machine-wide
SQLite index with reconcile, FTS5 search, and the read/board verbs (`list`,
`status`, `get`, `meta`, `next`, `deps`, `search`, `query`, `lens`); the authoring
hot path (`create`, `mark`, `reorder`, `edit`, `next --claim`) with local git
self-commit;
`pj doctor` with its integrity repairs and the closed token catalogue; P6a's
frontmatter merge package (`internal/fmmerge`), the rebase driver
(`internal/rebasedriver`), and the read/integrate/push half of the git wrapper;
P6b's `pj sync` — the sole push boundary (snapshot, fetch-and-integrate, sync-time
integrity, push), the per-git-root preflight, the layer-4 resume contract, the `--all`
per-root failure isolation, and the reentrant lock span (self-commit and repair
orchestration split into acquiring wrappers over locks-held cores); and P7's
`pj skill` — the 18-section agent contract (embedded `skill.md` as the sole runtime
source, with structure/token/handoff tests; no design-doc dependency) plus the
hard-refuse `skill install`/`list`/`uninstall` placeholders.

- Prefer packages, tests, and the embedded skill over prose when they disagree.
- Short-ids are letter-first by construction (the `IsShortID` predicate and the
  mint both forbid a leading digit); any `<scope>-<short-id>` example follows
  that rule.
- Do not invent behaviour that contradicts closed contracts already in code
  (token catalogue, id/order/slug grammars, exit codes, sole push boundary). If
  behaviour is unclear, flag it rather than guessing from archive prose.

## Project documents and archiving

Project documents are ordinary `pj` project files under the `projects/` scope
(`<id>-<slug>.md`). Active work lives at the scope dir root; terminal status
moves a file into `projects/archive/` via `pj mark` (do not hand-move).

- When a project is complete, set a terminal status with `pj mark <id> done`
  (or another terminal status). That renames into `archive/` in the same write.
- Cross-project references use logical labels (`P1`…`P8`) or full ids
  (`pj-mwtc`, …); path or filename references need rewriting after id/slug
  changes.
- Completed historical projects (P1–P5, P6a, P6b, P7, P8, and later) live under
  `projects/archive/` as first-class done projects. Pre-`pj` design prose is at
  `docs/archive/design.md` (history only; not live authority).
- The sync and merge boundary is split across two projects: P6a (frontmatter
  merge package, rebase driver, git plumbing) and P6b (`pj sync`). Documents
  written before that split refer to the pair as `P6`; a `P6` reference to the
  merge package, the driver, or `internal/git` plumbing means P6a, and one to
  `pj sync`, its integrity step, or its push means P6b. The labels were kept as
  `P6a`/`P6b` rather than renumbering so every existing `P6` and `P7` reference
  in the archive stays valid.

## Module and layout

- Module path: `github.com/start-cli/pj`
- Go version: 1.26 (pure Go, no cgo)
- `cmd/pj/main.go` — minimal entry point: run, map a signal or error to an exit
  code, exit (all command logic is in `internal/cli`)
- `internal/` — pure wire-contract primitives, then the engines built on them:
  - `id` — scope/short-id/full-id predicates, `crypto/rand` mint, collision-repair extension
  - `slug` — `Slugify` and the closed slug grammar
  - `order` — the fractional-index `order` wire format and `KeyBetween`
  - `frontmatter` — fence split, YAML parse/serialize, raw fence-slice API
  - `status` — built-in statuses, the `Category` set, and the terminal predicate
  - `title` — ATX-H1 title extraction
  - `scope` — `--auto-name` derivation
  - `token` — the closed stderr token strings (`name_drift:`, `config_unparseable:`, …)
  - `pathutil` — boundary-safe path predicates (nesting, disjointness)
  - `xdg` — XDG config dir resolution and the machine-global flock
  - `flock` — the POSIX advisory-lock helper behind the scope and git-root locks
  - `atomicfile` — same-dir temp write plus rename, so no reader sees a half-written file
  - `gitroot` — `git rev-parse` code-root/git-root derivation
  - `scopeconfig` — scope `pj.cue` evaluation into the validated `ScopeSchema`
  - `registry` — the XDG registry/lens model, CUE read + atomic regenerate
  - `resolve` — ambient scope resolution and name-drift fail-closed
  - `scopeadmin` — scope verbs and the shared registration checks
  - `index` — the machine-wide SQLite read model (WAL, FTS5, projects + edges)
  - `reconcile` — git-free read-through that brings the index up to date from the files
  - `git` — the external-git wrapper; full read/integrate/push surface (fetch, rebase,
    stage enumeration and reads, blob merge, author date, push, unpushed count)
  - `gitstate` — per-git-root XDG ops state (`sync.lock`, `last-push-error` read/write/clear)
  - `selfcommit` — the single reusable self-commit step for auto-commit scopes
  - `rewrite` — the shared multi-file rewrite durability engine
  - `repair` — deterministic integrity repairs (collision pick, re-space, archive move);
    exports the shared `KeepBefore` loser pick the merge package reuses
  - `fmmerge` — the pure 3-way frontmatter merge over raw stage blobs (P6a)
  - `rebasedriver` — resolves one conflicted project `.md` at a paused rebase (P6a)
  - `skill` — embedded agent skill contract (`skill.md`; sole source, no design-doc dependency) (P7)
  - `cli` — Cobra command tree, exit codes, signals, colour/TTY, path hand-off

## Build, test, lint, format

| Task | Command |
|---|---|
| Build | `go build ./...` |
| Test | `go test ./...` |
| Format check | `gofmt -l .` (empty output = clean) |
| Format write | `gofmt -w .` |
| Vet | `go vet ./...` |
| Lint | `golangci-lint run ./...` (config in `.golangci.yml`, schema v2) |

## Intended stack

| Concern | Choice | Notes |
|---|---|---|
| Language | Go | Pure Go, no cgo (a git subprocess is not cgo). |
| Frontmatter/config YAML | `github.com/goccy/go-yaml` | Actively maintained pure Go; AST/style control for the force-quoted `order` and undeclared-key retention. |
| Unicode | `golang.org/x/text` | NFKC normalisation for `slugify` (Go has no stdlib normalisation). |
| Config | CUE (`cuelang.org/go`) | Typed, validated schema for scope config and frontmatter. |
| Index | SQLite (`modernc.org/sqlite`) | Pure Go, FTS5 compiled in, WAL mode. |
| Version control | External `git` binary | Shelled out, owner `pj` scopes only. Full commit and read/integrate/push surface built (P6a); `pj sync` is the sole push boundary and wires it (P6b). |

TIP: Both `modernc.org/sqlite` and `cuelang.org/go` are pure Go by design. Do not
introduce a cgo-based SQLite driver (e.g. `mattn/go-sqlite3`) — it breaks the
"pure Go, no cgo" invariant.

## Go CLI design guide is advisory

The Go CLI design guide (`start get golang/design/cli`) is advisory only.
Adopt its repo-shape conventions — standard layout (`cmd/pj/main.go` minimal,
`internal/…`), table-driven tests with `testdata/`, and a `.golangci.yml`.
The implemented contracts (code and embedded skill) override it on every conflict;
a later project does not restate this. Known override points where the tree wins:

- Exit codes and error classes — usage/bad-id `exit 2`, unknown id generic
  non-zero, `duplicate_id:` refuse — over the guide's mapping.
- Output contract — path-centric with TSV/stdout hand-off, not the guide's
  JSON-envelope-first model.
- Configuration model — per-scope `pj.cue` plus a machine-wide registry, not the
  guide's XDG/profile precedence chain.
- Command semantics — one-op-one-commit and path hand-off, not the guide's
  async-job ledger.
