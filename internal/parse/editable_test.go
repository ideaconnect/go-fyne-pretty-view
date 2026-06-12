package parse

import "testing"

// TestParseEditableLineCounts pins the edit-mode projection's defining behavior: a
// trailing newline yields a trailing empty line (so the caret has somewhere to sit),
// and empty input is one empty line — unlike the viewer's rawParser.
func TestParseEditableLineCounts(t *testing.T) {
	cases := []struct {
		src  string
		want int
	}{
		{"", 1},
		{"ab", 1},
		{"ab\n", 2},
		{"ab\ncd", 2},
		{"a\nb\n", 3},
		{"\n", 2},
	}
	for _, c := range cases {
		if got := ParseEditableColored([]byte(c.src), FormatRaw, 0, 4).TotalLines(); got != c.want {
			t.Errorf("ParseEditableColored(%q) lines = %d, want %d", c.src, got, c.want)
		}
	}

	// The viewer's raw parse, by contrast, omits the phantom trailing blank line.
	if got := Parse([]byte("ab\n"), FormatRaw, 0).TotalLines(); got != 1 {
		t.Errorf("viewer raw parse of %q = %d lines, want 1 (no phantom blank)", "ab\n", got)
	}
}
