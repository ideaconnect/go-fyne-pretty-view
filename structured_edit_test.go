package prettyview

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

// TestStructuredCaretEditLandsAtCaret guards the caret behavior after a buffer-rewriting
// reformat: a caret placed (SetCaret) on a line in the prettified buffer must edit there,
// not at a stale end-of-buffer offset.
func TestStructuredCaretEditLandsAtCaret(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"a":1,"b":2}`)
	pv.Reformat() // the buffer is now pretty, multi-line

	bLine := -1
	for li := 0; li < pv.doc.TotalLines(); li++ {
		if strings.Contains(pv.doc.LineString(int32(li)), `"b"`) {
			bLine = li
			break
		}
	}
	if bLine < 0 {
		t.Fatal("no \"b\" line in the reformatted doc")
	}
	if !pv.SetCaret(bLine, 0) {
		t.Fatal("SetCaret to the \"b\" line failed")
	}
	pv.TypedRune('X')

	// The X must land on the "b" line, not at a stale end-of-buffer caret.
	got := string(pv.buf.Bytes())
	xLine := ""
	for _, ln := range strings.Split(got, "\n") {
		if strings.ContainsRune(ln, 'X') {
			xLine = ln
			break
		}
	}
	if !strings.Contains(xLine, `"b"`) {
		t.Errorf("typed X landed on line %q, want the \"b\" line; full buffer %q", xLine, got)
	}
}

// TestStructuredSelectionEditReplaces: typing or pasting over an active selection in the
// pretty projection must REPLACE the selection (not append at a collapsed caret).
func TestStructuredSelectionEditReplaces(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"a":1}`)
	pv.Reformat() // buffer is now pretty, multi-line
	pv.SelectAll()
	if !pv.sel.active {
		t.Fatal("SelectAll should produce an active selection")
	}
	pv.TypedRune('Z')

	got := string(pv.buf.Bytes())
	if strings.Contains(got, `"a"`) {
		t.Errorf("select-all + type must replace, got %q (selection not removed)", got)
	}
	if got != "Z" {
		t.Errorf("select-all + type 'Z' = %q, want %q", got, "Z")
	}
}

// TestStructuredSelectionCut: Cut over a structured selection copies the selected text
// and removes exactly those bytes (one undo unit).
func TestStructuredSelectionCut(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"a":1}`)
	pv.Reformat()
	pretty := string(pv.buf.Bytes()) // the prettified buffer (a reformat is its own undo unit)
	pv.SelectAll()
	pv.Cut()

	if got := string(pv.buf.Bytes()); got != "" {
		t.Errorf("Cut(SelectAll) should empty the buffer, got %q", got)
	}
	if cb := fyne.CurrentApp().Clipboard().Content(); !strings.Contains(cb, "a") {
		t.Errorf("Cut clipboard should hold the selected text, got %q", cb)
	}
	pv.Undo()
	if got := string(pv.buf.Bytes()); got != pretty {
		t.Errorf("one Undo after Cut should restore the buffer, got %q want %q", got, pretty)
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
