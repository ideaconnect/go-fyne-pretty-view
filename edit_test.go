package prettyview

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/geometry"
)

// renderEditable puts an editable PrettyView (optionally seeded with src) in a test
// window and forces a layout pass.
func renderEditable(t *testing.T, src []byte, w, h float32) (*PrettyView, fyne.Window) {
	t.Helper()
	test.NewApp()
	pv := New(WithEditable())
	if src != nil {
		pv.SetData(src, FormatRaw)
	}
	win := test.NewWindow(pv)
	win.Resize(fyne.NewSize(w, h))
	pv.Refresh()
	if pv.r == nil {
		t.Fatal("renderer was not created")
	}
	return pv, win
}

func TestEditableDefaultsReadOnly(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":1}`), FormatJSON)
	if pv.Editable() {
		t.Fatal("New()/NewWithData() must default to read-only")
	}
	if pv.buf != nil {
		t.Error("a read-only widget must not allocate an edit buffer")
	}
	before := append([]byte(nil), pv.Source()...)
	pv.FocusGained()
	pv.TypedRune('x') // must be a no-op, exactly as v1
	if !bytes.Equal(pv.Source(), before) {
		t.Errorf("TypedRune mutated a read-only viewer: %q -> %q", before, pv.Source())
	}
}

func TestWithEditableSnapshotsBufferAtBuild(t *testing.T) {
	test.NewApp()
	if pv := New(); pv.buf != nil {
		t.Error("read-only New() should allocate no edit buffer")
	}
	if pv := New(WithEditable()); pv.buf == nil {
		t.Error("New(WithEditable()) should own an edit buffer at construction")
	}

	src := []byte(`{"a":1}`)
	pv := NewWithData(src, FormatJSON, WithEditable())
	if got := pv.buf.Bytes(); !bytes.Equal(got, src) {
		t.Errorf("buffer = %q, want seeded from src %q", got, src)
	}
	// The buffer must own its bytes: mutating the caller's src must not bleed in.
	src[1] = 'X'
	if bytes.Contains(pv.buf.Bytes(), []byte("X")) {
		t.Errorf("edit buffer aliases the caller's src slice: %q", pv.buf.Bytes())
	}
}

// TestNoRuntimeEditableToggle pins DECISION V2-3: the mode is construction-time only.
// (TestExportedSurfaceGolden is the comprehensive surface guard; this is the targeted
// assertion that no runtime setter/hook method exists on the widget.)
func TestNoRuntimeEditableToggle(t *testing.T) {
	pt := reflect.TypeOf(&PrettyView{})
	for _, name := range []string{"SetEditable", "OnEditModeChanged", "SetOnEditModeChanged"} {
		if _, ok := pt.MethodByName(name); ok {
			t.Errorf("%s exists — input/output mode must be fixed at construction (deferred to #54)", name)
		}
	}
	if !New(WithEditable()).Editable() {
		t.Fatal("Editable() should report the constructed mode")
	}
}

func TestEditInsertDeleteRoundTrip(t *testing.T) {
	pv, win := renderEditable(t, nil, 600, 400)
	defer win.Close()
	pv.FocusGained()

	for i, r := range "hello" {
		pv.TypedRune(r)
		if got := pv.sel.focus.col; got != i+1 {
			t.Errorf("after typing %q caret col=%d, want %d", string(r), got, i+1)
		}
	}
	if got := string(pv.buf.Bytes()); got != "hello" {
		t.Fatalf("buffer after typing = %q, want %q", got, "hello")
	}

	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if got := string(pv.buf.Bytes()); got != "hel" {
		t.Errorf("after 2 backspaces buffer = %q, want %q", got, "hel")
	}
	if got := pv.sel.focus.col; got != 3 {
		t.Errorf("caret col after backspaces = %d, want 3", got)
	}

	// Forward delete at end-of-line is a no-op; at line start it joins lines.
	pv.TypedRune('\n')
	pv.TypedRune('x')                  // buffer: "hel\nx", caret line 1 col 1
	pv.keyMoveCaret(0, 0, true, false) // Home -> line 1 col 0
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if got := string(pv.buf.Bytes()); got != "helx" {
		t.Errorf("backspace at line start should join lines: %q, want %q", got, "helx")
	}
}

// TestEditEnterAtEndShowsCaret guards the trailing-empty-line behavior: pressing Enter
// at the end of the buffer must put the caret on a visible new empty line (not vanish
// because the viewer's raw parser drops the line after a final newline).
func TestEditEnterAtEndShowsCaret(t *testing.T) {
	pv, win := renderEditable(t, nil, 600, 400)
	defer win.Close()
	pv.FocusGained()

	pv.TypedRune('a')
	pv.TypedRune('b')
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn}) // "ab\n" -> caret on the new empty line
	if got := pv.sel.focus; got != (modelPos{line: 1, col: 0}) {
		t.Fatalf("caret after Enter-at-end = %v, want (1,0)", got)
	}
	if got := pv.doc.TotalLines(); got != 2 {
		t.Errorf("doc has %d lines, want 2 (a trailing empty line for the caret)", got)
	}
	if cr := pv.r.caretRect; cr == nil || !cr.Visible() {
		t.Error("caret must stay visible on the trailing empty line after Enter")
	}
	pv.TypedRune('c')
	if got := string(pv.buf.Bytes()); got != "ab\nc" {
		t.Errorf("typing after Enter-at-end = %q, want %q", got, "ab\nc")
	}
}

