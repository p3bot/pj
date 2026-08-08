# tk — Agent Ticket Management CLI

`tk` tracks feature work as plain markdown files, one ticket per file, edited in
place. It indexes, queues, and locates tickets; the filesystem is the editor.

The implementation is the source of truth.

## Supported platforms

macOS and Linux only. Windows is not supported — there is no flock or path
substitute, and `tk` fails with a clear startup error on an unsupported OS rather
than half-running. This is a deliberate v1 scope limit.

## Installation

### Homebrew (Linux/macOS)

```bash
brew tap p3bot/tap
brew trust p3bot/tap
brew install p3bot/tap/tk
```

### Go Install

```bash
go install github.com/p3bot/tk/cmd/tk@latest
```

### Build from Source

```bash
go build ./...
go build -o tk ./cmd/tk
```

Requires Go 1.26. Pure Go, no cgo. The external `git` binary is used only to
derive a code-root / git-root; it is shelled out, never linked.

## Scopes

A scope is a directory of ticket markdown files plus its `tk.cue` (the scope's
name, auto-commit mode, and schema). Scopes are registered per machine in the XDG
config directory (`${XDG_CONFIG_HOME:-~/.config}/tk/`).

```sh
tk scope init <dir> (--name <name> | --auto-name) [--code-root <path>] [--auto-commit]
tk scope import <dir> [--code-root <path>]
tk scope rebind <dir> --name <name> [--code-root <path>]
tk scope forget <name>
tk scope list          # bare `tk scope` and `tk scopes` also run list
```

- `init` creates and registers a new scope, writing a minimal `tk.cue` and a
  `.gitignore` covering `.tk.lock`. Exactly one of `--name` / `--auto-name` is
  required. In a dedicated tk repo, pass `--auto-commit` (omitting it registers
  repo-driven).
- `import` registers an existing on-disk scope, files in place; its name and
  auto-commit mode come from the on-disk `tk.cue`.
- `rebind` rewrites a registered scope's paths after a move or clone.
- `forget` unregisters a scope (registry and lens entries only); it never touches
  the scope's files.
- `list` prints parse-stable TSV, one line per scope: `name\tdir\troot\tmode`,
  where `mode` is `tk-driven`, `repo-driven`, `plain-files`, or `unknown`.

## Output and exit codes

- stdout is a path or closed TSV; diagnostics and closed tokens go to stderr.
- Exit `0` success; `2` for usage / bad CLI input; other failures are generic
  non-zero. There is no `--json` and no colour on stdout. `NO_COLOR` suppresses
  all ANSI.

## Upgrading (from pj)

`tk` is a hard cut from the former `pj` product name. There is no dual-read of
old wire names. Operator steps on a machine that still has `pj` installed:

1. Install `tk` (`go install github.com/p3bot/tk/cmd/tk@latest`, build from
   source, or `brew install p3bot/tap/tk` once the operator has tagged a
   post-cutover release and filled the formula `url` / `sha256`).
2. Rename on-disk scope wires that still use the old product names:
   `pj.cue` → `tk.cue`, and any `.gitignore` / lock entry `.pj.lock` → `.tk.lock`.
   (This product tree already ships `tk.cue` / `.tk.lock` under `.agents/tk/`.)
3. Use a fresh XDG state directory under `tk`
   (`${XDG_STATE_HOME:-~/.local/state}/tk/`). Do not copy or rename `index.db`
   from the old `pj` state dir — the schema table is now `tickets`, and cutover
   rebuilds from markdown via re-import.
4. Switch shell profiles and automation from `PJ_SCOPE` to `TK_SCOPE`.
5. Re-import or rebind scopes so the registry and index are built under `tk`
   (`tk scope import` / `tk scope rebind` as needed). Optional:
   `tk doctor --reindex`.
6. Reinstall the agent skill (`tk skill uninstall` then `tk skill install`, or
   install fresh). Retire old `pj` skill copies.
7. Retire the `pj` binary and formula (`brew uninstall p3bot/tap/pj` if present).
   The old `pj` XDG config/state dirs may be deleted at convenience.

## Development

| Task | Command |
|---|---|
| Build | `go build ./...` |
| Test | `go test ./...` |
| Format | `gofmt -w .` |
| Vet | `go vet ./...` |
| Lint | `golangci-lint run ./...` |
