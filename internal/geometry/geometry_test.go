package geometry

import (
	"os"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/parse"
)

func testMetrics() Metrics {
	return NewMetrics(9, 18, 16, 4)
}

func loadDoc(t *testing.T, name string) *model.Document {
	t.Helper()
	src, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return parse.Parse(src, model.FormatJSON, 0)
}

func TestMetricsAreIntegral(t *testing.T) {
	m := testMetrics()
	for _, v := range []float32{m.CharWidth, m.RowH, m.triangleSlot, m.indentStep, m.leftPad} {
		if v != roundf(v) {
			t.Errorf("metric %v is not integral", v)
		}
	}
}

func TestColRoundTrip(t *testing.T) {
	m := testMetrics()
	for _, depth := range []uint8{0, 1, 7, 30} {
		for col := 0; col < 200; col += 7 {
			x := m.ColX(depth, col)
			if got := m.ColAtX(depth, x); got != col {
				t.Fatalf("depth %d col %d -> x %.1f -> col %d", depth, col, x, got)
			}
		}
	}
}

func TestColAtXHalfGlyphRounding(t *testing.T) {
	m := testMetrics()
	var depth uint8 = 2
	origin := m.TextOriginX(depth)
	// Just past the left edge of glyph 3 -> col 3; past the midpoint -> col 4.
	if got := m.ColAtX(depth, origin+3*m.CharWidth+1); got != 3 {
		t.Errorf("left part of glyph 3 -> col %d, want 3", got)
	}
	if got := m.ColAtX(depth, origin+3*m.CharWidth+m.CharWidth*0.6); got != 4 {
		t.Errorf("right part of glyph 3 -> col %d, want 4", got)
	}
	if got := m.ColAtX(depth, origin-5); got != 0 {
		t.Errorf("left of text origin -> col %d, want 0", got)
	}
}

// TestHitTestGoldenRoundTrip is the single most important correctness guard: for
// a variety of (line, col) positions it computes the content-space cell origin
// and checks HitTest maps it back exactly. Any missing-origin / offset bug shows
// up here.
func TestHitTestGoldenRoundTrip(t *testing.T) {
	d := loadDoc(t, "openapi.json")
	m := testMetrics()
	total := d.TotalVisibleRows()

	rows := []int32{0, 1, 2, 5, total / 2, total - 2, total - 1}
	for _, r := range rows {
		if r < 0 || r >= total {
			continue
		}
		li := d.LineAtRow(r)
		runeLen := d.LineRuneLen(li)
		for _, col := range []int{0, 1, runeLen / 2, runeLen} {
			if col < 0 || col > runeLen {
				continue
			}
			cx, cy := CellOrigin(d, m, li, col)
			// Sample slightly below the row top to avoid the exact top boundary.
			gotLine, gotCol := HitTest(d, m, cx, cy+1)
			if gotLine != li || gotCol != col {
				t.Fatalf("row %d line %d col %d -> origin (%.1f,%.1f) -> {line %d col %d}",
					r, li, col, cx, cy, gotLine, gotCol)
			}
		}
	}
}

func TestHitTestClampsBeyondEnd(t *testing.T) {
	d := loadDoc(t, "small.json")
	m := testMetrics()
	total := d.TotalVisibleRows()
	// Click far below the last row clamps to the last line's end column.
	gotLine, gotCol := HitTest(d, m, 9999, m.RowY(int(total))+500)
	lastLine := d.LineAtRow(total - 1)
	if gotLine != lastLine || gotCol != d.LineRuneLen(lastLine) {
		t.Errorf("clamp-to-end = {line %d col %d}, want {line %d col %d}",
			gotLine, gotCol, lastLine, d.LineRuneLen(lastLine))
	}
}
