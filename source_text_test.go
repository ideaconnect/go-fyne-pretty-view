package prettyview

import (
	"strings"
	"testing"
)

func TestSourceReflectsEdits(t *testing.T) {
	pv, win := renderEditable(t, []byte("abc"), 600, 400)
	defer win.Close()
	pv.FocusGained()

	pv.keyMoveCaret(0, 0, false, true) // caret to end
	typeStr(pv, "XYZ")
	if got := string(pv.Source()); got != "abcXYZ" {
		t.Errorf("Source() after edits = %q, want %q", got, "abcXYZ")
	}
	if string(pv.Source()) != string(pv.buf.Bytes()) {
		t.Error("Source() must equal the live buffer bytes in edit mode")
	}
}

func TestTextVsSource(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"a":1}`)
	// Before any reformat, Source() is exactly the typed (minified) bytes.
	if got := string(pv.Source()); got != `{"a":1}` {
		t.Errorf("Source() before reformat = %q, want the typed bytes %q", got, `{"a":1}`)
	}

	pv.Reformat()
	// Reformat pretty-prints the buffer in place, so Source() is now the indented bytes.
	if src := string(pv.Source()); !strings.Contains(src, "\n") || !strings.Contains(src, `"a"`) {
		t.Errorf("Source() after reformat = %q, want the pretty multi-line form", src)
	}
	// Text() is the displayed text of the (now pretty) buffer.
	if txt := pv.Text(); !strings.Contains(txt, "\n") || !strings.Contains(txt, `"a"`) {
		t.Errorf("Text() = %q, want the pretty multi-line form", txt)
	}

	// A read-only viewer keeps v1 Source semantics (the loaded bytes).
	ro := NewWithData([]byte(`{"x":9}`), FormatJSON)
	if got := string(ro.Source()); got != `{"x":9}` {
		t.Errorf("read-only Source() = %q, want v1 %q", got, `{"x":9}`)
	}
}

func TestRawFormattedRoundTrip(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"a":1,"b":2}`)
	pv.Reformat()

	// Reloading the raw Source into a fresh viewer reproduces an equivalent Document.
	raw := pv.Source()
	pv2 := NewWithData(raw, FormatJSON)
	if pv2.doc.TotalLines() != pv.doc.TotalLines() {
		t.Errorf("round-trip line count = %d, want %d", pv2.doc.TotalLines(), pv.doc.TotalLines())
	}
	if pv2.Text() != pv.Text() {
		t.Errorf("round-trip Text mismatch:\n got  %q\n want %q", pv2.Text(), pv.Text())
	}
}
