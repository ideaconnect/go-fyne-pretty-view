package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
)

// tallDoc is a many-line JSON array, so vertical scrolling and paging have somewhere to go.
func tallDoc() []byte {
	var b strings.Builder
	b.WriteString("[\n")
	for i := 0; i < 400; i++ {
		b.WriteString("  \"line\",\n")
	}
	b.WriteString("  \"end\"\n]")
	return []byte(b.String())
}

// TestReadOnlyNavKeys exercises the read-only keyboard navigation switch in TypedKey:
// every arrow scrolls, Space/PageDown and PageUp page, Home/End jump, and Escape clears
// the selection. (Edit mode reroutes these; this is the v1 viewer path.)
func TestReadOnlyNavKeys(t *testing.T) {
	pv, win := renderInWindow(t, tallDoc(), FormatJSON, 400, 200)
	defer win.Close()
	pv.FocusGained()

	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd}) // jump to bottom
	if pv.r.scroll.Offset.Y <= 0 {
		t.Fatal("End should scroll to the bottom")
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome}) // back to top
	if pv.r.scroll.Offset.Y != 0 {
		t.Fatalf("Home should return to the top, y=%.1f", pv.r.scroll.Offset.Y)
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyPageDown})
	pgY := pv.r.scroll.Offset.Y
	if pgY <= 0 {
		t.Fatal("PageDown should scroll down a viewport")
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyPageUp})
	if pv.r.scroll.Offset.Y >= pgY {
		t.Error("PageUp should scroll back up")
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeySpace}) // Space pages down (v1 meaning)
	if pv.r.scroll.Offset.Y <= 0 {
		t.Error("Space should page down in read-only mode")
	}

	// Horizontal arrows scroll left/right against a wide line.
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	x0 := pv.r.scroll.Offset.X
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if pv.r.scroll.Offset.X < x0 {
		t.Error("Right arrow should not scroll left")
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})

	// Escape clears an active selection (read-only path).
	pv.SelectAll()
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if pv.sel.active {
		t.Error("Escape should clear the selection in read-only mode")
	}
}

// TestShiftArrowExtendFromUnplaced covers keyExtend's "no caret yet" seeding branch and
// the Shift+Up / Shift+Home / Shift+End extend cases in TypedKey.
func TestShiftArrowExtendFromUnplaced(t *testing.T) {
	pv, win := renderInWindow(t, tallDoc(), FormatJSON, 400, 200)
	defer win.Close()
	pv.FocusGained()
	pv.ClearSelection() // ensure no caret is placed

	pv.shiftHeld = true
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown}) // first shift move seeds the caret, then extends
	if !pv.sel.placed {
		t.Fatal("a shift-arrow must seed a caret when none is placed")
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	if !pv.sel.active {
		t.Error("Shift+End should extend an active selection from the caret")
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	pv.shiftHeld = false
}
