package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/p3bot/pj/internal/flock"
	"github.com/p3bot/pj/internal/git"
	"github.com/p3bot/pj/internal/gitroot"
	"github.com/p3bot/pj/internal/gitstate"
	"github.com/p3bot/pj/internal/scopeconfig"
	"github.com/p3bot/pj/internal/token"
)

// rootOutcome: only NeedsAttention makes the run exit non-zero.
type rootOutcome int

const (
	outcomeSynced rootOutcome = iota
	outcomeNeedsAttention
)

type syncReport struct {
	label       string   // participating scope names, for human-facing lines
	snapshotN   int      // allowlisted paths committed in the snapshot
	residueN    int      // non-allowlist paths left uncommitted
	collidedIDs []string // add/add collided ids awaiting an edge_verify sweep
	pushed      bool
	unpushed    int
}

// syncRoot isolates one git-root so --all continues past a bad sibling.
func (e *engine) syncRoot(c *cobra.Command, t syncTarget) rootOutcome {
	ctx := c.Context()
	rep := &syncReport{label: participantLabel(t.participants)}

	if !e.syncPreflight(c, t.root) {
		return outcomeNeedsAttention
	}

	release, err := e.acquireSyncLocks(t)
	if err != nil {
		stderrln(c, fmt.Sprintf("%s: could not acquire sync locks: %v", rep.label, err))
		return outcomeNeedsAttention
	}
	defer release()

	defer func() {
		if err := e.drainEdgeVerify(c, rep); err != nil {
			stderrln(c, fmt.Sprintf("%s: edge_verify query failed: %v", rep.label, err))
		}
	}()

	var res integrateResult
	snapshotted := false
	// Mid-rebase entry: skip snapshot (no commit on temporary HEAD), resume, then fall through.
	if git.MidRebase(ctx, t.root) {
		res = e.resumeRebase(c, t, rep)
	} else {
		if !git.HasUpstream(ctx, t.root) {
			stderrln(c, token.Line(token.SyncDisabled,
				fmt.Sprintf("%s: git-root %s has no upstream — add a remote, then pj sync", rep.label, t.root)))
			return outcomeNeedsAttention
		}
		if err := e.snapshot(c, t, rep); err != nil {
			stderrln(c, fmt.Sprintf("%s: snapshot failed: %v", rep.label, err))
			return outcomeNeedsAttention
		}
		snapshotted = true
		res = e.fetchAndIntegrate(c, t, rep)
	}

	switch res {
	case integrateCompleted:
		if !snapshotted {
			if err := e.snapshot(c, t, rep); err != nil {
				stderrln(c, fmt.Sprintf("%s: snapshot failed: %v", rep.label, err))
				return outcomeNeedsAttention
			}
		}
		return e.finishSynced(c, t, rep)
	case integratePaused:
		e.reportPaused(c, rep)
		return outcomeNeedsAttention
	default: // integrateError
		return outcomeNeedsAttention
	}
}

func (e *engine) finishSynced(c *cobra.Command, t syncTarget, rep *syncReport) rootOutcome {
	if err := e.syncIntegrity(c, t); err != nil {
		stderrln(c, fmt.Sprintf("%s: integrity step failed: %v", rep.label, err))
		return outcomeNeedsAttention
	}
	if err := e.drainEdgeVerify(c, rep); err != nil {
		stderrln(c, fmt.Sprintf("%s: edge_verify query failed: %v", rep.label, err))
		return outcomeNeedsAttention
	}
	switch e.pushIfAhead(c, t, rep) {
	case pushPaused:
		e.reportPaused(c, rep)
		return outcomeNeedsAttention
	case pushFailed:
		return outcomeNeedsAttention
	}
	e.reportSuccess(c, rep)
	return outcomeSynced
}

