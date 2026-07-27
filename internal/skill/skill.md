# Agent skill contract

Locate/mutate verbs print one **absolute** path line;
agents open that path with file tools. Never assume a cwd-relative path. There is no
`--json` and no "print full project markdown" verb — the body is the file.
`pj meta <id>` is the read-only header inspect (preamble + raw frontmatter); it is not
path hand-off and not a body dump (`path:` in the preamble is absolute for humans only).

## Core work loop

Primary agent loop for project work in a registered ambient scope:

```text
pj create "Title"               → path (status: draft; frontmatter + H1)
# file tools: fill body under the H1 (skill Body conventions)
pj status <id> todo             → path (promote when implementable)
pj next --claim                 → path (select + claim in-progress)
# file tools on that path
pj status <id> done             → path under archive/ (or review / blocked / cancelled)
# end of turn: see End of turn (by autoCommit mode) — not always pj sync
```

Rules:
- Prefer `pj next --claim` when starting implementation (select + claim under flock).
  `pj next` without `--claim` is a pure read (inspect only) — never assume a claim from
  it. Manual claim remains valid: `pj status <id> in-progress` after `pj next` or
  when the id is known.
- Only built-in `todo` is next-eligible. Custom statuses (even `category: active` or
  names like `ready`) never enter `next` — promote to `todo` when work should be queued.
- Do not `pj next`-expect or claim a `draft`. Promote with `pj status <id> todo` only when
  the body is implementable.
- After claim, edit the file at the printed path. Do not re-resolve by guessing filenames.
- Known id: `pj get <id>` → path. Short form is an exact short-id match (length 4–8) in
  the ambient scope (`--scope` / `PJ_SCOPE` / cwd) — including collision-repaired ids, not
  only create-length 4; full `<scope>-<short-id>` addresses any registered scope (no
  ambient needed for full id). Every ambient project verb accepts optional `--scope`
  (Resolution); doctor does not.
- Inspect header without opening the body: `pj meta <id>` (read-only; preamble + raw
  frontmatter). Do not parse `path:` from `meta` for hand-off — use `get`/`next`/`status`/
  `create`. Do not invent `pj meta` mutation.
- Status values are labels, not a workflow graph: any known status jump is legal
  (`draft -> todo`, `draft -> done`, `todo -> draft`, …); pj validates membership only
  (built-in or CUE custom).
- End of turn is mode-dependent (End of turn section). Do not cargo-cult `pj sync` on
  repo-driven or plain-files scopes. Any `pj create` this turn makes the mode-appropriate
  durability boundary **mandatory** (sync / host commit / disk-only) — see Capture.
- When stderr carries integrity or doctor-class warnings, run bare `pj doctor` first
  (report only). For `duplicate_id:` / `equal_order:` / `archive_non_terminal:` /
  `archive_terminal_at_root:`, run `pj doctor --repair` when ready to mutate **with
  ambient or `PJ_SCOPE` set** (or prefer `pj status` for archive layout when the intended
  status is clear); use `--all` only when the human wants every registered scope repaired.
  Without ambient, do not invent machine-wide `--repair`. For over-long order,
  `pj doctor --re-space-order` only if the report calls for it (same target rules).
  Escalate human-only classes (conflicts, name drift, residue). Skill body: Doctor and
  integrity warnings.

## Capture

- `pj create "<title>"` scaffolds frontmatter (default status `draft`) plus H1 `# <title>`,
  prints path; never self-commits (any status). Fill the rest of the body via file tools.
- Durability after create (locked — do not assume more): on disk and in the index only.
  Not durable-in-git and not durable-remote until the mode-appropriate boundary:
  pj-driven → `pj sync` (snapshot commits the allowlisted scaffold); repo-driven → host
  repo commit/PR; plain-files → disk is the whole story (no git). Same durability class
  for every create status, including terminal. A path under `archive/` with status `done`
  is **not** proof of a git commit. A crashed session after create without that boundary
  can leave an orphan scaffold on one machine only.
- **Any create this turn makes the mode-appropriate durability boundary mandatory** before
  the agent ends the turn (not optional hygiene). Pj-driven: `pj sync` when git is ready
  (or report `sync_disabled:` and that files are disk-only until setup). Repo-driven: ensure
  host commit/PR includes the create (heed `uncommitted:`). Plain-files: disk is enough;
  still report the new path. Do not leave a create this turn unsynced / uncommitted when
  the mode has a git boundary.