func TestCaretRendersAndAdvances(t *testing.T) {
	pv, win := renderEditable(t, nil, 600, 400)
	defer win.Close()
	pv.FocusGained()

	for _, r := range "abc" {
		pv.TypedRune(r)
		cr := pv.r.caretRect
		if cr == nil || !cr.Visible() {
			t.Fatalf("caret not rendered after typing %q", string(r))
		}
		wantX, wantY := geometry.CellOrigin(pv.doc, pv.met, pv.sel.focus.line, pv.sel.focus.col)
		if cr.Position().X != wantX || cr.Position().Y != wantY {
			t.Errorf("caret at %v, want CellOrigin(%v,%v)", cr.Position(), wantX, wantY)
		}
	}

	// The caret disappears when focus is lost and returns on focus.
	pv.FocusLost()
	if pv.r.caretRect != nil && pv.r.caretRect.Visible() {
		t.Error("caret should hide when the widget loses focus")
	}
	pv.FocusGained()
	if pv.r.caretRect == nil || !pv.r.caretRect.Visible() {
		t.Error("caret should reappear on focus")
	}
}

func TestEditEnterAndArrows(t *testing.T) {
	pv, win := renderEditable(t, []byte("abcd"), 600, 400)
	defer win.Close()
	pv.FocusGained()

	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome}) // place caret at (0,0)
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight}) // caret -> (0,2)
	if pv.sel.focus != (modelPos{line: 0, col: 2}) {
		t.Fatalf("caret = %v, want (0,2)", pv.sel.focus)
	}

	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got := string(pv.buf.Bytes()); got != "ab\ncd" {
		t.Errorf("Enter buffer = %q, want %q", got, "ab\ncd")
	}
	if pv.sel.focus != (modelPos{line: 1, col: 0}) {
		t.Errorf("caret after Enter = %v, want (1,0)", pv.sel.focus)
	}

	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	if pv.sel.focus != (modelPos{line: 1, col: 2}) {
		t.Errorf("End -> %v, want (1,2)", pv.sel.focus)
	}

	// A plain arrow clears an active selection.
	pv.SelectAll()
	if !pv.sel.active {
		t.Fatal("SelectAll should produce an active selection")
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if pv.sel.active {
		t.Error("a plain arrow should clear the selection in edit mode")
	}
}

func TestReadOnlyKeysUnchanged(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"a":{"b":1}}`), FormatJSON, 400, 300)
	defer win.Close()
	pv.FocusGained()
	if pv.Editable() {
		t.Fatal("precondition: read-only widget")
	}

	// Enter still toggles the fold (v1), and the source is never mutated.
	a := findFoldHead(pv.doc, `"a"`)
	pv.sel.focus = modelPos{line: pv.doc.Nodes[a].HeadLine, col: 0}
	pv.sel.placed = true
	srcBefore := append([]byte(nil), pv.Source()...)
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !pv.doc.Collapsed(a) {
		t.Error("Enter did not fold-toggle in read-only mode")
	}
	if !bytes.Equal(pv.Source(), srcBefore) {
		t.Error("Enter mutated the source in read-only mode")
	}

	// A plain arrow still scrolls (does not edit).
	long := strings.Repeat("abcdefghij", 200)
	pv2, win2 := renderInWindow(t, []byte(`["`+long+`"]`), FormatJSON, 300, 200)
	defer win2.Close()
	pv2.FocusGained()
	x0 := pv2.r.scroll.Offset.X
	pv2.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if pv2.r.scroll.Offset.X <= x0 {
		t.Error("Right arrow did not scroll in read-only mode")
	}
}

func TestEditKeepsViewportManyRows(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 5000; i++ {
		sb.WriteString("line of text\n")
	}
	pv, win := renderEditable(t, []byte(sb.String()), 800, 600)
	defer win.Close()
	pv.FocusGained()

	bound := int(600/pv.met.RowH) + 4
	for _, r := range "typing" {
		pv.TypedRune(r)
		if n := len(pv.r.live); n > bound {
			t.Fatalf("live rows %d exceed viewport bound %d while editing — virtualization broke", n, bound)
		}
	}
}

func TestCaretBeyondViewportScrolls(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("x\n")
	}
	pv, win := renderEditable(t, []byte(sb.String()), 600, 200)
	defer win.Close()
	pv.FocusGained()

	before := pv.r.scroll.Offset.Y
	for i := 0; i < 500; i++ { // drive the caret down past the viewport
		pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	}
	if after := pv.r.scroll.Offset.Y; after <= before {
		t.Errorf("driving the caret below the fold did not scroll: before=%.1f after=%.1f", before, after)
	}
}
