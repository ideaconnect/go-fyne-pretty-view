package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// TestInputHandlersSmoke exercises the pointer/focus/keyboard handlers and the
// clipboard shortcuts that the higher-level tests don't reach directly.
func TestInputHandlersSmoke(t *testing.T) {
	src := []byte(`{"a":{"b":"value"},"c":[1,2,3]}`)
	pv, win := renderInWindow(t, src, FormatJSON, 500, 400)
	defer win.Close()

	// Hover over a fold triangle flips the cursor to a pointer; off it, the I-beam.
	head := findFoldHead(pv.doc, `"a"`)
	depth := pv.doc.Lines[pv.doc.Nodes[head].HeadLine].Depth
	row := int(pv.doc.RowOfLine(pv.doc.Nodes[head].HeadLine))
	over := fyne.NewPos(pv.met.TriangleX(depth)+2, pv.met.RowY(row)+1)
	pv.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: over}})
	if !pv.overTriangle || pv.Cursor() != desktop.PointerCursor {
		t.Errorf("over a triangle: overTriangle=%v cursor=%v", pv.overTriangle, pv.Cursor())
	}
	pv.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(2000, 2000)}})
	if pv.overTriangle || pv.Cursor() != desktop.TextCursor {
		t.Errorf("off any triangle: overTriangle=%v cursor=%v", pv.overTriangle, pv.Cursor())
	}

	// Focus state + no-op handlers.
	pv.FocusGained()
	if !pv.focused {
		t.Error("FocusGained did not set focused")
	}
	pv.FocusLost()
	if pv.focused {
		t.Error("FocusLost did not clear focused")
	}
	pv.MouseIn(nil)
	pv.MouseOut()
	pv.MouseUp(&desktop.MouseEvent{})
	pv.TypedRune('x')

	// Secondary-button press is reserved (early return, selection untouched).
	pv.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonSecondary})

	// Ctrl/Cmd+A selects all; Ctrl/Cmd+C copies it to the clipboard.
	pv.TypedShortcut(&fyne.ShortcutSelectAll{})
	if !pv.sel.active {
		t.Error("ShortcutSelectAll did not activate a selection")
	}
	pv.TypedShortcut(&fyne.ShortcutCopy{})
	if got := fyne.CurrentApp().Clipboard().Content(); !strings.Contains(got, `"value"`) {
		t.Errorf("Ctrl+C clipboard = %q, want it to contain the document text", got)
	}

	// Ctrl/Cmd+F routes to the search-requested hook.
	requested := false
	pv.SetOnSearchRequested(func() { requested = true })
	pv.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault})
	if !requested {
		t.Error("Ctrl+F did not invoke the search-requested hook")
	}

	// CopySubtree copies the node spanning a byte offset.
	off := strings.Index(string(src), `"b"`)
	pv.CopySubtree(off)
	if got := fyne.CurrentApp().Clipboard().Content(); !strings.Contains(got, "value") {
		t.Errorf("CopySubtree clipboard = %q", got)
	}
}

// TestAutoscrollEdge covers the drag-to-edge auto-scroll nudges in all four
// directions plus the no-op center case.
func TestAutoscrollEdge(t *testing.T) {
	src := make([]byte, 0, 4096)
	src = append(src, '[')
	for i := 0; i < 400; i++ {
		if i > 0 {
			src = append(src, ',')
		}
		src = append(src, '0')
	}
	src = append(src, ']')
	pv, win := renderInWindow(t, src, FormatJSON, 300, 200)
	defer win.Close()

	sz := pv.r.scroll.Size()
	// Establish a drag anchor so the edge nudges have a selection to extend.
	pv.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(20, 20)}})
	for _, p := range []fyne.Position{
		{X: sz.Width - 2, Y: sz.Height - 2}, // bottom-right
		{X: 2, Y: 2},                        // top-left
		{X: sz.Width / 2, Y: sz.Height / 2}, // center (no nudge)
	} {
		pv.autoscrollEdge(p)
	}
	pv.DragEnd()
}

// TestRowRendererBoilerplate covers the row renderer's WidgetRenderer methods that
// are pure plumbing (no-op Destroy/Layout, fixed MinSize, Objects).
func TestRowRendererBoilerplate(t *testing.T) {
	pv := New()
	rw := newRowWidget(pv)
	rr := rw.CreateRenderer().(*rowRenderer)
	_ = rr.Objects()
	_ = rr.MinSize()
	rr.Layout(fyne.NewSize(10, 10))
	rr.Destroy()
}