- Optional second positional: any known status. `todo` when the body is already known in
  the same turn; `backlog` for capture without authoring soon; `done` / `cancelled` (or
  custom done-class) as a terminal status **label shortcut** (no fake queue ceremony) —
  not a complete-state self-commit and not proof of git durability. Terminal create writes
  under `archive/` and may print a stderr durability cue; non-terminal create writes at
  dir root.
- After create: write the project writing-guide sections under the H1, then
  `pj status <id> todo` when implementable. Leaving a bare scaffold as `todo` is a misuse
  — that is what `draft` is for. Note: promoting with `pj status` *does* self-commit when
  auto-commit is available; create never does.
- Summary, depends, related, tags, links: set by direct frontmatter edit after create (no
  create flags for those in v1).
- Create always appends on `order` after the scope-wide max key; placement on the active
  board is `pj reorder` after promote when needed.

## Frontmatter mutation

Inspect (read-only): `pj meta <id>` prints a fixed preamble (`id`, `title` from H1,
`path`) and the project's frontmatter YAML exactly as stored — never the body, never a
write. Use after direct edits to confirm; agents still locate for edit via `get`/`next`/
`status`/`create` paths.

| Key | How to change |
|---|---|
| `id` | Never. Minted at create; stable forever. |
| `created` | Never. Set once at create. |
| `order` | Only via `pj reorder`. Never hand-edit. |
| `status` | Prefer `pj status <id> <status>` when the file parses (load-bearing: verb moves root ↔ `archive/` on the terminal boundary). Claim-from-queue: `pj next --claim` sets `in-progress` only. Free FM edit of `status` alone does not move the file — layout drift until `pj status` or `--repair`. If `parse_error:`, do not use the verb — fix the file at the `get` path first. Direct edit when resolving `status_conflict` or mid-file repair under human direction. |
| `status_conflict` | Only when resolving a status dispute (at least one terminal involved): set `status`, remove this key. Never invent it. |
| `depends`, `related` | Direct frontmatter edit; each entry a **full** `<scope>-<short-id>` (never bare short-id). Inspect lists in `pj meta`; neighbourhood with `pj deps` (read-only). |
| `tags`, `links`, `summary` | Direct frontmatter edit. Inspect with `pj meta`. |
| Custom fields (`pj.cue` `fields`) | Direct frontmatter edit. No `pj set`. Inspect with `pj meta`. Absent is always legal. |
| Undeclared keys | Avoid; doctor warns. Do not invent schema. Still visible in `pj meta`. |

After direct edits on auto-commit scopes, end-of-turn `pj sync` commits them. Prefer verbs
for status/order so self-commit and validation run on the write path.

## Body conventions

The markdown body is the project document handed to a fresh implementer session. CLI does
not model tasks or sections — convention only. This skill is self-contained: the section
list below is the primary contract. An org writing guide (e.g. `start get
project/writing` or equivalent) is an **optional** equivalent when present — not a
required dependency for open `pj`.

`pj create` writes the H1 (`# <title>` from the create argument). Retitle that H1 freely
afterward (slug stays frozen). Under the H1, use this section order when authoring for a
fresh implementer:

1. Goal
2. Scope
3. Current State
4. References (omit if none)
5. Requirements
6. Constraints
7. Implementation Plan
8. Implementation Guidance (omit if nothing non-obvious)
9. Acceptance Criteria

Right-size: omit sections that do not apply. Bias the document at the principled
long-term solution; define what/why, not keystrokes; no conversation references
("as discussed").

Also:
- Optional checkbox tasks under Implementation Plan or a Tasks subsection for local
  progress — never via CLI.
- When `status` is `blocked`, put the reason in the body (a short Blocked note is fine).
- `summary` frontmatter: one-line what/why for list/search; keep in sync with Goal when
  practical.
- List/search/meta "title" is the body H1 per closed extraction (below). Create provides
  an ATX H1 immediately. Fill the guide sections under the H1 before `pj status <id> todo`
  when authoring for the queue.

DECISION: title extraction for `list` / `meta` / search display (shared pure helper):
- Scan the markdown **body only** (after the closing frontmatter fence).
- Recognise **ATX H1 only**: a line matching `^#\s+.+` (one `#`, not `##`). Strip the
  leading `#` and surrounding whitespace; that text is the title.
