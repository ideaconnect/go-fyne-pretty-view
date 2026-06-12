package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/parse"
)

func TestSetCaretRoundTrip(t *testing.T) {
	pv, win := renderInWindow(t, []byte("{\n  \"a\": 1,\n  \"b\": 2\n}"), FormatJSON, 600, 400)
	defer win.Close()

	if ok := pv.SetCaret(2, 3); !ok {
		t.Fatal("SetCaret on a visible position should return true")
	}
	if l, c := pv.Caret(); l != 2 || c != 3 {
		t.Errorf("Caret() = (%d,%d), want (2,3)", l, c)
	}

	// Column past the line end clamps; the call still succeeds.
	if ok := pv.SetCaret(1, 9999); !ok {
		t.Fatal("SetCaret with an over-long column should clamp and return true")
	}
	if l, c := pv.Caret(); l != 1 || c != pv.doc.LineRuneLen(1) {
		t.Errorf("Caret() = (%d,%d), want (1, %d) clamped", l, c, pv.doc.LineRuneLen(1))
	}

	// Out-of-range line returns false and leaves the caret put.
	before := pv.sel.focus
	if ok := pv.SetCaret(99999, 0); ok {
		t.Error("SetCaret past the last line should return false")
	}
	if pv.sel.focus != before {
		t.Error("a failed SetCaret must leave the caret unchanged")
	}

	// In read-only mode SetCaret is a logical navigation position only: even focused,
	// no caret bar is drawn (the visible caret is editor-only). This locks the SetCaret
	// godoc against the earlier "navigable caret" overclaim.
	pv.FocusGained()
	pv.SetCaret(2, 1)
	pv.Refresh()
	if pv.r.caretRect != nil && pv.r.caretRect.Visible() {
		t.Error("read-only SetCaret must not render a caret bar")
	}
}

func TestMaxEditBytesRejected(t *testing.T) {
	test.NewApp()
	pv := New(WithEditable(), WithInputConfig(InputConfig{AutoFormat: AutoFormatOff, MaxEditBytes: 5}))
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))
	pv.Refresh()
	pv.FocusGained()

	typeStr(pv, "abcde") // exactly at the cap
	if got := string(pv.buf.Bytes()); got != "abcde" {
		t.Fatalf("buffer = %q, want %q", got, "abcde")
	}
	pv.TypedRune('f') // over the cap -> rejected
	if got := string(pv.buf.Bytes()); got != "abcde" {
		t.Errorf("over-cap insert changed the text to %q, want it rejected", got)
	}

	// A delete is always allowed, and then an insert fits again.
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	pv.TypedRune('Z')
	if got := string(pv.buf.Bytes()); got != "abcdZ" {
		t.Errorf("after delete+insert = %q, want %q", got, "abcdZ")
	}
}

func TestEditAboveCapSkipsAutoReparse(t *testing.T) {
	test.NewApp()
	// Cap below the seeded content, so the buffer starts above the cap.
	pv := New(WithEditable(), WithInputConfig(InputConfig{AutoFormat: AutoFormatOnPause, MaxEditBytes: 4}))
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))
	pv.SetData([]byte(`{"a":1}`), FormatAuto) // 7 bytes > cap 4
	pv.Refresh()
	pv.FocusGained()

	pv.editSettled() // auto path: suppressed above the cap
	if strings.Contains(string(pv.buf.Bytes()), "\n") {
		t.Error("auto-format-on-pause must be suppressed above MaxEditBytes")
	}
	pv.Reformat() // explicit reformat still works above the cap
	if !strings.Contains(string(pv.buf.Bytes()), "\n") || pv.Format() != FormatJSON {
		t.Errorf("explicit Reformat() must work even above the cap, buffer = %q", pv.buf.Bytes())
	}
}

// TestEditMemoryWithinBound checks the edit buffer stays ≈ content-sized (a gap buffer,
// not a 5–7× model), so editing does not balloon memory beyond the documented delta.
func TestEditMemoryWithinBound(t *testing.T) {
	src := []byte(strings.Repeat("{\"k\":\"value\"}\n", 4000)) // ~56 KB
	pv, win := renderEditable(t, src, 800, 600)
	defer win.Close()
	pv.FocusGained()

	// The buffer holds exactly the content (plus an internal gap, never a multiple of it).
	if got := pv.buf.Len(); got != len(src) {
		t.Errorf("buffer Len = %d, want the content size %d", got, len(src))
	}
	for _, r := range "edits" { // a few edits must not grow the logical length unboundedly
		pv.TypedRune(r)
	}
	if got, want := pv.buf.Len(), len(src)+len("edits"); got != want {
		t.Errorf("buffer Len after edits = %d, want %d (content + typed)", got, want)
	}
}

// TestKeyExtendNilRendererSafe is the #76 guard: keyExtend reads pv.r.firstRow, so it must
// be safe when the widget has never been rendered (pv.r == nil). The edit caret-move paths
// must not panic before first render.
func TestKeyExtendNilRendererSafe(t *testing.T) {
	test.NewApp()
	pv := New(WithEditable())
	// Install a non-empty doc WITHOUT rendering (SetData would Refresh -> CreateRenderer and
	// set pv.r). With visible rows and pv.r == nil, keyExtend reaches the pv.r.firstRow deref.
	pv.doc = parse.Parse([]byte("a\nb\nc"), parse.FormatRaw, 0)
	if pv.r != nil {
		t.Fatal("precondition: an unrendered widget should have pv.r == nil")
	}
	if pv.doc.TotalVisibleRows() == 0 {
		t.Fatal("precondition: doc should have visible rows to reach the guarded deref")
	}
	pv.keyExtend(1, keepCol, false, false)    // would deref pv.r.firstRow without the guard
	pv.keyMoveCaret(1, keepCol, false, false) // routes through keyExtend
}
