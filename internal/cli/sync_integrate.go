package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/start-cli/pj/internal/fmmerge"
	"github.com/start-cli/pj/internal/frontmatter"
	"github.com/start-cli/pj/internal/git"
	"github.com/start-cli/pj/internal/rebasedriver"
	"github.com/start-cli/pj/internal/scopeconfig"
	"github.com/start-cli/pj/internal/token"
)

// integrateResult is the outcome of driving a rebase: it completed, it paused for a
// human at a stop pj could not resolve, or an operational fault aborted the integrate.
type integrateResult int

const (
	integrateCompleted integrateResult = iota
	integratePaused
	integrateError
)

// conflictMarkers are the git merge-conflict marker line prefixes.
var conflictMarkers = [][]byte{[]byte("<<<<<<<"), []byte("======="), []byte(">>>>>>>")}

// conflictKind is what pj can do with a path a rebase stop left unmerged. It is one value
// rather than a set of flags because the four cases are mutually exclusive and two of them
// differ in exactly the way flags invite you to conflate: pj.cue and .gitignore are both
// allowlisted files sync commits, and both are staged from a human's resolution at a
// resume — but only pj.cue types frontmatter, so only pj.cue may gate a field-merge.
type conflictKind int

const (
	// kindOther is a path pj can neither own nor classify: outside every participating
	// scope dir, or inside one but off the closed allowlist. pj never touches it.
	kindOther conflictKind = iota
	// kindSchema is a scope's pj.cue. A real text conflict here leaves the scope's schema
	// untrustworthy, so every project .md under it fails closed until a human resolves it.
	kindSchema
	// kindIgnore is a scope's .gitignore: allowlisted and resolved like pj.cue, but it
	// types nothing, so a conflict in it gates no field-merge and is not a schema failure.
	kindIgnore
	// kindProject is a project .md the rebase driver field-merges.
	kindProject
)

// isConfig reports whether the kind is a config file sync commits and stages from a human's
// in-place resolution — pj.cue or .gitignore. Only kindSchema additionally gates a scope's
// project .md field-merges.
func (k conflictKind) isConfig() bool { return k == kindSchema || k == kindIgnore }

// conflictItem is one path a rebase stop left unmerged, classified against the scope that
// owns it. owner is meaningful for every kind but kindOther, which by definition has none.
type conflictItem struct {
	path  string // repo-relative
	abs   string
	owner participant
	kind  conflictKind
}

// fetchAndIntegrate is step 2 on a fresh (not mid-rebase) root: always fetch, then rebase
// local commits onto the upstream. A clean fast-forward completes immediately; otherwise
// the per-stop procedure drives every stop pj can resolve on its own and the loop carries
// the rebase to completion or pauses at the first stop needing a human.
func (e *engine) fetchAndIntegrate(c *cobra.Command, t syncTarget, rep *syncReport) integrateResult {
	ctx := c.Context()
	if err := git.Fetch(ctx, t.root); err != nil {
		stderrln(c, fmt.Sprintf("%s: fetch failed: %v", rep.label, err))
		return integrateError
	}
	paused, err := git.Rebase(ctx, t.root, "@{u}")
	if err != nil {
		stderrln(c, fmt.Sprintf("%s: rebase failed: %v", rep.label, err))
		return integrateError
	}
	if !paused {
		return integrateCompleted
	}
	driver := e.newDriver(t)
	return e.runStops(c, t, driver, rep, func() (bool, error) {
		return e.driveStop(c, t, driver, rep)
	})
}

// resumeRebase is the layer-4 resume: entered mid-rebase, it re-runs the per-stop
// procedure at the current (human-touched) stop with the merged-vs-unmerged discriminator
// (requirement 7), then hands to the same continue loop, where any later replayed commit
// pj resolves on its own is driven exactly as in a fresh integrate.
func (e *engine) resumeRebase(c *cobra.Command, t syncTarget, rep *syncReport) integrateResult {
	driver := e.newDriver(t)
	return e.runStops(c, t, driver, rep, func() (bool, error) {
		return e.resolveResumeStop(c, t, driver, rep)
	})
}

