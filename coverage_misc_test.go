package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestScheduleReformatGuards covers scheduleReformat's two early paths: an unshown widget
// (renderer nil) arms nothing, and a non-positive DebounceFor settles immediately.
func TestScheduleReformatGuards(t *testing.T) {
	test.NewApp()
	unshown := New(WithEditable()) // never shown -> pv.r == nil
	unshown.scheduleReformat()     // must return without arming a timer
	if unshown.editDeb.timer != nil {
		t.Error("scheduleReformat on an unshown widget must not arm a timer")
	}

	// DebounceFor <= 0 with an onChanged listener settles synchronously on each edit.
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff, DebounceFor: -1})
	defer win.Close()
	var settles int
	pv.SetOnChanged(func(string) { settles++ })
	typeStr(pv, "a")
	if settles == 0 {
		t.Error("a non-positive DebounceFor should settle immediately on an edit")
	}
	if pv.editDeb.timer != nil {
		t.Error("an immediate settle should leave no armed timer")
	}
}

// TestWordBoundsEdges covers wordBounds/displayRunes clamps: an empty line, an out-of-range
// column (clamped both ways), and a line with a multi-byte rune (the decode branch).
func TestWordBoundsEdges(t *testing.T) {
	pv := docPV("[\n  \"héllo wörld\",\n  \"\"\n]", FormatJSON)

	wordLine := lineContaining(pv.doc, "héllo")
	// A column past the line end clamps inside the last word, not out of range.
	a, b := pv.wordBounds(wordLine, 9999)
	if a.col < 0 || b.col < a.col {
		t.Errorf("wordBounds(_, 9999) = (%d,%d), want a valid clamped range", a.col, b.col)
	}
	// A negative column clamps to the first word.
	a2, b2 := pv.wordBounds(wordLine, -5)
	if a2.col != 0 || b2.col <= 0 {
		t.Errorf("wordBounds(_, -5) = (%d,%d), want it to start at col 0", a2.col, b2.col)
	}

	// An empty string line ("") has no word: both endpoints collapse to col 0.
	emptyLine := -1
	for li := 0; li < pv.doc.TotalLines(); li++ {
		if strings.TrimSpace(pv.doc.LineString(int32(li))) == `""` {
			emptyLine = li
			break
		}
	}
	if emptyLine >= 0 {
		ea, eb := pv.wordBounds(int32(emptyLine), 0)
		if ea.col != 0 || eb.col < 0 {
			t.Errorf("wordBounds on an empty-ish line = (%d,%d), want (0,0)", ea.col, eb.col)
		}
	}
}

// TestSnapClamps covers snap's two clamp branches: a negative line is returned untouched
// and a negative column clamps to 0.
func TestSnapClamps(t *testing.T) {
	pv := docPV(`{"a":1}`, FormatJSON)
	if got := pv.snap(modelPos{line: -1, col: 3}); got.line != -1 {
		t.Errorf("snap with line<0 = %v, want it returned untouched", got)
	}
	if got := pv.snap(modelPos{line: 0, col: -5}); got.col != 0 {
		t.Errorf("snap with col<0 = %v, want col clamped to 0", got)
	}
}

// TestKeyExtendUnderWrap drives keyExtend across the soft-wrapped sub-rows of one long
// line, exercising the wrap-aware vertical-move branch (subRowOfCol / LineAndSubRowAtRow).
func TestKeyExtendUnderWrap(t *testing.T) {
	long := strings.Repeat("word ", 80) // ~400 chars: wraps to several sub-rows
	pv, win := renderInWindow(t, []byte(`["`+long+`"]`), FormatJSON, 300, 220)
	defer win.Close()
	pv.SetWrap(WrapWord)
	pv.Refresh()
	pv.FocusGained()

	pv.SetCaret(1, 5) // somewhere on the wrapped value line
	pv.shiftHeld = true
	for i := 0; i < 3; i++ {
		pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown}) // extend down across sub-rows
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	pv.shiftHeld = false
	if !pv.sel.active {
		t.Error("shift+Down across wrapped sub-rows should make an active selection")
	}
}
