package prettyview

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestForwardDeleteAndEdgeNoops covers editDelete's forward branch and the no-op edges:
// forward-delete at end-of-buffer and backspace at offset 0 must both do nothing.
func TestForwardDeleteAndEdgeNoops(t *testing.T) {
	pv, win := renderEditable(t, []byte("abc"), 600, 400)
	defer win.Close()
	pv.FocusGained()

	pv.keyMoveCaret(0, 0, true, false) // Home -> caret at (0,0)
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDelete})
	if got := string(pv.buf.Bytes()); got != "bc" {
		t.Errorf("forward delete at line start = %q, want %q", got, "bc")
	}

	pv.keyMoveCaret(0, 0, false, true) // End -> caret past "bc"
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDelete})
	if got := string(pv.buf.Bytes()); got != "bc" {
		t.Errorf("forward delete at end-of-buffer must be a no-op, got %q", got)
	}

	pv.keyMoveCaret(0, 0, true, false) // Home -> caret at (0,0)
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if got := string(pv.buf.Bytes()); got != "bc" {
		t.Errorf("backspace at offset 0 must be a no-op, got %q", got)
	}
}

// TestCaretStepRuneEdges covers caretStepRune: the first arrow on an unplaced caret seeds
// it at the top, and stepping off either end of the buffer is a no-op.
func TestCaretStepRuneEdges(t *testing.T) {
	pv, win := renderEditable(t, []byte("ab"), 600, 400)
	defer win.Close()
	pv.FocusGained()
	pv.ClearSelection() // drop any placed caret so the first arrow must seed it

	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight}) // first arrow seeds the caret at the top
	if pv.sel.focus != (modelPos{line: 0, col: 0}) {
		t.Errorf("first arrow on an unplaced caret = %v, want (0,0)", pv.sel.focus)
	}

	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft}) // already at offset 0 -> no-op
	if pv.caretOff() != 0 {
		t.Errorf("left arrow at start moved the caret to %d, want 0", pv.caretOff())
	}

	pv.keyMoveCaret(0, 0, false, true)               // End -> caret at offset 2 (end of "ab")
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight}) // forward past EOF -> no-op
	if pv.caretOff() != 2 {
		t.Errorf("right arrow at end moved the caret to %d, want 2", pv.caretOff())
	}
}

// TestEditKeyUpDownAndSpace covers editKey's KeyUp/KeyDown caret moves and the Space key
// (swallowed so it does not page; the actual space char arrives via TypedRune).
func TestEditKeyUpDownAndSpace(t *testing.T) {
	pv, win := renderEditable(t, []byte("ab\ncd"), 600, 400)
	defer win.Close()
	pv.FocusGained()

	pv.SetCaret(1, 1) // on the second line
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	if pv.sel.focus.line != 0 {
		t.Errorf("Up in edit mode = line %d, want 0", pv.sel.focus.line)
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if pv.sel.focus.line != 1 {
		t.Errorf("Down in edit mode = line %d, want 1", pv.sel.focus.line)
	}

	// Space is swallowed by editKey (returns true) so it neither pages nor inserts here.
	before := string(pv.buf.Bytes())
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeySpace})
	if got := string(pv.buf.Bytes()); got != before {
		t.Errorf("KeySpace must not itself edit the buffer (TypedRune inserts), got %q", got)
	}
}

// TestEditKeyDefaultFallsThrough: a key edit mode does not own (Escape) returns false from
// editKey and reaches the read-only handler, which clears the selection.
func TestEditKeyDefaultFallsThrough(t *testing.T) {
	pv, win := renderEditable(t, []byte("abc"), 600, 400)
	defer win.Close()
	pv.FocusGained()
	pv.SelectAll()
	if !pv.sel.active {
		t.Fatal("SelectAll should produce an active selection")
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape}) // editKey returns false -> read-only Escape clears
	if pv.sel.active {
		t.Error("Escape should fall through to ClearSelection in edit mode")
	}
}

// TestPasteEmptyClipboardNoop: pasting an empty clipboard is a no-op (the content=="" guard).
func TestPasteEmptyClipboardNoop(t *testing.T) {
	pv, win := renderEditable(t, []byte("abc"), 600, 400)
	defer win.Close()
	pv.FocusGained()
	setClipboard("")
	pv.Paste()
	if got := string(pv.buf.Bytes()); got != "abc" {
		t.Errorf("paste of empty clipboard changed the buffer to %q", got)
	}
}

// TestEditInsertOverSelectionRespectsCap covers editInsert's selection-shrinks-growth path
// of the MaxEditBytes check: replacing a selection only counts the NET growth against the cap.
func TestEditInsertOverSelectionRespectsCap(t *testing.T) {
	test.NewApp()
	pv := New(WithEditable(), WithInputConfig(InputConfig{AutoFormat: AutoFormatOff, MaxEditBytes: 6}))
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))
	pv.Refresh()
	pv.FocusGained()

	typeStr(pv, "abcde") // 5 of 6 bytes used
	pv.sel.anchor = modelPos{line: 0, col: 2}
	pv.sel.focus = modelPos{line: 0, col: 4} // select "cd"
	pv.sel.active = true
	pv.sel.placed = true
	setClipboard("XYZ") // net growth = 3 - 2 = 1 -> fits the cap of 6
	pv.Paste()
	if got := string(pv.buf.Bytes()); got != "abXYZe" {
		t.Errorf("replace-over-selection within cap = %q, want %q", got, "abXYZe")
	}
}

// TestAfterEditFiresOnDataChanged: each edit notifies the data-changed observer (afterEdit).
func TestAfterEditFiresOnDataChanged(t *testing.T) {
	pv, win := renderEditable(t, nil, 600, 400)
	defer win.Close()
	pv.FocusGained()

	var n int
	pv.SetOnDataChanged(func() { n++ })
	pv.TypedRune('a')
	if n == 0 {
		t.Error("an edit should fire the onDataChanged observer")
	}
}