// runStops drives the rebase from a first-stop procedure to completion. After each stop
// is fully staged it continues the rebase; a later stop is a fresh per-stop drive; a stop
// that leaves any path unstaged pauses the whole command. Only a stop needing a human ends
// it — a stop pj resolves itself is continued, never surfaced as work for a human.
func (e *engine) runStops(c *cobra.Command, t syncTarget, driver *rebasedriver.Driver, rep *syncReport, first func() (bool, error)) integrateResult {
	ctx := c.Context()
	allStaged, err := first()
	if err != nil {
		stderrln(c, fmt.Sprintf("%s: %v", rep.label, err))
		return integrateError
	}
	for {
		if !allStaged {
			return integratePaused
		}
		paused, err := git.RebaseContinue(ctx, t.root)
		if err != nil {
			stderrln(c, fmt.Sprintf("%s: rebase --continue failed: %v", rep.label, err))
			return integrateError
		}
		if !paused {
			return integrateCompleted
		}
		allStaged, err = e.driveStop(c, t, driver, rep)
		if err != nil {
			stderrln(c, fmt.Sprintf("%s: %v", rep.label, err))
			return integrateError
		}
	}
}

// driveStop runs the fresh per-stop procedure (requirement 4): resolve every conflicted
// pj.cue first (schema-before-data), then drive every conflicted project .md through the
// rebase driver. It returns whether every conflicted path ended up staged, which the
// continue loop reads to decide continue-or-pause.
func (e *engine) driveStop(c *cobra.Command, t syncTarget, driver *rebasedriver.Driver, rep *syncReport) (bool, error) {
	ctx := c.Context()
	items := classifyStop(ctx, t)
	allStaged := true

	// Pass 1, schema before data: a conflicted config file git could not auto-merge on a
	// fresh stop is a real text conflict — never field-merged — so it pauses for a human.
	// Only a conflicted pj.cue also fail-closes its scope's project .md in pass 2, because
	// it is the file that types frontmatter; a conflicted .gitignore pauses on its own and
	// gates nothing. A delete/edit is classified by stage set so the report names the three
	// (or two) ways out rather than markers the file does not carry.
	schemaConflicted := map[string]bool{}
	for _, it := range items {
		if !it.kind.isConfig() {
			continue
		}
		if it.kind == kindSchema {
			schemaConflicted[it.owner.dir] = true
		}
		stages, err := git.ConflictStages(ctx, t.root, it.path)
		if err != nil {
			return false, fmt.Errorf("enumerate conflict stages for %s: %w", it.path, err)
		}
		reportConflictedConfig(c, it, configDeleteEditSide(stages))
		allStaged = false
	}

	head, rebaseHead, err := git.RebaseSides(ctx, t.root)
	if err != nil {
		return false, fmt.Errorf("resolve rebase sides: %w", err)
	}
	for _, it := range items {
		if e.mdItemBlocked(c, it, schemaConflicted, &allStaged) {
			continue
		}
		if err := e.driveMD(ctx, c, driver, it, head, rebaseHead, rep, &allStaged); err != nil {
			return false, err
		}
	}
	return allStaged, nil
}

// reportConflictedConfig names a config file the human must resolve in place. When
// deletedSide is non-empty the path is a delete/edit and deletedSide is the reader's
// label for the deleting side; when empty the file carries real conflict markers.
// Only a conflicted pj.cue rides config_unparseable: that token is a frozen entry in the
// closed catalogue meaning a scope whose schema cannot be trusted, and a .gitignore types
// nothing — putting its scope into that state on the wire would tell every agent matching
// the prefix that the schema is unreadable when it is fine.
func reportConflictedConfig(c *cobra.Command, it conflictItem, deletedSide string) {
	if deletedSide != "" {
		if it.kind == kindSchema {
			stderrln(c, token.Line(token.ConfigUnparseable, fmt.Sprintf(
				"%s: delete/edit conflict: %s was deleted on %s while the other side edited it — edit %s or git add it to keep as-is, then run pj sync (making the deletion win takes the scope out of pj's hands first: while a registered scope has no schema, sync refuses the root)",
				it.owner.name, it.path, deletedSide, it.path)))
			return
		}
		stderrln(c, fmt.Sprintf(
			"delete/edit conflict: %s was deleted on %s while the other side edited it — remove %s, edit it, or git add it to keep as-is, then run pj sync",
			it.path, deletedSide, it.path))
		return
	}
	if it.kind == kindSchema {
		stderrln(c, token.Line(token.ConfigUnparseable, fmt.Sprintf(
			"%s: conflicted pj.cue — resolve %s in place, then run pj sync", it.owner.name, it.path)))
		return
	}
	stderrln(c, fmt.Sprintf(
		"conflicted .gitignore: resolve the conflict markers in %s, then run pj sync", it.path))
}

