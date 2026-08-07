---
name: pj
description: >-
  Project management with the pj CLI: plain markdown project, plan, or
  spec files in a scope. Use when doing feature or ticket work in a repo,
  or the user mentions pj, scope, the board, next, claim, mark, depends,
  tickets, or project files — even if they only say
  "pick up the next task", "what's on the board", "mark it done",
  or "create a project".
---

# Project management with pj

- A scope is a project directory plus its pj.cue
- pj create, get, next, and mark print a cleaned absolute path on stdout
- Use the path, do not create project files yourself
- A project is one markdown file: YAML frontmatter fence, then a single ATX H1, then the body.
- Edit the project document body under the H1
- Preserve the YAML frontmatter, never overwrite the document
- Do not hand-edit frontmatter keys and values
- Paths and table data on stdout; tokens and warnings on stderr
- pj writes take a per-scope flock; prefer `pj next --claim` so agents do not collide
- Active files live at the scope dir root; terminal status moves them to archive/
- Built-in statuses: draft, backlog, todo, in-progress, review, blocked, done, cancelled
- The todo status is next-eligible

## Frontmatter

- status → pj mark
- order → pj reorder
- id, created: never invent or "repair"
- status_conflict: not via meta; see Recovery
- summary / scalar customs → pj meta set
- depends, related, tags, links → pj meta add|rm
- related write is one-way on the subject only (no mirror on the target); deps shows both directions

## Commands

```
pj create <title> [status] [--scope S]                              # Scaffold project (FM + H1); print path
pj get <id> [--content] [--scope S]                                 # Resolve id to path; --content prints full file
pj mark <id> <status> [--scope S]                                   # Set status; rename on terminal boundary
pj reorder <id> (--before <id> | --after <id> | --first | --last) [--scope S]  # Move board order key
pj next [--scope S] [--no-lens] [--claim]                           # First runnable path; --claim sets in-progress

pj list [status...] [--scope S] [--tag T]... [--all] [--no-lens]    # Board inventory TSV
pj status [--scope S]                                               # Scope pulse (key/value counts)
pj meta get <id> [key] [--scope S]                                  # Full header (title/path/lines/words/characters + FM) or one key
pj meta set <id> <key> <value> [--scope S]                          # Set scalar frontmatter key
pj meta add <id> <key> <value> [--scope S]                          # Append multi-value frontmatter entry
pj meta rm <id> <key> <value> [--scope S]                           # Remove multi-value frontmatter entry
pj deps <id> [--scope S] [--transitive] [--tree]                    # Depends/related neighbourhood
pj search <terms> [--scope S]                                       # FTS5 search titles and bodies
pj query <sql>                                                      # Ad-hoc read-only SQL; schema unstable
pj query --schema                                                   # Debug only — do not script against it
pj lens [tags...] [--scope S]                                       # Set machine-local default tag view
pj lens --clear [--scope S]                                         # Clear the lens for a scope

pj scope init <dir> (--name <name> | --auto-name) [--code-root <path>] [--auto-commit]  # Create and register scope
pj scope import <dir> [--code-root <path>]                          # Register existing on-disk scope
pj scope rebind <dir> --name <name> [--code-root <path>]            # Rewrite registry paths after move/clone
pj scope forget <name>                                              # Unregister scope (registry + lens only)
pj scope list                                                       # List registered scopes (TSV)
pj scope rename <old> <new>                                         # Rename scope end-to-end
pj sync [--scope S] [--all]                                         # Sole push boundary (auto-commit roots)
pj doctor [--reindex] [--repair] [--re-space-order] [--all]         # Diagnose integrity; optional repair
pj skill                                                            # Print this agent skill contract
pj skill install [agents...] [--local]                              # Install into agentdex skills roots
pj skill list [--local]                                             # List installed skill copies (default agent set)
pj skill uninstall [agents...] [--local]                            # Remove owned pure skill installs
```

## Identifiers

- Full id is `<scope>-<short>`
- Short ids resolve in the ambient scope
- Full id resolves in any registered scope
- Prefer full ids on depends/related
- Create freezes the filename as `<id>-<slug>.md` from the title at create time
- Do not hand-rename project files
- Editing the H1 does not rename the file; leave the frozen slug

## Workflows

Orient: `pj status` -> `pj list` -> `pj next` | `pj get <id>`

Core work loop: `pj next --claim` -> edit body under H1 -> `pj mark <id> <status>` -> Durability

Capture: `pj create <title>` -> fill body -> optional meta/reorder/mark -> Durability

Board: `pj list` -> `pj reorder` | `pj lens` | `pj search`

Dependencies: `pj deps <id>` -> `pj meta add|rm depends|related` -> `pj next`

Manage scopes: `pj scope list` -> `init` | `import` | `rebind` | `forget` | `rename`

Durability:
- pj-driven: mutators self-commit -> `pj sync` (sole push; never host push/rebase)
- repo-driven: host git commit/push (no `pj sync`)
- plain-files: no git step

Integrity: `pj doctor` -> optional `--repair` | `--re-space-order` | `--reindex` | `--all`

Recovery: `pj status` -> `pj doctor` -> fix residue -> `pj sync` if pj-driven