- **First** such line wins; later H1s are ignored for the title field.
- **Setext** H1 (underline `===`) is **not** recognised — treat as body prose.
- No matching line → empty title string (not an error; list/meta still succeed).
- Do not use filename slug or `summary` as a fallback title in v1 (summary is its own
  field when shown).

## Title, slug, and filename

- Filename shape: `<id>-<slug>.md`. Slug is `slugify(create-title)` once at create
  (closed grammar and algorithm in design Project ids) and never updated. Empty title
  after trim is a usage error; do not invent a slug by hand.
- Retitle freely in the H1/body. Do not rename the file; do not edit `id`.
- `pj doctor` reports structural id/filename/slug-shape mismatch only — not H1/slug drift.
- Always reopen via `pj get`/`next`/`status` paths, not by reconstructing a slug from the
  current title or re-running `slugify` on the H1.

## Ordering

- Never hand-edit `order`. Use `pj reorder <id> (--before <id> | --after <id> | --first |
  --last)` only (destination flag required).
- `pj create` always appends (`keyBetween(last, null)`). `last` / `--last` / `--first` use
  the scope-wide max / min valid `order` among **all** projects in the scope (every
  status, including under `archive/`), not the default list set — intentional global
  domain (see Metadata order rank domain DECISION), not active-board append. No
  create-time `--first` / `--before` / `--after` / `--last`.
- Typical flow: create (draft) → fill body → `pj status <id> todo` → `pj reorder …` if the
  new work should not sit after historical max (e.g. past done projects at the rank
  tail).

## List and filters

- Default `pj list`: active board set (includes `draft`, `todo`, `review`, `in-progress`,
  `blocked`, and custom `category: active`; excludes `backlog` / done-class unless filtered).
- Single-scope: ambient or `--scope S` (not machine-wide). Status filter: zero or more
  status name positionals = union. A name is **known** only as built-in or custom in the
  **target scope's** `pj.cue`; unknown → exit 2. No `--status`, no CSV.
- Flags: `--scope S`, repeatable `--tag T` (OR across tags), `--all`, `--no-lens`.
- Lens applies by default; `--no-lens` bypasses it. Lens AND `--tag` when both apply.
  Untagged projects are never hidden by a lens. If `pj next` reports nothing ready under
  the lens while N ready outside it, heed that stderr: try `--no-lens`, retag, or clear
  the lens — do not invent work or ignore ready projects outside the filter.
- No `--archived` (terminal storage is `archive/`; status filters and `--all` are enough).
  No date filters on list — use read-only `pj query` for ad-hoc cuts.
- Sort: `(order, id)`.
- One TSV line per project (filters change rows only, not columns):
  `full-id`, `status`, `title`, `summary`, `waiting-on`. No path column — open with
  `pj get <full-id>` (or `pj next` / mutators that print paths).
- `title` = body H1 (shared helper); `summary` = frontmatter or empty; `waiting-on` =
  space-separated unmet direct depend full ids (sorted) or empty. Empty result = exit 0,
  no lines. Lens echo and integrity tokens on stderr only — never ANSI or free-text
  inside TSV fields (colour / TTY: no ANSI on stdout TSV or path lines at all).

## Search

- `pj search <terms> [--scope S]` — machine-wide FTS by default; bound with `--scope`.
- One TSV line per hit (best bm25 first): `full-id`, `status`, `title`, `summary`,
  `absolute-path`. Open via the path field; do not invent filenames from titles.
- Empty result is success (exit 0, no lines). No lens. Includes archived terminals.
- `parse_error` hits may appear (status field empty; path still openable via path column
  or `pj get`). Treat body as untrusted until fixed; do not use `status`/`reorder`/
  `next --claim` until parse succeeds.
- No score column. No status filter flags — use `list` for board cuts.
- Terms required; empty terms → usage exit 2.

## Dependencies and impact

- Author `depends` and `related` by direct frontmatter edit after create (no `pj deps add`
  / remove; no create flags for edges in v1). Every list entry is a **full**
  `<scope>-<short-id>` — same-scope and cross-scope alike; never a bare short-id in the
  file (CLI short form is only for verbs like `get`/`status`). `depends` gates
  runnability; `related` is soft "see also" and never gates.