// mdItemBlocked handles every conflicted path that is not a drivable project .md, plus the
// one project .md case the driver must not see. It returns true when the caller should skip
// the item, and marks the stop unstaged (via allStaged) only for the cases that genuinely
// block: a path pj can neither own nor classify, and a project .md whose scope's pj.cue is
// still conflicted (fail-closed). A config item was already reported by the caller's first
// pass, so it is skipped here without re-marking — that is what keeps a resume's
// human-resolved (and staged) config from being flipped back to unstaged.
func (e *engine) mdItemBlocked(c *cobra.Command, it conflictItem, schemaConflicted map[string]bool, allStaged *bool) bool {
	switch it.kind {
	case kindOther:
		// A path pj cannot classify or own: leave the rebase paused and name it, so
		// the closing "resolve the file(s) above" line has a file to point at.
		stderrln(c, fmt.Sprintf(
			"unresolvable conflict: resolve the conflict markers in %s, then run pj sync", it.path))
		*allStaged = false
		return true
	case kindSchema, kindIgnore:
		return true
	}
	if schemaConflicted[it.owner.dir] {
		stderrln(c, token.Line(token.ConfigUnparseable, fmt.Sprintf(
			"%s: %s not merged — its scope's pj.cue is conflicted; resolve pj.cue first", it.owner.name, it.path)))
		*allStaged = false
		return true
	}
	return false
}

// driveMD field-merges one drivable conflicted project .md through the rebase driver and
// records its outcome, marking the stop unstaged (via allStaged) when the driver left the
// file for a human. Callers reach it for a fresh stop, a resume's never-field-merged file
// (frontmatter still marked), and an unactioned delete/edit re-drive (report only).
func (e *engine) driveMD(ctx context.Context, c *cobra.Command, driver *rebasedriver.Driver, it conflictItem, head, rebaseHead string, rep *syncReport, allStaged *bool) error {
	outcome, derr := driver.Resolve(ctx, rebasedriver.Conflict{
		Path: it.path, ScopeDir: it.owner.dir, OursRev: head, TheirsRev: rebaseHead,
	})
	if derr != nil {
		return derr
	}
	if !e.applyDriverOutcome(c, outcome, rep) {
		*allStaged = false
	}
	return nil
}

