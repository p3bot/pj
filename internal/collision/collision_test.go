package collision

import (
	"bytes"
	"testing"
)

// KeepBefore is the shared collision pick; asserted in-memory so add/add and disk repair share one total order.
func TestKeepBeforeTotalOrder(t *testing.T) {
	older := Member{Created: "2026-01-01T00:00:00Z", Basename: "b", Raw: []byte("z"), Path: "/z"}
	newer := Member{Created: "2026-06-01T00:00:00Z", Basename: "a", Raw: []byte("a"), Path: "/a"}
	if !KeepBefore(older, newer) || KeepBefore(newer, older) {
		t.Error("older by created must be kept over newer regardless of the other terms")
	}

	degraded := Member{Created: "", Basename: "z", Raw: []byte("z"), Path: "/z"}
	valid := Member{Created: "2020-01-01T00:00:00Z", Basename: "a", Raw: []byte("a"), Path: "/a"}
	if !KeepBefore(degraded, valid) || KeepBefore(valid, degraded) {
		t.Error("a degraded created is not-newer-than-any and must be kept")
	}

	// Non-empty unparseable created is also degraded (not only the empty string).
	badCreated := Member{Created: "not-a-date", Basename: "z", Raw: []byte("z"), Path: "/z"}
	if !KeepBefore(badCreated, valid) || KeepBefore(valid, badCreated) {
		t.Error("unparseable created must be treated as degraded and kept over a valid instant")
	}

	// Equal created: smaller basename is kept.
	sameCreated := "2026-01-01T00:00:00Z"
	alpha := Member{Created: sameCreated, Basename: "wc-ab2c-alpha.md", Raw: []byte("x"), Path: "/z"}
	beta := Member{Created: sameCreated, Basename: "wc-ab2c-beta.md", Raw: []byte("y"), Path: "/a"}
	if !KeepBefore(alpha, beta) || KeepBefore(beta, alpha) {
		t.Error("equal created must break by smaller basename, symmetrically")
	}

	// Equal created and basename: smaller hash is kept.
	a := Member{Created: sameCreated, Basename: "", Raw: []byte("AAA"), Path: ""}
	b := Member{Created: sameCreated, Basename: "", Raw: []byte("BBB"), Path: ""}
	wantAKept := bytes.Compare(sha(a.Raw), sha(b.Raw)) < 0
	if KeepBefore(a, b) != wantAKept || KeepBefore(b, a) == wantAKept {
		t.Error("equal created+basename must break by smaller SHA-256 of raw bytes, symmetrically")
	}

	// Equal created, basename, and raw: smaller path is kept.
	sameRaw := []byte("same")
	left := Member{Created: sameCreated, Basename: "x.md", Raw: sameRaw, Path: "/a/x.md"}
	right := Member{Created: sameCreated, Basename: "x.md", Raw: sameRaw, Path: "/b/x.md"}
	if !KeepBefore(left, right) || KeepBefore(right, left) {
		t.Error("equal created+basename+raw must break by smaller path, symmetrically")
	}
}
