package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/start-cli/pj/internal/frontmatter"
	"github.com/start-cli/pj/internal/git"
	"github.com/start-cli/pj/internal/gitroot"
	"github.com/start-cli/pj/internal/gitstate"
	"github.com/start-cli/pj/internal/id"
	"github.com/start-cli/pj/internal/index"
	"github.com/start-cli/pj/internal/order"
	"github.com/start-cli/pj/internal/reconcile"
	"github.com/start-cli/pj/internal/repair"
	"github.com/start-cli/pj/internal/resolve"
	"github.com/start-cli/pj/internal/scopeconfig"
	"github.com/start-cli/pj/internal/status"
	"github.com/start-cli/pj/internal/token"
)

const staleInProgress = 72 * time.Hour

func (e *engine) diagnose(c *cobra.Command, scopes []string, res *reconcile.Result) ([]string, error) {
	d, err := e.newDiagnoser(c, res)
	if err != nil {
		return nil, err
	}
	for _, scope := range scopes {
		if err := d.scope(scope); err != nil {
			return nil, err
		}
	}
	return d.lines, nil
}

type diagnoser struct {
	e         *engine
	c         *cobra.Command
	res       *reconcile.Result
	now       time.Time
	hasRow    map[string]bool
	rowByID   map[string]*index.Project
	edges     []index.Edge
	schemaFor map[string]*scopeconfig.Schema
	seenRoot  map[string]bool
	// registered: cross-scope targets unresolvable when their scope is not here.
	registered map[string]bool
	// cfgReported dedupes config_unparseable across diagnosed scope and sibling preflight.
	cfgReported map[string]bool
	lines       []string
}

func (e *engine) newDiagnoser(c *cobra.Command, res *reconcile.Result) (*diagnoser, error) {
	all, err := e.db.AllProjects()
	if err != nil {
		return nil, err
	}
	edges, err := e.db.AllEdges()
	if err != nil {
		return nil, err
	}
	d := &diagnoser{
		e: e, c: c, res: res, now: time.Now(),
		hasRow: map[string]bool{}, rowByID: map[string]*index.Project{},
		edges: edges, schemaFor: map[string]*scopeconfig.Schema{}, seenRoot: map[string]bool{},
		registered: e.registeredSet(), cfgReported: map[string]bool{},
	}
	for _, p := range all {
		d.hasRow[p.ID] = true
		if _, ok := d.rowByID[p.ID]; !ok {
			d.rowByID[p.ID] = p
		}
	}
	return d, nil
}

func (d *diagnoser) add(line string) { d.lines = append(d.lines, line) }

// scope short-circuits project checks on unreachable/drifted scopes (still emits tokens).
func (d *diagnoser) scope(scope string) error {
	entry, ok := d.e.reg.Scopes[scope]
	if !ok {
		return nil
	}
	dir := entry.Dir

	if d.res.Unreachable[scope] {
		d.add(token.Line(token.UnreachableScope, fmt.Sprintf("%s: dir %s could not be read — rows left in place", scope, dir)))
		return nil
	}
	if pjName, err := scopeconfig.ReadName(d.e.app.Ctx, dir); err == nil && pjName != scope {
		d.add(resolve.DriftLine(scope, pjName, dir, resolve.SuggestCodeRoot(dir, entry.Root)))
		return nil
	}

	schema := d.res.Schema(scope)
	if cfgErr, ok := d.res.ConfigErrs[scope]; ok {
		d.configUnparseable(scope, cfgErr)
	}

	rows, err := d.e.db.ScopeProjects(scope)
	if err != nil {
		return err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })

	if err := d.collisions(scope); err != nil {
		return err
	}
	autoCommit, autoCommitKnown := d.autoCommitFor(scope, schema)
	d.perRow(dir, rows, schema, autoCommit, autoCommitKnown)
	d.edgeClasses(scope)
	if err := d.repoHealth(scope, dir, schema); err != nil {
		return err
	}
	d.residue(scope, dir)
	return nil
}