// resolveResumeStop is the discriminating per-stop procedure at a resumed stop. It stages
// a config file the human resolved (freeing that scope's project .md to be driven) and
// pauses on one still marked or still-unactioned as a delete/edit; then, for each
// conflicted project .md, it field-merges one whose frontmatter still carries markers
// (never merged — the fail-closed leftover of a pj.cue conflict), re-drives an unactioned
// delete/edit for its report, and for driver output (clean frontmatter, both sides
// present) stages the human's resolution — except a still-present status_conflict, which
// blocks the continue.
func (e *engine) resolveResumeStop(c *cobra.Command, t syncTarget, driver *rebasedriver.Driver, rep *syncReport) (bool, error) {
	ctx := c.Context()
	items := classifyStop(ctx, t)
	allStaged := true

	schemaConflicted := map[string]bool{}
	for _, it := range items {
		if !it.kind.isConfig() {
			continue
		}
		stages, err := git.ConflictStages(ctx, t.root, it.path)
		if err != nil {
			return false, fmt.Errorf("enumerate conflict stages for %s: %w", it.path, err)
		}
		if isDeleteEditStages(stages) {
			acted, err := stageDeleteEditIfActed(ctx, t.root, it.path, it.abs, stages)
			if err != nil {
				return false, err
			}
			if acted {
				continue
			}
			// Unactioned: re-report and, for pj.cue, keep the scope's project .md fail-closed.
			if it.kind == kindSchema {
				schemaConflicted[it.owner.dir] = true
			}
			reportConflictedConfig(c, it, configDeleteEditSide(stages))
			allStaged = false
			continue
		}
		if fileHasConflictMarkers(it.abs) {
			if it.kind == kindSchema {
				schemaConflicted[it.owner.dir] = true
			}
			reportConflictedConfig(c, it, "")
			allStaged = false
			continue
		}
		// The human resolved it and left it unstaged; stage it so the rebase can continue
		// and so the driver can merge that scope's now-freed project .md this same stop.
		if err := git.Add(ctx, t.root, []string{it.path}); err != nil {
			return false, fmt.Errorf("stage resolved %s: %w", it.path, err)
		}
	}

	head, rebaseHead, err := git.RebaseSides(ctx, t.root)
	if err != nil {
		return false, fmt.Errorf("resolve rebase sides: %w", err)
	}
	for _, it := range items {
		if e.mdItemBlocked(c, it, schemaConflicted, &allStaged) {
			continue
		}
		stages, err := git.ConflictStages(ctx, t.root, it.path)
		if err != nil {
			return false, fmt.Errorf("enumerate conflict stages for %s: %w", it.path, err)
		}
		if isDeleteEditStages(stages) {
			// Delete/edit: stage only when the human removed or modified the worktree file;
			// an unactioned re-run re-drives for the same handoff report as the first pause.
			// Re-driving is read-only here — a one-side-absent stage set cannot write.
			acted, err := stageDeleteEditIfActed(ctx, t.root, it.path, it.abs, stages)
			if err != nil {
				return false, err
			}
			if acted {
				continue
			}
			if err := e.driveMD(ctx, c, driver, it, head, rebaseHead, rep, &allStaged); err != nil {
				return false, err
			}
			continue
		}
		if frontmatterHasMarkers(it.abs) {
			// Never field-merged (the state a pj.cue conflict leaves behind): drive it now,
			// through the schema the human's resolved pj.cue parses to this time.
			if err := e.driveMD(ctx, c, driver, it, head, rebaseHead, rep, &allStaged); err != nil {
				return false, err
			}
			continue
		}
		// Driver output — clean field-merged frontmatter, a body or status decision awaiting
		// the human. Never re-drive: re-reading its still-present stages would overwrite the
		// body the human just resolved.
		if frontmatterHasStatusConflict(it.abs) {
			stderrln(c, token.Line(token.StatusConflict, fmt.Sprintf(
				"%s: unresolved status_conflict — set status to one value and delete status_conflict in %s, then run pj sync", it.owner.name, it.path)))
			allStaged = false
			continue
		}
		// A body conflict is the human's say-so: stage and push whatever body they left,
		// never scanning it for marker-like lines (legitimate prose can carry them).
		if err := git.Add(ctx, t.root, []string{it.path}); err != nil {
			return false, fmt.Errorf("stage resolved %s: %w", it.path, err)
		}
	}
	return allStaged, nil
}

