// Package pathutil holds boundary-safe path predicates over filesystem paths.
// Comparisons canonicalise both sides (clean + symlink-resolve) then use an
// explicit separator boundary so "/a/bc" is never nested under "/a/b".
package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

// Canonical cleans abs and symlink-resolves the longest existing prefix so that
// macOS /var vs /private/var (and other symlink spellings of the same directory)
// collapse to one form. A missing tail is rejoined after the resolved prefix.
// Callers should pass an absolute path; relative inputs are cleaned only.
func Canonical(abs string) string {
	abs = filepath.Clean(abs)
	var tail []string
	cur := abs
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}

// UnderOrEqual reports whether child is the same as ancestor or lies within it
// after both paths are canonicalised, on a path-separator boundary.
func UnderOrEqual(child, ancestor string) bool {
	child = Canonical(child)
	ancestor = Canonical(ancestor)
	if child == ancestor {
		return true
	}
	sep := string(os.PathSeparator)
	if !strings.HasSuffix(ancestor, sep) {
		ancestor += sep
	}
	return strings.HasPrefix(child, ancestor)
}

// Overlap reports whether a and b are equal or one is nested within the other
// (both sides canonicalised).
func Overlap(a, b string) bool {
	return UnderOrEqual(a, b) || UnderOrEqual(b, a)
}