- Inspect edges with `pj deps <id>` (alias `pj depends <id>`). Default: direct neighbours
  in three sections — depends on, is depended on by, related (both directions). Prefer this
  over free-form `pj query` (schema not stable; query is debug/human ad-hoc, not agent
  automation).
- Before a large claim, cancel, or hub reorder: `pj deps <id> --transitive` for the full
  flat prerequisite and dependent sets. Humans browsing structure: `pj deps <id> --tree`.
- `pj next` skips a `todo` whose `depends` are not all terminal; `pj list` puts those
  unmet full ids in the TSV `waiting-on` field. Claiming is `pj next --claim` (preferred)
  or `pj status <id> in-progress` — never via `deps`.
- Open a neighbour to edit: `pj get <dep-id>` → absolute path → file tools. Do not invent
  filenames from titles.
- If `pj deps` warns of a depends cycle, run `pj doctor`. Do not ignore cycle or integrity
  warnings.
- Unresolvable targets stay listed and annotated (held for gates — see design Status and
  dependencies).

## Archive

- Location follows terminal-ness (see design "Done and archive"). There is no `pj archive`
  or `pj unarchive` verb.
- Finish work with `pj status <id> done` (or `cancelled` / custom done-class). The status
  verb moves the file under `archive/` and prints the post-move path. Create with a
  terminal status writes the scaffold under `archive/` already.
- Reopen with `pj status <id> todo` (or another non-terminal): the status verb moves the
  file back to the dir root. Labels, not a workflow — legal; rare for agents.
- Do not hand-move project files between dir root and `archive/`. Layout drift from
  hand-edited status is reported as `archive_non_terminal:` /
  `archive_terminal_at_root:`; fix with `pj status` (preferred) or `pj doctor --repair`.
- `pj next` never hands a path under `archive/`. Terminal projects stay get / meta /
  search / deps resolvable; default list hides done-class; `--all` brings them back.

## End of turn (by autoCommit mode)

Branch on the ambient scope's mode (from `pj.cue` `autoCommit` + whether the dir is in
git — labels: pj-driven / repo-driven / plain-files):

| Mode | End of turn |
|---|---|
| pj-driven (`autoCommit: true`) | **Mandatory** when this turn ran `pj create` or other allowlisted dirty work needs the push boundary: `pj sync` when git+upstream exist (use `pj sync --all` when cross-scope gates need fresh remotes). If stderr shows `sync_disabled:`, set up the repo/remote with plain git first — file writes already landed; no inventing `git init` via pj. Sync is the first git/remote durability for any create scaffold (including terminal under `archive/`). |
| repo-driven (`false` inside git) | Do **not** call `pj sync` (it refuses). **Mandatory** host commit/PR for any create or complete-state write this turn. After the turn, bare `pj doctor` (or heed write-side warnings): if `uncommitted:` appears, stop and ensure host commit/PR — do not invent pj commit. |
| plain-files (`false` outside git) | Do **not** call `pj sync` (it refuses). Run bare `pj doctor` if integrity warnings appeared or after multi-machine file sync; `pj doctor --repair` under ambient/`PJ_SCOPE` when acting on `duplicate_id:` / `equal_order:` — **one machine at a time** (flock is not cross-host); `--all` only if every local scope should be repaired; let external sync settle before another peer repairs the same tree. Creates are disk-only (no git boundary). |

Never invent `pj save` / `pj end`. Mode is a property of the scope, not a per-command flag.
Never treat `pj create`'s printed path (or `archive/` + `done`) as proof of a git commit
or remote push. Any create this turn → durability boundary is mandatory (Capture).

## Conflicts and paused sync

Fail fast. Do not keep authoring on a conflicted or mid-rebase auto-commit git-root.