// applyDriverOutcome records one driver Outcome and reports its handoff. It returns
// whether the path is staged (the rebase can continue over it): a clean merge and an
// add/add rename are staged; a body conflict, a status dispute, a delete/edit, and a
// fail-closed merge are left unstaged and reported for a human.
func (e *engine) applyDriverOutcome(c *cobra.Command, o rebasedriver.Outcome, rep *syncReport) bool {
	for _, w := range o.Warnings {
		stderrln(c, w)
	}
	switch o.Class {
	case rebasedriver.ClassClean:
		return true
	case rebasedriver.ClassRename:
		// The driver wrote and staged both files; this project records the collided id and
		// runs its inbound-edge check later — that is index work, not the driver's.
		rep.collidedIDs = append(rep.collidedIDs, o.Rename.OldID)
		stdoutln(c, fmt.Sprintf("repaired add/add duplicate: %s kept, renamed to %s (%s)",
			o.Rename.OldID, o.Rename.NewID, o.Rename.NewPath))
		return true
	case rebasedriver.ClassBodyConflict:
		stderrln(c, fmt.Sprintf("body conflict: resolve the merge markers in the body of %s, then run pj sync", o.Path))
		return false
	case rebasedriver.ClassStatusDispute:
		stderrln(c, token.Line(token.StatusConflict, fmt.Sprintf(
			"%s: %s — set status to one value and delete status_conflict in %s, then run pj sync",
			o.Path, strings.Join(o.StatusConflict, " vs "), o.Path)))
		return false
	case rebasedriver.ClassDeleteEdit:
		stderrln(c, fmt.Sprintf(
			"delete/edit conflict: %s was deleted on %s while the other side edited it (status %q) — remove %s, edit it, or git add it to keep as-is, then run pj sync",
			o.Path, deleteEditStageLabel(sideToStage(o.DeleteEdit.Deleted)), o.DeleteEdit.SurvivingStatus, o.Path))
		return false
	case rebasedriver.ClassFailClosed:
		stderrln(c, token.Line(token.ConfigUnparseable, fmt.Sprintf(
			"%s: merge failed closed on key %q (%s) — resolve %s in place, then run pj sync",
			o.Path, o.FailClosed.Key, o.FailClosed.Reason, o.Path)))
		return false
	default:
		return false
	}
}

// newDriver builds a rebase driver for one root, typing each field merge through the
// reconcile-cache-backed schema loader keyed on the on-disk pj.cue as it stands at call
// time — never a snapshot captured before the fetch.
func (e *engine) newDriver(t syncTarget) *rebasedriver.Driver {
	dirToScope := make(map[string]string, len(t.participants))
	for _, p := range t.participants {
		dirToScope[p.dir] = p.name
	}
	load := func(scopeDir string) (*scopeconfig.Schema, error) {
		name, ok := dirToScope[scopeDir]
		if !ok {
			return scopeconfig.Load(e.app.Ctx, scopeDir)
		}
		schema, cfgErr := e.rec.SchemaOrError(name, scopeDir)
		if cfgErr != nil {
			return nil, cfgErr
		}
		return schema, nil
	}
	return rebasedriver.New(t.root, load)
}

// classifyStop reads the unmerged paths at the current stop and classifies each against
// the scope that owns it — the single git read the per-stop procedures share.
func classifyStop(ctx context.Context, t syncTarget) []conflictItem {
	var items []conflictItem
	for _, path := range git.UnmergedFiles(ctx, t.root) {
		it := conflictItem{path: path, abs: filepath.Join(t.root, path)}
		if p, ok := owningParticipant(path, t); ok {
			it.owner = p
			it.kind = classifyConflict(it.abs, p)
		}
		items = append(items, it)
	}
	return items
}

// classifyConflict decides what an absolute conflicted path under a scope dir is: the
// scope's schema (pj.cue), its .gitignore, or a project .md the driver resolves. A path
// outside the closed allowlist is kindOther — pj does not touch it.
func classifyConflict(abs string, p participant) conflictKind {
	if !isAllowlistedScopeFile(abs, p.dir) {
		return kindOther
	}
	switch filepath.Base(abs) {
	case "pj.cue":
		return kindSchema
	case ".gitignore":
		return kindIgnore
	default:
		return kindProject
	}
}

// owningParticipant finds the auto-commit scope whose dir contains a repo-relative
// conflicted path. Scope dirs are disjoint, so at most one matches.
func owningParticipant(repoRelPath string, t syncTarget) (participant, bool) {
	for _, p := range t.participants {
		rel, err := filepath.Rel(t.root, p.dir)
		if err != nil {
			continue
		}
		if rel == "." || repoRelPath == rel || strings.HasPrefix(repoRelPath, rel+string(filepath.Separator)) {
			return p, true
		}
	}
	return participant{}, false
}

// isDeleteEditStages reports whether the unmerged path's stage set is a delete/edit:
// base present and exactly one of stage 2 / stage 3 present. Field-merged driver output
// still has both sides in the index until staged, so stage-set — not marker-freeness —
// is what separates the two shapes.
func isDeleteEditStages(s git.Stages) bool {
	return s.Base && s.Ours != s.Theirs
}

