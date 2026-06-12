package prettyview

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestTypedKeyNoRenderer covers TypedKey's renderer-nil guard: before the widget is shown,
// Escape clears the selection and any other key is a safe no-op.
func TestTypedKeyNoRenderer(t *testing.T) {
	test.NewApp()
	pv := New() // never shown -> pv.r == nil
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown}) // non-Escape: returns without touching a nil renderer
	if pv.sel.active {
		t.Error("an unshown widget should have no active selection after Escape")
	}
}

// TestKeyExtendEmptyDocGuard covers keyExtend's no-visible-rows guard: a shift-arrow on a
// shown but empty document returns early instead of indexing an empty projection.
func TestKeyExtendEmptyDocGuard(t *testing.T) {
	pv, win := renderInWindow(t, []byte(""), FormatRaw, 300, 200)
	defer win.Close()
	pv.FocusGained()
	if pv.doc.TotalVisibleRows() != 0 {
		t.Fatalf("empty raw input should produce no visible rows, got %d (#78: was a silent skip)", pv.doc.TotalVisibleRows())
	}
	pv.shiftHeld = true
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown}) // keyExtend -> early return, no panic
	pv.shiftHeld = false
	if pv.sel.active {
		t.Error("a shift-arrow on an empty doc should not make a selection")
	}
}

// TestTripleClickDragGrabsLines covers applyHit's line-grab branch: a triple-click arms a
// line grab and dragging extends the selection by whole lines.
func TestTripleClickDragGrabsLines(t *testing.T) {
	pv, win := renderInWindow(t, []byte("{\n  \"a\": 1,\n  \"b\": 2\n}"), FormatJSON, 500, 300)
	defer win.Close()

	li := pv.doc.LineAtRow(1)
	x := pv.met.TextOriginX(pv.doc.Lines[li].Depth) + 2
	y := pv.met.RowY(1) + 1
	pv.MouseDown(desktopMouse(x, y)) // click 1
	pv.MouseDown(desktopMouse(x, y)) // click 2
	pv.MouseDown(desktopMouse(x, y)) // click 3 -> line grab
	if pv.sel.grab != grabLine {
		t.Fatalf("triple-click grab = %v, want grabLine", pv.sel.grab)
	}
	pv.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(x, y+pv.met.RowH*2)}})
	if !pv.sel.active {
		t.Error("line-grab drag should keep an active selection")
	}
	pv.DragEnd()
}
