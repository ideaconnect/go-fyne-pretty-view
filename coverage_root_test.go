package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
)

// TestTypedShortcutRouting covers TypedShortcut's edit actions (cut/paste/undo/redo) and
// confirms those same shortcuts are no-ops on a read-only viewer.
func TestTypedShortcutRouting(t *testing.T) {
	ed, win := renderEditable(t, nil, 600, 400)
	defer win.Close()
	ed.FocusGained()
	typeStr(ed, "hello")

	ed.SelectAll()
	ed.TypedShortcut(&fyne.ShortcutCut{}) // cut the whole buffer
	if got := string(ed.buf.Bytes()); got != "" {
		t.Errorf("Cut shortcut left %q, want empty", got)
	}
	ed.TypedShortcut(&fyne.ShortcutPaste{}) // paste it back
	if got := string(ed.buf.Bytes()); got != "hello" {
		t.Errorf("Paste shortcut = %q, want %q", got, "hello")
	}
	ed.TypedShortcut(&fyne.ShortcutUndo{}) // undo the paste
	if strings.Contains(string(ed.buf.Bytes()), "hello") {
		t.Error("Undo shortcut should revert the paste")
	}
	ed.TypedShortcut(&fyne.ShortcutRedo{}) // redo it
	if got := string(ed.buf.Bytes()); got != "hello" {
		t.Errorf("Redo shortcut = %q, want %q", got, "hello")
	}

	ro, w2 := renderInWindow(t, []byte(`{"a":1}`), FormatJSON, 400, 300)
	defer w2.Close()
	before := string(ro.Source())
	ro.SelectAll()
	ro.TypedShortcut(&fyne.ShortcutCut{})
	ro.TypedShortcut(&fyne.ShortcutPaste{})
	ro.TypedShortcut(&fyne.ShortcutUndo{})
	ro.TypedShortcut(&fyne.ShortcutRedo{})
	if string(ro.Source()) != before {
		t.Error("edit shortcuts must not mutate a read-only viewer")
	}
}

// TestMouseFoldPressAndEmptyClick covers MouseDown's fold-triangle press (which must NOT
// arm a drag) and a click far below all content (which resolves to no line).
func TestMouseFoldPressAndEmptyClick(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"a":{"b":1}}`), FormatJSON, 500, 400)
	defer win.Close()

	li := pv.doc.LineAtRow(0)
	tx := pv.met.TriangleX(pv.doc.Lines[li].Depth) + 1
	pv.MouseDown(desktopMouse(tx, 1)) // press on the fold triangle
	if pv.r.dragArmed {
		t.Error("a press on the fold triangle must not arm a drag")
	}

	// A click on a document with no rows hits no line (hitTest returns -1) and arms nothing.
	empty, w2 := renderInWindow(t, []byte(""), FormatRaw, 400, 300)
	defer w2.Close()
	empty.MouseDown(desktopMouse(20, 50))
	if empty.r.dragArmed {
		t.Error("a click on an empty document must not arm a drag")
	}
}

// TestDragSelectsAndDragEnd covers Dragged/applyHit(default)/DragEnd: a plain drag makes
// an active selection while the drag is armed, and DragEnd finalizes it (disarming).
func TestDragSelectsAndDragEnd(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"key":"value"}`), FormatJSON, 500, 200)
	defer win.Close()

	li := pv.doc.LineAtRow(1)
	x0 := pv.met.TextOriginX(pv.doc.Lines[li].Depth) + 1
	y := pv.met.RowY(1) + 1
	pv.MouseDown(desktopMouse(x0, y))
	pv.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(x0+200, y)}})
	if !pv.sel.active || !pv.r.dragArmed {
		t.Fatalf("a drag should make an active selection with an armed drag (active=%v armed=%v)", pv.sel.active, pv.r.dragArmed)
	}
	pv.DragEnd()
	if pv.r.dragArmed {
		t.Error("DragEnd should disarm the drag")
	}
	if pv.SelectedText() == "" {
		t.Error("a rightward drag should select some text")
	}
}

// TestGrabDragExtendsByWord covers applyHit's word-grab branch: a double-click arms a
// word grab and dragging keeps the selection extended by whole words.
func TestGrabDragExtendsByWord(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"alpha":"beta gamma"}`), FormatJSON, 600, 200)
	defer win.Close()

	li := pv.doc.LineAtRow(1)
	x := pv.met.TextOriginX(pv.doc.Lines[li].Depth) + 2
	y := pv.met.RowY(1) + 1
	pv.MouseDown(desktopMouse(x, y)) // click 1
	pv.MouseDown(desktopMouse(x, y)) // click 2 -> word grab
	if pv.sel.grab != grabWord {
		t.Fatalf("double-click grab = %v, want grabWord", pv.sel.grab)
	}
	pv.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(x+150, y)}})
	if !pv.sel.active {
		t.Error("word-grab drag should keep an active selection")
	}
	pv.DragEnd()
}

// TestSearchEdgeCases covers the empty-query early return, navigation with no matches,
// and an invalid regex (SearchError set, no matches produced).
func TestSearchEdgeCases(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"a":"hello"}`), FormatJSON, 400, 300)
	defer win.Close()

	pv.Search(SearchQuery{Text: ""}) // shorter than minQueryLen -> early return
	if n := len(pv.Matches()); n != 0 {
		t.Errorf("empty query produced %d matches, want 0", n)
	}
	pv.SearchNext() // step with no matches -> no-op
	pv.SearchPrev()

	pv.Search(SearchQuery{Text: "(unclosed", Mode: SearchRegex}) // does not compile
	if pv.SearchError() == nil {
		t.Error("an invalid regex should set SearchError")
	}
	if n := len(pv.Matches()); n != 0 {
		t.Errorf("invalid regex produced %d matches, want 0", n)
	}
}
