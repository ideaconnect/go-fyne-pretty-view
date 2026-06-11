package parse

import (
	"bytes"
	"strings"
	"testing"
)

// TestLineAtSourceOffset checks the coarse Src-offset -> display-line map that the v2
// format engine uses to re-place the caret after a raw->structured projection swap. It
// lives in the parse package because building a structured Document needs Parse.
func TestLineAtSourceOffset(t *testing.T) {
	src := []byte("{\n  \"a\": 1,\n  \"b\": 22\n}")
	d := Parse(src, FormatJSON, 0)

	if got := d.LineAtSourceOffset(0); got != 0 {
		t.Errorf("offset 0 -> line %d, want 0", got)
	}

	// The byte offset of the "22" value should map to the line rendering key "b".
	off := bytes.Index(src, []byte("22"))
	li := d.LineAtSourceOffset(off)
	if got := d.LineString(li); !strings.Contains(got, "b") {
		t.Errorf("offset of \"22\" mapped to line %q, want the \"b\" line", got)
	}

	// An out-of-range offset clamps to a valid line (the monotonic scan never overruns).
	if got := d.LineAtSourceOffset(len(src) + 100); int(got) >= d.TotalLines() {
		t.Errorf("out-of-range offset -> line %d, want < %d", got, d.TotalLines())
	}
}

// TestLineColAtSourceOffset checks the rune-precise Src-offset -> (line, col) map used
// to anchor the caret across a non-destructive reformat (#41).
func TestLineColAtSourceOffset(t *testing.T) {
	src := []byte("{\n  \"a\": 1,\n  \"bb\": 22\n}")
	d := Parse(src, FormatJSON, 0)

	off := bytes.Index(src, []byte("22"))
	line, col := d.LineColAtSourceOffset(off)
	lt := d.LineString(line)
	if !strings.Contains(lt, "bb") {
		t.Fatalf("offset of \"22\" mapped to line %q, want the \"bb\" line", lt)
	}
	runes := []rune(lt)
	if col < 0 || col > len(runes) {
		t.Fatalf("col %d out of range for %q", col, lt)
	}
	if !strings.HasPrefix(string(runes[col:]), "22") {
		t.Errorf("col %d in %q does not land at the \"22\" token (got %q)", col, lt, string(runes[col:]))
	}
}

// TestSourceOffsetAtRoundTrip checks the structured (line,col) -> Src offset map and its
// round-trip with LineColAtSourceOffset, including the synthesized close-brace line.
func TestSourceOffsetAtRoundTrip(t *testing.T) {
	src := []byte(`{"a":1,"bb":22}`)
	d := Parse(src, FormatJSON, 0)

	// The "bb" key's first byte round-trips: offset -> (line,col) -> offset.
	off := bytes.Index(src, []byte(`"bb"`))
	line, col := d.LineColAtSourceOffset(off)
	if got := d.SourceOffsetAt(line, col); got != off {
		t.Errorf("round-trip SourceOffsetAt(LineColAtSourceOffset(%d)) = %d, want %d", off, got, off)
	}

	// The close-brace line (synthesized, no source segment) anchors at the container end,
	// not its start — so a select-to-`}` reaches the buffer end, not offset 0.
	closeLine := int32(d.TotalLines() - 1)
	if got := d.SourceOffsetAt(closeLine, d.LineRuneLen(closeLine)); got < off {
		t.Errorf("close-line SourceOffsetAt = %d, want >= the \"bb\" offset %d (near buffer end)", got, off)
	}
}
