package geometry

import (
	"os"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/parse"
)

func testMetrics() Metrics {
	return NewMetrics(9, 18, 16)
}

// TestGutterShiftsOrigin: the line-number gutter widens TextOriginX uniformly, so
// every derived coordinate (ColX, TriangleX, hit-test) shifts by the same amount —
// the one coordinate convention stays intact. Off (width 0) by default.
func TestGutterShiftsOrigin(t *testing.T) {
	m := testMetrics()
	if m.GutterWidth() != 0 {
		t.Fatalf("default gutter width = %v, want 0", m.GutterWidth())
	}
	x0 := m.TextOriginX(0)

	m.SetGutterWidth(40)
	if got := m.GutterWidth(); got != 40 {
		t.Errorf("GutterWidth = %v, want 40", got)
	}
	if got := m.TextOriginX(0); got != x0+40 {
		t.Errorf("TextOriginX with gutter = %v, want %v", got, x0+40)
	}
	if got := m.ColX(0, 5); got != x0+40+5*m.CharWidth {
		t.Errorf("ColX did not shift with the gutter: %v", got)
	}
	// A negative width is clamped to 0 (gutter off).
	m.SetGutterWidth(-10)
	if m.GutterWidth() != 0 {
		t.Errorf("negative gutter width = %v, want clamped to 0", m.GutterWidth())
	}
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
	// RowH and the gutter/indent metrics are rounded to whole pixels. CharWidth is
	// intentionally NOT rounded — it must equal the font's exact advance so the
	// column grid matches the rendered text (see the Metrics doc comment), so it is
	// excluded here.
	for _, v := range []float32{m.RowH, m.triangleSlot, m.indentStep, m.leftPad} {
		if v != roundf(v) {
			t.Errorf("metric %v is not integral", v)
		}
	}
}

