package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestLineNumbersHorizontalScrollRender drives row.build's horizontal-cull path with the
// line-number gutter on: a very wide line scrolled right renders only its visible column
// window while the gutter still shows the line number.
func TestLineNumbersHorizontalScrollRender(t *testing.T) {
	test.NewApp()
	long := strings.Repeat("abcdefghij", 120) // ~1200 chars, far wider than the viewport
	pv := New(WithLineNumbers())
	pv.SetData([]byte(`["`+long+`"]`), FormatJSON)
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(300, 200))
	pv.Refresh()

	pv.SetScrollOffset(fyne.NewPos(500, 0)) // scroll right -> the wide line is horizontally culled
	pv.Refresh()

	gutterShown := false
	for _, rw := range pv.r.live {
		if rw.rr != nil && rw.rr.gutter != nil && rw.rr.gutter.Visible() {
			gutterShown = true
			break
		}
	}
	if !gutterShown {
		t.Error("the line-number gutter should still render while the wide line is scrolled")
	}

	// The load-bearing M-1 invariant this test's name advertises: the wide string line (display
	// line 1; line 0 is "[") must be horizontally CULLED — only its visible column window renders,
	// far shorter than the full ~1200-char line — not painted in full.
	if got, ok := rowText(pv.r, pv.doc.LineAtRow(1)); !ok {
		t.Error("the wide line has no live row after scrolling")
	} else if len(got) == 0 || len(got) >= len(long) {
		t.Errorf("horizontal cull failed: rendered %d chars of the %d-char line, want a bounded window", len(got), len(long))
	}
}