func (d *diagnoser) collisions(scope string) error {
	dups, err := d.e.db.DuplicateIDs([]string{scope})
	if err != nil {
		return err
	}
	for _, col := range dups {
		d.add(token.Line(token.DuplicateID, fmt.Sprintf("%s claimed by %s — run pj doctor --repair", col.Key, strings.Join(col.Members, ", "))))
	}
	eq, err := d.e.db.EqualOrders([]string{scope})
	if err != nil {
		return err
	}
	for _, col := range eq {
		d.add(token.Line(token.EqualOrder, fmt.Sprintf("%s share order %q: %s — run pj doctor --repair", scope, col.Key, strings.Join(col.Members, ", "))))
	}
	return nil
}

func (d *diagnoser) perRow(dir string, rows []*index.Project, schema *scopeconfig.Schema, autoCommit, autoCommitKnown bool) {
	custom := schemaCustom(schema)
	for _, p := range rows {
		if p.ParseError {
			d.add(token.Line(token.ParseError, fmt.Sprintf("%s: %s (%s)", p.ID, p.ParseMsg, p.Path)))
			continue
		}
		// Invalid order is not quarantined (still sorts); exclusive with order_long.
		switch {
		case p.OrderKey == "":
			// Missing vs explicit "" are indistinguishable after parse.
			d.add(token.Line(token.SchemaError, fmt.Sprintf("%s has a missing or empty order key (%s) — set a quoted order key, or run pj reorder", p.ID, p.Path)))
		case !order.Valid(p.OrderKey):
			d.add(token.Line(token.SchemaError, fmt.Sprintf("%s has an invalid order key %q (%s) — outside the closed order grammar; set a quoted valid key, or run pj reorder", p.ID, p.OrderKey, p.Path)))
		case len(p.OrderKey) > repair.OrderLongThreshold:
			d.add(token.Line(token.OrderLong, fmt.Sprintf("%s order key is %d chars (%s) — run pj doctor --re-space-order", p.ID, len(p.OrderKey), p.Path)))
		}
		terminal := status.IsTerminal(p.Status, custom)
		switch {
		case p.Archived && !terminal:
			d.add(token.Line(token.ArchiveNonTerminal, fmt.Sprintf("%s is %s under archive/ (%s)", p.ID, p.Status, p.Path)))
		case !p.Archived && terminal:
			d.add(token.Line(token.ArchiveTerminalAtRoot, fmt.Sprintf("%s is %s at dir root (%s)", p.ID, p.Status, p.Path)))
		}
		if len(p.StatusConflict) > 0 {
			d.add(d.statusConflictLine(p, dir, autoCommit, autoCommitKnown))
		}
		if p.Status == status.InProgress && p.MtimeNS > 0 && d.now.Sub(time.Unix(0, p.MtimeNS)) > staleInProgress {
			age := d.now.Sub(time.Unix(0, p.MtimeNS)).Round(time.Hour)
			d.add(token.Line(token.StaleInProgress, fmt.Sprintf("%s has been in-progress for %s (%s) — inspect; maybe reopen to todo", p.ID, age, p.Path)))
		}
		if p.SchemaError {
			d.add(token.Line(token.SchemaError, fmt.Sprintf("%s has a depends/related entry that is not a legal full project id (%s)", p.ID, p.Path)))
		}
		d.frontmatterChecks(p, schema)
	}
}

// statusConflictLine: mid-rebase tail only for known autoCommit; unknown autoCommit omits both tails.
func (d *diagnoser) statusConflictLine(p *index.Project, dir string, autoCommit bool, autoCommitKnown bool) string {
	disputed := strings.Join(p.StatusConflict, " vs ")
	if !autoCommitKnown {
		return token.Line(token.StatusConflict, fmt.Sprintf("%s disputes %s (%s) — set status and clear status_conflict", p.ID, disputed, p.Path))
	}
	// Skip git lookup when not auto-commit — answer cannot change the line.
	if autoCommit {
		if root, ok := gitRootFor(dir); ok && git.MidRebase(d.c.Context(), root) {
			return token.Line(token.StatusConflict, fmt.Sprintf("%s disputes %s (%s) — resolve in file, then pj sync", p.ID, disputed, p.Path))
		}
	}
	return token.Line(token.StatusConflict, fmt.Sprintf("%s disputes %s (%s) — stale residue: set status and clear status_conflict", p.ID, disputed, p.Path))
}