// survivingStageNumber returns the present side's stage number (2 or 3). Call only when
// isDeleteEditStages is true so exactly one side is present.
func survivingStageNumber(s git.Stages) int {
	if s.Ours {
		return 2
	}
	return 3
}

// configDeleteEditSide returns the reader's label for the deleting side when s is a
// delete/edit, or "" when it is not. Labelling keys off the deleted stage number — the
// stable rebase coordinate — never an ours/theirs side string.
func configDeleteEditSide(s git.Stages) string {
	if !isDeleteEditStages(s) {
		return ""
	}
	// Survivor present ⇒ the other stage deleted.
	if s.Ours {
		return deleteEditStageLabel(3)
	}
	return deleteEditStageLabel(2)
}

// sideToStage maps the driver's deleted side onto its git stage number (ours=:2,
// theirs=:3). Human-facing text then keys only on that stage, never on the side label.
func sideToStage(s fmmerge.Side) int {
	if s == fmmerge.SideTheirs {
		return 3
	}
	return 2
}

// deleteEditStageLabel names the deleting side for a human mid-rebase. Each label carries
// its own article so format strings can say "on %s" without forcing "the" onto every side.
// Stage 2 is the incoming upstream tip; stage 3 is this machine's commit being replayed.
func deleteEditStageLabel(stage int) string {
	switch stage {
	case 2:
		return "the incoming side (fetched from the remote)"
	case 3:
		return "this machine's replayed commit"
	default:
		return fmt.Sprintf("stage %d", stage)
	}
}

// stageDeleteEditIfActed stages a delete/edit path when the human has acted on it —
// removed the worktree file (deletion wins) or modified it (edit wins) — and returns
// true when it staged. An unactioned path (worktree still equals the surviving stage
// blob) returns false without staging. Only a does-not-exist result is a removal;
// every other worktree or stage-blob read error aborts.
func stageDeleteEditIfActed(ctx context.Context, gitRoot, repoPath, abs string, s git.Stages) (acted bool, err error) {
	blob, err := git.ShowStage(ctx, gitRoot, survivingStageNumber(s), repoPath)
	if err != nil {
		return false, fmt.Errorf("read surviving stage for %s: %w", repoPath, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			if err := git.Add(ctx, gitRoot, []string{repoPath}); err != nil {
				return false, fmt.Errorf("stage deletion of %s: %w", repoPath, err)
			}
			return true, nil
		}
		return false, fmt.Errorf("read worktree %s: %w", repoPath, err)
	}
	if bytes.Equal(data, blob) {
		return false, nil
	}
	if err := git.Add(ctx, gitRoot, []string{repoPath}); err != nil {
		return false, fmt.Errorf("stage resolved %s: %w", repoPath, err)
	}
	return true, nil
}

// fileHasConflictMarkers reports whether a whole file carries any git conflict-marker
// line — the resume-time test for whether a human has resolved a config file yet.
func fileHasConflictMarkers(abs string) bool {
	data, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	return hasConflictMarker(data)
}

// frontmatterHasMarkers reports whether a project file's frontmatter still carries
// conflict markers — the resume discriminator for a never-field-merged file. A broken
// fence (markers spanning it) reads as markers present: it too has never been merged.
func frontmatterHasMarkers(abs string) bool {
	data, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	interior, _, present := frontmatter.Split(data)
	if !present {
		return true
	}
	return hasConflictMarker(interior)
}

// frontmatterHasStatusConflict reports whether a clean-frontmatter project file still
// carries the structured status_conflict key — the one reliable signal a terminal
// dispute is unresolved, which gates rebase --continue.
func frontmatterHasStatusConflict(abs string) bool {
	data, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	interior, _, present := frontmatter.Split(data)
	if !present {
		return false
	}
	m, err := frontmatter.Parse(interior)
	if err != nil {
		return false
	}
	return len(m.StatusConflict) > 0
}

func hasConflictMarker(data []byte) bool {
	for len(data) > 0 {
		var line []byte
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i], data[i+1:]
		} else {
			line, data = data, nil
		}
		for _, marker := range conflictMarkers {
			if bytes.HasPrefix(line, marker) {
				return true
			}
		}
	}
	return false
}
