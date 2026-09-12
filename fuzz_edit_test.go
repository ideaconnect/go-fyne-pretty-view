package prettyview

import (
	"strings"
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
	f.Add(strings.Repeat("a\n", 300)) // #72: forces >200 ops; would over-evict at the default undo cap
	// One app + window for the whole run, NOT one per input. Fyne's global renderer cache
	// keeps every widget's renderer (and, through it, the parsed font faces) alive for at
	// least a minute after its window closes, and the test driver only runs the cleaner on
	// Capture(). Four workers opening a window per exec therefore grew past the CI runner's
	// memory in under a minute and the nightly job died with SIGTERM (exit 143), never
	// reaching a verdict. A single widget reset with SetText("") per input is bounded:
	// SetData re-seeds the buffer and drops the undo/redo history, which is exactly the
	// fresh start the round-trip oracle needs.
	test.NewApp()
	// An effectively-unlimited undo so the round-trip oracle is sound: with the default
	// cap a long input legitimately evicts the oldest ops, and undo-all then cannot reach
	// the empty start — correct behavior the round-trip must not mistake for a bug.
	pv := New(WithEditable(), WithUndoLimit(1<<30), WithInputConfig(InputConfig{AutoFormat: AutoFormatOff}))
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))
	pv.Refresh()
	pv.FocusGained()

	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1000 {
			return
		}
		pv.SetText("") // fresh buffer, empty history, caret at 0
		if len(pv.hist.undo) != 0 || len(pv.hist.redo) != 0 || pv.buf.Len() != 0 {
			t.Fatalf("SetText(\"\") did not reset the editor: undo=%d redo=%d len=%d",
				len(pv.hist.undo), len(pv.hist.redo), pv.buf.Len())
		}

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
