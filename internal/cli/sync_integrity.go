package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/start-cli/pj/internal/git"
	"github.com/start-cli/pj/internal/gitstate"
	"github.com/start-cli/pj/internal/token"
)

// syncIntegrity is step 3, the auto-commit twin of pj doctor --repair over the merged
// tree. It reconciles every participating scope first — the merged rows describe the
// post-rebase tree, and repairing from stale pre-rebase rows would miss a duplicate that
// arrived from the other machine — then runs P5's automatic repairs per scope. It runs
// under sync's held locks, so it drives the repair batches through the locks-held core,
// never the acquiring wrapper. This is the only reconcile pj sync runs.
func (e *engine) syncIntegrity(c *cobra.Command, t syncTarget) error {
	targets := make(map[string]string, len(t.participants))
	for _, p := range t.participants {
		targets[p.name] = p.dir
	}
	res, err := e.reconcileResult(targets)
	if err != nil {
		return err
	}
	for _, p := range t.participants {
		schema := res.Schema(p.name)
		if schema == nil {
			continue // an unusable config the preflight would already have refused
		}
		target := &repairTarget{
			scope:      p.name,
			dir:        p.dir,
			schema:     schema,
			autoCommit: schemaAutoCommit(schema),
			root:       t.root,
			hasRoot:    true,
		}
		if err := e.runRepairBatches(c, target, doctorFlags{repair: true}); err != nil {
			return err
		}
	}
	return nil
}

// pushResult is the outcome of step 4: pushed (or nothing to push), a genuine push
// failure recorded as last-push-error, or a re-integrate that paused for a human when the
// fetch→push race brought in a conflict.
type pushResult int

const (
	pushOK pushResult = iota
	pushFailed
	pushPaused
)

// pushIfAhead is step 4: push synchronously if ahead, handling the fetch→push race by
// re-integrating once and retrying. A sync with nothing to push (a read-only machine that
// only pulled) skips the push. A genuine push failure is recorded under the git-root ops
// state for doctor and write-command warnings; a successful push clears it.
func (e *engine) pushIfAhead(c *cobra.Command, t syncTarget, rep *syncReport) pushResult {
	ctx := c.Context()
	ahead, err := git.UnpushedCount(ctx, t.root)
	if err != nil {
		stderrln(c, fmt.Sprintf("%s: could not count unpushed commits: %v", rep.label, err))
		return pushFailed
	}
	if ahead == 0 {
		rep.unpushed = 0
		return pushOK // already pulled in step 2; nothing to push
	}

	if err := git.Push(ctx, t.root); err != nil {
		// The remote may have moved in the fetch→push window: re-integrate once and retry.
		// This loops back to step 2 (fetch + integrate) only — deliberately not step 3
		// (integrity) or the edge_verify sweep. The re-integrate can merge in a duplicate id
		// or add/add collision from the racing machine, and re-running integrity here would
		// close that window no more than the first pass did: the repair itself makes a commit
		// that can lose the same race one level down, and the fetch→push gap is irreducible
		// without a distributed lock git cannot give. Integrity is self-healing — the next
		// sync on any machine reconciles the merged tree and repairs what it finds — so a
		// briefly integrity-dirty push here converges rather than persisting. Keeping the
		// retry step-2-only keeps the loop bounded; deepening it only moves the race down.
		stderrln(c, fmt.Sprintf("%s: push rejected — re-integrating and retrying (%v)", rep.label, err))
		switch e.fetchAndIntegrate(c, t, rep) {
		case integratePaused:
			e.recordPushFailure(c, t, rep, err, "resolve the conflict reported above, then run pj sync")
			return pushPaused
		case integrateError:
			e.recordPushFailure(c, t, rep, err, "clear the re-integrate error above, then run pj sync")
			return pushFailed
		}
		if err2 := git.Push(ctx, t.root); err2 != nil {
			e.recordPushFailure(c, t, rep, err2, "fix the remote/auth, then pj sync")
			return pushFailed
		}
	}

	if err := gitstate.ClearLastPushError(e.app.StateDir, t.root); err != nil {
		stderrln(c, fmt.Sprintf("%s: could not clear last-push-error marker: %v", rep.label, err))
	}
	rep.pushed = true
	if n, err := git.UnpushedCount(ctx, t.root); err == nil {
		rep.unpushed = n
	}
	return pushOK
}

// recordPushFailure marks the git-root as carrying a push that did not land and reports it.
// Every exit from step 4 where a push was attempted and the commits are still local goes
// through here, not only the second attempt: the marker is what pj doctor and every
// complete-state write verb read to warn that this repo has work stranded on this machine,
// and an unrecorded failure is forgotten the moment the run ends. The guidance names the
// next action for this particular exit — a paused rebase is not fixed by fixing auth. The
// probe that could not count unpushed commits is deliberately not a caller: no push was
// attempted there, so there is no push error to record.
//
// The write is best-effort. A state-dir failure must not mask the push failure itself,
// which the token line below reports either way.
func (e *engine) recordPushFailure(c *cobra.Command, t syncTarget, rep *syncReport, cause error, guidance string) {
	_ = gitstate.WriteLastPushError(e.app.StateDir, t.root, cause.Error())
	stderrln(c, token.Line(token.LastPushError, fmt.Sprintf(
		"%s: push did not land (%v) — %s", rep.label, cause, guidance)))
}