// syncPreflight refuses the whole root on unparseable/drifted/mismatched siblings.
func (e *engine) syncPreflight(c *cobra.Command, root string) bool {
	refuse := false
	seenTrue, seenFalse := false, false
	for _, name := range e.siblingScopeNames(root) {
		dir := e.reg.Scopes[name].Dir
		if pjName, err := scopeconfig.ReadName(e.app.Ctx, dir); err == nil && pjName != name {
			stderrln(c, token.Line(token.NameDrift, fmt.Sprintf(
				"%s (%s): registry key %q but pj.cue name is %q — recover with pj scope forget %s then pj scope import; the whole git-root %s is refused",
				name, dir, name, pjName, name, root)))
			refuse = true
			continue
		}
		schema, cfgErr := e.rec.SchemaOrError(name, dir)
		if cfgErr != nil {
			stderrln(c, token.Line(token.ConfigUnparseable, fmt.Sprintf(
				"%s (%s): %s — fix pj.cue before sync can merge this git-root", name, cfgErr.Dir, cfgErr.Reason)))
			refuse = true
			continue
		}
		if schema.AutoCommit {
			seenTrue = true
		} else {
			seenFalse = true
		}
	}
	if seenTrue && seenFalse {
		stderrln(c, token.Line(token.AutoCommitMismatch, fmt.Sprintf(
			"scopes sharing git-root %s disagree on autoCommit — split the divergent scope into its own repo", root)))
		refuse = true
	}
	return !refuse
}

func (e *engine) siblingScopeNames(root string) []string {
	var out []string
	for name, entry := range e.reg.Scopes {
		if sgr, ok := gitroot.RepoRoot(entry.Dir); ok && sgr == root {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// acquireSyncLocks: scope locks first (name order), then git-root — reverse would deadlock write verbs.
func (e *engine) acquireSyncLocks(t syncTarget) (func(), error) {
	var locks []*flock.Lock
	release := func() {
		for i := len(locks) - 1; i >= 0; i-- {
			_ = locks[i].Release()
		}
	}
	for _, p := range t.participants {
		l, err := acquireScopeLock(p.dir)
		if err != nil {
			release()
			return nil, err
		}
		locks = append(locks, l)
	}
	gl, err := gitstate.AcquireCommitLock(e.app.StateDir, t.root)
	if err != nil {
		release()
		return nil, err
	}
	locks = append(locks, gl)
	return release, nil
}

// drainEdgeVerify: report-and-clear so deferred backstop and step-3 are mutually no-op.
func (e *engine) drainEdgeVerify(c *cobra.Command, rep *syncReport) error {
	if len(rep.collidedIDs) == 0 {
		return nil
	}
	ids := rep.collidedIDs
	rep.collidedIDs = nil
	return e.reportEdgeVerify(c, ids)
}

func (e *engine) reportPaused(c *cobra.Command, rep *syncReport) {
	stderrln(c, fmt.Sprintf("%s: rebase paused for a human — resolve the file(s) above in place, then run pj sync again", rep.label))
}

func (e *engine) reportSuccess(c *cobra.Command, rep *syncReport) {
	var parts []string
	if rep.snapshotN > 0 {
		parts = append(parts, fmt.Sprintf("snapshot %d path(s)", rep.snapshotN))
	}
	// Test unpushed before pushed so a non-zero re-count is not flattened to "pushed".
	switch {
	case rep.unpushed > 0:
		parts = append(parts, fmt.Sprintf("%d commit(s) still unpushed", rep.unpushed))
	case rep.pushed:
		parts = append(parts, "pushed")
	default:
		parts = append(parts, "up to date")
	}
	if rep.residueN > 0 {
		parts = append(parts, fmt.Sprintf("%d non-allowlist path(s) left", rep.residueN))
	}
	stderrln(c, fmt.Sprintf("pj sync %s: %s", rep.label, strings.Join(parts, ", ")))
}

func participantLabel(parts []participant) string {
	names := make([]string, len(parts))
	for i, p := range parts {
		names[i] = p.name
	}
	return strings.Join(names, ", ")
}
