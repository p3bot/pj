// Package scope implements --auto-name derivation from a code-root basename.
// Pure, no I/O; basenames that cannot yield a legal name return ErrCannotDerive.
package scope

import (
	"errors"
	"strings"
	"unicode"

	"github.com/start-cli/pj/internal/id"
)

const (
	nameMax = id.ScopeNameMax

	// autoNameLetters drops i/l/o for typeability; a strict subset of the full scope alphabet.
	autoNameLetters = id.LetterAlphabet
	// autoNameDigits drops 0/1, matching the short-id typeable digit set.
	autoNameDigits = id.DigitAlphabet
)

// ErrCannotDerive signals that no legal scope name can be derived; the caller should pass --name.
var ErrCannotDerive = errors.New("cannot derive scope name from code-root basename; pass --name")

// AutoName derives a proposed scope name from a code-root basename.
// Tokenise on [-_. ] and camelCase, seed from initials (multi-token) or first two chars
// (opaque), restrict to the auto-name alphabet, ensure a letter leads, cap at nameMax.
func AutoName(basename string) (string, error) {
	tokens := tokenise(lastSegment(basename))

	var seed string
	switch {
	case len(tokens) >= 2:
		var b strings.Builder
		for _, t := range tokens {
			b.WriteString(string([]rune(t)[:1]))
		}
		seed = b.String()
	case len(tokens) == 1:
		seed = firstRunes(tokens[0], 2)
	default:
		seed = ""
	}

	name := sanitize(seed)
	if !accept(name) {
		return "", ErrCannotDerive
	}
	return name, nil
}

func lastSegment(s string) string {
	s = strings.TrimRight(s, "/")
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func tokenise(s string) []string {
	var tokens []string
	for _, piece := range strings.FieldsFunc(s, isSeparator) {
		tokens = append(tokens, splitCamel(piece)...)
	}
	for i, t := range tokens {
		tokens[i] = strings.ToLower(t)
	}
	return tokens
}

func isSeparator(r rune) bool {
	return r == '-' || r == '_' || r == '.' || r == ' '
}

// splitCamel breaks at camelCase boundaries, including acronym runs (HTMLParser → HTML, Parser).
func splitCamel(piece string) []string {
	runes := []rune(piece)
	if len(runes) == 0 {
		return nil
	}
	var tokens []string
	start := 0
	for i := 1; i < len(runes); i++ {
		prev, cur := runes[i-1], runes[i]
		boundary := false
		if unicode.IsUpper(cur) {
			if !unicode.IsUpper(prev) {
				boundary = true
			} else if i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				boundary = true
			}
		}
		if boundary {
			tokens = append(tokens, string(runes[start:i]))
			start = i
		}
	}
	return append(tokens, string(runes[start:]))
}

func sanitize(seed string) string {
	var kept []byte
	for i := 0; i < len(seed); i++ {
		c := seed[i]
		if inAlphabet(c) {
			kept = append(kept, c)
		}
	}
	for len(kept) > 0 && !isAutoLetter(kept[0]) {
		kept = kept[1:]
	}
	if len(kept) > nameMax {
		kept = kept[:nameMax]
	}
	return string(kept)
}

func accept(name string) bool {
	if len(name) < 1 || len(name) > nameMax {
		return false
	}
	if !isAutoLetter(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !inAlphabet(name[i]) {
			return false
		}
	}
	return true
}

func inAlphabet(c byte) bool {
	return isAutoLetter(c) || strings.IndexByte(autoNameDigits, c) >= 0
}

func isAutoLetter(c byte) bool {
	return strings.IndexByte(autoNameLetters, c) >= 0
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
