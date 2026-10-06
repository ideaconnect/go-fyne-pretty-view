package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// caretStepModes builds the same raw text as a read-only viewer and as an editor, so each
// caret-step test runs against both: the viewer has a logical caret too (SetCaret,
// keyExtend), and #129 was missing in both.
func caretStepModes(t *testing.T, src string) map[string]func() (*PrettyView, fyne.Window) {
	return map[string]func() (*PrettyView, fyne.Window){
		"viewer": func() (*PrettyView, fyne.Window) {
			return renderInWindow(t, []byte(src), FormatRaw, 600, 400)
		},
		"editor": func() (*PrettyView, fyne.Window) {
			return renderEditable(t, []byte(src), 600, 400)
		},
	}
}

// wordKey sends the platform word-step shortcut (Ctrl or Option + Left/Right), plus Shift
// when shift is set, the way the desktop driver delivers it: as a CustomShortcut.
func wordKey(pv *PrettyView, key fyne.KeyName, shift bool) {
	mod := wordStepModifier
	if shift {
		mod |= fyne.KeyModifierShift
	}
	pv.TypedShortcut(&desktop.CustomShortcut{KeyName: key, Modifier: mod})
}

// TestShiftHorizontalExtendsSelection: Shift+Right/Left grow the selection one rune at a time
// from the caret, cross a line end into the next line, stop at the document start, and leave
// Shift+Down working as before (#129).
func TestShiftHorizontalExtendsSelection(t *testing.T) {
	for name, build := range caretStepModes(t, "abc def\nghi") {
		t.Run(name, func(t *testing.T) {
			pv, win := build()
			defer win.Close()
			pv.FocusGained()

			pv.SetCaret(0, 1)
			pv.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
			for i := 0; i < 3; i++ {
				pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
			}
			if got := pv.SelectedText(); got != "bc " {
				t.Errorf("3x Shift+Right from (0,1) selected %q, want %q", got, "bc ")
			}
			for i := 0; i < 5; i++ { // back past the anchor and into the document start
				pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
			}
			if got := pv.SelectedText(); got != "a" {
				t.Errorf("then 5x Shift+Left selected %q, want %q", got, "a")
			}
			if l, c := pv.Caret(); l != 0 || c != 0 {
				t.Errorf("caret after Shift+Left past the start = (%d,%d), want (0,0)", l, c)
			}

			pv.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
			pv.SetCaret(0, 7) // end of the first line
			pv.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
			pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
			if got := pv.SelectedText(); got != "\n" {
				t.Errorf("Shift+Right at a line end selected %q, want the line break", got)
			}
			if l, c := pv.Caret(); l != 1 || c != 0 {
				t.Errorf("caret after Shift+Right at a line end = (%d,%d), want (1,0)", l, c)
			}

			pv.SetCaret(0, 4)
			pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown}) // regression: vertical extend unchanged
			if got := pv.SelectedText(); got != "def\nghi" {
				t.Errorf("Shift+Down from (0,4) selected %q, want %q", got, "def\nghi")
			}
		})
	}
}

// TestWordStepCaret: the platform word modifier + Left/Right moves the caret to the next word
// boundary using the double-click classes (punctuation is its own word, whitespace is
// skipped), crosses line ends, stops at both document ends and collapses a selection.
func TestWordStepCaret(t *testing.T) {
	type pos struct{ line, col int }
	forward := []pos{{0, 3}, {0, 4}, {0, 7}, {0, 12}, {1, 0}, {1, 3}, {1, 3}}
	backward := []pos{{1, 0}, {0, 12}, {0, 9}, {0, 4}, {0, 3}, {0, 0}, {0, 0}}
	for name, build := range caretStepModes(t, "foo.bar  baz\nqux") {
		t.Run(name, func(t *testing.T) {
			pv, win := build()
			defer win.Close()
			pv.FocusGained()

			pv.SetCaret(0, 0)
			for i, want := range forward {
				wordKey(pv, fyne.KeyRight, false)
				if l, c := pv.Caret(); l != want.line || c != want.col {
					t.Errorf("word-right #%d = (%d,%d), want (%d,%d)", i+1, l, c, want.line, want.col)
				}
			}
			for i, want := range backward {
				wordKey(pv, fyne.KeyLeft, false)
				if l, c := pv.Caret(); l != want.line || c != want.col {
					t.Errorf("word-left #%d = (%d,%d), want (%d,%d)", i+1, l, c, want.line, want.col)
				}
			}

			// The other platform's modifier is not a word step.
			other := fyne.KeyModifierControl
			if wordStepModifier == fyne.KeyModifierControl {
				other = fyne.KeyModifierAlt
			}
			pv.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyRight, Modifier: other})
			if l, c := pv.Caret(); l != 0 || c != 0 {
				t.Errorf("non-platform modifier moved the caret to (%d,%d)", l, c)
			}

			pv.SelectAll()
			wordKey(pv, fyne.KeyLeft, false)
			if pv.SelectedText() != "" {
				t.Errorf("a word step without Shift kept the selection %q", pv.SelectedText())
			}
			if l, c := pv.Caret(); l != 1 || c != 0 {
				t.Errorf("word-left from a select-all focus = (%d,%d), want (1,0)", l, c)
			}
		})
	}
}

