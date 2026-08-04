package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/p3bot/pj/internal/index"
	"github.com/p3bot/pj/internal/scopeadmin"
	"github.com/p3bot/pj/internal/scopeconfig"
	"github.com/p3bot/pj/internal/status"
)

// statusKeys is the locked stdout key order for pj status (pad + single tab).
var statusKeys = []string{
	"scope",
	"dir",
	"resolved",
	"mode",
	"lens",
	"total",
	"todo",
	"review",
	"in-progress",
	"blocked",
	"draft",
	"backlog",
	"done",
	"cancelled",
	"next",
	"claimed",
	"blocked_ids",
	"dangling",
	"integrity",
	"uncommitted",
}

// statusKeyWidth is the longest locked key (shared with tests).
var statusKeyWidth = maxStatusKeyWidth()

func maxStatusKeyWidth() int {
	w := 0
	for _, k := range statusKeys {
		if n := len(k); n > w {
			w = n
		}
	}
	return w
}

func newStatusCmd(app *App) *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "status [--scope S]",
		Short: "Scope pulse: key/value counts, next, claimed, integrity",
		Long: "Print a pure-read orientation block for one scope as parse-stable key\\tvalue\n" +
			"lines (no header). Keys are left-justified to a fixed column (longest key width)\n" +
			"before the tab so values align for humans; parse by splitting on the first tab\n" +
			"and trimming trailing spaces from the key. Empty values still emit the key line\n" +
			"(`key\\t` with padding) so agents can rely on key presence. Exit 0 whenever\n" +
			"ambient resolve succeeds — including when next is empty (an empty queue is a\n" +
			"pulse value, not a failure).\n" +
			"\n" +
			"Locked keys (order fixed): scope, dir, resolved, mode, lens, total, todo,\n" +
			"review, in-progress, blocked, draft, backlog, done, cancelled, next, claimed,\n" +
			"blocked_ids, dangling, integrity, uncommitted.\n" +
			"\n" +
			"resolved is how the scope was chosen: flag (--scope), env (PJ_SCOPE), or cwd\n" +
			"(longest-prefix code-root).\n" +
			"\n" +
			"mode is one of pj-driven | repo-driven | plain-files only. With a known schema:\n" +
			"autoCommit true → pj-driven (with or without a git-root — the planned no-repo\n" +
			"layout stays pj-driven); autoCommit false + git-root → repo-driven; false and\n" +
			"no git-root → plain-files. When the schema is unusable, mode is plain-files and\n" +
			"config_unparseable: rides stderr — do not read plain-files as healthy host files\n" +
			"without checking stderr. uncommitted is non-zero only in repo-driven mode.\n" +
			"\n" +
			"The active lens filters the working board (non-terminal status counts, claimed,\n" +
			"blocked_ids) the same way bare list and pj next do. total is the full-scope\n" +
			"count of parseable projects (dir root and archive/, every status including\n" +
			"backlog and terminals) and ignores the lens. Working-board built-in counts\n" +
			"include backlog (unlike bare list). Terminal tallies (done, cancelled) are full-scope\n" +
			"including archive/ and ignore the lens. Identity and health keys (dangling,\n" +
			"integrity, uncommitted) ignore the lens.\n" +
			"\n" +
			"next reuses pj next selection (reconcileClosure + depends gate + lens) but never\n" +
			"surfaces next's empty-queue diagnostic: empty next still exits 0 with the full\n" +
			"key block. dangling is the edge-count of same-scope depends targets missing a\n" +
			"project in this scope (matches doctor's per-edge depends_dangling findings).\n" +
			"integrity is ok or issues for the ambient scope only (parse_error rows,\n" +
			"duplicate_id, equal_order, archive layout drift) — not soft doctor classes or\n" +
			"depended-on scopes from the next closure.\n" +
			"\n" +
			"To change a project's status, use `pj mark <id> <status>`.",
		Args: usageArgs(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			return runStatus(app, c, scope)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "scope to pulse (defaults to ambient; wins over ambient)")
	return cmd
}

