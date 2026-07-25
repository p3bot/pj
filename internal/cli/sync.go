package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/start-cli/pj/internal/gitroot"
	"github.com/start-cli/pj/internal/resolve"
	"github.com/start-cli/pj/internal/token"
)

// newSyncCmd builds the sole push boundary. pj sync targets the ambient scope's
// git-root (or every auto-commit git-root under --all or no ambient scope), snapshots
// allowlisted dirt in one commit, fetches and integrates unconditionally, runs the
// sync-time integrity repairs, and pushes if ahead. It applies only to
// autoCommit: true scopes.
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

	// A run-level note per registered scope pj could not stat: skipped, never a refuse,
	// so one unmounted drive never blocks a healthy repo (requirement 2). Emitted before
	// the empty-set check below so an all-unreachable fleet still surfaces why nothing
	// synced — an unreachable scope is skipped before it can be counted a candidate, so
	// without this the notes would be dropped and the run would claim nothing is
	// registered when a real scope is merely unmounted.
	for _, msg := range sel.unreachable {
		stderrln(c, msg)
	}

	// Empty eligible set (no auto-commit scopes pj could evaluate) is not a failure —
	// discovery friendliness: an empty fleet exits 0 with a terse note. It is distinct
	// from an auto-commit scope that lacks a repo (sync_disabled below), which is a
	// candidate. Unreachable is a non-fatal skip (requirement 2), so this still exits 0;
	// the message only distinguishes "none registered" from "all registered ones
	// unreachable" so the terse line is not misleading.
	if sel.candidates == 0 {
		if len(sel.unreachable) > 0 {
			stderrln(c, "nothing to sync: every registered auto-commit scope is unreachable")
		} else {
			stderrln(c, "nothing to sync: no auto-commit git-roots registered")
		}
		return nil
	}

	needsAttention := false
	// An auto-commit scope that is a candidate but lacks a git repo/upstream (or git is
	// absent): reported with sync_disabled and counted as needing attention — the same
	// fail-closed outcome the ambient case carries (requirement 1).
	for _, msg := range sel.disabled {
		stderrln(c, msg)
		needsAttention = true
	}

	// An auto-commit-intent scope whose pj.cue will not parse and which no per-root
	// preflight reaches (a root with no healthy participant): surfaced here rather than
	// dropped, and counted as needing attention — the same fail-closed outcome the ambient
	// path's preflight refuse carries.
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

// participant is one auto-commit scope sharing a git-root: the snapshot dir the root's
// sweep covers and the scope lock the span holds.
type participant struct {
	name string
	dir  string
}

// syncTarget is one git-root unit of work and the auto-commit scopes that share it,
// sorted by scope name so the lock order and the snapshot are deterministic.
type syncTarget struct {
	root         string
	participants []participant
}

// selection is the resolved plan for one pj sync invocation: the git-roots to run, the
// sync_disabled, config_unparseable, and unreachable notes discovered while selecting,
// and the count of auto-commit scopes considered (zero means the empty-eligible-set exit
// 0).
type selection struct {
	targets     []syncTarget
	disabled    []string
	configErrs  []string
	unreachable []string
	candidates  int
}

// selectSyncTargets resolves the invocation's git-roots. --all (which wins over any
// ambient selector) and a missing ambient scope both fan out to every auto-commit
// git-root; an ambient scope targets its own root as the single unit. The returned
// error is the ambient-only hard refuse — a non-auto-commit mode error, name drift, or
// an unreachable/unusable ambient target — which fails the command outright.
func (e *engine) selectSyncTargets(scopeFlag string, all bool) (selection, error) {
	if !all {
		resolved, err := e.resolveAmbient(scopeFlag)
		switch {
		case err == nil:
			return e.ambientSelection(resolved)
		case errors.Is(err, resolve.ErrNoScope):
			// No ambient scope: fall through to every auto-commit git-root.
		default:
			// A name-drifted ambient scope fails closed here exactly as P2's resolve does;
			// its DriftError already carries the name_drift line and the recovery.
			return selection{}, err
		}
	}
	return e.allSelection(), nil
}

