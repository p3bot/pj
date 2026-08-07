package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/p3bot/pj/internal/git"
	"github.com/p3bot/pj/internal/gitstate"
	"github.com/p3bot/pj/internal/reconcile"
	"github.com/p3bot/pj/internal/scopefile"
	"github.com/p3bot/pj/internal/selfcommit"
	"github.com/p3bot/pj/internal/token"
)

// refuseUnusableScope refuses writes when the dir is unreachable or pj.cue is unusable.
func refuseUnusableScope(res *reconcile.Result, scope, dir string) error {
	if res.Unreachable[scope] {
		return fmt.Errorf("%s", token.Line(token.UnreachableScope,
			fmt.Sprintf("%s: dir %s is not reachable", scope, dir)))
	}
	if cfgErr, ok := res.ConfigErrs[scope]; ok {
		return fmt.Errorf("%s", token.Line(token.ConfigUnparseable,
			fmt.Sprintf("%s (%s): %s — fix pj.cue before writing", scope, cfgErr.Dir, cfgErr.Reason)))
	}
	return nil
}

// checkMidRebase refuses auto-commit writes on a mid-rebase git-root (repo-granular).
func checkMidRebase(ctx context.Context, scope string, autoCommit bool, root string, hasRoot bool) error {
	if !autoCommit || !hasRoot {
		return nil
	}
	if !git.MidRebase(ctx, root) {
		return nil
	}
	where := "the conflicted file"
	if files := git.UnmergedFiles(ctx, root); len(files) > 0 {
		where = strings.Join(files, ", ")
	}
	return fmt.Errorf("%s is mid-sync-conflict in shared repo %s — resolve %s then run pj sync",
		scope, root, where)
}

// completeStateDurability: auto-commit self-commits or rides sync_disabled; else uncommitted:.
func (e *engine) completeStateDurability(ctx context.Context, c *cobra.Command, scope, dir string, autoCommit bool, message, newPath, oldPath, root string, hasRoot bool) error {
	if !autoCommit {
		e.repoDirtyHealth(ctx, c, dir, root, hasRoot)
		return nil
	}
	if !hasRoot {
		stderrln(c, token.Line(token.SyncDisabled,
			fmt.Sprintf("%s: no git repository for %s — files written but not committed", scope, dir)))
		return nil
	}
	req := selfcommit.Request{
		StateDir: e.app.StateDir,
		GitRoot:  root,
		Message:  message,
		NewPath:  newPath,
		OldPath:  oldPath,
	}
	if err := selfcommit.Commit(ctx, req); err != nil {
		return fmt.Errorf("self-commit %s: %w", scope, err)
	}
	if detail, present := gitstate.ReadLastPushError(e.app.StateDir, root); present {
		stderrln(c, fmt.Sprintf("note: %s has a failed push on record (%s) — run pj sync", scope, detail))
	}
	return nil
}

// createDurability: create never self-commits; terminal scaffolds get a durability note.
func (e *engine) createDurability(ctx context.Context, c *cobra.Command, dir string, autoCommit, terminal bool, fullID, root string, hasRoot bool) {
	if terminal {
		stderrln(c, fmt.Sprintf("note: %s scaffolded under archive/ — a terminal create is not git-durable until pj sync (auto-commit) or a host commit", fullID))
	}
	if !autoCommit {
		e.repoDirtyHealth(ctx, c, dir, root, hasRoot)
	}
}

// repoDirtyHealth rides uncommitted: for repo-driven scopes (detect-only).
func (e *engine) repoDirtyHealth(ctx context.Context, c *cobra.Command, dir, root string, hasRoot bool) {
	n := scopefile.CountAllowlistedDirty(ctx, dir, root, hasRoot)
	if n > 0 {
		stderrln(c, token.Line(token.Uncommitted,
			fmt.Sprintf("%d allowlisted path(s) under %s uncommitted — commit with the host repo", n, dir)))
	}
}