| Signal | Agent action |
|---|---|
| Body conflict markers in a project file | Stop. Report path. Do not pick a side or delete markers unless the human already directed the resolution. Body-only markers do **not** set `parse_error` (FM still indexed); do not treat body prose as trusted until markers are gone. Human edits body → `pj sync` to resume. |
| Markers inside frontmatter / broken YAML | `parse_error:` quarantine — open `pj get` path; fix FM; `status`/`reorder`/`next --claim` refuse until parse succeeds. |
| `status_conflict` in frontmatter | Stop. Report path and the two disputed statuses (`pj meta` / `pj get` / doctor). Do not choose unless the human (or explicit task) already picked one; then set `status` (either listed value or another known status), remove `status_conflict`, `pj sync`. |
| `pj sync` reports a delete/edit handoff | Stop. One machine deleted the project file while the other edited it; sync resolves neither. Report the path, which side deleted, and the surviving edit's `status`. Do not restore or re-delete on your own judgement — the human decides, then `pj sync` resumes. Re-running `pj sync` without acting re-pauses on the same handoff **by design**: it is not a transient failure and retrying is not progress. The three resolutions are the human's — remove the file (deletion wins), edit it (surviving edit wins), or `git add` it (kept as-is). |
| Self-commit / complete-state verb refuses mid-rebase | Stop. Do not retry `status`/`reorder`/`next --claim`/`create`/`doctor --repair`. Report the refused command and named file/scope. Resolve body markers / `status_conflict` via `pj edit` or file tools, then `pj sync`. Do not invent alternate write verbs. |
| `pj sync` pauses / reports unresolvable conflict | Stop the turn's project work on that repo. Surface sync output. No parallel "fix it in the background". |

Never invent merge resolution heuristics (prefer-done, LWW body, etc.). Non-auto-commit scopes have no pj rebase seam; host-repo markers still stop-and-report — body-only: resolve text in-file (no `parse_error`); FM markers / broken YAML: `parse_error` / doctor.

## Concurrent agents

`pj next` without `--claim` is a pure read (two inspects can see the same ready
project). **Same working
tree:** `pj next --claim` serialises under the scope flock with local CAS — two claim
attempts cannot both hand off the same id. **Cross-clone / multi-machine:** no distributed
lock; claims become visible after `pj sync` (or host/external sync). No assignee field,
no claim lease, no push-on-claim in v1. Scope `flock` does not cover body edits via file
tools. No extra file-lock machinery in v1.

Safe practice:
- Prefer one writer agent per scope working tree when possible.
- Start work with `pj next --claim` (not `pj next` then a delayed status write).
- Do not file-tool write (or `$EDITOR` via `pj edit`) on a project body without a claim
  (`in-progress` via `--claim` or `pj status`) — flock covers pj verbs only, not
  concurrent body edits.
- If the project is already `in-progress` (or another agent clearly owns it), do not take
  it: run `pj next --claim` again or stop and report. Do not double-edit the same path.
- Multi-user / multi-clone: after claim, `pj sync` when others may take queue work so the
  claim is visible; do not invent a second lock in the project file.
- Abandoned claim: if work stops mid-claim, leave a clear body note when possible; doctor
  soft-warns `stale_in_progress:` after 72h without file mtime activity. Recovery is a
  deliberate `pj status` back to `todo` (or `blocked`), never automatic.

## Cold start and import

Registry only — pj never scans the tree for an unregistered `pj.cue` and never
auto-registers on clone. When importing on a second machine, use the same scope **name**
already in that scope's `pj.cue` (import reads it); do not invent a different local name
for the same work set.

Bootstrap (user/org choice — see Discovery): `pj skill` on demand, skill install when
available, or own handoff (AGENTS.md / docs). All paths still need registration before
project verbs.

When there is no ambient scope:
- Do not probe for scope dirs or invent paths.
- Do not treat `pj skill install` as available in v1 (hard-refuse placeholder).
- Prefer `pj skill` if the agent can run the CLI and needs the contract; otherwise use a
  path from the human or from project docs (e.g. a one-liner in AGENTS.md naming the
  scope dir). Then: `pj scope import <dir> [--code-root <path>]` or
  `pj scope rebind <dir> --name <scope> [--code-root …]` / `--scope` / `PJ_SCOPE` as
  appropriate.
- `pj skill` itself needs no scope — print the contract, then fix registration before
  project verbs.

Own bootstrap (human-authored, never written by pj, never auto-committed by pj): document
the scope dir in the host repo (repo-root AGENTS.md or equivalent) so a cold agent can
import without guessing. Durability of that handoff is host git, not `pj sync`.

## Cross-scope work

- Address other scopes with full ids (`<scope>-<short-id>`). Never invent a scope name;
  only use names from `pj scope list` / registered registry.