// TestShiftWordStepExtends: Shift plus the word modifier extends the selection to each word
// boundary, both ways, keeping the anchor.
func TestShiftWordStepExtends(t *testing.T) {
	for name, build := range caretStepModes(t, "foo.bar  baz\nqux") {
		t.Run(name, func(t *testing.T) {
			pv, win := build()
			defer win.Close()
			pv.FocusGained()

			pv.SetCaret(0, 4)
			wordKey(pv, fyne.KeyRight, true)
			wordKey(pv, fyne.KeyRight, true)
			if got := pv.SelectedText(); got != "bar  baz" {
				t.Errorf("2x Shift+word-right from (0,4) selected %q, want %q", got, "bar  baz")
			}
			wordKey(pv, fyne.KeyRight, true) // across the line end
			if got := pv.SelectedText(); got != "bar  baz\n" {
				t.Errorf("3x Shift+word-right selected %q, want %q", got, "bar  baz\n")
			}
			for i := 0; i < 4; i++ { // back over the anchor: baz, bar, then "." on its left
				wordKey(pv, fyne.KeyLeft, true)
			}
			if got := pv.SelectedText(); got != "." {
				t.Errorf("then 4x Shift+word-left selected %q, want %q", got, ".")
			}
		})
	}
}

// TestWordModifierFor pins the per-OS word-step modifier: Option on macOS, Ctrl elsewhere,
// matching widget.Entry.
func TestWordModifierFor(t *testing.T) {
	for goos, want := range map[string]fyne.KeyModifier{
		"darwin":  fyne.KeyModifierAlt,
		"linux":   fyne.KeyModifierControl,
		"windows": fyne.KeyModifierControl,
		"freebsd": fyne.KeyModifierControl,
	} {
		if got := wordModifierFor(goos); got != want {
			t.Errorf("wordModifierFor(%q) = %v, want %v", goos, got, want)
		}
	}
}

// TestPlainArrowCollapsesSelection: in the editor a plain Left/Right with a selection drops it
// onto its left/right end (whichever way it was made), instead of stepping from the focus.
func TestPlainArrowCollapsesSelection(t *testing.T) {
	pv, win := renderEditable(t, []byte("abcdefgh"), 600, 400)
	defer win.Close()
	pv.FocusGained()

	for _, tc := range []struct {
		name          string
		anchor, focus int
		key           fyne.KeyName
		want          int
	}{
		{"forward selection, Left", 2, 5, fyne.KeyLeft, 2},
		{"forward selection, Right", 2, 5, fyne.KeyRight, 5},
		{"backward selection, Left", 5, 2, fyne.KeyLeft, 2},
		{"backward selection, Right", 5, 2, fyne.KeyRight, 5},
	} {
		pv.SetCaret(0, tc.anchor)
		pv.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
		for pv.sel.focus.col != tc.focus {
			k := fyne.KeyRight
			if tc.focus < tc.anchor {
				k = fyne.KeyLeft
			}
			pv.TypedKey(&fyne.KeyEvent{Name: k})
		}
		pv.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
		pv.TypedKey(&fyne.KeyEvent{Name: tc.key})
		if l, c := pv.Caret(); l != 0 || c != tc.want || pv.SelectedText() != "" {
			t.Errorf("%s: caret (%d,%d) sel %q, want (0,%d) and no selection", tc.name, l, c, pv.SelectedText(), tc.want)
		}
	}
}

// TestWordStepEndsTypingRun: a word step is a caret move, so it ends the undo typing run even
// when the caret cannot move (at the end of the buffer): Undo then removes only the later run.
func TestWordStepEndsTypingRun(t *testing.T) {
	pv, win := renderEditable(t, nil, 600, 400)
	defer win.Close()
	pv.FocusGained()

	typeStr(pv, "ab")
	wordKey(pv, fyne.KeyRight, false)
	typeStr(pv, "cd")
	pv.Undo()
	if got := string(pv.buf.Bytes()); got != "ab" {
		t.Errorf("Undo after a word step = %q, want %q (the word step must break the run)", got, "ab")
	}
}

// TestStepPosSkipsFoldAndWrap: a step off a line end lands on the next VISIBLE line, past the
// lines a collapsed node hides, and past every sub-row of a soft-wrapped line.
func TestStepPosSkipsFoldAndWrap(t *testing.T) {
	src := "{\n  \"a\": {\n    \"x\": 1\n  },\n  \"b\": \"" + strings.Repeat("word ", 60) + "\",\n  \"c\": 3\n}"
	pv, win := renderInWindow(t, []byte(src), FormatJSON, 200, 400)
	defer win.Close()
	pv.FocusGained()

	head := lineContaining(pv.doc, `"a"`)
	pv.toggleFold(pv.doc.Lines[head].Fold)
	pv.SetCaret(int(head), 9999) // end of the collapsed head
	pv.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if want := lineContaining(pv.doc, `"b"`); pv.sel.focus != (modelPos{line: want, col: 0}) {
		t.Errorf("Shift+Right off a collapsed head = %v, want line %d col 0", pv.sel.focus, want)
	}

	pv.SetWrap(WrapWord)
	pv.Refresh()
	b := lineContaining(pv.doc, `"b"`)
	if pv.doc.RowsOfLine(b) < 2 {
		t.Fatalf("precondition: the \"b\" line should wrap, rows=%d", pv.doc.RowsOfLine(b))
	}
	pv.SetCaret(int(b), 9999)
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if want := lineContaining(pv.doc, `"c"`); pv.sel.focus != (modelPos{line: want, col: 0}) {
		t.Errorf("Shift+Right off a wrapped line = %v, want line %d col 0", pv.sel.focus, want)
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft}) // and back onto the wrapped line's end
	if want := (modelPos{line: b, col: pv.doc.LineRuneLen(b)}); pv.sel.focus != want {
		t.Errorf("Shift+Left back = %v, want %v", pv.sel.focus, want)
	}
}