func (d *diagnoser) frontmatterChecks(p *index.Project, schema *scopeconfig.Schema) {
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return
	}
	interior, _, present := frontmatter.Split(data)
	if !present {
		return
	}
	m, err := frontmatter.Parse(interior)
	if err != nil {
		return
	}

	// Indexer accepts any short-id tail; allowlist requires a valid slug — doctor bridges the gap.
	base := filepath.Base(p.Path)
	switch {
	case m.ID == "" || !strings.HasPrefix(base, m.ID+"-") && strings.TrimSuffix(base, ".md") != m.ID:
		d.add(fmt.Sprintf("filename/id mismatch: %s does not begin with its frontmatter id %q", base, m.ID))
	case !id.IsFullProjectID(m.ID):
		d.add(fmt.Sprintf("filename/id mismatch: %s has a non-project-shaped id %q", base, m.ID))
	case !looksLikeProjectFile(base):
		d.add(fmt.Sprintf("filename/id mismatch: %s is not a project file shape (<id>-<slug>.md) — rename it to a valid slug", base))
	}
	// Token-less prose: must not open with "word:" (closed-token shape).
	if !validRFC3339(m.Created) {
		d.add(fmt.Sprintf("created value missing or not RFC3339 in %s: %q (%s) — fix so the id-collision repair order stays strong", p.ID, m.Created, p.Path))
	}

	if contains(m.Depends, m.ID) {
		d.add(token.Line(token.DependsSelf, fmt.Sprintf("%s lists its own id in depends — remove the self-edge (%s)", p.ID, p.Path)))
	}
	if contains(m.Related, m.ID) {
		d.add(token.Line(token.SchemaWarn, fmt.Sprintf("%s lists its own id in related (%s)", p.ID, p.Path)))
	}
	for _, kind := range []struct {
		name string
		list []string
	}{{"depends", m.Depends}, {"related", m.Related}, {"tags", m.Tags}, {"links", m.Links}} {
		if dup := firstDuplicate(kind.list); dup != "" {
			d.add(token.Line(token.SchemaWarn, fmt.Sprintf("%s has a duplicate %s entry %q (%s)", p.ID, kind.name, dup, p.Path)))
		}
	}
	for _, link := range m.Links {
		if id.IsFullProjectID(link) {
			d.add(token.Line(token.SchemaWarn, fmt.Sprintf("%s links entry %q is project-id-shaped — use related/depends for project ids (%s)", p.ID, link, p.Path)))
		}
	}
	if schema != nil {
		if m.Status != "" && !schema.StatusKnown(m.Status) {
			d.add(token.Line(token.SchemaError, fmt.Sprintf("%s has unknown status %q (%s)", p.ID, m.Status, p.Path)))
		}
		for _, f := range m.Custom {
			field, declared := schema.Fields[f.Key]
			if !declared {
				d.add(token.Line(token.SchemaWarn, fmt.Sprintf("%s has undeclared frontmatter key %q (%s)", p.ID, f.Key, p.Path)))
				continue
			}
			if reason := fieldTypeError(field, f.Value); reason != "" {
				d.add(token.Line(token.SchemaError, fmt.Sprintf("%s field %q %s (%s)", p.ID, f.Key, reason, p.Path)))
				continue
			}
			// Duplicate check after type check so non-lists report type errors only.
			if field.Type == scopeconfig.FieldStrings {
				if dup := firstDuplicate(stringElems(f.Value)); dup != "" {
					d.add(token.Line(token.SchemaWarn, fmt.Sprintf("%s has a duplicate %s entry %q (%s)", p.ID, f.Key, dup, p.Path)))
				}
			}
		}
		if len(schema.KnownTags) > 0 {
			known := sliceSet(schema.KnownTags)
			for _, tag := range m.Tags {
				if !known[tag] {
					d.add(token.Line(token.SchemaWarn, fmt.Sprintf("%s tag %q is not in knownTags — likely a typo (%s)", p.ID, tag, p.Path)))
				}
			}
		}
	}
}

