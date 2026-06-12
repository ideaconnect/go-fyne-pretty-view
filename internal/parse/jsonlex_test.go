package parse

import "testing"

// TestScanNumberExtent locks the shared number charset (issue #55): both the structured
// parser and the live colorizer scan a number through this one function, so the charset
// (digits, signs, '.', e/E) and its tolerant stop can't drift between them.
func TestScanNumberExtent(t *testing.T) {
	cases := []struct {
		src  string
		from int
		want int
	}{
		{"123", 0, 3},
		{"-1.5e+10", 0, 8},
		{"42,", 0, 2},   // stops at the comma
		{"3.14}", 0, 4}, // stops at the brace
		{"0xFF", 0, 1},  // 'x' is not in the charset -> stops after '0'
		{"  9", 2, 3},   // scans from an offset
		{"", 0, 0},      // empty
		{"abc", 0, 0},   // no number bytes
	}
	for _, c := range cases {
		if got := scanNumberExtent([]byte(c.src), c.from); got != c.want {
			t.Errorf("scanNumberExtent(%q, %d) = %d, want %d", c.src, c.from, got, c.want)
		}
	}
}

// TestMatchLiteralAt covers the shared bare-word matcher, including the over-EOF boundary
// (must not panic or read past the slice) and a non-match.
func TestMatchLiteralAt(t *testing.T) {
	src := []byte("truefalse")
	if !matchLiteralAt(src, 0, "true") {
		t.Error(`matchLiteralAt should match "true" at 0`)
	}
	if !matchLiteralAt(src, 4, "false") {
		t.Error(`matchLiteralAt should match "false" at 4`)
	}
	if matchLiteralAt(src, 0, "false") {
		t.Error(`"false" must not match at 0`)
	}
	if matchLiteralAt([]byte("nul"), 0, "null") {
		t.Error("a word longer than the remaining bytes must not match (no over-read)")
	}
}

// TestScanCommentExtents covers the shared comment scanners, including the block-comment
// EOF case (unterminated -> len(src)) the issue called out as a drift hazard.
func TestScanCommentExtents(t *testing.T) {
	// Line comment: up to but not including the newline; or EOF.
	if got := scanLineCommentExtent([]byte("// hi\nx"), 0); got != 5 {
		t.Errorf("scanLineCommentExtent stops before newline: got %d, want 5", got)
	}
	if got := scanLineCommentExtent([]byte("// to eof"), 0); got != 9 {
		t.Errorf("scanLineCommentExtent to EOF: got %d, want 9", got)
	}
	// Block comment: just past the closing */.
	if got := scanBlockCommentExtent([]byte("/* x */y"), 0); got != 7 {
		t.Errorf("scanBlockCommentExtent past close: got %d, want 7", got)
	}
	// Unterminated block comment runs to EOF, never past it.
	if got := scanBlockCommentExtent([]byte("/* open"), 0); got != 7 {
		t.Errorf("unterminated scanBlockCommentExtent: got %d, want 7 (EOF)", got)
	}
	// A bare "/*" with nothing after also clamps to EOF.
	if got := scanBlockCommentExtent([]byte("/*"), 0); got != 2 {
		t.Errorf("scanBlockCommentExtent(%q) = %d, want 2 (EOF)", "/*", got)
	}
}

// TestIsASCIISpace pins the shared whitespace set (must equal the ASCII subset of
// unicode.IsSpace that auto-detection trims).
func TestIsASCIISpace(t *testing.T) {
	for _, c := range []byte{' ', '\t', '\n', '\r', '\f', '\v'} {
		if !isASCIISpace(c) {
			t.Errorf("isASCIISpace(%q) = false, want true", c)
		}
	}
	for _, c := range []byte{'a', '0', '{', 0xA0 /* NBSP byte, non-ASCII */} {
		if isASCIISpace(c) {
			t.Errorf("isASCIISpace(%q) = true, want false", c)
		}
	}
}
