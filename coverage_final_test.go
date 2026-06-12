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
}
