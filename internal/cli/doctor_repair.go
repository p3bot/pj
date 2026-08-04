package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/p3bot/pj/internal/git"
	"github.com/p3bot/pj/internal/gitstate"
	"github.com/p3bot/pj/internal/index"
	"github.com/p3bot/pj/internal/reconcile"
	"github.com/p3bot/pj/internal/repair"
	"github.com/p3bot/pj/internal/rewrite"
	"github.com/p3bot/pj/internal/scopeconfig"
	"github.com/p3bot/pj/internal/selfcommit"
	"github.com/p3bot/pj/internal/status"
	"github.com/p3bot/pj/internal/token"
)

// repairTarget holds values resolved once under flock for the whole repair run.
type repairTarget struct {
	scope      string
	dir        string
	schema     *scopeconfig.Schema
	autoCommit bool
	root       string
	hasRoot    bool
}

func (e *engine) runRepairs(c *cobra.Command, scopes []string, f doctorFlags) error {
	for _, scope := range scopes {
		entry, ok := e.reg.Scopes[scope]
		if !ok {
			continue
		}
		if err := e.repairScope(c, scope, entry.Dir, f); err != nil {
			return err
		}
	}
	return nil
}

// repairScope acquires scope flock (+ git-root lock for auto-commit) across reconcile and batches.
func (e *engine) repairScope(c *cobra.Command, scope, dir string, f doctorFlags) error {
	// Stat before flock: flock creates a file and would abort --all on one unmounted drive.
	if _, err := os.Stat(dir); err != nil {
		stderrln(c, fmt.Sprintf("skipping %s: dir unreachable", scope))
		return nil
	}
	lock, err := acquireScopeLock(dir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	res, err := e.reconcileResult(single(scope, dir))
	if err != nil {
		return err
	}
	t, err := e.repairPreflight(c, scope, dir, res, f)
	if err != nil || t == nil {
		return err
	}

	if t.autoCommit && t.hasRoot {
		gitLock, err := gitstate.AcquireCommitLock(e.app.StateDir, t.root)
		if err != nil {
			return err
		}
		defer func() { _ = gitLock.Release() }()
	}
	return e.runRepairBatches(c, t, f)
}

// runRepairBatches is the locks-held core (doctor acquires; sync already holds both).
func (e *engine) runRepairBatches(c *cobra.Command, t *repairTarget, f doctorFlags) error {
	if f.repair {
		if err := e.repairArchive(c, t, false); err != nil {
			return err
		}
		if err := e.repairCollisions(c, t); err != nil {
			return err
		}
		// Second layout pass: land moves deferred until collisions got distinct basenames.
		if err := e.repairArchive(c, t, true); err != nil {
			return err
		}
		if err := e.repairEqualOrder(c, t); err != nil {
			return err
		}
	}
	if f.reSpaceOrder {
		if err := e.repairLongOrder(c, t); err != nil {
			return err
		}
	}
	return nil
}

// repairPreflight: nil,nil is a skip; mid-rebase hard-refuses ambient, skips under --all.
func (e *engine) repairPreflight(c *cobra.Command, scope, dir string, res *reconcile.Result, f doctorFlags) (*repairTarget, error) {
	if res.Unreachable[scope] {
		stderrln(c, fmt.Sprintf("skipping %s: dir unreachable", scope))
		return nil, nil
	}
	if pjName, err := scopeconfig.ReadName(e.app.Ctx, dir); err == nil && pjName != scope {
		stderrln(c, token.Line(token.NameDrift, fmt.Sprintf("skipping %s: registry key %q but pj.cue name is %q — recover with pj scope forget/import", scope, scope, pjName)))
		return nil, nil
	}
	if _, bad := res.ConfigErrs[scope]; bad {
		stderrln(c, token.Line(token.ConfigUnparseable, fmt.Sprintf("skipping %s: fix pj.cue before repairing", scope)))
		return nil, nil
	}

	schema := res.Schema(scope)
	t := &repairTarget{scope: scope, dir: dir, schema: schema, autoCommit: schemaAutoCommit(schema)}
	t.root, t.hasRoot = gitRootFor(dir)
	if t.autoCommit && t.hasRoot && git.MidRebase(c.Context(), t.root) {
		if !f.all {
			return nil, midRebaseRefusal(c, scope, t.root)
		}
		stderrln(c, fmt.Sprintf("skipping %s: git-root %s is mid-rebase — resolve then re-run", scope, t.root))
		return nil, nil
	}
	return t, nil
}

// repairCollisions: edge_verify is read after all collisions so referrer ids are post-repair.
func (e *engine) repairCollisions(c *cobra.Command, t *repairTarget) error {
	dups, err := e.db.DuplicateIDs([]string{t.scope})
	if err != nil {
		return err
	}
	if len(dups) == 0 {
		return nil
	}
	rows, err := e.db.ScopeProjects(t.scope)
	if err != nil {
		return err
	}
	occupied := shortIDPaths(rows)
	byPath := map[string]*index.Project{}
	for _, p := range rows {
		byPath[p.Path] = p
	}

	var repaired []string
	for _, col := range dups {
		members := rowsForPaths(byPath, col.Members)
		mid, err := repair.InterruptedMove(t.dir, toRepairRows(members))
		if err != nil {
			return err
		}
		if mid {
			stderrln(c, fmt.Sprintf("skipping %s: unfinished archive-layout move, not a collision — re-run pj doctor --repair to complete it", col.Key))
			continue
		}
		if anyParseError(members) {
			stderrln(c, token.Line(token.ParseError, fmt.Sprintf("%s: collision includes a quarantined file — fix its frontmatter before repair", col.Key)))
			continue
		}
		ops, renames, err := repair.DuplicateID(t.scope, toRepairRows(members), occupied)
		if err != nil {
			return err
		}
		if err := e.applyRepairBatch(c, t, ops, collisionMessage(renames)); err != nil {
			return err
		}
		for _, r := range renames {
			stdoutln(c, fmt.Sprintf("repaired duplicate id: %s -> %s (%s)", r.OldID, r.NewID, r.NewPath))
		}
		repaired = append(repaired, col.Key)
	}
	return e.reportEdgeVerify(c, repaired)
}

// reportEdgeVerify: only actually-repaired ids; kept side still holds the collided id.
func (e *engine) reportEdgeVerify(c *cobra.Command, collidedIDs []string) error {
	for _, collidedID := range collidedIDs {
		inbound, err := e.db.EdgesByTarget(collidedID)
		if err != nil {
			return err
		}
		for _, ed := range inbound {
			stdoutln(c, token.Line(token.EdgeVerify, fmt.Sprintf("%s %s %s — target was collision-repaired, verify this reference", ed.FromID, ed.Kind, collidedID)))
		}
	}
	return nil
}

func (e *engine) repairEqualOrder(c *cobra.Command, t *repairTarget) error {
	rows, err := e.db.ScopeProjects(t.scope)
	if err != nil {
		return err
	}
	ops, err := repair.EqualOrder(toRepairRows(rows))
	if err != nil {
		return err
	}
	if len(ops) == 0 {
		return nil
	}
	if err := e.applyRepairBatch(c, t, ops, "pj: repair equal order"); err != nil {
		return err
	}
	stdoutln(c, fmt.Sprintf("re-spaced %d equal order key(s) in %s", len(ops), t.scope))
	return nil
}

// repairArchive defers ids still in genuine collisions (shared basename would clobber).
func (e *engine) repairArchive(c *cobra.Command, t *repairTarget, reportDeferred bool) error {
	rows, err := e.db.ScopeProjects(t.scope)
	if err != nil {
		return err
	}
	collided, err := e.genuineCollisionIDs(t.scope, t.dir)
	if err != nil {
		return err
	}
	deferred := map[string]bool{}
	custom := schemaCustom(t.schema)
	for _, p := range rows {
		if p.ParseError {
			continue
		}
		if collided[p.ID] {
			if reportDeferred && !deferred[p.ID] {
				deferred[p.ID] = true
				stderrln(c, fmt.Sprintf("archive layout for %s left as is: its id is still duplicated — repair the collision first", p.ID))
			}
			continue
		}
		terminal := status.IsTerminal(p.Status, custom)
		if p.Archived == terminal {
			continue
		}
		op, err := repair.ArchiveMove(t.dir, toRepairRow(p), terminal)
		if err != nil {
			return err
		}
		msg := fmt.Sprintf("pj: repair archive layout %s", p.ID)
		if err := e.applyRepairBatch(c, t, []rewrite.Op{op}, msg); err != nil {
			return err
		}
		stdoutln(c, fmt.Sprintf("moved archive layout: %s -> %s", p.ID, op.NewPath))
	}
	return nil
}

func (e *engine) repairLongOrder(c *cobra.Command, t *repairTarget) error {
	rows, err := e.db.ScopeProjects(t.scope)
	if err != nil {
		return err
	}
	ops, err := repair.LongOrder(toRepairRows(rows))
	if err != nil {
		return err
	}
	if len(ops) == 0 {
		return nil
	}
	if err := e.applyRepairBatch(c, t, ops, "pj: re-space order"); err != nil {
		return err
	}
	stdoutln(c, fmt.Sprintf("re-spaced %d over-long order key(s) in %s", len(ops), t.scope))
	return nil
}

// applyRepairBatch uses CommitPathsCore — callers already hold the git-root lock (re-acquire deadlocks).
func (e *engine) applyRepairBatch(c *cobra.Command, t *repairTarget, ops []rewrite.Op, message string) error {
	if len(ops) == 0 {
		return nil
	}
	touched, err := rewrite.Apply(ops)
	if err != nil {
		return err
	}
	if err := e.rec.SyncPaths(t.scope, touched); err != nil {
		return err
	}
	if !t.autoCommit {
		return nil
	}
	if !t.hasRoot {
		stderrln(c, token.Line(token.SyncDisabled, fmt.Sprintf("%s: no git repository — repaired files written but not committed", t.scope)))
		return nil
	}
	return selfcommit.CommitPathsCore(c.Context(), selfcommit.BatchRequest{
		StateDir: e.app.StateDir, GitRoot: t.root, Message: message, Paths: touched,
	})
}

func midRebaseRefusal(c *cobra.Command, scope, root string) error {
	where := "the conflicted file"
	if files := git.UnmergedFiles(c.Context(), root); len(files) > 0 {
		where = strings.Join(files, ", ")
	}
	return fmt.Errorf("%s is mid-sync-conflict in shared repo %s — resolve %s then run pj sync before repairing", scope, root, where)
}

func collisionMessage(renames []repair.Rename) string {
	newIDs := make([]string, len(renames))
	for i, r := range renames {
		newIDs[i] = r.NewID
	}
	return fmt.Sprintf("pj: repair duplicate id %s -> %s", renames[0].OldID, strings.Join(newIDs, ", "))
}

func toRepairRows(rows []*index.Project) []repair.Row {
	out := make([]repair.Row, len(rows))
	for i, p := range rows {
		out[i] = toRepairRow(p)
	}
	return out
}

func toRepairRow(p *index.Project) repair.Row {
	return repair.Row{Path: p.Path, FullID: p.ID, ShortID: p.ShortID, OrderKey: p.OrderKey, ParseError: p.ParseError}
}

// shortIDPaths: collided short-id maps to lexicographically smallest path (dirent-stable).
func shortIDPaths(rows []*index.Project) map[string]string {
	out := make(map[string]string, len(rows))
	for _, p := range rows {
		if p.ShortID == "" {
			continue
		}
		if prev, ok := out[p.ShortID]; !ok || p.Path < prev {
			out[p.ShortID] = p.Path
		}
	}
	return out
}

// genuineCollisionIDs excludes interrupted archive moves (byte-identical both-present window).
func (e *engine) genuineCollisionIDs(scope, dir string) (map[string]bool, error) {
	dups, err := e.db.DuplicateIDs([]string{scope})
	if err != nil || len(dups) == 0 {
		return nil, err
	}
	rows, err := e.db.ScopeProjects(scope)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]*index.Project, len(rows))
	for _, p := range rows {
		byPath[p.Path] = p
	}
	out := map[string]bool{}
	for _, col := range dups {
		mid, err := repair.InterruptedMove(dir, toRepairRows(rowsForPaths(byPath, col.Members)))
		if err != nil {
			return nil, err
		}
		if !mid {
			out[col.Key] = true
		}
	}
	return out, nil
}

func rowsForPaths(byPath map[string]*index.Project, paths []string) []*index.Project {
	var out []*index.Project
	for _, p := range paths {
		if row, ok := byPath[p]; ok {
			out = append(out, row)
		}
	}
	return out
}

func anyParseError(rows []*index.Project) bool {
	for _, p := range rows {
		if p.ParseError {
			return true
		}
	}
	return false
}
