package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/start-cli/pj/internal/flock"
	"github.com/start-cli/pj/internal/git"
	"github.com/start-cli/pj/internal/gitroot"
	"github.com/start-cli/pj/internal/gitstate"
	"github.com/start-cli/pj/internal/id"
	"github.com/start-cli/pj/internal/reconcile"
	"github.com/start-cli/pj/internal/selfcommit"
	"github.com/start-cli/pj/internal/slug"
	"github.com/start-cli/pj/internal/token"
)

// Per-scope flock: writers hold it across the whole reconcile→read→write span.
const scopeLockName = ".pj.lock"

func acquireScopeLock(dir string) (*flock.Lock, error) {
	return flock.Acquire(filepath.Join(dir, scopeLockName))
}

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

// gitRootFor resolves once per write command so every durability helper agrees.
func gitRootFor(dir string) (root string, hasRoot bool) {
	if !git.Available() {
		return "", false
	}
	return gitroot.RepoRoot(dir)
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
	n := countAllowlistedDirty(ctx, dir, root, hasRoot)
	if n > 0 {
		stderrln(c, token.Line(token.Uncommitted,
			fmt.Sprintf("%d allowlisted path(s) under %s uncommitted — commit with the host repo", n, dir)))
	}
}

func countAllowlistedDirty(ctx context.Context, dir, root string, hasRoot bool) int {
	if !hasRoot {
		return 0
	}
	dirty, err := git.DirtyPaths(ctx, root, dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, p := range dirty {
		if isAllowlistedScopeFile(p, dir) {
			n++
		}
	}
	return n
}

// isAllowlistedScopeFile: project .md at dir root or archive/, or pj.cue/.gitignore at root.
func isAllowlistedScopeFile(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	base := filepath.Base(rel)
	switch filepath.Dir(rel) {
	case ".":
		return base == "pj.cue" || base == ".gitignore" || looksLikeProjectFile(base)
	case "archive":
		return looksLikeProjectFile(base)
	default:
		return false
	}
}

// looksLikeProjectFile: first two hyphen segments form a full id; optional slug tail must be valid.
func looksLikeProjectFile(base string) bool {
	stem, ok := strings.CutSuffix(base, ".md")
	if !ok {
		return false
	}
	parts := strings.SplitN(stem, "-", 3)
	if len(parts) < 2 {
		return false
	}
	if !id.IsFullProjectID(parts[0] + "-" + parts[1]) {
		return false
	}
	if len(parts) == 3 {
		return slug.Valid(parts[2])
	}
	return true
}
