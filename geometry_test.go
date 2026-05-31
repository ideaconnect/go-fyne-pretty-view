package prettyview

import "testing"

func testMetrics() metrics {
	return newMetrics(defaultConfig(), 9, 18)
}

func TestMetricsAreIntegral(t *testing.T) {
	m := testMetrics()
	for _, v := range []float32{m.charWidth, m.rowH, m.triangleSlot, m.indentStep, m.leftPad} {
		if v != roundf(v) {
			t.Errorf("metric %v is not integral", v)
		}
	}
}

func TestColRoundTrip(t *testing.T) {
	m := testMetrics()
	for _, depth := range []uint8{0, 1, 7, 30} {
		for col := 0; col < 200; col += 7 {
			x := m.colX(depth, col)
			if got := m.colAtX(depth, x); got != col {
				t.Fatalf("depth %d col %d -> x %.1f -> col %d", depth, col, x, got)
			}
		}
	}
}

func TestColAtXHalfGlyphRounding(t *testing.T) {
	m := testMetrics()
	var depth uint8 = 2
	origin := m.textOriginX(depth)
	// Just past the left edge of glyph 3 -> col 3; just before its right half -> col 3;
	// past the midpoint of glyph 3 -> col 4.
	if got := m.colAtX(depth, origin+3*m.charWidth+1); got != 3 {
		t.Errorf("left part of glyph 3 -> col %d, want 3", got)
	}
	if got := m.colAtX(depth, origin+3*m.charWidth+m.charWidth*0.6); got != 4 {
		t.Errorf("right part of glyph 3 -> col %d, want 4", got)
	}
	if got := m.colAtX(depth, origin-5); got != 0 {
		t.Errorf("left of text origin -> col %d, want 0", got)
	}
}

// TestHitTestGoldenRoundTrip is the single most important correctness guard: for
// a variety of (line, col) positions it computes the content-space cell origin
// and checks hitTest maps it back exactly. Any missing-origin / offset bug shows
// up here.
func TestHitTestGoldenRoundTrip(t *testing.T) {
	d := loadDoc(t, "openapi.json", FormatJSON)
	m := testMetrics()
	total := d.fold.TotalVisibleRows()

	rows := []int32{0, 1, 2, 5, total / 2, total - 2, total - 1}
	for _, r := range rows {
		if r < 0 || r >= total {
			continue
		}
		li := d.fold.lineAtRow(r)
		runeLen := d.lineRuneLen(li)
		for _, col := range []int{0, 1, runeLen / 2, runeLen} {
			if col < 0 || col > runeLen {
				continue
			}
			cx, cy := d.cellOrigin(m, li, col)
			// Sample slightly below the row top to avoid the exact top boundary.
			got := d.hitTest(m, cx, cy+1)
			if got.line != li || got.col != col {
				t.Fatalf("row %d line %d col %d -> origin (%.1f,%.1f) -> {line %d col %d}",
					r, li, col, cx, cy, got.line, got.col)
			}
		}
	}
}

func TestHitTestClampsBeyondEnd(t *testing.T) {
	d := loadDoc(t, "small.json", FormatJSON)
	m := testMetrics()
	total := d.fold.TotalVisibleRows()
	// Click far below the last row clamps to the last line's end column.
	got := d.hitTest(m, 9999, m.rowY(int(total))+500)
	lastLine := d.fold.lineAtRow(total - 1)
	if got.line != lastLine || got.col != d.lineRuneLen(lastLine) {
		t.Errorf("clamp-to-end = {line %d col %d}, want {line %d col %d}",
			got.line, got.col, lastLine, d.lineRuneLen(lastLine))
	}
}
