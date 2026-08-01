package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/start-cli/pj/internal/git"
	"github.com/start-cli/pj/internal/gitstate"
	"github.com/start-cli/pj/internal/token"
)

// syncIntegrity: only reconcile pj sync runs; uses locks-held repair core over merged tree.
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

type pushResult int

const (
	pushOK pushResult = iota
	pushFailed
	pushPaused
)

// pushIfAhead: fetch→push race re-integrates once (step 2 only, not integrity).
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

// recordPushFailure: best-effort state write must not mask the push failure.
func (e *engine) recordPushFailure(c *cobra.Command, t syncTarget, rep *syncReport, cause error, guidance string) {
	_ = gitstate.WriteLastPushError(e.app.StateDir, t.root, cause.Error())
	stderrln(c, token.Line(token.LastPushError, fmt.Sprintf(
		"%s: push did not land (%v) — %s", rep.label, cause, guidance)))
}