- Scope names are fleet-global **by convention**, enforced only per machine. On every
  machine that resolves a cross-scope id, register the **same** name for the same scope
  dir. Do not reuse a short name (e.g. `api`) for a different scope on another machine —
  pj cannot detect that clash and a `depends` gate would hit the wrong project.
- If `name_drift:` appears (registry key ≠ `pj.cue` name after a remote scope rename):
  that scope is fail-closed — forget+import before any project verb. Do not rely on
  short ids under the old registration.
- Author `depends` / `related` by direct frontmatter edit (same- or cross-scope). Inspect
  with `pj deps <id>` — read-only; do not invent edge verbs.
- If `pj next` / `list` / `deps` annotate that a depended-on scope is not registered here:
  stop and ask for import/clone of that scope. Do **not** clear the edge to “unblock”
  yourself — the hold is intentional.
- Cross-scope gate freshness: `pj next` / list only reconcile **local disk** for
  depended-on scopes — they do not fetch remotes. Bare `pj sync` only fetches the ambient
  auto-commit repo. When work depends on status in another auto-commit scope (especially
  after multi-machine edits), run `pj sync --all` or sync that scope before trusting the
  gate. Repo-driven / plain-files: no pj sync — freshness is the host/external sync of
  those trees.
- Shared git-root coupling: several auto-commit scopes in one repo share one push and one
  freeze domain. A conflict, `status_conflict`, or unparseable sibling `pj.cue` can block
  sync/writes for **all** those scopes until fixed. That is the price of one-push sync —
  not a bug. Need isolation → separate git-root (do not invent per-scope sync isolation
  flags). See Auto-commit DECISION on multi-scope messaging.

## Waiting and external blockers

Use the right mechanism — do not overload one label for every kind of wait:

| Situation | Use | Do not |
|---|---|---|
| This project cannot start until another **project** is terminal | `depends: [<scope>-<short-id>]` full id only (same- or cross-scope) | `blocked` alone for a project dependency; bare short-id in the list |
| Stalled on a **human or external** factor with no project id | `pj status <id> blocked` and write the reason in the body; put PR/issue/URL in `links` | Fake a `depends` on a non-project |
| The **work product** is under review (plan or result) | `pj status <id> review` | `blocked` unless review is stuck on a person/process outside the review itself |
| Soft “see also” / provenance | `related: [<scope>-<short-id>]` full id only | Using `related` or tags as a runnability gate; bare short-id in the list |
| Topic / area only | `tags` | Encoding wait state in tags |

`depends` is the only project-to-project gate for `pj next`. `blocked` is manual and
human-owned — pj never auto-sets it. Inspect edges with `pj deps`; edit edges in
frontmatter.

## Unsupported operations

Do not invent verbs or flags. v1 does not support:

| Do not | Instead |
|---|---|
| Transfer / split / merge / copy a project across scopes | `pj create` in the target scope; `related` or `depends` as needed; `cancelled` or leave the old one |
| Task-level CLI (checkboxes as objects) | Edit body checkboxes/sections with file tools |
| `--json` or machine envelopes | Paths + short text; open the file |
| `pj deps` mutation (`add`/`rm`) | Edit `depends` / `related` in frontmatter; `deps` is read-only |
| `pj set` / `pj field` / `pj meta` mutation | Direct frontmatter edit (customs per `pj.cue`); `meta` is read-only inspect |
| `pj query` mutating SQL (`INSERT`/`UPDATE`/`DELETE`/`DROP`/…) | Read-only `SELECT` (and read-only explain); durable change is files / doctor |
| Agent automation via free-form `pj query` / index schema | Prefer `deps` / `list` / `search` / `next` / `get` / `meta` — query is debug/human ad-hoc only; schema not stable |
| `pj archive` / `pj unarchive` | Location follows terminal: `pj status … done` moves into `archive/`; `pj status` to non-terminal moves out; doctor `--repair` fixes drift |
| `pj next` (no `--claim`) as a claim | `pj next --claim`, or `pj next` then `pj status <id> in-progress` |
| Claim leases, push-on-claim, `claim_term` / assignee fields | Local CAS on `--claim` only; multi-clone visibility via sync; stale → doctor + deliberate status |
| `pj skill install` (v1) | `pj skill` print; human AGENTS.md path for import |
| Hand-edit `id`, `created`, or `order` | Verbs: create/status/reorder only for those concerns |
| Hand-rename `<id>-<slug>.md` to chase a new title | Slug frozen; retitle H1 (and optional `summary`) only — three names may diverge |

