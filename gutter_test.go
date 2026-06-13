package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestLineNumberGutter: WithLineNumbers widens the gutter and draws each line's
// 1-based number on its first row; off (the default) draws nothing.
func TestLineNumberGutter(t *testing.T) {
	// Off by default: zero gutter width, no visible gutter text on any row.
	off, w0 := renderInWindow(t, []byte("[1,2,3]"), FormatJSON, 400, 300)
	defer w0.Close()
	if off.met.GutterWidth() != 0 {
		t.Errorf("default GutterWidth = %v, want 0", off.met.GutterWidth())
	}
	for _, rw := range off.r.live {
		if rw.rr.gutter != nil && rw.rr.gutter.Visible() {
			t.Error("a gutter number rendered with line numbers off")
		}
	}

	// On: positive gutter width, and the first line's row shows "1".
	test.NewApp()
	pv := New(WithLineNumbers())
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))
	pv.SetData([]byte("[1,2,3,4,5]"), FormatJSON) // pretty-prints to one element per line
	pv.Refresh()

	if pv.met.GutterWidth() <= 0 {
		t.Fatalf("WithLineNumbers GutterWidth = %v, want > 0", pv.met.GutterWidth())
	}
	found := false
	for _, rw := range pv.r.live {
		if rw.line != 0 {
			continue
		}
		found = true
		if rw.rr.gutter == nil || !rw.rr.gutter.Visible() {
			t.Error("the first row has no visible line-number gutter")
		} else if rw.rr.gutter.Text != "1" {
			t.Errorf("first row gutter = %q, want \"1\"", rw.rr.gutter.Text)
		}
	}
	if !found {
		t.Error("the first line was not among the live rows")
	}
}

// TestGutterWidthGrowsWithLineCount guards the #77 gutter digit-width memo: the width must
// recompute when the line count crosses a digit boundary (9->10->100), not stay fixed at the
// first document's digit count. A regression to a compute-once memo would clip the numbers
// past the boundary while the rest of the suite stayed green.
func TestGutterWidthGrowsWithLineCount(t *testing.T) {
	lines := func(n int) []byte { return []byte(strings.Repeat("x\n", n-1) + "x") } // exactly n lines

	test.NewApp()
	pv := New(WithLineNumbers())
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))

	pv.SetData(lines(5), FormatRaw) // 1-digit line numbers
	pv.Refresh()
	w1 := pv.met.GutterWidth()

	pv.SetData(lines(120), FormatRaw) // crosses into 3-digit line numbers
	pv.Refresh()
	w3 := pv.met.GutterWidth()
	if !(w3 > w1) {
		t.Errorf("gutter width must grow with the line count: 5 lines=%v, 120 lines=%v", w1, w3)
	}

	pv.SetData(lines(7), FormatRaw) // back to 1 digit -> width must shrink again
	pv.Refresh()
	if w := pv.met.GutterWidth(); w >= w3 {
		t.Errorf("gutter width must shrink when the line count drops: %v (was %v at 120 lines)", w, w3)
	}
}