func runStatus(app *App, c *cobra.Command, scopeFlag string) error {
	e, err := app.openEngine(c)
	if err != nil {
		return err
	}
	defer e.close()

	resolved, err := e.resolveAmbient(scopeFlag)
	if err != nil {
		return err
	}
	scope := resolved.Name
	dir := resolved.Entry.Dir
	absDir, err := absPath(dir)
	if err != nil {
		return err
	}

	// Same closure + gate as pj next for cross-scope depends freshness.
	res, targets, err := e.reconcileClosure(c, scope, dir)
	if err != nil {
		return err
	}
	gate, err := e.buildGate(res, targets)
	if err != nil {
		return err
	}

	rows, err := e.db.ScopeProjects(scope)
	if err != nil {
		return err
	}
	schema := res.Schema(scope)
	lens := e.reg.Lens[scope]

	root, hasRoot := gitRootFor(dir)
	mode := statusMode(schema, res.ConfigErrs[scope] != nil, hasRoot)

	pulse := map[string]string{
		"scope":    scope,
		"dir":      absDir,
		"resolved": resolved.Source,
		"mode":     mode,
		"lens":     strings.Join(lens, " "),
	}

	var (
		total, todo, review, inProgress, blocked, draft, backlog int
		done, cancelled                                          int
		claimed, blockedIDs                                      []*index.Project
	)
	for _, p := range rows {
		if p.ParseError {
			continue
		}
		// Full-scope total: every parseable project, including archive/ and backlog.
		total++
		// Terminal tallies ignore lens and include archive/.
		switch p.Status {
		case status.Done:
			done++
		case status.Cancelled:
			cancelled++
		}
		if !workingBoardMember(p, lens) {
			continue
		}
		switch p.Status {
		case status.Todo:
			todo++
		case status.Review:
			review++
		case status.InProgress:
			inProgress++
			claimed = append(claimed, p)
		case status.Blocked:
			blocked++
			blockedIDs = append(blockedIDs, p)
		case status.Draft:
			draft++
		case status.Backlog:
			backlog++
		}
	}
	sortProjects(claimed)
	sortProjects(blockedIDs)

	// Same next walk as unclaimed pj next; empty next is still a pulse value.
	sel := selectNext(gate, rows, lens, false)
	nextID := ""
	if sel.Chosen != nil {
		nextID = sel.Chosen.ID
	}

	dangling, err := countSameScopeDangling(e, scope)
	if err != nil {
		return err
	}
	integrity, err := ambientIntegrity(e, scope, schema)
	if err != nil {
		return err
	}
	uncommitted := 0
	if mode == scopeadmin.ModeRepoDriven {
		uncommitted = countAllowlistedDirty(c.Context(), dir, root, hasRoot)
	}

	pulse["total"] = strconv.Itoa(total)
	pulse["todo"] = strconv.Itoa(todo)
	pulse["review"] = strconv.Itoa(review)
	pulse["in-progress"] = strconv.Itoa(inProgress)
	pulse["blocked"] = strconv.Itoa(blocked)
	pulse["draft"] = strconv.Itoa(draft)
	pulse["backlog"] = strconv.Itoa(backlog)
	pulse["done"] = strconv.Itoa(done)
	pulse["cancelled"] = strconv.Itoa(cancelled)
	pulse["next"] = nextID
	pulse["claimed"] = joinIDs(claimed)
	pulse["blocked_ids"] = joinIDs(blockedIDs)
	pulse["dangling"] = strconv.Itoa(dangling)
	pulse["integrity"] = integrity
	pulse["uncommitted"] = strconv.Itoa(uncommitted)

	sel.writeDiagnostics(c)
	for _, key := range statusKeys {
		stdoutln(c, fmt.Sprintf("%-*s\t%s", statusKeyWidth, key, pulse[key]))
	}
	return nil
}

// workingBoardMember: non-quarantined, non-archive, passes lens (untagged visible).
func workingBoardMember(p *index.Project, lens []string) bool {
	if p.ParseError || p.Archived {
		return false
	}
	return passesLens(p, lens)
}

// statusMode: unusable schema → plain-files (never guess repo-driven).
func statusMode(schema *scopeconfig.Schema, configUnusable bool, hasRoot bool) string {
	if configUnusable || schema == nil {
		return scopeadmin.ModePlainFiles
	}
	return scopeadmin.DeriveMode(schema.AutoCommit, hasRoot)
}

func countSameScopeDangling(e *engine, scope string) (int, error) {
	edges, err := e.db.AllEdges()
	if err != nil {
		return 0, err
	}
	rows, err := e.db.ScopeProjects(scope)
	if err != nil {
		return 0, err
	}
	have := map[string]bool{}
	for _, p := range rows {
		have[p.ID] = true
	}
	n := 0
	for _, ed := range edges {
		if ed.Kind != index.EdgeDepends || ed.FromScope != scope {
			continue
		}
		if ed.ToScope != scope {
			continue
		}
		if !have[ed.ToID] {
			n++
		}
	}
	return n, nil
}

// ambientIntegrity: parse_error/duplicate/equal_order/archive drift → issues.
func ambientIntegrity(e *engine, scope string, schema *scopeconfig.Schema) (string, error) {
	scopes := []string{scope}
	n, err := e.db.ParseErrorCount(scopes)
	if err != nil {
		return "", err
	}
	if n > 0 {
		return "issues", nil
	}
	dups, err := e.db.DuplicateIDs(scopes)
	if err != nil {
		return "", err
	}
	if len(dups) > 0 {
		return "issues", nil
	}
	eq, err := e.db.EqualOrders(scopes)
	if err != nil {
		return "", err
	}
	if len(eq) > 0 {
		return "issues", nil
	}
	rows, err := e.db.ScopeProjects(scope)
	if err != nil {
		return "", err
	}
	custom := schemaCustom(schema)
	for _, p := range rows {
		if p.ParseError {
			continue
		}
		terminal := status.IsTerminal(p.Status, custom)
		if (p.Archived && !terminal) || (!p.Archived && terminal) {
			return "issues", nil
		}
	}
	return "ok", nil
}

func joinIDs(rows []*index.Project) string {
	if len(rows) == 0 {
		return ""
	}
	ids := make([]string, len(rows))
	for i, p := range rows {
		ids[i] = p.ID
	}
	return strings.Join(ids, " ")
}
