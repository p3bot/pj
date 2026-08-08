package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/p3bot/tk/internal/git"
	"github.com/p3bot/tk/internal/gitstate"
	"github.com/p3bot/tk/internal/reconcile"
	"github.com/p3bot/tk/internal/scopefile"
	"github.com/p3bot/tk/internal/selfcommit"
	"github.com/p3bot/tk/internal/token"
)

// refuseUnusableScope refuses writes when the dir is unreachable or tk.cue is unusable.
func refuseUnusableScope(res *reconcile.Result, scope, dir string) error {
	if res.Unreachable[scope] {
		return fmt.Errorf("%s", token.Line(token.UnreachableScope,
			fmt.Sprintf("%s: dir %s is not reachable", scope, dir)))
	}
	if cfgErr, ok := res.ConfigErrs[scope]; ok {
		return fmt.Errorf("%s", token.Line(token.ConfigUnparseable,
			fmt.Sprintf("%s (%s): %s — fix tk.cue before writing", scope, cfgErr.Dir, cfgErr.Reason)))
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
	return fmt.Errorf("%s is mid-sync-conflict in shared repo %s — resolve %s then run tk sync",
		scope, root, where)
}

// completeStateDurability: auto-commit self-commits or rides sync_disabled;
// repo-driven writes stay quiet (host git owns durability).
func (e *engine) completeStateDurability(ctx context.Context, c *cobra.Command, scope, dir string, autoCommit bool, message, newPath, oldPath, root string, hasRoot bool) error {
	if !autoCommit {
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
	e.tkDrivenSyncNeeded(ctx, c, dir, root)
	return nil
}

// createDurability: create never self-commits; terminal scaffolds get a durability note.
// Repo-driven writes stay quiet; tk-driven with a git-root may ride sync_needed:.
func (e *engine) createDurability(ctx context.Context, c *cobra.Command, dir string, autoCommit, terminal bool, fullID, root string, hasRoot bool) {
	if terminal {
		stderrln(c, fmt.Sprintf("note: %s scaffolded under archive/ — a terminal create is not git-durable until tk sync (auto-commit) or a host commit", fullID))
	}
	if !autoCommit || !hasRoot {
		return
	}
	e.tkDrivenSyncNeeded(ctx, c, dir, root)
}

// tkDrivenSyncNeeded emits at most one sync_needed: line after a tk-driven write.
// Priority when several conditions hold: push failed, then dirty, then unpushed.
// Reason strings are catalogue-stable: "push failed", "dirty", "unpushed".
func (e *engine) tkDrivenSyncNeeded(ctx context.Context, c *cobra.Command, dir, root string) {
	if _, present := gitstate.ReadLastPushError(e.app.StateDir, root); present {
		stderrln(c, token.Line(token.SyncNeeded, "push failed"))
		return
	}
	if n := scopefile.CountAllowlistedDirty(ctx, dir, root, true); n > 0 {
		stderrln(c, token.Line(token.SyncNeeded, "dirty"))
		return
	}
	if n, err := git.UnpushedCount(ctx, root); err == nil && n > 0 {
		stderrln(c, token.Line(token.SyncNeeded, "unpushed"))
	}
}
