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

type integrateResult int

const (
	integrateCompleted integrateResult = iota
	integratePaused
	integrateError
)

var conflictMarkers = [][]byte{[]byte("<<<<<<<"), []byte("======="), []byte(">>>>>>>")}

// conflictKind: mutually exclusive; only kindSchema gates field-merge.
type conflictKind int

const (
	kindOther conflictKind = iota
	kindSchema
	kindIgnore
	kindProject
)

// isConfig: pj.cue or .gitignore; only kindSchema gates .md merges.
func (k conflictKind) isConfig() bool { return k == kindSchema || k == kindIgnore }

type conflictItem struct {
	path  string // repo-relative
	abs   string
	owner participant
	kind  conflictKind
}

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

// resumeRebase: mid-rebase entry skips snapshot (no commit on temporary HEAD).
func (e *engine) resumeRebase(c *cobra.Command, t syncTarget, rep *syncReport) integrateResult {
	driver := e.newDriver(t)
	return e.runStops(c, t, driver, rep, func() (bool, error) {
		return e.resolveResumeStop(c, t, driver, rep)
	})
}

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

// driveStop: schema-before-data; only conflicted pj.cue fail-closes project .md.
func (e *engine) driveStop(c *cobra.Command, t syncTarget, driver *rebasedriver.Driver, rep *syncReport) (bool, error) {
	ctx := c.Context()
	items := classifyStop(ctx, t)
	allStaged := true

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

// reportConflictedConfig: config_unparseable only for pj.cue, never .gitignore.
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

func (e *engine) mdItemBlocked(c *cobra.Command, it conflictItem, schemaConflicted map[string]bool, allStaged *bool) bool {
	switch it.kind {
	case kindOther:
		// A path pj cannot classify or own: leave the rebase paused and name it, so the closing "resolve the file(s)
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
			if err := e.driveMD(ctx, c, driver, it, head, rebaseHead, rep, &allStaged); err != nil {
				return false, err
			}
			continue
		}
		if frontmatterHasStatusConflict(it.abs) {
			stderrln(c, token.Line(token.StatusConflict, fmt.Sprintf(
				"%s: unresolved status_conflict — set status to one value and delete status_conflict in %s, then run pj sync", it.owner.name, it.path)))
			allStaged = false
			continue
		}
		if err := git.Add(ctx, t.root, []string{it.path}); err != nil {
			return false, fmt.Errorf("stage resolved %s: %w", it.path, err)
		}
	}
	return allStaged, nil
}

func (e *engine) applyDriverOutcome(c *cobra.Command, o rebasedriver.Outcome, rep *syncReport) bool {
	for _, w := range o.Warnings {
		stderrln(c, w)
	}
	switch o.Class {
	case rebasedriver.ClassClean:
		return true
	case rebasedriver.ClassRename:
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

// newDriver loads schema from on-disk pj.cue at call time, never a pre-fetch snapshot.
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

// isDeleteEditStages: stage-set separates delete/edit from driver output (both sides still in index).
func isDeleteEditStages(s git.Stages) bool {
	return s.Base && s.Ours != s.Theirs
}

func survivingStageNumber(s git.Stages) int {
	if s.Ours {
		return 2
	}
	return 3
}

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

func sideToStage(s fmmerge.Side) int {
	if s == fmmerge.SideTheirs {
		return 3
	}
	return 2
}

// deleteEditStageLabel: stage 2 = incoming upstream; stage 3 = local replay.
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

// stageDeleteEditIfActed: unactioned (worktree == survivor blob) does not stage.
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

func fileHasConflictMarkers(abs string) bool {
	data, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	return hasConflictMarker(data)
}

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
