package parse

import (
	"strings"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// editDocOf builds the editable projection of src (raw coloring is irrelevant to guides).
func editDocOf(src string) *model.Document {
	return ParseEditableColored([]byte(src), FormatRaw, 0)
}

func TestLeadingIndentCols(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		wantCols  int
		wantBlank bool
	}{
		{"flush", "abc", 0, false},
		{"spaces", "    x", 4, false},
		{"tabs are one column each", "\t\tx", 2, false},
		{"mixed", " \t x", 3, false},
		{"empty", "", 0, true},
		{"whitespace only", "   ", 3, true},
		{"crlf blank", "  \r", 2, true},
		{"cr then text is not blank", "  \rx", 2, false},
		{"lone cr", "\r", 0, true},
	}
	for _, c := range cases {
		d := editDocOf(c.src)
		cols, blank := LeadingIndentCols(d, 0)
		if cols != c.wantCols || blank != c.wantBlank {
			t.Errorf("%s: LeadingIndentCols(%q) = (%d, %v), want (%d, %v)", c.name, c.src, cols, blank, c.wantCols, c.wantBlank)
		}
	}
	d := editDocOf("x")
	for _, li := range []int32{-1, int32(len(d.Lines))} {
		if cols, blank := LeadingIndentCols(d, li); cols != 0 || !blank {
			t.Errorf("out-of-range line %d = (%d, %v), want (0, true)", li, cols, blank)
		}
	}
}

// TestLeadingIndentColsCapsDeepIndent pins the scan bound: a line of more leading whitespace
// than maxIndentScanCols reports exactly the cap and counts as non-blank (never walked to the
// end), while one just under the cap is measured exactly.
func TestLeadingIndentColsCapsDeepIndent(t *testing.T) {
	deep := editDocOf(strings.Repeat(" ", maxIndentScanCols+500))
	if cols, blank := LeadingIndentCols(deep, 0); cols != maxIndentScanCols || blank {
		t.Errorf("deep whitespace = (%d, %v), want (%d, false)", cols, blank, maxIndentScanCols)
	}
	under := editDocOf(strings.Repeat(" ", maxIndentScanCols-1) + "x")
	if cols, blank := LeadingIndentCols(under, 0); cols != maxIndentScanCols-1 || blank {
		t.Errorf("just-under-cap = (%d, %v), want (%d, false)", cols, blank, maxIndentScanCols-1)
	}
}

func TestEditIndentUnit(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"two spaces", "{\n  \"a\": {\n    \"b\": 1\n  }\n}", 2},
		{"four spaces", "{\n    \"a\": {\n        \"b\": 1\n    }\n}", 4},
		{"tabs", "{\n\t\"a\": {\n\t\t\"b\": 1\n\t}\n}", 1},
		{"flat => default", "a\nb\nc", DefaultEditIndentUnit},
		{"empty => default", "", DefaultEditIndentUnit},
		{"most frequent step wins over noise", "a\n   b\nc\n    d\ne\n    f\ng\n    h", 4},
		{"tie goes to the smaller step", "a\n   b\nc\n    d", 3},
		{"blank lines are skipped", "a\n\n\n   b", 3},
		{"step of 8 is detected", "a\n        b", 8},
		{"step over 8 is noise", "a\n         b", DefaultEditIndentUnit},
		{"dedent steps are ignored", "        a\nb\n c", 1},
	}
	for _, c := range cases {
		if got := EditIndentUnit(editDocOf(c.src)); got != c.want {
			t.Errorf("%s: EditIndentUnit = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestEditGuideLevel(t *testing.T) {
	src := strings.Join([]string{
		"{",              // 0: level 0
		"  \"a\": {",     // 1: level 1
		"",               // 2: blank, up 1 down 2 => 2
		"    \"b\": 1,",  // 3: level 2
		"     \"c\": 2,", // 4: 5 cols => floor 2
		"",               // 5: blank, up 2 down 1 => 2
		"  },",           // 6: level 1
		"   ",            // 7: whitespace-only, up 1 down 1 => 1
		"  \"d\": [",     // 8: level 1
		"",               // 9: blank, up 1 down 3 => min+1 = 2
		"      7",        // 10: level 3
		"  ]",            // 11: level 1
		"}",              // 12: level 0
		"",               // 13: trailing caret line, up 0 down none(0) => 0
	}, "\n")
	d := editDocOf(src)
	want := []int{0, 1, 2, 2, 2, 2, 1, 1, 1, 2, 3, 1, 0, 0}
	if len(d.Lines) != len(want) {
		t.Fatalf("projection has %d lines, want %d", len(d.Lines), len(want))
	}
	for li, w := range want {
		if got := EditGuideLevel(d, int32(li), 2); got != w {
			t.Errorf("line %d: EditGuideLevel = %d, want %d", li, got, w)
		}
	}
	// A non-positive unit is clamped to 1 rather than dividing by zero.
	if got := EditGuideLevel(d, 3, 0); got != 4 {
		t.Errorf("unit 0: level of a 4-column line = %d, want 4 (unit clamped to 1)", got)
	}
	// A leading blank line has no line above: up counts as 0.
	lead := editDocOf("\n    x")
	if got := EditGuideLevel(lead, 0, 2); got != 1 {
		t.Errorf("leading blank above a level-2 line = %d, want 1 (min(0,2)+1)", got)
	}
}

// TestEditGuideLevelBlankScanBound pins maxEditGuideScan: a blank line whose non-blank
// neighbour is exactly at the bound still sees it; one beyond the bound treats the
// neighbour as level 0.
func TestEditGuideLevelBlankScanBound(t *testing.T) {
	build := func(blanks int) string {
		return "    a\n" + strings.Repeat("\n", blanks) + "    b"
	}
	// blanks run of maxEditGuideScan*2-1 lines: the middle one is exactly
	// maxEditGuideScan lines from each neighbour.
	in := editDocOf(build(2*maxEditGuideScan - 1))
	if got := EditGuideLevel(in, maxEditGuideScan, 2); got != 2 {
		t.Errorf("neighbours at the scan bound: level = %d, want 2", got)
	}
	out := editDocOf(build(2*maxEditGuideScan + 1))
	if got := EditGuideLevel(out, maxEditGuideScan+1, 2); got != 0 {
		t.Errorf("neighbours past the scan bound: level = %d, want 0", got)
	}
	// Past the bound on one side only: min(0, 2)+1.
	if got := EditGuideLevel(out, 1, 2); got != 1 {
		t.Errorf("one neighbour past the bound: level = %d, want 1", got)
	}
}

// TestEditIndentUnitAllocs: the per-reproject unit detection must not allocate (#80).
func TestEditIndentUnitAllocs(t *testing.T) {
	d := ParseEditableColored(makeJSON(64<<10), FormatJSON, 0)
	if n := testing.AllocsPerRun(20, func() { _ = EditIndentUnit(d) }); n != 0 {
		t.Errorf("EditIndentUnit allocated %.0f times per run, want 0", n)
	}
	if n := testing.AllocsPerRun(20, func() { _ = EditGuideLevel(d, 3, 2) }); n != 0 {
		t.Errorf("EditGuideLevel allocated %.0f times per run, want 0", n)
	}
}
