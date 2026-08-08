package cli

import (
	"context"
	"fmt"

	"github.com/p3bot/tk/internal/scopefile"

	"github.com/spf13/cobra"

	"github.com/p3bot/tk/internal/index"
	"github.com/p3bot/tk/internal/status"
)

// runClaim: scope flock spans reconcile→claim so candidates stay valid under the same lock.
func runClaim(app *App, c *cobra.Command, scopeFlag string, noLens bool) error {
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

	lock, err := scopefile.AcquireLock(dir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	ctx := c.Context()
	res, targets, err := e.reconcileClosureResult(scope, dir)
	if err != nil {
		return err
	}
	// Ambient config must be usable; sibling config failures only hold gates.
	if err := refuseUnusableScope(res, scope, dir); err != nil {
		return err
	}
	schema := res.Schema(scope)
	autoCommit := schemaAutoCommit(schema)
	root, hasRoot := scopefile.GitRoot(dir)
	if err := checkMidRebase(ctx, scope, autoCommit, root, hasRoot); err != nil {
		return err
	}
	e.printWarnings(c, res.Warnings)

	gate, err := e.buildGate(res, targets)
	if err != nil {
		return err
	}
	rows, err := e.db.ScopeTickets(scope)
	if err != nil {
		return err
	}
	candidates := nextCandidates(rows)
	sortTickets(candidates)

	lens := e.reg.Lens[scope]
	applyLens := !noLens && len(lens) > 0

	tokens := newTokenSet()
	blocked, readyOutsideLens := 0, 0
	var claimed *index.Ticket
	for _, p := range candidates {
		ds := gate.evalDepends(p)
		tokens.add(ds.Tokens)
		if !gate.nextEligible(p, ds) {
			if ds.Held() {
				blocked++
			}
			continue
		}
		if applyLens && !passesLens(p, lens) {
			readyOutsideLens++
			continue
		}
		ok, err := e.claimTicket(ctx, c, scope, dir, autoCommit, p, root, hasRoot)
		if err != nil {
			return err
		}
		if ok {
			claimed = p
			break
		}
	}

	if applyLens {
		stderrln(c, lensEcho(lens))
	}
	for _, line := range tokens.lines() {
		stderrln(c, line)
	}

	if claimed != nil {
		out, err := absPath(claimed.Path)
		if err != nil {
			return err
		}
		stdoutln(c, out)
		return nil
	}
	return emptyQueueError(applyLens, lens, blocked, readyOutsideLens)
}

func (e *engine) claimTicket(ctx context.Context, c *cobra.Command, scope, dir string, autoCommit bool, p *index.Ticket, root string, hasRoot bool) (bool, error) {
	m, body, err := readTicketFile(p.Path)
	if err != nil {
		return false, nil
	}
	if m.Status != status.Todo {
		return false, nil
	}
	m.Status = status.InProgress
	if err := writeTicketFile(p.Path, m, body); err != nil {
		return false, err
	}
	if err := e.rec.SyncPaths(scope, writtenPaths(p.Path, "")); err != nil {
		return false, err
	}
	message := fmt.Sprintf("tk: %s -> %s", p.ID, status.InProgress)
	if err := e.completeStateDurability(ctx, c, scope, dir, autoCommit, message, p.Path, "", root, hasRoot); err != nil {
		return false, err
	}
	return true, nil
}
