package prettyview

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// FuzzEditUndoRoundTrip is the property fuzz for the editor's undo/redo (#72): inserting an
// arbitrary sequence of runes, then undoing every recorded op, must restore the empty start
// buffer; redoing every op must restore the typed text. This exercises the inverse-splice
// undo model (coalescing included) against random multi-line / multibyte / control input.
func FuzzEditUndoRoundTrip(f *testing.F) {
	f.Add("hello world")
	f.Add("a\nb\nc")
	f.Add("中é\tx\x01y")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1000 {
			return
		}
		test.NewApp()
		pv := New(WithEditable(), WithInputConfig(InputConfig{AutoFormat: AutoFormatOff}))
		win := test.NewWindow(pv)
		defer win.Close()
		win.Resize(fyne.NewSize(400, 300))
		pv.Refresh()
		pv.FocusGained()

		for _, r := range s {
			pv.editInsert([]byte(string(r)))
		}
		typed := string(pv.Source())

		for len(pv.hist.undo) > 0 {
			before := len(pv.hist.undo)
			pv.Undo()
			if len(pv.hist.undo) >= before {
				t.Fatalf("Undo did not shrink the stack (stuck at %d)", before)
			}
		}
		if got := string(pv.Source()); got != "" {
			t.Fatalf("undo-all did not restore the empty start: got %q (typed %q)", got, typed)
		}
		for len(pv.hist.redo) > 0 {
			before := len(pv.hist.redo)
			pv.Redo()
			if len(pv.hist.redo) >= before {
				t.Fatalf("Redo did not shrink the redo stack (stuck at %d)", before)
			}
		}
		if got := string(pv.Source()); got != typed {
			t.Fatalf("redo-all did not restore the typed text: got %q want %q", got, typed)
		}
	})
}
