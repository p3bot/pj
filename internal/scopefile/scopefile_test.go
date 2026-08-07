package scopefile

import (
	"path/filepath"
	"testing"
)

func TestLooksLikeProject(t *testing.T) {
	cases := []struct {
		base string
		want bool
	}{
		{"wc-ab2c-network-redesign.md", true},
		{"wc-ab2c-x.md", true},
		{"wc-ab2c.md", true},
		{"wc-ab2c-.md", false},     // empty slug tail is not a valid slug
		{"wc-ab2c-Bad.md", false},  // slug must be the closed lowercase grammar
		{"wc-abcdefgh-x.md", true}, // 8-char short id (post-repair length)
		{"wc-9b2c-x.md", false},    // short id must be letter-first
		{"wc-ab2c-x.txt", false},
		{"pj.cue", false},
		{"AGENTS.md", false},
		{"random.md", false},
		{"wc.md", false},
	}
	for _, c := range cases {
		if got := LooksLikeProject(c.base); got != c.want {
			t.Errorf("LooksLikeProject(%q) = %v want %v", c.base, got, c.want)
		}
	}
}

func TestIsAllowlisted(t *testing.T) {
	dir := filepath.FromSlash("/scope/wc")
	cases := []struct {
		rel  string
		want bool
	}{
		{"wc-ab2c-x.md", true},
		{"pj.cue", true},
		{".gitignore", true},
		{"archive/wc-ab2c-x.md", true},
		{"archive/pj.cue", false},           // only projects under archive/
		{"archive/sub/wc-ab2c-x.md", false}, // deeper than archive/ is residue
		{"random.txt", false},
		{"AGENTS.md", false},
		{"sub/wc-ab2c-x.md", false}, // no scanned subdirectory other than archive/
	}
	for _, c := range cases {
		path := filepath.Join(dir, filepath.FromSlash(c.rel))
		if got := IsAllowlisted(path, dir); got != c.want {
			t.Errorf("IsAllowlisted(%q) = %v want %v", c.rel, got, c.want)
		}
	}
	// Dir itself and paths outside dir are never allowlisted.
	if IsAllowlisted(dir, dir) {
		t.Error("the scope dir itself must not be allowlisted")
	}
	if IsAllowlisted(filepath.FromSlash("/other/wc-ab2c-x.md"), dir) {
		t.Error("a path outside the scope dir must not be allowlisted")
	}
}
