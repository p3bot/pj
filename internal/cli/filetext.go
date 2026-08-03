package cli

import (
	"bytes"
	"unicode"
	"unicode/utf8"
)

// countFileText returns whole-file text stats in wc order: lines, words, characters.
//
//   - lines: count of '\n' bytes (same as wc -l)
//   - words: maximal runs of non-whitespace (unicode.IsSpace separators)
//   - characters: UTF-8 code points / runes (closer to wc -m than wc -c bytes)
func countFileText(data []byte) (lines, words, characters int) {
	lines = bytes.Count(data, []byte{'\n'})
	inWord := false
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		i += size
		characters++
		if unicode.IsSpace(r) {
			inWord = false
			continue
		}
		if !inWord {
			words++
			inWord = true
		}
	}
	return lines, words, characters
}
