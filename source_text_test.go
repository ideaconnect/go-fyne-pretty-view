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
	pv.Reformat()

	// Source is the raw bytes the user typed (minified).
	if got := string(pv.Source()); got != `{"a":1}` {
		t.Errorf("Source() = %q, want the raw typed bytes %q", got, `{"a":1}`)
	}
	// Text is the pretty, multi-line displayed form — distinct from raw Source.
	txt := pv.Text()
	if !strings.Contains(txt, "\n") || !strings.Contains(txt, `"a"`) {
		t.Errorf("Text() = %q, want the pretty multi-line form", txt)
	}
	if txt == string(pv.Source()) {
		t.Error("Text() should differ from raw Source() after a reformat")
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
