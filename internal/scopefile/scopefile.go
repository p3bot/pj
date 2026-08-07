// Package scopefile holds scope-directory file policy shared by write verbs,
// doctor, and sync: allowlist classification, dirty counting, and the per-scope
// flock path. One definition so snapshot, uncommitted:, non_allowlist:, and
// status stay on the same product rule.
package scopefile

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/p3bot/pj/internal/flock"
	"github.com/p3bot/pj/internal/git"
	"github.com/p3bot/pj/internal/gitroot"
	"github.com/p3bot/pj/internal/id"
	"github.com/p3bot/pj/internal/slug"
)

// LockName is the per-scope advisory lock file at the scope dir root.
const LockName = ".pj.lock"

// AcquireLock takes the exclusive per-scope flock at dir/LockName.
// The scope directory must already exist.
func AcquireLock(dir string) (*flock.Lock, error) {
	return flock.Acquire(filepath.Join(dir, LockName))
}

// GitRoot resolves the enclosing git repository once so durability helpers agree.
// Returns ok=false when git is unavailable or dir is outside any repo.
func GitRoot(dir string) (root string, ok bool) {
	if !git.Available() {
		return "", false
	}
	return gitroot.RepoRoot(dir)
}

// CountAllowlistedDirty counts dirty paths under dir that pass IsAllowlisted.
// Returns 0 when there is no git root or DirtyPaths fails.
func CountAllowlistedDirty(ctx context.Context, dir, root string, hasRoot bool) int {
	if !hasRoot {
		return 0
	}
	dirty, err := git.DirtyPaths(ctx, root, dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, p := range dirty {
		if IsAllowlisted(p, dir) {
			n++
		}
	}
	return n
}

// IsAllowlisted reports whether path is a project .md at dir root or archive/,
// or pj.cue/.gitignore at root.
func IsAllowlisted(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	base := filepath.Base(rel)
	switch filepath.Dir(rel) {
	case ".":
		return base == "pj.cue" || base == ".gitignore" || LooksLikeProject(base)
	case "archive":
		return LooksLikeProject(base)
	default:
		return false
	}
}

// LooksLikeProject reports whether base is a project filename: full-id stem
// with optional valid slug tail.
func LooksLikeProject(base string) bool {
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
