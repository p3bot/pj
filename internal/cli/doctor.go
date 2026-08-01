package cli

import (
	"errors"
	"sort"

	"github.com/spf13/cobra"

	"github.com/start-cli/pj/internal/resolve"
)

// doctorFlags: bare doctor diagnoses only; --repair / --re-space-order mutate; --reindex rebuilds.
type doctorFlags struct {
	reindex      bool
	repair       bool
	reSpaceOrder bool
	all          bool
}

func (f doctorFlags) mutating() bool { return f.repair || f.reSpaceOrder }

func newDoctorCmd(app *App) *cobra.Command {
	var f doctorFlags
	cmd := &cobra.Command{
		Use:   "doctor [--reindex] [--repair] [--re-space-order] [--all]",
		Short: "Diagnose integrity, and optionally repair, across scopes",
		Long: "Diagnose every integrity class over the ambient scope (or every registered\n" +
			"scope when there is none), reporting each with its stable token. Bare doctor\n" +
			"never mutates project files or pj.cue. --repair fixes id collisions, equal order\n" +
			"keys, and archive layout drift; --re-space-order shortens a band of over-long\n" +
			"order keys; both need a scope (ambient, PJ_SCOPE, or --all) and refuse on a\n" +
			"mid-rebase auto-commit git-root. --reindex rebuilds the index from files. There\n" +
			"is no --scope flag on doctor.",
		Args: usageArgs(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			return runDoctor(app, c, f)
		},
	}
	cmd.Flags().BoolVar(&f.reindex, "reindex", false, "rebuild the index from files (never mutates project files)")
	cmd.Flags().BoolVar(&f.repair, "repair", false, "repair id collisions, equal order keys, and archive layout drift")
	cmd.Flags().BoolVar(&f.reSpaceOrder, "re-space-order", false, "re-space a band of pathologically long order keys")
	cmd.Flags().BoolVar(&f.all, "all", false, "act on every registered scope (mutating flags only)")
	return cmd
}

func runDoctor(app *App, c *cobra.Command, f doctorFlags) error {
	e, err := app.openEngine(c)
	if err != nil {
		return err
	}
	defer e.close()

	reportScopes, mutateScopes, err := e.doctorScopes(f.mutating(), f.all)
	if err != nil {
		return err
	}

	// Repairs first under flock so chosen rows cannot change between read and write.
	if f.mutating() {
		if err := e.runRepairs(c, mutateScopes, f); err != nil {
			return err
		}
	}

	// Rebuild then machine-wide reconcile: Rebuild only drops tables; reconcile repopulates.
	targets := e.targetsFor(reportScopes)
	if f.reindex {
		if err := e.db.Rebuild(); err != nil {
			return err
		}
		targets = e.allTargets()
	}

	// Post-repair reconcile feeds the report; doctor prints its own report, so no ride-along echo.
	res, err := e.reconcileResult(targets)
	if err != nil {
		return err
	}

	report, err := e.diagnose(c, reportScopes, res)
	if err != nil {
		return err
	}
	for _, line := range report {
		stdoutln(c, line)
	}
	if len(report) == 0 {
		stderrln(c, "pj doctor: no integrity issues found")
	}
	return nil
}

// doctorScopes: report is ambient or all; mutate needs ambient/--all (never silent machine-wide).
func (e *engine) doctorScopes(mutating, all bool) (report, mutate []string, err error) {
	name, ok, err := e.ambientForDoctor()
	if err != nil {
		return nil, nil, err
	}
	allRegistered := e.sortedRegistered()

	if ok {
		report = []string{name}
	} else {
		report = allRegistered
	}

	if !mutating {
		return report, nil, nil
	}
	switch {
	case all:
		mutate = allRegistered
	case ok:
		mutate = []string{name}
	default:
		return nil, nil, usageErrorf("pj doctor --repair / --re-space-order needs a scope to act on: run inside a registered code-root, set PJ_SCOPE=<name>, or pass --all")
	}
	return report, mutate, nil
}

func (e *engine) ambientForDoctor() (name string, ok bool, err error) {
	opts, err := ambientOptions("")
	if err != nil {
		return "", false, err
	}
	resolved, err := resolve.Resolve(e.app.Ctx, e.reg, opts)
	if err == nil {
		return resolved.Name, true, nil
	}
	var drift *resolve.DriftError
	if errors.As(err, &drift) {
		return drift.Key, true, nil
	}
	if errors.Is(err, resolve.ErrNoScope) {
		return "", false, nil
	}
	return "", false, err
}

func (e *engine) sortedRegistered() []string {
	names := make([]string, 0, len(e.reg.Scopes))
	for name := range e.reg.Scopes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (e *engine) targetsFor(scopes []string) map[string]string {
	out := make(map[string]string, len(scopes))
	for _, s := range scopes {
		if entry, ok := e.reg.Scopes[s]; ok {
			out[s] = entry.Dir
		}
	}
	return out
}
