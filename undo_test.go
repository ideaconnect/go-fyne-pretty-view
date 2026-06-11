package prettyview

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestUndoRestoresTextAndCaret(t *testing.T) {
	pv, win := renderEditable(t, []byte("abc"), 600, 400)
	defer win.Close()
	pv.FocusGained()

	pv.keyMoveCaret(0, 0, false, true) // caret to end of "abc" (col 3)
	pv.TypedRune('X')                  // "abcX", caret 4
	if got := string(pv.buf.Bytes()); got != "abcX" {
		t.Fatalf("after insert buffer = %q, want %q", got, "abcX")
	}

	pv.Undo()
	if got := string(pv.buf.Bytes()); got != "abc" {
		t.Errorf("after undo buffer = %q, want %q", got, "abc")
	}
	if got := pv.caretOff(); got != 3 {
		t.Errorf("after undo caret offset = %d, want 3 (pre-edit)", got)
	}
}

func TestRedoReappliesEdit(t *testing.T) {
	pv, win := renderEditable(t, nil, 600, 400)
	defer win.Close()
	pv.FocusGained()

	typeStr(pv, "hi")
	pv.Undo() // coalesced run -> back to empty
	if got := string(pv.buf.Bytes()); got != "" {
		t.Fatalf("after undo buffer = %q, want empty", got)
	}
	pv.Redo()
	if got := string(pv.buf.Bytes()); got != "hi" {
		t.Errorf("after redo buffer = %q, want %q", got, "hi")
	}
	if got := pv.caretOff(); got != 2 {
		t.Errorf("after redo caret = %d, want 2 (post-edit)", got)
	}
}

func TestUndoCoalescesWordTyping(t *testing.T) {
	pv, win := renderEditable(t, nil, 600, 400)
	defer win.Close()
	pv.FocusGained()

	typeStr(pv, "hello") // a run of single-rune inserts coalesces to one undo unit
	if len(pv.hist.undo) != 1 {
		t.Errorf("typing a word made %d undo entries, want 1 (coalesced)", len(pv.hist.undo))
	}
	pv.Undo()
	if got := string(pv.buf.Bytes()); got != "" {
		t.Errorf("one Undo after typing a word left %q, want empty", got)
	}

	// A caret move breaks the run: two words -> two undo units.
	typeStr(pv, "foo")
	pv.keyMoveCaret(0, 0, false, true) // End -> caret move breaks coalescing
	typeStr(pv, "bar")
	pv.Undo()
	if got := string(pv.buf.Bytes()); got != "foo" {
		t.Errorf("Undo after a caret move should remove only the second run, got %q, want %q", got, "foo")
	}
}

func TestUndoHistoryBounded(t *testing.T) {
	test.NewApp()
	pv := New(WithEditable(), WithUndoLimit(3),
		WithInputConfig(InputConfig{AutoFormat: AutoFormatOff}))
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))
	pv.Refresh()
	pv.FocusGained()

	// Distinct undo units (a caret move between each so they don't coalesce).
	for i := 0; i < 10; i++ {
		pv.TypedRune('x')
		pv.keyMoveCaret(0, 0, false, true)
	}
	if n := len(pv.hist.undo); n > 3 {
		t.Errorf("undo history length = %d, want <= 3 (bounded by WithUndoLimit)", n)
	}
}

func TestUndoNoopReadOnlyAndEmpty(t *testing.T) {
	// Read-only: Undo/Redo do nothing and never panic.
	ro, w1 := renderInWindow(t, []byte(`{"a":1}`), FormatJSON, 400, 300)
	defer w1.Close()
	before := string(ro.Source())
	ro.Undo()
	ro.Redo()
	if string(ro.Source()) != before {
		t.Error("Undo/Redo must be no-ops in read-only mode")
	}

	// Editable but empty stack: no-op.
	ed, w2 := renderEditable(t, []byte("abc"), 400, 300)
	defer w2.Close()
	ed.Undo() // nothing recorded yet
	ed.Redo()
	if got := string(ed.buf.Bytes()); got != "abc" {
		t.Errorf("Undo/Redo on an empty stack changed the buffer to %q", got)
	}
}