// fieldTypeError errs toward silence on YAML-decode ambiguity.
func fieldTypeError(field scopeconfig.Field, value any) string {
	switch field.Type {
	case scopeconfig.FieldStrings:
		list, ok := value.([]any)
		if !ok {
			return "should be a list of strings"
		}
		// Non-string elements before enum so mixed lists report type, not enum.
		for _, e := range list {
			if _, ok := e.(string); !ok {
				return fmt.Sprintf("has a non-string entry (%v)", e)
			}
		}
		if len(field.Values) > 0 {
			allowed := sliceSet(field.Values)
			for _, e := range list {
				if s, ok := e.(string); ok && !allowed[s] {
					return fmt.Sprintf("has value %q outside its declared values", s)
				}
			}
		}
	case scopeconfig.FieldString:
		s, ok := value.(string)
		if !ok {
			return "should be a string"
		}
		if len(field.Values) > 0 && !sliceSet(field.Values)[s] {
			return fmt.Sprintf("value %q is outside its declared values", s)
		}
	case scopeconfig.FieldInt:
		if !isIntKind(value) {
			return "should be an integer"
		}
	case scopeconfig.FieldBool:
		if _, ok := value.(bool); !ok {
			return "should be a bool"
		}
	}
	return ""
}

func isIntKind(v any) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func (d *diagnoser) edgeClasses(scope string) {
	for _, id := range d.cycleNodes(scope) {
		d.add(token.Line(token.DependsCycle, fmt.Sprintf("%s is in a depends cycle — fix the edges", id)))
	}
	for _, ed := range d.edges {
		if ed.FromScope != scope || ed.FromID == ed.ToID {
			continue // self-edges: frontmatter model owns depends_self
		}
		if ed.Kind == index.EdgeRelated {
			if !d.hasRow[ed.ToID] {
				d.add(token.Line(token.RelatedUnresolvable, fmt.Sprintf("%s related target %s is not resolvable here", ed.FromID, ed.ToID)))
			}
			continue
		}
		if ed.ToScope == scope {
			if !d.hasRow[ed.ToID] {
				d.add(token.Line(token.DependsDangling, fmt.Sprintf("%s depends on %s which has no project in this scope", ed.FromID, ed.ToID)))
			}
		} else if !d.registered[ed.ToScope] || !d.hasRow[ed.ToID] {
			d.add(token.Line(token.DependsUnresolvable, fmt.Sprintf("%s depends on cross-scope %s which is not resolvable here", ed.FromID, ed.ToID)))
		}
		if d.dependsOnAbandoned(ed.ToID, ed.ToScope) {
			d.add(token.Line(token.DependsOnCancelled, fmt.Sprintf("%s depends on %s which is cancelled/abandoned — decide if it still applies", ed.FromID, ed.ToID)))
		}
	}
}

// dependsOnAbandoned: cancelled/custom-done only; built-in done is normal completion.
func (d *diagnoser) dependsOnAbandoned(toID, toScope string) bool {
	row, ok := d.rowByID[toID]
	if !ok {
		return false
	}
	if row.Status == status.Cancelled {
		return true
	}
	if status.IsBuiltin(row.Status) {
		return false
	}
	schema := d.targetSchema(toScope)
	return schema != nil && schema.StatusTerminal(row.Status)
}

func (d *diagnoser) targetSchema(scope string) *scopeconfig.Schema {
	if s, ok := d.schemaFor[scope]; ok {
		return s
	}
	var s *scopeconfig.Schema
	if entry, ok := d.e.reg.Scopes[scope]; ok {
		s = d.e.rec.SchemaCached(scope, entry.Dir)
	}
	d.schemaFor[scope] = s
	return s
}

