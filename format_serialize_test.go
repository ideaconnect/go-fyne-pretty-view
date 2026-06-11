package prettyview

import (
	"strings"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/parse"
)

func TestSerializePrettyJSON(t *testing.T) {
	d := parse.Parse([]byte(`{"a":1,"b":[2,3]}`), parse.FormatJSON, 0)
	out, spans := serializePretty(d)
	want := "{\n  \"a\": 1,\n  \"b\": [\n    2,\n    3\n  ]\n}"
	if string(out) != want {
		t.Errorf("serializePretty =\n%q\nwant\n%q", out, want)
	}
	if len(spans) == 0 {
		t.Error("expected source spans for the JSON tokens")
	}
	// Spans must be ascending by old offset (remapCaretOffset relies on it).
	for i := 1; i < len(spans); i++ {
		if spans[i].oldStart < spans[i-1].oldStart {
			t.Errorf("spans not ascending at %d: %+v before %+v", i, spans[i-1], spans[i])
		}
	}
}

func TestRemapCaretOffset(t *testing.T) {
	spans := []srcSpan{{oldStart: 0, oldEnd: 1, newStart: 0}, {oldStart: 5, oldEnd: 8, newStart: 2}}
	cases := []struct{ off, want int }{
		{0, 0},  // inside the first token
		{6, 3},  // inside the second: 2 + (6-5)
		{3, 1},  // gap after the first token -> its end (0 + 1)
		{7, 4},  // inside the second
		{20, 5}, // past everything -> end of the last (2 + 3)
	}
	for _, c := range cases {
		if got := remapCaretOffset(spans, c.off, 100); got != c.want {
			t.Errorf("remapCaretOffset(off=%d) = %d, want %d", c.off, got, c.want)
		}
	}
	if got := remapCaretOffset(nil, 5, 100); got != 0 {
		t.Errorf("remap with no spans = %d, want 0 (caret to the top)", got)
	}

	// Three spans: an offset in the gap after the MIDDLE token exercises the loop-continue
	// path (the first span did not resolve it).
	spans3 := []srcSpan{{oldStart: 0, oldEnd: 2, newStart: 0}, {oldStart: 4, oldEnd: 6, newStart: 5}, {oldStart: 10, oldEnd: 12, newStart: 20}}
	if got := remapCaretOffset(spans3, 8, 100); got != 7 { // gap after span[1] -> its end (5 + (6-4))
		t.Errorf("remap(8) over 3 spans = %d, want 7", got)
	}
	if got := remapCaretOffset(spans3, 11, 100); got != 21 { // inside span[2]
		t.Errorf("remap(11) over 3 spans = %d, want 21", got)
	}
	// Clamp to newLen when a span maps past it.
	if got := remapCaretOffset([]srcSpan{{oldStart: 0, oldEnd: 100, newStart: 0}}, 50, 10); got != 10 {
		t.Errorf("remap clamp = %d, want 10", got)
	}
}

// TestLiveColorWhileTyping is the headline behavior: with no reformat, the displayed
// projection is already syntax-colored — highlighting is live on every keystroke.
func TestLiveColorWhileTyping(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"key":42}`)
	var sawKey, sawNumber bool
	for li := 0; li < pv.doc.TotalLines(); li++ {
		for _, s := range pv.doc.LineSegs(int32(li)) {
			switch string(pv.doc.SegBytes(s)) {
			case `"key"`:
				sawKey = s.Role == model.RoleKey
			case `42`:
				sawNumber = s.Role == model.RoleNumber
			}
		}
	}
	if !sawKey {
		t.Error("the live projection should color the object key (RoleKey) while typing")
	}
	if !sawNumber {
		t.Error("the live projection should color the number (RoleNumber) while typing")
	}
}

func TestReformatIdempotent(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"a":1,"b":2}`)
	pv.Reformat()
	pretty := string(pv.buf.Bytes())
	caret := pv.caretOff()

	pv.Reformat() // already pretty: a no-op rewrite, no caret jump
	if got := string(pv.buf.Bytes()); got != pretty {
		t.Errorf("second Reformat changed the buffer:\n %q ->\n %q", pretty, got)
	}
	if pv.caretOff() != caret {
		t.Errorf("idempotent Reformat moved the caret %d -> %d", caret, pv.caretOff())
	}
}

func TestUndoRevertsReformat(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"a":1}`)
	mini := string(pv.buf.Bytes())
	pv.Reformat()
	if !strings.Contains(string(pv.buf.Bytes()), "\n") {
		t.Fatal("precondition: Reformat should make the buffer pretty")
	}
	pv.Undo() // a reformat is one undo unit
	if got := string(pv.buf.Bytes()); got != mini {
		t.Errorf("Undo after Reformat = %q, want the pre-reformat bytes %q", got, mini)
	}
}

func TestReformatXMLRewritesBuffer(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `<root><item>hi</item></root>`)
	pv.Reformat()
	if pv.Format() != FormatXML {
		t.Errorf("format = %v, want XML", pv.Format())
	}
	if !strings.Contains(string(pv.buf.Bytes()), "\n") {
		t.Errorf("XML reformat should pretty-print the buffer, got %q", pv.buf.Bytes())
	}
	// XML tokens carry no source offsets, so the caret falls to the top — it must stay valid.
	if l := int(pv.sel.focus.line); l < 0 || l >= pv.doc.TotalLines() {
		t.Errorf("caret line %d out of range after XML reformat (lines=%d)", l, pv.doc.TotalLines())
	}
}
