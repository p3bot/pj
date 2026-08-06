// Package collision is the pure total order for same-id collision keeper pick.
// Disk-backed duplicate-id repair and frontmatter merge add/add share KeepBefore
// so both routes keep the same side. No I/O, clock, dirent order, or pointer identity.
package collision

import (
	"bytes"
	"crypto/sha256"
	"time"
)

// Member carries the fields the deterministic collision keeper pick consults.
type Member struct {
	// Created is the frontmatter created value (RFC3339, or degraded = not-newer-than-any).
	Created string
	// Basename is the file basename, or a shared placeholder in an add/add.
	Basename string
	// Raw is whole file/stage bytes for the SHA-256 residual — never frontmatter-only.
	Raw []byte
	// Path is absolute, or a shared placeholder in an add/add.
	Path string
}

// KeepBefore reports whether a is kept over b under the closed total pre-order:
// older Created (degraded = not-newer-than-any), then smaller Basename, then smaller
// SHA-256 of Raw, then smaller Path.
func KeepBefore(a, b Member) bool {
	ta, aok := parseInstant(a.Created)
	tb, bok := parseInstant(b.Created)
	if aok != bok {
		// Degraded side (not ok) is not-newer-than-any: sorts first (kept).
		return !aok
	}
	if aok && !ta.Equal(tb) {
		return ta.Before(tb)
	}
	if a.Basename != b.Basename {
		return a.Basename < b.Basename
	}
	if c := bytes.Compare(sha(a.Raw), sha(b.Raw)); c != 0 {
		return c < 0
	}
	return a.Path < b.Path
}

// parseInstant strictly parses RFC3339. ok is false for degraded values (not-newer-than-any).
func parseInstant(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func sha(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}