// cycleNodes excludes self-edges (depends_self owns those); walks machine-wide for cross-scope.
func (d *diagnoser) cycleNodes(scope string) []string {
	adj := map[string][]string{}
	for _, ed := range d.edges {
		if ed.Kind == index.EdgeDepends && ed.FromID != ed.ToID {
			adj[ed.FromID] = append(adj[ed.FromID], ed.ToID)
		}
	}
	var out []string
	for _, p := range d.scopeIDs(scope) {
		if reaches(p, p, adj, map[string]bool{}) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func (d *diagnoser) scopeIDs(scope string) []string {
	var out []string
	for id, p := range d.rowByID {
		if p.Scope == scope {
			out = append(out, id)
		}
	}
	return out
}

// autoCommitFor: unparseable pj.cue makes autoCommit unknown, not false.
func (d *diagnoser) autoCommitFor(scope string, schema *scopeconfig.Schema) (value bool, known bool) {
	if _, unusable := d.res.ConfigErrs[scope]; unusable {
		return false, false
	}
	return schemaAutoCommit(schema), true
}

func (d *diagnoser) repoHealth(scope, dir string, schema *scopeconfig.Schema) error {
	root, hasRoot := gitRootFor(dir)

	if hasRoot && !d.seenRoot[root] {
		d.seenRoot[root] = true
		d.rootPreflight(root)
	}

	// Unknown autoCommit: skip classes below (config_unparseable already rode).
	autoCommit, known := d.autoCommitFor(scope, schema)
	if !known {
		return nil
	}

	switch {
	case autoCommit:
		if !hasRoot || !git.HasUpstream(d.c.Context(), root) {
			d.add(token.Line(token.SyncDisabled, fmt.Sprintf("%s: no git repository with an upstream — set one up, then pj sync", scope)))
		}
		if hasRoot {
			if detail, ok := gitstate.ReadLastPushError(d.e.app.StateDir, root); ok {
				d.add(token.Line(token.LastPushError, fmt.Sprintf("%s: last push failed (%s) — fix the remote/auth, then pj sync", scope, detail)))
			}
		}
	case hasRoot: // repo-driven
		if n := countAllowlistedDirty(d.c.Context(), dir, root, hasRoot); n > 0 {
			d.add(token.Line(token.Uncommitted, fmt.Sprintf("%s: %d allowlisted path(s) under %s uncommitted — commit with the host repo", scope, n, dir)))
		}
	}
	return nil
}

// rootPreflight scans all siblings sharing root (not just diagnosed scopes) for mismatch/unusable.
func (d *diagnoser) rootPreflight(root string) {
	seenTrue, seenFalse := false, false
	for _, name := range d.siblingScopes(root) {
		schema, cfgErr := d.e.rec.SchemaOrError(name, d.e.reg.Scopes[name].Dir)
		if cfgErr != nil {
			d.configUnparseable(name, cfgErr)
			continue
		}
		if schema == nil {
			continue
		}
		if schema.AutoCommit {
			seenTrue = true
		} else {
			seenFalse = true
		}
	}
	if seenTrue && seenFalse {
		d.add(token.Line(token.AutoCommitMismatch, fmt.Sprintf("scopes sharing git-root %s disagree on autoCommit — split the divergent scope into its own repo", root)))
	}
}

func (d *diagnoser) siblingScopes(root string) []string {
	var out []string
	for name, entry := range d.e.reg.Scopes {
		if sgr, ok := gitroot.RepoRoot(entry.Dir); ok && sgr == root {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// configUnparseable: at most once per scope (diagnosed path and sibling preflight).
func (d *diagnoser) configUnparseable(scope string, cfgErr *scopeconfig.ConfigError) {
	if d.cfgReported[scope] {
		return
	}
	d.cfgReported[scope] = true
	d.add(token.Line(token.ConfigUnparseable, fmt.Sprintf("%s (%s): %s — fix pj.cue", scope, cfgErr.Dir, cfgErr.Reason)))
}

func (d *diagnoser) residue(scope, dir string) {
	_ = filepath.WalkDir(dir, func(path string, ent os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ent.IsDir() {
			if ent.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		base := ent.Name()
		if base == scopeLockName {
			return nil
		}
		if isAllowlistedScopeFile(path, dir) {
			return nil
		}
		d.add(token.Line(token.NonAllowlist, fmt.Sprintf("%s: %s is under the scope dir but outside the allowlist — move or remove it", scope, path)))
		return nil
	})
}

func validRFC3339(s string) bool {
	if s == "" {
		return false
	}
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}

func contains(list []string, v string) bool {
	for _, e := range list {
		if e == v {
			return true
		}
	}
	return false
}

func stringElems(value any) []string {
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func firstDuplicate(list []string) string {
	seen := map[string]bool{}
	for _, e := range list {
		if seen[e] {
			return e
		}
		seen[e] = true
	}
	return ""
}

func sliceSet(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, e := range list {
		out[e] = true
	}
	return out
}

// reaches does not mark the start visited, so a return to start is a cycle.
func reaches(node, target string, adj map[string][]string, visited map[string]bool) bool {
	for _, next := range adj[node] {
		if next == target {
			return true
		}
		if visited[next] {
			continue
		}
		visited[next] = true
		if reaches(next, target, adj, visited) {
			return true
		}
	}
	return false
}