// ambientSelection builds the single-root plan for an ambient scope. A non-auto-commit
// scope is refused with the mode-named error; an unreachable or unusable target fails;
// an auto-commit scope with no git-root rides sync_disabled; otherwise its git-root is
// the one target and every auto-commit scope sharing it participates.
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
		// autoCommit is unreadable — the same fail-closed class as a mismatch — and with no
		// git-root there is no per-root preflight to catch it. Refusing here is what keeps a
		// broken pj.cue from being reported as a missing repo by the sync_disabled line below,
		// which would assert a cause it never checked and send the operator to set up a repo
		// the scope does not yet need. --all's own sweep over uncovered config errors already
		// does this; without it the same scope is diagnosed differently by flag.
		return selection{}, fmt.Errorf("%s", token.Line(token.ConfigUnparseable, fmt.Sprintf(
			"%s (%s): %s — fix pj.cue before sync can evaluate this scope", scope, cfgErr.Dir, cfgErr.Reason)))
	case badConfig:
		// A git-root exists, so the per-root preflight refuses the whole root by name and
		// reports every sibling violating the shared-repo invariant (requirement 2). Fall
		// through to it rather than pre-empting it with a single-scope refuse.
	case !schemaAutoCommit(res.Schema(scope)):
		return selection{}, e.nonAutoCommitRefusal(scope, hasRoot)
	}
	// The scope is auto-commit, or unparseable with a git-root the preflight will refuse
	// whole. Without a git-root there is nothing to push: ride sync_disabled.
	if !hasRoot {
		return selection{candidates: 1, disabled: []string{syncDisabledLine(scope, dir)}}, nil
	}
	parts := e.autoCommitParticipants(root)
	return selection{candidates: 1, targets: []syncTarget{{root: root, participants: parts}}}, nil
}

// allSelection builds the plan for every registered auto-commit scope, grouped by
// git-root and deduplicated so a shared repo is visited once. Non-auto-commit scopes
// are skipped, an unparseable config is left to the per-root preflight, an unreachable
// dir rides a note, and an auto-commit scope with no git-root rides sync_disabled.
func (e *engine) allSelection() selection {
	var sel selection
	byRoot := map[string][]participant{}
	// A scope whose pj.cue will not parse: autoCommit cannot be read, so it is set aside
	// here and surfaced after the plan is built — see the badConfig sweep below.
	type badConfig struct {
		scope, dir, reason, root string
		hasRoot                  bool
	}
	var badConfigs []badConfig
	for _, scope := range e.sortedRegistered() {
		dir := e.reg.Scopes[scope].Dir
		if _, err := os.Stat(dir); err != nil {
			// Cannot stat the dir: the preflight's premise — that a git-root is derivable
			// from it — cannot even be evaluated, so the scope is skipped with a note, never
			// fail-closed. One unmounted drive must not block every other repo (requirement 2).
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
	// A scope whose pj.cue will not parse cannot be evaluated for autoCommit. When it
	// shares a git-root a healthy sibling put on the plan, the per-root preflight already
	// refuses the whole root by name; but a root with no healthy participant is never
	// preflighted, so surface the config error here (and count it, so an all-unparseable
	// fleet exits non-zero rather than through the misleading empty-set branch) rather
	// than dropping the scope silently.
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

// autoCommitParticipants is every registered auto-commit scope whose dir derives root,
// sorted by name. An unparseable or unreachable sibling is excluded here (it cannot be
// a snapshot dir), but the per-root preflight still scans it to refuse the whole root.
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

// nonAutoCommitRefusal is the mode-named refuse for an ambient non-auto-commit scope:
// repo-driven when a git-root exists (the host repo owns the commit), plain-files when
// none does (there is no pj sync at all).
func (e *engine) nonAutoCommitRefusal(scope string, hasRoot bool) error {
	if hasRoot {
		return fmt.Errorf("sync is for auto-commit scopes only — %s is repo-driven; commit its project files with the host repo", scope)
	}
	return fmt.Errorf("sync is for auto-commit scopes only — %s is plain-files; there is no pj sync — run pj doctor if integrity warnings appear", scope)
}

// syncDisabledLine is the sync_disabled diagnostic for an auto-commit scope lacking a
// git repository with an upstream (or on a machine with no git) — the token, never a
// raw git or exec error.
func syncDisabledLine(scope, dir string) string {
	return token.Line(token.SyncDisabled,
		fmt.Sprintf("%s: no git repository with a remote for %s — set one up, then pj sync", scope, dir))
}
