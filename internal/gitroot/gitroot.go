// Package gitroot derives a directory's enclosing git repository root via the external git binary.
// Missing repo, missing dir, and missing git binary all return ok=false with no error distinction.
package gitroot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/p3bot/tk/internal/pathutil"
)

// RepoRoot returns the cleaned, symlink-resolved absolute path of the git repository containing dir.
// Canonicalisation matches CLI path hand-off so gitstate keys and registry roots agree on macOS
// (/var vs /private/var) and other symlink-spelled temp roots.
// ok is false — never an error — when dir is outside any repo, does not exist, or git is not on PATH.
func RepoRoot(dir string) (root string, ok bool) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	root = strings.TrimSpace(string(out))
	if root == "" {
		return "", false
	}
	return pathutil.Canonical(root), true
}

// RepoRootForNew derives the git root a not-yet-created dir would belong to by
// walking up to the nearest existing ancestor. Same uniform (root, ok) contract as RepoRoot.
func RepoRootForNew(dir string) (root string, ok bool) {
	for {
		if _, err := os.Stat(dir); err == nil {
			return RepoRoot(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