If a need is not on the CLI surface, stop and ask — do not improvise a parallel tool.

## Doctor and integrity warnings

DECISION: machine-actionable integrity and doctor-class signals use a **closed set of
stable tokens** as line prefixes (stderr on ordinary commands; doctor report lines too).
No `--json` envelope. Human-readable detail may follow the token on the same line (or
subsequent indented lines). Adding a token is a conscious design change; do not invent
ad-hoc prefixes in implementation. Token characters themselves are never ANSI-coloured
or interrupted (see colour / TTY DECISION); agents may match prefixes without an ANSI
strip step.

Two consumption rules (both required — tokens alone are not the whole doctor UX):

1. **Command stderr:** when a line prefix is in the closed set, never ignore it — act per
   the agent rules below or run bare `pj doctor`.
2. **Bare `pj doctor`:** read the **full report** (token lines and any short human
   summary). Do not claim “tokens only, skip the rest of doctor.” Doctor may still use
   free prose for rare or purely informational notes; those without a token are
   human-priority, not agent-automation keys.

Closed v1 token set (prefix form, including the trailing colon). Every class that should
drive agent action has a token; implementers emit these from doctor and from hot-path
warnings where the design already rides stderr.

| Token | Meaning / agent action class |
|---|---|
| `duplicate_id:` | Two or more projects share an id — bare doctor then `--repair` when ready; id-taking verbs refuse (no path) until unique |
| `equal_order:` | Two or more projects share an `order` key — bare doctor then `--repair` when ready |
| `order_long:` | Pathologically long `order` key(s) — report; optional `--re-space-order` |
| `parse_error:` | Frontmatter unparseable (broken fence/YAML, markers **inside** FM, bad `order`, …) — quarantined; `get` hands path **exit 0** + this token on stderr; `status`/`reorder`/`next --claim` refuse (non-zero) until parse succeeds. Body-only conflict markers do **not** emit this token |
| `unreachable_scope:` | Registered dir could not be stated/opened (missing, unmounted, permission, I/O) — report only; keep index rows; do not auto-forget; doctor may include the OS error string; human decides wait vs `pj scope forget` |
| `non_allowlist:` | Path under scope dir outside allowlist — move/remove; never force-commit |
| `config_unparseable:` | `pj.cue` or XDG CUE will not load — fix config; writes/sync may be blocked |
| `status_conflict:` | Status dispute key present — resolve in-file; mid-rebase then sync |
| `depends_cycle:` | Depends cycle — fix edges; do not ignore |
| `depends_dangling:` | Same-scope `depends` target missing — hard; fix or remove edge |
| `depends_self:` | Project lists its own id in `depends` — hard; remove self-edge |
| `depends_unresolvable:` | Cross-scope `depends` target not resolvable here — informational hold; import/clone or clear edge only with human intent |
| `depends_on_cancelled:` | Depends on a cancelled (or done-class abandoned) project — human decide if still valid |
| `edge_verify:` | Edge (`depends` or `related`) may mispoint — inbound edge to a collision-repaired id, or cross-scope inbound after scope rename. Emitted only in the repairing operation's output (sync integrity / `doctor --repair` / `scope rename`), never by later bare doctor (not persisted, not re-derivable) — human verify from that report |
| `related_unresolvable:` | Soft related target missing — cosmetic; note only |
| `auto_commit_mismatch:` | autoCommit disagree across shared git-root — fix before sync |
| `archive_non_terminal:` | Non-terminal status under `archive/` — layout drift; `status` or `--repair` moves to root |
| `archive_terminal_at_root:` | Terminal status still at dir root — layout drift; `status` or `--repair` moves to `archive/` |
| `sync_disabled:` | Auto-commit: no git-root and/or no upstream for sync — see Writes / Sync |
| `last_push_error:` | Last auto-commit push failed — XDG `git-roots/<key>/last-push-error`; fix remote/auth, sync again |
| `stale_in_progress:` | Built-in `in-progress`, mtime older than 72h — inspect; maybe reopen to todo |
| `name_drift:` | Registry key ≠ `pj.cue` name — forget+import; scope unusable until then |
| `uncommitted:` | Repo-driven allowlisted dirty files — host commit |
| `schema_error:` | Frontmatter hard schema violation (unknown status, bad field type/`values`, `depends`/`related` entry not a legal full project id) — fix file; a malformed `depends` entry counts as unmet and holds the project out of `pj next` |
| `schema_warn:` | Soft schema (undeclared key, `knownTags` typo, self-`related`, duplicate list entries, id-shaped `links`) — fix or ignore deliberately; no separate `related_self:` |

