package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/p3bot/pj/internal/resolve"
	"github.com/p3bot/pj/internal/syncengine"
)

// newSyncCmd is the sole push boundary (autoCommit scopes only).
func newSyncCmd(app *App) *cobra.Command {
	var scope string
	var all bool
	cmd := &cobra.Command{
		Use:   "sync [--scope S] [--all]",
		Short: "Snapshot, fetch, integrate, repair, and push an auto-commit git-root",
		Long: "Sync is pj's only push boundary and applies only to auto-commit scopes. It\n" +
			"snapshots allowlisted dirty files in one commit, fetches and rebases the remote\n" +
			"in, resolves frontmatter conflicts, runs the sync-time integrity repairs, and\n" +
			"pushes if ahead. With no flag it targets the ambient scope's whole git-root;\n" +
			"--all (which wins over --scope/PJ_SCOPE) syncs every auto-commit git-root, each\n" +
			"an independent unit whose failure never strands the others. A non-auto-commit\n" +
			"scope is refused (ambient) or skipped (--all); an empty auto-commit set exits 0.",
		Args: usageArgs(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			return runSync(app, c, scope, all)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "ambient scope whose git-root is targeted (ignored under --all)")
	cmd.Flags().BoolVar(&all, "all", false, "sync every auto-commit git-root (wins over --scope/PJ_SCOPE)")
	return cmd
}

func runSync(app *App, c *cobra.Command, scopeFlag string, all bool) error {
	e, err := app.openEngine(c)
	if err != nil {
		return err
	}
	defer e.close()

	in, err := e.syncInput(scopeFlag, all)
	if err != nil {
		return err
	}

	deps := syncengine.Deps{
		Ctx:      c.Context(),
		Cue:      e.app.Ctx,
		StateDir: e.app.StateDir,
		Reg:      e.reg,
		DB:       e.db,
		Rec:      e.rec,
	}
	result, err := syncengine.Run(deps, cobraReporter{c: c}, in)
	if err != nil {
		return err
	}
	if result.NeedsAttention {
		return &ExitError{Code: exitFailure, Plain: true, Err: syncengine.ErrNeedsAttention}
	}
	return nil
}

// syncInput maps the three CLI invocation shapes onto the two package inputs.
// Ambient success → ambient; --all or resolve.ErrNoScope → all-registered;
// other resolve errors fail here before the package runs.
func (e *engine) syncInput(scopeFlag string, all bool) (syncengine.Input, error) {
	if all {
		return syncengine.Input{AllRegistered: true}, nil
	}
	resolved, err := e.resolveAmbient(scopeFlag)
	switch {
	case err == nil:
		return syncengine.Input{Ambient: &syncengine.AmbientScope{
			Name: resolved.Name,
			Dir:  resolved.Entry.Dir,
		}}, nil
	case errors.Is(err, resolve.ErrNoScope):
		return syncengine.Input{AllRegistered: true}, nil
	default:
		return syncengine.Input{}, err
	}
}
