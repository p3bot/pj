package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/start-cli/pj/internal/flock"
	"github.com/start-cli/pj/internal/git"
	"github.com/start-cli/pj/internal/gitroot"
	"github.com/start-cli/pj/internal/gitstate"
	"github.com/start-cli/pj/internal/scopeconfig"
	"github.com/start-cli/pj/internal/token"
)

// rootOutcome is one git-root's result. Both synced and "nothing to do" exit 0; only a
// root that ended needing a human or a retry is outcomeNeedsAttention, which makes the
// whole run exit non-zero (an ambient sync's single root, or any root under --all).
type rootOutcome int

const (
	outcomeSynced rootOutcome = iota
	outcomeNeedsAttention
)

// syncReport accumulates one root's outcome for the closing summary. Token lines
// (non_allowlist, edge_verify, the handoffs) are emitted as they happen; this carries
// the counts the summary names and the add/add collided ids whose inbound edges are
// verified once the rebase settles.
type syncReport struct {
	label       string   // participating scope names, for human-facing lines
	snapshotN   int      // allowlisted paths committed in the snapshot
	residueN    int      // non-allowlist paths left uncommitted
	collidedIDs []string // add/add collided ids awaiting an edge_verify sweep
	pushed      bool
	unpushed    int
}

// syncRoot runs one git-root through the five steps under its lock span. It is the unit
// of work --all isolates: it reports its own outcome and never returns an error that
// would strand a sibling root, so the caller continues past it.
func (e *engine) syncRoot(c *cobra.Command, t syncTarget) rootOutcome {
	ctx := c.Context()
	rep := &syncReport{label: participantLabel(t.participants)}

	// Preflight before the lock span (requirement 2): refuse the whole root under a
	// violated or unverifiable shared-repo invariant rather than pushing under it.
	if !e.syncPreflight(c, t.root) {
		return outcomeNeedsAttention
	}

	// Lock span (requirement 8): every participating scope's .pj.lock in sorted name
	// order, then the git-root sync.lock, held across snapshot, integrate, repair, and
	// push, and released on every exit path below.
	release, err := e.acquireSyncLocks(t)
	if err != nil {
		stderrln(c, fmt.Sprintf("%s: could not acquire sync locks: %v", rep.label, err))
		return outcomeNeedsAttention
	}
	defer release()

	// The add/add edge_verify sweep is drained on every exit from here, not only the two
	// that used to reach it (step 3 and the paused report). Five exits can carry a recorded
	// collision — a failed integrity step, a failed post-resume snapshot, an integrate that
	// errored at a later stop, a failed push, and a push that succeeded only after the race
	// retry re-integrated and repaired one more. The token is operation-time only and is
	// never persisted, and the next sync finds no duplicate to re-report because this run
	// repaired it, so an exit that skips the sweep loses the signal for good. Deferring it
	// against the report's own lifetime is what makes that unforgettable rather than a rule
	// each new exit path has to remember. Registered after release so it runs before the
	// locks drop (defers are LIFO), keeping the index read inside the span.
	defer func() {
		if err := e.drainEdgeVerify(c, rep); err != nil {
			stderrln(c, fmt.Sprintf("%s: edge_verify query failed: %v", rep.label, err))
		}
	}()

	// Mid-rebase entry (requirement 7): a paused rebase is an entry condition of the
	// whole command, not a case inside step 2. Skip the snapshot entirely — a snapshot
	// commit on the rebase's temporary HEAD is the exact write the freeze prevents — and
	// resume the paused rebase. The upstream precondition is not checked here: a paused
	// rebase detaches HEAD, so @{u} does not resolve mid-rebase; it resolves again once the
	// rebase completes, before the push.
	var res integrateResult
	snapshotted := false
	if git.MidRebase(ctx, t.root) {
		res = e.resumeRebase(c, t, rep)
	} else {
		// The repo exists (a git-root derived) but sync still needs an upstream to push to.
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
		// A resume that completed the rebase falls through into step 1 (requirement 7): the
		// same invocation snapshots the leftover dirt the mid-rebase entry skipped, so it too
		// runs the integrity step and pushes. The fresh path already snapshotted before the
		// integrate, so it does not repeat it here.
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

// finishSynced runs step 3 (integrity), the deferred add/add edge_verify sweep, step 4
// (push with the fetch→push race loop), and step 5 (report) once the rebase completed.
func (e *engine) finishSynced(c *cobra.Command, t syncTarget, rep *syncReport) rootOutcome {
	if err := e.syncIntegrity(c, t); err != nil {
		stderrln(c, fmt.Sprintf("%s: integrity step failed: %v", rep.label, err))
		return outcomeNeedsAttention
	}
	// The rebase completed and the integrity step reconciled: the merged rows are live, so
	// the add/add inbound-edge check runs here, in step 3, as requirement 4 places it —
	// before the push, not after the closing report. syncRoot's deferred drain is only the
	// backstop for ids discovered after this point; draining here is what keeps the ordinary
	// case in its documented step. A query failure is a fault worth stopping on while the
	// push is still ahead of us.
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

// syncPreflight refuses the whole git-root when a scope sharing it violates a shared-repo
// invariant sync cannot push under: an unparseable sibling pj.cue (autoCommit
// unverifiable), a name-drifted sibling (its files ride the push under an unverifiable
// name), or divergent autoCommit values. It returns true when the root is clear. An
// unreachable sibling is invisible here — a git-root cannot be derived from a dir pj
// cannot stat, so it can never be shown to share this root (requirement 2).
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

// siblingScopeNames is every registered scope whose dir derives root, sorted. An
// unreachable dir derives no root and is excluded, matching the preflight's premise.
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

// acquireSyncLocks takes every participating scope's .pj.lock in sorted name order, then
// the git-root sync.lock, and returns a release closure that drops them in reverse. The
// order is fixed: the write verbs take the scope lock first and the git-root lock inside
// it, so taking the git-root lock first here would deadlock sync against a concurrent
// pj status. The participants are already sorted by name.
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

// drainEdgeVerify reports the inbound-edge check for every add/add collided id recorded so
// far and clears the list, so each id is emitted exactly once however the root exits. The
// report-and-clear is one step on purpose: it is what lets syncRoot defer this as a
// backstop over every exit path while the step-3 call keeps the ordinary case in its
// documented place — whichever runs first emits, and the other is a silent no-op.
func (e *engine) drainEdgeVerify(c *cobra.Command, rep *syncReport) error {
	if len(rep.collidedIDs) == 0 {
		return nil
	}
	ids := rep.collidedIDs
	rep.collidedIDs = nil
	return e.reportEdgeVerify(c, ids)
}

// reportPaused surfaces the unresolved stop that left the rebase paused. The handoff detail
// line was already printed where the stop was classified; this closes with the resume
// instruction. The add/add edge_verify sweep is not run here — syncRoot's deferred drain
// covers this exit along with every other, off whatever rows are available before exiting
// (requirement 4).
func (e *engine) reportPaused(c *cobra.Command, rep *syncReport) {
	stderrln(c, fmt.Sprintf("%s: rebase paused for a human — resolve the file(s) above in place, then run pj sync again", rep.label))
}

// reportSuccess prints the closing summary line for a synced root: what the snapshot
// committed, the push state, and what residue was left (requirement 5). Per-event
// detail — the integrity step's repairs, the add/add renames, the conflict handoffs —
// is emitted inline as it happens during the run, not restated here.
func (e *engine) reportSuccess(c *cobra.Command, rep *syncReport) {
	var parts []string
	if rep.snapshotN > 0 {
		parts = append(parts, fmt.Sprintf("snapshot %d path(s)", rep.snapshotN))
	}
	// The unpushed count is tested before the pushed flag, not after. Both OK exits from the
	// push step leave the count at zero — nothing was ahead, or the push landed — so a
	// non-zero count here means the ref did not advance the way a successful push implied.
	// Testing pushed first would render that as a flat "pushed" and throw away the one
	// reading the re-count exists to produce. This is requirement 6's unpushed count in the
	// report: zero is spelled as "pushed" or "up to date", and anything else is named.
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

// participantLabel joins the participating scope names for the human-facing report
// lines — the scopes whose files this one push carries.
func participantLabel(parts []participant) string {
	names := make([]string, len(parts))
	for i, p := range parts {
		names[i] = p.name
	}
	return strings.Join(names, ", ")
}
