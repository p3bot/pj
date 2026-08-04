package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/p3bot/pj/internal/gitroot"
	"github.com/p3bot/pj/internal/resolve"
	"github.com/p3bot/pj/internal/token"
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

	sel, err := e.selectSyncTargets(scopeFlag, all)
	if err != nil {
		return err
	}

	for _, msg := range sel.unreachable {
		stderrln(c, msg)
	}

	if sel.candidates == 0 {
		if len(sel.unreachable) > 0 {
			stderrln(c, "nothing to sync: every registered auto-commit scope is unreachable")
		} else {
			stderrln(c, "nothing to sync: no auto-commit git-roots registered")
		}
		return nil
	}

	needsAttention := false
	for _, msg := range sel.disabled {
		stderrln(c, msg)
		needsAttention = true
	}

	for _, msg := range sel.configErrs {
		stderrln(c, msg)
		needsAttention = true
	}

	for _, t := range sel.targets {
		if e.syncRoot(c, t) == outcomeNeedsAttention {
			needsAttention = true
		}
	}

	if needsAttention {
		return &ExitError{Code: exitFailure, Plain: true, Err: errors.New("pj sync: one or more roots need attention (see the lines above)")}
	}
	return nil
}

type participant struct {
	name string
	dir  string
}

type syncTarget struct {
	root         string
	participants []participant
}

type selection struct {
	targets     []syncTarget
	disabled    []string
	configErrs  []string
	unreachable []string
	candidates  int
}

// selectSyncTargets: --all/no ambient fans out; ambient hard-refuses non-auto-commit.
func (e *engine) selectSyncTargets(scopeFlag string, all bool) (selection, error) {
	if !all {
		resolved, err := e.resolveAmbient(scopeFlag)
		switch {
		case err == nil:
			return e.ambientSelection(resolved)
		case errors.Is(err, resolve.ErrNoScope):
			// No ambient scope: fall through to every auto-commit git-root.
		default:
			return selection{}, err
		}
	}
	return e.allSelection(), nil
}

func (e *engine) ambientSelection(resolved *resolve.Resolved) (selection, error) {
	scope := resolved.Name
	dir := resolved.Entry.Dir
	root, hasRoot := gitRootFor(dir)

	res, err := e.reconcileResult(single(scope, dir))
	if err != nil {
		return selection{}, err
	}
	if res.Unreachable[scope] {
		return selection{}, fmt.Errorf("%s", token.Line(token.UnreachableScope,
			fmt.Sprintf("%s: dir %s is not reachable — cannot sync", scope, dir)))
	}
	cfgErr, badConfig := res.ConfigErrs[scope]
	switch {
	case badConfig && !hasRoot:
		return selection{}, fmt.Errorf("%s", token.Line(token.ConfigUnparseable, fmt.Sprintf(
			"%s (%s): %s — fix pj.cue before sync can evaluate this scope", scope, cfgErr.Dir, cfgErr.Reason)))
	case badConfig:
	case !schemaAutoCommit(res.Schema(scope)):
		return selection{}, e.nonAutoCommitRefusal(scope, hasRoot)
	}
	if !hasRoot {
		return selection{candidates: 1, disabled: []string{syncDisabledLine(scope, dir)}}, nil
	}
	parts := e.autoCommitParticipants(root)
	return selection{candidates: 1, targets: []syncTarget{{root: root, participants: parts}}}, nil
}

func (e *engine) allSelection() selection {
	var sel selection
	byRoot := map[string][]participant{}
	type badConfig struct {
		scope, dir, reason, root string
		hasRoot                  bool
	}
	var badConfigs []badConfig
	for _, scope := range e.sortedRegistered() {
		dir := e.reg.Scopes[scope].Dir
		if _, err := os.Stat(dir); err != nil {
			sel.unreachable = append(sel.unreachable, token.Line(token.UnreachableScope,
				fmt.Sprintf("%s: dir %s is not reachable — skipped", scope, dir)))
			continue
		}
		schema, cfgErr := e.rec.SchemaOrError(scope, dir)
		if cfgErr != nil {
			root, hasRoot := gitRootFor(dir)
			badConfigs = append(badConfigs, badConfig{scope: scope, dir: cfgErr.Dir, reason: cfgErr.Reason, root: root, hasRoot: hasRoot})
			continue
		}
		if schema == nil || !schema.AutoCommit {
			continue // non-auto-commit: not this command's business
		}
		sel.candidates++
		root, hasRoot := gitRootFor(dir)
		if !hasRoot {
			sel.disabled = append(sel.disabled, syncDisabledLine(scope, dir))
			continue
		}
		byRoot[root] = append(byRoot[root], participant{name: scope, dir: dir})
	}
	roots := make([]string, 0, len(byRoot))
	for root := range byRoot {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	for _, root := range roots {
		parts := byRoot[root]
		sort.Slice(parts, func(i, j int) bool { return parts[i].name < parts[j].name })
		sel.targets = append(sel.targets, syncTarget{root: root, participants: parts})
	}
	for _, bc := range badConfigs {
		if bc.hasRoot {
			if _, covered := byRoot[bc.root]; covered {
				continue // the per-root preflight already refuses this root by name
			}
		}
		sel.candidates++
		sel.configErrs = append(sel.configErrs, token.Line(token.ConfigUnparseable,
			fmt.Sprintf("%s (%s): %s — fix pj.cue before sync can evaluate this scope", bc.scope, bc.dir, bc.reason)))
	}
	return sel
}

func (e *engine) autoCommitParticipants(root string) []participant {
	var parts []participant
	for _, scope := range e.sortedRegistered() {
		dir := e.reg.Scopes[scope].Dir
		sgr, ok := gitroot.RepoRoot(dir)
		if !ok || sgr != root {
			continue
		}
		schema, cfgErr := e.rec.SchemaOrError(scope, dir)
		if cfgErr != nil || schema == nil || !schema.AutoCommit {
			continue
		}
		parts = append(parts, participant{name: scope, dir: dir})
	}
	return parts
}

func (e *engine) nonAutoCommitRefusal(scope string, hasRoot bool) error {
	if hasRoot {
		return fmt.Errorf("sync is for auto-commit scopes only — %s is repo-driven; commit its project files with the host repo", scope)
	}
	return fmt.Errorf("sync is for auto-commit scopes only — %s is plain-files; there is no pj sync — run pj doctor if integrity warnings appear", scope)
}

func syncDisabledLine(scope, dir string) string {
	return token.Line(token.SyncDisabled,
		fmt.Sprintf("%s: no git repository with a remote for %s — set one up, then pj sync", scope, dir))
}
