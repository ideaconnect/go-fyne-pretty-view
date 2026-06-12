package prettyview

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

// TestSearchEntryShiftRouting covers the search box's key handling: Shift tracking
// (KeyDown sets / KeyUp & FocusLost clear the held flag) and the Enter/Esc routing that
// depends on it — Shift+Enter finds previous, Esc clears.
func TestSearchEntryShiftRouting(t *testing.T) {
	test.NewApp()
	e := newSearchEntry()
	win := test.NewWindow(e)
	defer win.Close()

	var prev, esc int
	e.onPrev = func() { prev++ }
	e.onEscape = func() { esc++ }

	e.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	if !e.shiftHeld {
		t.Fatal("KeyDown(Shift) should set shiftHeld")
	}
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn}) // Shift+Enter -> find previous
	if prev != 1 {
		t.Errorf("Shift+Enter should call onPrev once, got %d", prev)
	}

	e.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftRight})
	if e.shiftHeld {
		t.Error("KeyUp(Shift) should clear shiftHeld")
	}

	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape}) // Esc -> clear
	if esc != 1 {
		t.Errorf("Esc should call onEscape once, got %d", esc)
	}

	// A Shift released while the box is unfocused would otherwise stick — FocusLost clears it.
	e.shiftHeld = true
	e.FocusLost()
	if e.shiftHeld {
		t.Error("FocusLost should clear a stuck shiftHeld")
	}

	// A plain Enter with no shift held falls through to the embedded Entry (no onPrev call).
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if prev != 1 {
		t.Errorf("plain Enter must not call onPrev, got %d", prev)
	}
}
