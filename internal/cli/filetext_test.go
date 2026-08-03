package cli

import (
	"testing"
)

func TestCountFileText(t *testing.T) {
	cases := []struct {
		name                string
		in                  string
		lines, words, chars int
	}{
		{name: "empty", in: "", lines: 0, words: 0, chars: 0},
		{name: "no newline", in: "hello", lines: 0, words: 1, chars: 5},
		{name: "one line", in: "hello\n", lines: 1, words: 1, chars: 6},
		{name: "two words", in: "hi there\n", lines: 1, words: 2, chars: 9},
		{name: "blank line", in: "a\n\nb\n", lines: 3, words: 2, chars: 5},
		{name: "tabs and spaces", in: "  one\ttwo  three\n", lines: 1, words: 3, chars: 17},
		// "café\n" = c a f é \n → 5 runes; é is one code point (2 bytes)
		{name: "unicode", in: "café\n", lines: 1, words: 1, chars: 5},
		{name: "cjk", in: "日本語\n", lines: 1, words: 1, chars: 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines, words, chars := countFileText([]byte(tc.in))
			if lines != tc.lines || words != tc.words || chars != tc.chars {
				t.Fatalf("countFileText(%q) = %d %d %d; want %d %d %d",
					tc.in, lines, words, chars, tc.lines, tc.words, tc.chars)
			}
		})
	}
}
