package prettyview

import (
	"image/color"
	"math/rand"
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

// TestAppendPrettyLine covers the shared model routine directly: it appends after the
// existing buffer, emits `indent` leading spaces, and invokes the span callback for
// source-backed segments with offsets that point into the appended region.
func TestAppendPrettyLine(t *testing.T) {
	d := parse.Parse([]byte(`{"a":1}`), parse.FormatJSON, 0) // -> "{" / "  \"a\": 1" / "}"
	nSpans := 0
	firstOut := -1
	buf := d.AppendPrettyLine(1, 4, []byte("X"), func(srcStart, srcEnd uint32, outStart int) {
		if firstOut < 0 {
			firstOut = outStart
		}
		nSpans++
	})
	got := string(buf)
	if !strings.HasPrefix(got, "X    ") { // pre-existing "X" + 4 indent spaces
		t.Errorf("AppendPrettyLine must append after buf and indent 4: %q", got)
	}
	if !strings.Contains(got, `"a"`) {
		t.Errorf("AppendPrettyLine dropped the line text: %q", got)
	}
	if nSpans == 0 {
		t.Error("expected at least one BufSrc span callback for the member's tokens")
	}
	if firstOut < 5 { // after the "X" + 4 spaces
		t.Errorf("span outStart %d should point past the indent (>=5)", firstOut)
	}
}

// TestPrettyLineConventionConsistency locks the unification (issue #58): Text and
// serializePretty share one routine, so for any document the Reformat bytes must equal the
// viewer Text minus its single trailing newline. If the indent/line conventions ever drift,
// this fails.
func TestPrettyLineConventionConsistency(t *testing.T) {
	srcs := []string{`{"a":1,"b":[2,3],"c":{"d":4}}`, `[1,[2,[3,[4]]]]`, `{}`}
	for _, src := range srcs {
		pv := NewWithData([]byte(src), FormatJSON)
		out, _ := serializePretty(pv.doc)
		if got, want := string(out), strings.TrimSuffix(pv.Text(), "\n"); got != want {
			t.Errorf("src %q: serializePretty != Text-minus-trailing-newline\n got: %q\nwant: %q", src, got, want)
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

// TestLiveColorPaintsInEditRenderer closes the gap between "the model carries a role" and
// "the renderer paints it": after typing JSON, the live row's "key" text run must be drawn
// with palette[RoleKey] — proving real-time highlighting reaches the screen while editing,
// not just that the projection assigned a role.
func TestLiveColorPaintsInEditRenderer(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	typeStr(pv, `{"key":42}`)

	want := pv.palette[model.RoleKey]
	if want == pv.palette[model.RolePunct] {
		t.Skip("theme maps key and punct to the same color; render-color distinction not assertable")
	}
	var got color.Color
	found := false
	for _, rw := range pv.r.live {
		if rw.rr == nil {
			continue
		}
		for _, txt := range rw.rr.texts {
			if txt.Visible() && txt.Text == `"key"` {
				got, found = txt.Color, true
			}
		}
	}
	if !found {
		t.Fatal(`no painted "key" run in the editable view — live highlighting did not render`)
	}
	if got != want {
		t.Errorf("live-typed key painted %v, want palette[RoleKey] %v", got, want)
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

// TestRemapCaretOffsetMatchesLinearOracle locks the #77 binary-search rewrite against the
// previous linear-scan logic (kept here as an oracle) over random ascending, non-overlapping
// span sets and offsets. Any divergence — the kind a subtle off-by-one in the search bound
// would cause — fails.
func TestRemapCaretOffsetMatchesLinearOracle(t *testing.T) {
	linear := func(spans []srcSpan, oldOff, newLen int) int {
		if len(spans) == 0 {
			return 0
		}
		for i, s := range spans {
			switch {
			case oldOff < s.oldStart:
				return clamp(s.newStart, 0, newLen)
			case oldOff < s.oldEnd:
				return clamp(s.newStart+(oldOff-s.oldStart), 0, newLen)
			case i+1 >= len(spans) || oldOff < spans[i+1].oldStart:
				return clamp(s.newStart+(s.oldEnd-s.oldStart), 0, newLen)
			}
		}
		last := spans[len(spans)-1]
		return clamp(last.newStart+(last.oldEnd-last.oldStart), 0, newLen)
	}
	rng := rand.New(rand.NewSource(1))
	for iter := 0; iter < 3000; iter++ {
		n := rng.Intn(8)
		spans := make([]srcSpan, 0, n)
		oldPos, newPos := 0, 0
		for k := 0; k < n; k++ {
			oldPos += rng.Intn(4)     // gap (removed whitespace/punct)
			length := 1 + rng.Intn(5) // token length
			spans = append(spans, srcSpan{oldStart: oldPos, oldEnd: oldPos + length, newStart: newPos})
			oldPos += length
			newPos += length + rng.Intn(3)
		}
		newLen := newPos + rng.Intn(5)
		for q := 0; q < 5; q++ {
			off := rng.Intn(oldPos + 5)
			if got, want := remapCaretOffset(spans, off, newLen), linear(spans, off, newLen); got != want {
				t.Fatalf("remap mismatch: spans=%v off=%d newLen=%d: binary=%d linear=%d", spans, off, newLen, got, want)
			}
		}
	}
}
