// Package slug implements create-time filename slugify: pure, deterministic,
// no uniqueness pass. Legal grammar: ^[a-z0-9]+(-[a-z0-9]+)*$ length 1–SlugMax.
package slug

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// SlugMax is the maximum byte length of a legal slug.
const SlugMax = 48

const fallback = "x"

// Slugify converts a title into a legal slug: NFKC, lowercase, keep alphanumerics,
// other chars as separators, join with '-', fall back to "x", truncate preferring
// a cut at the last '-' within the cap. Always total (empty input yields "x").
func Slugify(title string) string {
	normalised := norm.NFKC.String(title)

	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range normalised {
		switch {
		case r >= 'A' && r <= 'Z':
			cur.WriteRune(r + ('a' - 'A'))
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			cur.WriteRune(r)
		default:
			flush()
		}
	}
	flush()

	if len(tokens) == 0 {
		return fallback
	}
	s := strings.Join(tokens, "-")
	if len(s) > SlugMax {
		s = truncate(s)
	}
	if s == "" {
		return fallback
	}
	return s
}

// truncate reduces s to at most SlugMax bytes, preferring a cut at the last '-' within the cap.
func truncate(s string) string {
	if cut := strings.LastIndexByte(s[:SlugMax+1], '-'); cut >= 0 {
		return s[:cut]
	}
	return strings.TrimRight(s[:SlugMax], "-")
}

// Valid reports whether s satisfies the closed slug grammar.
func Valid(s string) bool {
	if len(s) < 1 || len(s) > SlugMax {
		return false
	}
	prevHyphen := true // guards against a leading '-'
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			prevHyphen = false
		case c == '-':
			if prevHyphen {
				return false // leading '-' or '--'
			}
			prevHyphen = true
		default:
			return false
		}
	}
	return !prevHyphen // trailing '-'
}
