// Package title extracts the first ATX H1 from a markdown body.
// Pure and never fails: no match yields "", never a slug or summary fallback.
package title

import (
	"bytes"
	"regexp"
	"strings"
)

// atxH1 matches a single-# ATX heading: one '#' then whitespace then text.
// Required whitespace means '##' never matches.
var atxH1 = regexp.MustCompile(`^#\s+.+`)

// Extract returns the text of the first ATX H1 with actual text, or "".
// Skips empty '#   ' headings so a later real H1 is still found; ignores setext.
func Extract(body []byte) string {
	for len(body) > 0 {
		var line []byte
		if i := bytes.IndexByte(body, '\n'); i >= 0 {
			line, body = body[:i], body[i+1:]
		} else {
			line, body = body, nil
		}
		line = bytes.TrimSuffix(line, []byte("\r"))
		if atxH1.Match(line) {
			if text := strings.TrimSpace(strings.TrimPrefix(string(line), "#")); text != "" {
				return text
			}
		}
	}
	return ""
}
