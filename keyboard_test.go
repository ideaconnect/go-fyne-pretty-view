package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
)

// TestKeyboardHorizontalScroll: Left/Right arrows scroll horizontally (so the
// README's "arrows" claim is accurate, not just Up/Down).
func TestKeyboardHorizontalScroll(t *testing.T) {
	long := strings.Repeat("abcdefghij", 300)
	pv, win := renderInWindow(t, []byte(`["`+long+`"]`), FormatJSON, 300, 200)
	defer win.Close()
	pv.FocusGained()

	x0 := pv.r.scroll.Offset.X
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if pv.r.scroll.Offset.X <= x0 {
		t.Errorf("KeyRight did not scroll right: %.0f -> %.0f", x0, pv.r.scroll.Offset.X)
	}
	x1 := pv.r.scroll.Offset.X
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if pv.r.scroll.Offset.X >= x1 {
		t.Errorf("KeyLeft did not scroll left: %.0f -> %.0f", x1, pv.r.scroll.Offset.X)
	}
}

// TestKeyboardFoldToggle: Enter toggles the fold on the caret's line when it is a
// fold head.
func TestKeyboardFoldToggle(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"a":{"b":1}}`), FormatJSON, 400, 300)
	defer win.Close()

	a := findFoldHead(pv.doc, `"a"`)
	pv.sel.focus = modelPos{line: pv.doc.Nodes[a].HeadLine, col: 0}
	pv.sel.placed = true

	if pv.doc.Collapsed(a) {
		t.Fatal("precondition: 'a' should be expanded")
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !pv.doc.Collapsed(a) {
		t.Error("Enter did not collapse the fold at the caret line")
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if pv.doc.Collapsed(a) {
		t.Error("Enter did not re-expand the fold")
	}
}

// TestKeyboardShiftSelection: Shift+Down/Shift+End extend a keyboard selection from
// the caret; a plain arrow (no Shift) does not.
func TestKeyboardShiftSelection(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"a":1,"b":2,"c":3}`), FormatJSON, 400, 300)
	defer win.Close()
	pv.FocusGained()

	pv.sel.anchor = modelPos{line: pv.doc.LineAtRow(1), col: 0}
	pv.sel.focus = pv.sel.anchor
	pv.sel.placed = true

	// Plain Down scrolls, does not select.
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if pv.sel.active {
		t.Error("a plain Down arrow should scroll, not select")
	}

	// Shift+Down extends the selection down one row.
	pv.shiftHeld = true
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if !pv.sel.active {
		t.Fatal("Shift+Down did not activate a selection")
	}
	if got := pv.SelectedText(); !strings.Contains(got, `"a"`) {
		t.Errorf("Shift+Down selection = %q, want it to include the 'a' line", got)
	}

	// Shift+End moves the focus to the line end.
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	wantCol := pv.doc.LineRuneLen(pv.doc.VisibleLine(pv.sel.focus.line))
	if pv.sel.focus.col != wantCol {
		t.Errorf("Shift+End focus col = %d, want line end %d", pv.sel.focus.col, wantCol)
	}
}
