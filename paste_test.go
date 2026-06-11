package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
)

func setClipboard(s string) {
	if app := fyne.CurrentApp(); app != nil {
		app.Clipboard().SetContent(s)
	}
}

func hasRawControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}

// displayText concatenates every display line's text with NO separator, so a raw
// control char can only come from a line's own content (not an inserted newline).
func displayText(pv *PrettyView) string {
	var b strings.Builder
	for li := 0; li < pv.doc.TotalLines(); li++ {
		b.WriteString(pv.doc.LineString(int32(li)))
	}
	return b.String()
}

func TestPasteInsertsAtCaret(t *testing.T) {
	pv, win := renderEditable(t, []byte("abXcd"), 600, 400)
	defer win.Close()
	pv.FocusGained()
	pv.sel.focus = modelPos{line: 0, col: 2} // after "ab"
	pv.sel.placed = true

	setClipboard("ZZ")
	pv.Paste()
	if got := string(pv.buf.Bytes()); got != "abZZXcd" {
		t.Errorf("paste = %q, want %q", got, "abZZXcd")
	}
	if got := pv.caretOff(); got != 4 {
		t.Errorf("caret after paste = %d, want 4 (after the pasted text)", got)
	}
}

func TestPasteReplacesSelection(t *testing.T) {
	pv, win := renderEditable(t, []byte("abcdef"), 600, 400)
	defer win.Close()
	pv.FocusGained()
	pv.sel.anchor = modelPos{line: 0, col: 2}
	pv.sel.focus = modelPos{line: 0, col: 4} // select "cd"
	pv.sel.active = true
	pv.sel.placed = true

	setClipboard("XY")
	pv.Paste()
	if got := string(pv.buf.Bytes()); got != "abXYef" {
		t.Errorf("paste over selection = %q, want %q", got, "abXYef")
	}
}

func TestCutCopiesAndDeletes(t *testing.T) {
	pv, win := renderEditable(t, []byte("abcdef"), 600, 400)
	defer win.Close()
	pv.FocusGained()
	pv.sel.anchor = modelPos{line: 0, col: 2}
	pv.sel.focus = modelPos{line: 0, col: 4} // select "cd"
	pv.sel.active = true
	pv.sel.placed = true

	pv.Cut()
	if got := string(pv.buf.Bytes()); got != "abef" {
		t.Errorf("after cut buffer = %q, want %q", got, "abef")
	}
	if cb := fyne.CurrentApp().Clipboard().Content(); cb != "cd" {
		t.Errorf("clipboard after cut = %q, want %q", cb, "cd")
	}
	pv.Undo()
	if got := string(pv.buf.Bytes()); got != "abcdef" {
		t.Errorf("one Undo after cut = %q, want %q (cut is one undo unit)", got, "abcdef")
	}
}

func TestPasteMultilineAndControlBytes(t *testing.T) {
	pv, win := renderEditable(t, nil, 600, 400)
	defer win.Close()
	pv.FocusGained()

	setClipboard("a\nb\tc\x01") // newline + tab + a C0 control
	pv.Paste()

	if pv.doc.TotalLines() < 2 {
		t.Errorf("multi-line paste made %d lines, want >= 2 (newline -> real lines)", pv.doc.TotalLines())
	}
	for li := 0; li < pv.doc.TotalLines(); li++ {
		if s := pv.doc.LineString(int32(li)); hasRawControl(s) {
			t.Errorf("display line %d has a raw control char: %q", li, s)
		}
	}
	// Non-destructive: the buffer keeps the pasted bytes (only CRLF is normalized).
	if got := string(pv.buf.Bytes()); got != "a\nb\tc\x01" {
		t.Errorf("buffer = %q, want the pasted bytes preserved", got)
	}

	// A raw tab inside a JSON string renders as a visible \t escape once reformatted.
	pv2, win2 := renderEditable(t, nil, 600, 400)
	defer win2.Close()
	pv2.FocusGained()
	setClipboard("{\"k\":\"x\ty\"}") // raw tab byte inside the JSON string value
	pv2.Paste()
	pv2.Reformat()
	disp := displayText(pv2)
	if hasRawControl(disp) {
		t.Errorf("structured reformat left a raw control char: %q", disp)
	}
	if !strings.Contains(disp, `\t`) {
		t.Errorf("structured reformat should show a visible \\t escape, got %q", disp)
	}
}

func TestCutPasteNoopReadOnly(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"a":1}`), FormatJSON, 400, 300)
	defer win.Close()
	pv.FocusGained()

	before := string(pv.Source())
	setClipboard("XYZ")
	pv.Paste() // no-op in read-only mode
	pv.SelectAll()
	pv.Cut() // no-op in read-only mode
	if string(pv.Source()) != before {
		t.Error("Cut/Paste must not modify a read-only viewer")
	}
}