Example shape (illustrative):

```text
duplicate_id: wc-ab2c in scope wc (2 files) — run pj doctor
equal_order: 2 projects in scope wc share order "a1" — run pj doctor
depends_dangling: wc-ab2c depends on wc-zzzz (missing)
```

Agent rules:
- Never ignore a closed-set token on stderr. Prefer bare `pj doctor` for the full picture,
  then act. On bare doctor, read the whole report, not only lines you regex for tokens.
- `duplicate_id:` / `equal_order:` → after reviewing the report, `pj doctor --repair`
  under ambient or `PJ_SCOPE` for that scope (mutates; auto-commit self-commits when a
  git-root exists). Machine-wide only with explicit `pj doctor --repair --all` when the
  human intends every scope. Do not assume bare doctor rewrote files. While
  `duplicate_id:` stands, do not expect `get`/`status`/… on that id to return a path —
  they refuse until repair. No `pj doctor --scope` — use ambient cwd, `PJ_SCOPE=<name>`,
  or `--all` for mutating flags; bare `--repair` with no ambient is a usage error.
- `order_long:` → `pj doctor --re-space-order` only when chosen; not part of `--repair`
  (same mutating target set: ambient / `PJ_SCOPE` / `--all`).
- Plain-files multi-machine: no `pj sync` seam — bare doctor when tokens appear and
  periodically after external file sync; `--repair` when acting on collision/equal-order
  on **one** machine at a time under ambient/`PJ_SCOPE` (do not race dual `--repair`
  across peers; do not use `--all` unless every local scope should be repaired).
- After human conflict resolution (body markers / `status_conflict`), run bare `pj doctor`
  if unsure, then `pj sync` on pj-driven scopes to resume.
- `pj doctor --reindex` only when the mtime heuristic is fooled (restore, clock skew) —
  rare escape hatch, never routine; never mutates project files.
- After `--repair` / `--re-space-order`, read the report; do not re-introduce hand-fixed
  ids that fight the repair.
- `stale_in_progress:` → open the path, check whether work is still live; if abandoned,
  `pj status <id> todo` (or `blocked` + body reason). Never invent auto-reopen or
  auto-steal on `next --claim`.
- `name_drift:` → stop project work on that scope. Run exactly
  `pj scope forget <old>` then `pj scope import <dir> [--code-root …]` (names/paths from
  the doctor/error text). Re-set `pj lens` if needed. Do not invent ambient short-id
  workarounds or auto-rekey.
- `uncommitted:` → repo-driven only. Do not call `pj sync`. Hand off to host git/PR (or
  human). Writes already landed on disk; durability needs the host commit.
- `unreachable_scope:` → registered dir could not be stated/opened (missing, unmount,
  permission, I/O — one token; no separate “path gone forever” token). Do **not**
  `pj scope forget` solely for this. Retry when the path is back; forget only when the
  human decides the registration should end permanently.
- `parse_error:` → open the path from `pj get` (or doctor); `get` exit 0 + path is
  hand-off success, not a healthy project. Fix frontmatter (YAML, fence, markers inside
  FM). Do not expect `pj status`/`reorder`/`next --claim` to succeed until reconcile
  parses again. Body-only markers without this token are a conflict handoff, not
  quarantine — stop-and-report per Conflicts; do not invent FM repair.
- `depends_dangling:` / `depends_self:` / `schema_error:` / `config_unparseable:` /
  `auto_commit_mismatch:` / `last_push_error:` / `edge_verify:` / `depends_cycle:` → fix
  or escalate from the report; do not silence by ignoring the token.
- `archive_non_terminal:` / `archive_terminal_at_root:` → prefer `pj status` to the
  intended status (moves layout); or `pj doctor --repair` to reconcile layout to status.
  Do not hand-move files between root and `archive/`.
- `depends_unresolvable:` / `related_unresolvable:` / `schema_warn:` → note; do not clear
  edges or invent fixes without intent.
