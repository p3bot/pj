// Package git wraps the external git binary for path-scoped staging, commits,
// porcelain status, and mid-rebase detection. Commands run with cmd.Dir set to a
// caller-supplied git-root; pj never writes under .git/.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Available reports whether the git binary is on PATH.
// Callers treat absence like a missing git-root: writes still land, commit is skipped.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// run executes git with cmd.Dir=gitRoot; non-zero exit surfaces git's stderr.
func run(ctx context.Context, gitRoot string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = gitRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.Bytes(), fmt.Errorf("git %s: %s", args[0], msg)
	}
	return stdout.Bytes(), nil
}

// Add stages exactly the given pathspecs — never the whole tree.
// Staging specific paths leaves unrelated dirty files untouched and records
// deletions of tracked-but-gone paths.
func Add(ctx context.Context, gitRoot string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	_, err := run(ctx, gitRoot, args...)
	return err
}

// Commit records the staged changes with the given fixed message. Never pushes.
func Commit(ctx context.Context, gitRoot, message string) error {
	_, err := run(ctx, gitRoot, "commit", "-m", message)
	return err
}

// Tracked reports whether path is in gitRoot's index.
// Lets a never-committed old path be omitted from pathspec rather than erroring.
func Tracked(ctx context.Context, gitRoot, path string) bool {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--error-unmatch", "--", path)
	cmd.Dir = gitRoot
	return cmd.Run() == nil
}

// HasStagedChanges reports whether the index differs from HEAD.
// Self-commit checks before committing so a byte-identical rewrite is a clean no-op.
func HasStagedChanges(ctx context.Context, gitRoot string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet")
	cmd.Dir = gitRoot
	err := cmd.Run()
	if err == nil {
		return false, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return true, nil
	}
	return false, fmt.Errorf("git diff --cached: %w", err)
}

// HasUpstream reports whether the current branch has a configured upstream.
// Any error (no upstream, detached HEAD, no repo) is false.
func HasUpstream(ctx context.Context, gitRoot string) bool {
	_, err := run(ctx, gitRoot, "rev-parse", "--abbrev-ref", "@{u}")
	return err == nil
}

// MidRebase reports whether gitRoot is mid-rebase (rebase-merge or rebase-apply).
// Resolves the git dir via rev-parse so worktrees and submodules are correct;
// probe errors return false so callers refuse only on affirmative true.
func MidRebase(ctx context.Context, gitRoot string) bool {
	for _, marker := range []string{"rebase-merge", "rebase-apply"} {
		out, err := run(ctx, gitRoot, "rev-parse", "--git-path", marker)
		if err != nil {
			continue
		}
		p := strings.TrimSpace(string(out))
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(gitRoot, p)
		}
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// UnmergedFiles lists repo-relative paths currently unmerged (diff-filter=U).
// Best-effort: empty slice on any error.
func UnmergedFiles(ctx context.Context, gitRoot string) []string {
	out, err := run(ctx, gitRoot, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil
	}
	return nonEmptyLines(string(out))
}

// DirtyPaths returns absolute paths under dir that git status reports as dirty.
// Thin projection of DirtyEntries without the porcelain status code.
func DirtyPaths(ctx context.Context, gitRoot, dir string) ([]string, error) {
	entries, err := DirtyEntries(ctx, gitRoot, dir)
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	return paths, nil
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
