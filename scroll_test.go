package prettyview

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
)

func tallRaw(n int) []byte {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	return []byte(sb.String())
}

// TestScrollToLine scrolls a deep display line into view and rejects an out-of-range
// index.
func TestScrollToLine(t *testing.T) {
	pv, win := renderInWindow(t, tallRaw(500), FormatRaw, 400, 300)
	defer win.Close()

	if pv.ScrollToLine(99999) {
		t.Error("ScrollToLine out of range returned true, want false")
	}
	const target = 400
	if !pv.ScrollToLine(target) {
		t.Fatal("ScrollToLine in range returned false")
	}
	row := int(pv.doc.RowOfLine(int32(target)))
	if row < pv.r.firstRow || row > pv.r.lastRow {
		t.Errorf("line %d (row %d) not in visible range [%d,%d] after ScrollToLine",
			target, row, pv.r.firstRow, pv.r.lastRow)
	}
}

// TestScrollToLineRevealsFolded: a line hidden inside a collapsed container is
// revealed (ancestors expanded) and scrolled to.
func TestScrollToLineRevealsFolded(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"outer":{"deep":"x"}}`), FormatJSON, 400, 300)
	defer win.Close()

	o := findFoldHead(pv.doc, `"outer"`)
	pv.doc.Fold(o)
	deep := lineContaining(pv.doc, "deep")
	if deep < 0 || pv.doc.Visible(deep) {
		t.Fatal("precondition: the deep line should be hidden")
	}
	if !pv.ScrollToLine(int(deep)) {
		t.Fatal("ScrollToLine returned false for a valid (hidden) line")
	}
	if !pv.doc.Visible(deep) {
		t.Error("ScrollToLine did not reveal the folded line")
	}
}

// TestScrollOffsetRoundTrip: ScrollOffset + SetScrollOffset save and restore the
// scroll position.
func TestScrollOffsetRoundTrip(t *testing.T) {
	pv, win := renderInWindow(t, tallRaw(500), FormatRaw, 400, 300)
	defer win.Close()

	pv.ScrollToLine(300)
	saved := pv.ScrollOffset()
	if saved.Y == 0 {
		t.Fatal("expected a non-zero scroll offset after ScrollToLine(300)")
	}
	pv.SetScrollOffset(fyne.NewPos(0, 0))
	if pv.ScrollOffset().Y != 0 {
		t.Fatal("SetScrollOffset(0,0) did not scroll to the top")
	}
	pv.SetScrollOffset(saved)
	if got := pv.ScrollOffset(); got != saved {
		t.Errorf("restored offset = %v, want %v", got, saved)
	}
}
