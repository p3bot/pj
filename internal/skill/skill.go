// Package skill holds the agent skill contract printed by `pj skill`.
//
// skill.md is the sole source of the contract text: it is embedded into the
// binary and emitted to stdout. Edit skill.md when a locked agent rule changes.
// This package must not load any external design document.
package skill

import (
	_ "embed"
	"strings"
)

//go:embed skill.md
var embedded string

// requiredHeadings is the v1 locked TOC the embedded contract must contain, in
// order. Tests assert skill.md matches this list; the runtime only prints the
// file.
var requiredHeadings = []string{
	"Core work loop",
	"Capture",
	"Frontmatter mutation",
	"Body conventions",
	"Title, slug, and filename",
	"Ordering",
	"List and filters",
	"Search",
	"Dependencies and impact",
	"Archive",
	"End of turn (by autoCommit mode)",
	"Conflicts and paused sync",
	"Concurrent agents",
	"Cold start and import",
	"Cross-scope work",
	"Waiting and external blockers",
	"Unsupported operations",
	"Doctor and integrity warnings",
}

// Text returns the full agent skill contract as markdown, ready for stdout.
// A single trailing newline is guaranteed so writers do not double-terminate.
func Text() string {
	s := embedded
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

// RequiredHeadings returns the v1 section titles the contract must include, in
// order. Used by tests and callers that assert structure without parsing markdown.
func RequiredHeadings() []string {
	out := make([]string, len(requiredHeadings))
	copy(out, requiredHeadings)
	return out
}
