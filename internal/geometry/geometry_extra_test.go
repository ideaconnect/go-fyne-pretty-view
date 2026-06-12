package geometry

import (
	"strings"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/parse"
)

// TestSubRowOfColBranches covers SubRowOfCol across its three exits: a column inside an
// interior sub-row, a column past the last break (clamps to the last sub-row), and a
// degenerate single-element breaks slice (no wrapping -> sub-row 0).
func TestSubRowOfColBranches(t *testing.T) {
	cases := []struct {
		breaks []int32
		col    int
		want   int
	}{
		{[]int32{0, 4, 8}, 2, 0},   // first sub-row [0,4)
		{[]int32{0, 4, 8}, 5, 1},   // second sub-row [4,8)
		{[]int32{0, 4, 8}, 100, 1}, // past the end -> last sub-row (len-2)
		{[]int32{0, 8}, 100, 0},    // one span, past the end -> sub-row 0 (len-2)
		{[]int32{0}, 0, 0},         // degenerate (no [k,k+1) span) -> 0
	}
	for _, c := range cases {
		if got := SubRowOfCol(c.breaks, c.col); got != c.want {
			t.Errorf("SubRowOfCol(%v, %d) = %d, want %d", c.breaks, c.col, got, c.want)
		}
	}
}

// TestSpanOfSubClamps covers SpanOfSub: an in-range sub-row reads its [start,end) pair,
// and an out-of-range sub-row (negative or past the last) clamps into [0, len-2] — the
// boilerplate the reflow/highlight passes formerly open-coded.
func TestSpanOfSubClamps(t *testing.T) {
	breaks := []int32{0, 4, 9}
	cases := []struct {
		sub          int32
		wantS, wantE int32
	}{
		{0, 0, 4},  // first sub-row
		{1, 4, 9},  // last sub-row
		{2, 4, 9},  // past the end -> clamp to last
		{-1, 0, 4}, // negative -> clamp to first
	}
	for _, c := range cases {
		if s, e := SpanOfSub(breaks, c.sub); s != c.wantS || e != c.wantE {
			t.Errorf("SpanOfSub(%v, %d) = (%d,%d), want (%d,%d)", breaks, c.sub, s, e, c.wantS, c.wantE)
		}
	}
}

// TestNewMetricsClampsTiny covers the lower-bound clamps: a sub-pixel cell or indent step
// must still produce a usable (>=1) grid rather than a degenerate zero metric.
func TestNewMetricsClampsTiny(t *testing.T) {
	m := NewMetrics(0.4, 0.4, 0.4) // every input below 1 -> all three clamp branches
	if m.CharWidth < 1 {
		t.Errorf("CharWidth = %v, want clamped to >= 1", m.CharWidth)
	}
	if m.RowH < 1 {
		t.Errorf("RowH = %v, want clamped to >= 1", m.RowH)
	}
	if m.ColX(3, 0) < 0 {
		t.Errorf("indentStep clamp failed: ColX(3,0) = %v", m.ColX(3, 0))
	}
}

// TestColsForDepthFloors covers the minWrapCols floor: an indent so deep that no width
// remains must not collapse the wrap budget below the minimum.
func TestColsForDepthFloors(t *testing.T) {
	m := testMetrics()
	if cols := m.ColsForDepth(40, 10); cols != minWrapCols {
		t.Errorf("ColsForDepth(deep, narrow) = %d, want minWrapCols %d", cols, minWrapCols)
	}
}

// TestHitTestNegativeYClampsToTop covers the row<0 clamp: a click above the content
// resolves onto the first row, not a negative one.
func TestHitTestNegativeYClampsToTop(t *testing.T) {
	d := parse.Parse([]byte("{\n  \"a\": 1\n}"), model.FormatJSON, 0)
	m := testMetrics()
	li, col := HitTest(d, m, 0, -50)
	if li != 0 || col < 0 {
		t.Errorf("HitTest at negative Y = (%d,%d), want (0, >=0)", li, col)
	}
}

// TestHitTestWrappedClampsPastRowEnd covers HitTest's wrap branch where a click lands far
// to the right of a sub-row's text: the column clamps to that sub-row's end, not the next.
func TestHitTestWrappedClampsPastRowEnd(t *testing.T) {
	d := parse.Parse([]byte(`["`+strings.Repeat("alpha bravo ", 40)+`"]`), model.FormatJSON, 0)
	m := testMetrics()
	cols := make([]int, int(d.MaxDepth)+1)
	for depth := range cols {
		cols[depth] = m.ColsForDepth(uint8(depth), 200)
	}
	d.SetWrapColumns(cols)

	var li int32 = -1
	for r := int32(0); r < d.TotalVisibleRows(); r++ {
		l, _ := d.LineAndSubRowAtRow(r)
		if br := d.WrapBreaks(l, nil); len(br) > 2 {
			li = l
			break
		}
	}
	if li < 0 {
		t.Fatal("fixture must produce a wrapped line for the clamp test (#78: was a silent skip)")
	}
	cx, cy := CellOrigin(d, m, li, 0)
	_, col := HitTest(d, m, cx+100000, cy+1) // far right of the first sub-row
	br := d.WrapBreaks(li, nil)
	if col > int(br[1]) {
		t.Errorf("click past a wrapped sub-row should clamp to its end %d, got col %d", br[1], col)
	}
}
