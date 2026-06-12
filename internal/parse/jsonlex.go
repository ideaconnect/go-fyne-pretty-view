package parse

// jsonlex.go holds the low-level JSON/JSONC scan primitives shared by the structured
// parser (parse_json.go) and the live edit-colorizer (parse_editcolor.go) so the two can
// never drift on token boundaries — the drift hazard that motivated issue #55 (a change to
// the number charset or block-comment EOF handling in one would silently desync the other).
//
// ONLY these mechanical, position-only scanners are shared. The two parsers keep their own
// higher-level behavior on top: the structured parser reflows, validates, and can fail; the
// colorizer is layout-preserving and never fails. Notably, object-KEY detection is NOT
// shared — it is recursive-descent grammar position in the structured parser (interwoven
// with node/segment emission) but an explicit container stack in the flat colorizer, so it
// is not a tokenizer primitive. String scanning is also not shared: the colorizer stops an
// unterminated string at the next newline to keep lexing, while the structured scanner runs
// to EOF and fails.

// isASCIISpace reports whether c is an ASCII whitespace byte (space, tab, LF, CR, form-feed,
// vertical-tab). For JSON this set is load-bearing: it must match the ASCII subset of
// unicode.IsSpace that auto-detection trims, or an input confidently labelled JSON would
// stall mid-scan (see scanTrivia). The structured scanner additionally decodes non-ASCII
// Unicode spaces; the colorizer treats those as stray bytes.
func isASCIISpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

// scanNumberExtent returns the index just past the JSON number token starting at i. It is
// charset-only (digits, leading/exponent sign, '.', e/E) and deliberately tolerant —
// validating the number is the structured parser's job, not the scanner's.
func scanNumberExtent(src []byte, i int) int {
	for i < len(src) {
		c := src[i]
		if (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' {
			i++
			continue
		}
		break
	}
	return i
}

// matchLiteralAt reports whether the bytes at i are exactly word (true/false/null). Like
// both scanners it does not bounds-check the following byte — a tolerant bare-word match.
func matchLiteralAt(src []byte, i int, word string) bool {
	return i+len(word) <= len(src) && string(src[i:i+len(word)]) == word
}

// scanLineCommentExtent returns the index just past a // line comment whose first '/' is at
// i (the caller has verified src[i]=='/' && src[i+1]=='/'): up to, but not including, the
// next newline, or EOF.
func scanLineCommentExtent(src []byte, i int) int {
	i += 2
	for i < len(src) && src[i] != '\n' {
		i++
	}
	return i
}

// scanBlockCommentExtent returns the index just past a /* */ block comment whose first '/'
// is at i (the caller has verified src[i]=='/' && src[i+1]=='*'): just past the closing
// */, or len(src) if the comment is unterminated at EOF.
func scanBlockCommentExtent(src []byte, i int) int {
	i += 2
	for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
		i++
	}
	if i+1 < len(src) {
		return i + 2
	}
	return len(src)
}
