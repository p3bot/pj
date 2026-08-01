// Package pathutil holds boundary-safe path predicates over cleaned absolute paths.
// Comparisons use an explicit separator boundary so "/a/bc" is never nested under "/a/b".
package pathutil

import (
	"os"
	"strings"
)

// UnderOrEqual reports whether child is ancestor or lies within it, on a path-separator boundary.
func UnderOrEqual(child, ancestor string) bool {
	if child == ancestor {
		return true
	}
	sep := string(os.PathSeparator)
	if !strings.HasSuffix(ancestor, sep) {
		ancestor += sep
	}
	return strings.HasPrefix(child, ancestor)
}

// Overlap reports whether a and b are equal or one is nested within the other.
func Overlap(a, b string) bool {
	return UnderOrEqual(a, b) || UnderOrEqual(b, a)
}
