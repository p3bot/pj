---
name: pj
description: >-
  Project management with the pj CLI: plain markdown project, plan, or
  spec files in a scope. Use when the user is doing feature or ticket
  work in a pj-managed repo, or mentions pj, scope, the board, next,
  claim, mark, depends, tickets, or project files — even if they only
  say "pick up the next task", "what's on the board", "mark it done",
  or "create a project".
---

# Project management with pj

A scope is a project directory plus its pj.cue, registered on this machine
(ambient via cwd / PJ_SCOPE / --scope). create, get, next, and mark (on terminal
moves) print a cleaned absolute path on stdout — open that path; never invent
filenames. get --content prints the full file on stdout instead of the path.
Edit the body under the document H1; preserve the YAML frontmatter fence.
Change status with mark and order with reorder, not by hand-editing those keys.
Paths and table data on stdout; tokens and warnings on stderr — parse both.
Writes take a per-scope flock; prefer next --claim so agents do not collide on
the same todo.

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

## Project files

One project is one markdown file: YAML frontmatter fence, then a single ATX H1,
then the body.

```text
---
id: <scope>-<short>
status: draft
order: "…"
created: "…"
# optional: summary, depends, related, tags, links, customs
---

# Title

Body…
```

Active files live at the scope dir root; terminal status moves them to archive/
via mark (never hand-move). Built-in statuses: draft, backlog, todo, in-progress,
review, blocked, done, cancelled (plus CUE customs). Among built-ins only todo is
next-eligible (depends all terminal, lens, dir root, not a duplicate id).

Ownership:
- body under H1: file tools; never replace the frontmatter fence as a side effect
- status → pj mark; order → pj reorder
- id, created: never invent or "repair"
- status_conflict: not via meta; see Recovery
- summary / scalar customs → pj meta set
- depends, related, tags, links → pj meta add|rm (depends refuses self/dangling/unresolvable)

If a tool only supports full-file write: read first, pass the existing fence
through unchanged, change body only.

## Identifiers

Full id is `<scope>-<short>` (short is letter-first, never leading digit). Short
ids resolve in the ambient scope (`--scope` / `PJ_SCOPE` / cwd); a full id
resolves in any registered scope. Prefer full ids on depends/related.

create freezes the filename as `<id>-<slug>.md` from the title at create time.
Do not hand-rename project files (mark moves between dir root and archive/).
Editing the H1 does not rename the file; leave the frozen slug unless a repair
path says otherwise.

## Workflows

Orient:

```
pj status [--scope S]                    # mode, next, claimed, integrity, uncommitted
pj list [status...] [--tag T]... [--all] [--no-lens]
pj next [--no-lens]                      # next available todo document path (read-only; no --claim)
pj get <id>                              # absolute path when you already have an id
pj get <id> --content                    # full file on stdout (no path)
```

Core work loop:

```
pj next --claim                          # or: pj get <id> when the id is known
# --claim: first eligible todo → in-progress (scope flock); self-commits if pj-driven+git-root
# without --claim: pure read (Orient). empty queue: blocked-by-deps vs empty vs lens-emptied
# open the absolute path printed on stdout — never invent a path or filename
# edit body under the H1 only; preserve the YAML frontmatter fence
# do the work described in the project document
pj mark <id> <status>                    # in-progress | review | done | blocked | …
# if mark prints a new path (terminal boundary), use that path afterwards
# then: Durability
```

Capture:

```
pj create <title> [status]               # prints absolute path; default status draft
# open the printed path
# fill body under the H1; preserve the create-owned frontmatter fence
pj meta set <id> summary <text>          # optional
pj meta add <id> depends|related|tags|links <value>   # optional
pj mark <id> <status>                    # optional; terminal status lands under archive/
pj reorder <id> --first|--last|--before <id>|--after <id>   # optional
# then: Durability  (create never self-commits)
```

Board:

```
# list TSV (headerless): full-id \t status \t title \t waiting-on
# bare list = default active set; status args union-filter (include archive matches);
# --all expands the unfiltered board (incl. backlog/done/archive); --tag is OR
pj list [status...] [--tag T]... [--all] [--no-lens]
pj reorder <id> --before <id>|--after <id>|--first|--last
pj lens [tags...]                        # default tag view for list/next; untagged never hidden
pj lens --clear
pj search <terms> [--scope S]            # FTS5; TSV includes absolute path
```

Dependencies:

```
pj deps <id> [--transitive] [--tree]     # depends + related neighbourhood
pj meta add <id> depends <target-id>     # hard checks: self / dangling / unresolvable refuse
pj meta rm <id> depends <target-id>
pj meta add <id> related <target-id>     # soft; no existence check
pj meta rm <id> related <target-id>
pj next                                  # re-check eligibility after edge changes
```

Manage scopes:

```
pj scope list
pj scope init <dir> (--name <name> | --auto-name) [--code-root <path>] [--auto-commit]
# --auto-commit writes autoCommit: true (pj-driven); omit → repo-driven when a git-root exists
pj scope import <dir> [--code-root <path>]          # after clone; name/autoCommit from pj.cue
pj scope rebind <dir> --name <name> [--code-root <path>]   # after folder move
pj scope forget <name>                              # registry + lens only; files stay
pj scope rename <old> <new>                         # cue, ids, filenames, in-scope edges
```

Durability:

```
pj status                                # read mode (from pj.cue autoCommit + git-root)
# autoCommit true  → pj-driven (even without a git-root)
# autoCommit false + git-root → repo-driven
# autoCommit false + no git   → plain-files
# config_unparseable: on stderr → treat mode as unhealthy; fix pj.cue first

# pj-driven (auto-commit):
#   mutators (mark, reorder, next --claim, meta, …) self-commit when a git-root exists
#   create never self-commits — still needs the end-of-turn step below
#   pj sync [--all]  — sole push/integrate boundary (auto-commit git-roots only)
#     snapshot allowlisted dirty → fetch/rebase → resolve FM → integrity → push if ahead
#   ambient non-auto-commit: sync refuses; --all skips non-auto-commit roots
#   never host git push / rebase / force-push around pj sync

# repo-driven:
#   no pj self-commit; host git commit (and host push if needed)
#   uncommitted: on stderr means dirty board — not a prompt to pj sync
#   do not pj sync

# plain-files:
#   no git step
```

Integrity:

```
pj doctor                                # diagnose; tokens on stderr; no mutation
pj doctor --repair                       # id collisions, equal order, archive layout drift
pj doctor --re-space-order               # shorten pathologically long order keys
pj doctor --reindex                      # rebuild index from files only
pj doctor --all …                        # every registered scope (with mutating flags)
# doctor has no --scope; uses ambient / PJ_SCOPE / --all
# refuse repair on mid-rebase auto-commit git-root — finish Recovery first
# stable token: prefixes on stderr name the class; follow the message (no invented repairs)
```

Recovery:

```
pj status
pj doctor
# rebase paused (pj-driven): resolve named file(s) in place, then pj sync again
# status_conflict in a project file:
#   set status: to one value, delete the status_conflict key, save
#   pj-driven → pj sync; repo-driven/plain → no sync step for residue clear
# uncommitted: (repo-driven) → host commit; not pj sync
# sync_disabled: / last push error → fix remote/auth/upstream, then pj sync
# never force-push; never invent a parallel git integrate path around pj sync
```