// TestCharWidthExactAlignsSegments guards the fix for overlapping segments on long
// lines: with a fractional font advance, the column grid (ColX) must stay in
// lockstep with the rendered text width so a wide run never overruns the next
// column. A rounded CharWidth would drift by (advance-round(advance)) per glyph.
func TestCharWidthExactAlignsSegments(t *testing.T) {
	const advance = 8.4219 // a deliberately fractional monospace advance
	m := NewMetrics(advance, 18, 16)
	if m.CharWidth != advance {
		t.Fatalf("CharWidth = %v, want the exact (unrounded) advance %v", m.CharWidth, advance)
	}
	// A 50-glyph run starting at column 0 must end exactly where column 50 begins.
	const n = 50
	runEnd := m.ColX(0, 0) + n*m.CharWidth
	if next := m.ColX(0, n); next != runEnd {
		t.Errorf("column %d begins at %.3f but a %d-glyph run ends at %.3f (drift %.3f)",
			n, next, n, runEnd, next-runEnd)
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

// TestVisibleColumnCulling pins FirstVisibleCol / LastVisibleCol directly. These bound the
// per-row horizontal text window — the culling that keeps every canvas.Text within the
// viewport (CODE_BIBLE rule 1; an un-culled long line would allocate a giant bitmap). They are
// otherwise exercised only indirectly through the root rendering tests, so without a direct
// test their mutants land in gremlins' excluded "Not covered" bucket (#111). The asserts pin
// the Floor (first) vs Ceil (last) rounding and the rel<=0 clamp so a flipped rounding or a
// dropped guard fails here.
func TestVisibleColumnCulling(t *testing.T) {
	m := testMetrics() // CharWidth 9, TextOriginX(0) = 19
	const cw = 9
	origin0 := m.TextOriginX(0)
	if origin0 != 19 {
		t.Fatalf("precondition: TextOriginX(0) = %v, want 19", origin0)
	}
	tests := []struct {
		name                string
		depth               uint8
		x0, x1              float32
		wantFirst, wantLast int
	}{
		// A window at/left of the text origin clamps to column 0 on both ends.
		{"at origin", 0, origin0, origin0, 0, 0},
		// Edge-aligned window [col2, col5): first floors to 2, last ceils to 5.
		{"edge aligned", 0, origin0 + 2*cw, origin0 + 5*cw, 2, 5},
		// Partial glyphs: first FLOORS (the partly-visible left column 3 is included),
		// last CEILS (the partly-visible right column 6 is included).
		{"partial glyphs", 0, origin0 + 3*cw + 5, origin0 + 5*cw + 1, 3, 6},
		// Depth shifts the origin right by depth*indentStep, so the same x maps earlier.
		{"depth shifts origin", 1, m.TextOriginX(1) + 4*cw, m.TextOriginX(1) + 9*cw, 4, 9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.FirstVisibleCol(tc.depth, tc.x0); got != tc.wantFirst {
				t.Errorf("FirstVisibleCol(%d, %.1f) = %d, want %d", tc.depth, tc.x0, got, tc.wantFirst)
			}
			if got := m.LastVisibleCol(tc.depth, tc.x1); got != tc.wantLast {
				t.Errorf("LastVisibleCol(%d, %.1f) = %d, want %d", tc.depth, tc.x1, got, tc.wantLast)
			}
		})
	}
	// Left of the origin the relative x is negative; both must clamp to 0 rather than
	// returning a negative column from floor(neg) — guards the rel<=0 branch.
	if got := m.FirstVisibleCol(0, origin0-cw); got != 0 {
		t.Errorf("FirstVisibleCol left of origin = %d, want 0", got)
	}
	if got := m.LastVisibleCol(0, origin0-cw); got != 0 {
		t.Errorf("LastVisibleCol left of origin = %d, want 0", got)
	}
}

// TestRowAndTriangleMetrics pins TextY and TriangleX directly (same #111 mutation-gate gap as
// the column-culling pair: used only by the root row renderer, never the geometry tests).
func TestRowAndTriangleMetrics(t *testing.T) {
	m := testMetrics() // RowH 22, textH 18 -> TextY 2; triangleSlot 13, leftPad 6
	if got := m.TextY(); got != 2 {
		t.Errorf("TextY() = %v, want 2 (an 18px glyph centered in a 22px row)", got)
	}
	// The fold triangle sits exactly one triangleSlot left of the text origin at every depth,
	// so text aligns at TextOriginX whether or not the row is foldable.
	for _, depth := range []uint8{0, 1, 5} {
		if gap := m.TextOriginX(depth) - m.TriangleX(depth); gap != m.triangleSlot {
			t.Errorf("depth %d: TextOriginX-TriangleX = %v, want triangleSlot %v", depth, gap, m.triangleSlot)
		}
	}
	if got := m.TriangleX(0); got != m.leftPad {
		t.Errorf("TriangleX(0) = %v, want leftPad %v (gutter off, depth 0)", got, m.leftPad)
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

// TestHitTestGoldenRoundTripWrapped is the soft-wrap counterpart: with a narrow
// viewport forcing lines to wrap, CellOrigin∘HitTest must still round-trip across
// sub-rows, including columns that straddle a soft-break boundary.
func TestHitTestGoldenRoundTripWrapped(t *testing.T) {
	d := loadDoc(t, "openapi.json")
	m := testMetrics()
	const viewW = 360 // narrow: many lines wrap to several visual rows
	cols := make([]int, int(d.MaxDepth)+1)
	for depth := range cols {
		cols[depth] = m.ColsForDepth(uint8(depth), viewW)
	}
	d.SetWrapColumns(cols)

	total := d.TotalVisibleRows()
	rows := []int32{0, 1, 2, 5, 13, total / 2, total - 2, total - 1}
	var dst []int32
	for _, r := range rows {
		if r < 0 || r >= total {
			continue
		}
		li, sub := d.LineAndSubRowAtRow(r)
		dst = d.WrapBreaks(li, dst[:0])
		start, end := int(dst[sub]), int(dst[sub+1])
		for _, col := range []int{start, start + 1, (start + end) / 2, end} {
			if col < start || col > end {
				continue
			}
			cx, cy := CellOrigin(d, m, li, col)
			gotLine, gotCol := HitTest(d, m, cx, cy+1)
			if gotLine != li || gotCol != col {
				t.Fatalf("wrapped row %d (line %d sub %d) col %d -> origin (%.1f,%.1f) -> {line %d col %d}",
					r, li, sub, col, cx, cy, gotLine, gotCol)
			}
		}
	}
}

// TestHitTestEmptyDocument covers the no-rows guard: HitTest on a document with no
// visible rows returns the (-1, 0) sentinel rather than indexing an empty arena.
func TestHitTestEmptyDocument(t *testing.T) {
	d := model.EmptyDocument()
	if d.TotalVisibleRows() != 0 {
		t.Fatalf("EmptyDocument has %d visible rows, want 0", d.TotalVisibleRows())
	}
	if line, col := HitTest(d, testMetrics(), 50, 50); line != -1 || col != 0 {
		t.Errorf("HitTest on empty doc = (%d, %d), want (-1, 0)", line, col)
	}
}

func TestHitTestClampsBeyondEnd(t *testing.T) {
	d := loadDoc(t, "small.json")
	m := testMetrics()
	total := d.TotalVisibleRows()
	// A click below the last row AND far to the right lands on the last line's end
	// column (the column is clamped to the line length).
	gotLine, gotCol := HitTest(d, m, 9999, m.RowY(int(total))+500)
	lastLine := d.LineAtRow(total - 1)
	if gotLine != lastLine || gotCol != d.LineRuneLen(lastLine) {
		t.Errorf("clamp-to-end = {line %d col %d}, want {line %d col %d}",
			gotLine, gotCol, lastLine, d.LineRuneLen(lastLine))
	}
}

func TestHitTestBelowContentHonorsX(t *testing.T) {
	d := loadDoc(t, "small.json")
	m := testMetrics()
	total := d.TotalVisibleRows()
	lastLine := d.LineAtRow(total - 1)
	// A click below all content but at the far LEFT resolves to column 0 of the
	// last line (honoring X) rather than biasing to the end of the line.
	gotLine, gotCol := HitTest(d, m, 0, m.RowY(int(total))+500)
	if gotLine != lastLine || gotCol != 0 {
		t.Errorf("below-content left click = {line %d col %d}, want {line %d col 0}",
			gotLine, gotCol, lastLine)
	}
}
