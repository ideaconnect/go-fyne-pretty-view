package prettyview

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

// TestStructuredCaretEditLandsAtCaret is the regression guard for the structured-mode
// caret bug: a caret placed (click/SetCaret) in the pretty projection must edit the
// buffer at that position, not at a stale cached offset.
func TestStructuredCaretEditLandsAtCaret(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"a":1,"b":2}`)
	pv.Reformat()
	if !pv.editStructured {
		t.Fatal("precondition: structured projection")
	}

	bLine := -1
	for li := 0; li < pv.doc.TotalLines(); li++ {
		if strings.Contains(pv.doc.LineString(int32(li)), `"b"`) {
			bLine = li
			break
		}
	}
	if bLine < 0 {
		t.Fatal("no \"b\" line in the structured doc")
	}
	if !pv.SetCaret(bLine, 0) {
		t.Fatal("SetCaret to the \"b\" line failed")
	}
	pv.TypedRune('X')

	got := string(pv.buf.Bytes())
	if got == `{"a":1,"b":2}X` {
		t.Fatal("BUG: edit landed at the stale end-of-buffer caret")
	}
	if i := strings.IndexByte(got, 'X'); i < 0 || i > 9 {
		t.Errorf("typed X landed at index %d (%q), want near the \"b\" caret (~7)", i, got)
	}
}

// TestStructuredSelectionEditReplaces: typing or pasting over an active selection in the
// pretty projection must REPLACE the selection (not append at a collapsed caret).
func TestStructuredSelectionEditReplaces(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"a":1}`)
	pv.Reformat()
	if !pv.editStructured {
		t.Fatal("precondition: structured projection")
	}
	pv.SelectAll()
	if !pv.sel.active {
		t.Fatal("SelectAll should produce an active selection")
	}
	pv.TypedRune('Z')

	got := string(pv.buf.Bytes())
	if strings.Contains(got, `"a"`) {
		t.Errorf("structured select-all + type must replace, got %q (selection not removed)", got)
	}
	if got != "Z" {
		t.Errorf("structured select-all + type 'Z' = %q, want %q", got, "Z")
	}
}

// TestStructuredSelectionCut: Cut over a structured selection copies the selected text
// and removes exactly those bytes (one undo unit).
func TestStructuredSelectionCut(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"a":1}`)
	pv.Reformat()
	pv.SelectAll()
	pv.Cut()

	if got := string(pv.buf.Bytes()); got != "" {
		t.Errorf("structured Cut(SelectAll) should empty the buffer, got %q", got)
	}
	if cb := fyne.CurrentApp().Clipboard().Content(); !strings.Contains(cb, "a") {
		t.Errorf("Cut clipboard should hold the selected text, got %q", cb)
	}
	pv.Undo()
	if got := string(pv.buf.Bytes()); got != `{"a":1}` {
		t.Errorf("one Undo after Cut should restore the buffer, got %q", got)
	}
}

// TestEditDebounceBumpsGeneration: each edit bumps editGen, so a superseded debounced
// reparse that already queued its closure recognizes itself as stale and skips.
func TestEditDebounceBumpsGeneration(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{DebounceFor: time.Second, AutoFormat: AutoFormatOnPause})
	defer win.Close()

	g0 := pv.editGen
	pv.TypedRune('a')
	g1 := pv.editGen
	pv.TypedRune('b')
	g2 := pv.editGen
	if !(g1 > g0 && g2 > g1) {
		t.Errorf("each edit must bump editGen to invalidate a superseded reparse: %d, %d, %d", g0, g1, g2)
	}
}

// TestUndoCoalescesMultibyte: a run of single-rune inserts coalesces to one undo unit
// even for multi-byte runes.
func TestUndoCoalescesMultibyte(t *testing.T) {
	pv, win := renderEditable(t, nil, 600, 400)
	defer win.Close()
	pv.FocusGained()

	typeStr(pv, "café") // 'é' is two bytes
	if n := len(pv.hist.undo); n != 1 {
		t.Errorf("typing a word with a multi-byte rune made %d undo units, want 1", n)
	}
	pv.Undo()
	if got := string(pv.buf.Bytes()); got != "" {
		t.Errorf("one Undo should clear the coalesced word, got %q", got)
	}
}
