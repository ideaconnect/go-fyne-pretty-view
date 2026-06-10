package prettyview

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// Input interfaces implemented by PrettyView. Fold toggling rides on Tapped;
// character-level selection rides on the mouse/drag/focus interfaces handled in
// selection.go; the right-click context menu rides on TappedSecondary.
var (
	_ fyne.Tappable          = (*PrettyView)(nil)
	_ fyne.SecondaryTappable = (*PrettyView)(nil)
	_ desktop.Cursorable     = (*PrettyView)(nil)
	_ desktop.Mouseable      = (*PrettyView)(nil)
	_ desktop.Hoverable      = (*PrettyView)(nil)
	_ fyne.Draggable         = (*PrettyView)(nil)
	_ fyne.Focusable         = (*PrettyView)(nil)
	_ fyne.Shortcutable      = (*PrettyView)(nil)
)

// contentPos converts a widget-local pixel (as delivered to input handlers) into
// content space by adding the scroll offset. The scroll fills the widget, so the
// widget-local origin coincides with the viewport origin.
func (pv *PrettyView) contentPos(local fyne.Position) (float32, float32) {
	if pv.r == nil {
		return local.X, local.Y
	}
	off := pv.r.scroll.Offset
	return local.X + off.X, local.Y + off.Y
}

// foldNodeAt returns the foldable node whose triangle gutter contains the given
// content-space point, or model.NoNode.
func (pv *PrettyView) foldNodeAt(contentX, contentY float32) model.NodeID {
	if pv.doc == nil {
		return model.NoNode
	}
	total := pv.doc.TotalVisibleRows()
	if total == 0 {
		return model.NoNode
	}
	row := pv.met.RowAtY(contentY)
	if row < 0 || int32(row) >= total {
		return model.NoNode
	}
	li, sub := pv.doc.LineAndSubRowAtRow(int32(row))
	line := &pv.doc.Lines[li]
	if line.Fold == model.NoNode || sub != 0 {
		return model.NoNode // the fold triangle lives only on the head's first visual row
	}
	// Hot-zone: the triangle gutter just left of the text, plus the text origin
	// slack, so clicks slightly off the glyph still register.
	x0 := pv.met.TriangleX(line.Depth)
	x1 := pv.met.TextOriginX(line.Depth)
	if contentX >= x0-2 && contentX <= x1 {
		return line.Fold
	}
	return model.NoNode
}

// Tapped toggles a fold when the tap lands on a fold triangle. Other taps are
// left to the selection layer (M8).
func (pv *PrettyView) Tapped(e *fyne.PointEvent) {
	cx, cy := pv.contentPos(e.Position)
	if node := pv.foldNodeAt(cx, cy); node != model.NoNode {
		pv.toggleFold(node)
	}
}

// toggleFold flips a node's fold state and refreshes the view.
func (pv *PrettyView) toggleFold(node model.NodeID) {
	pv.doc.Toggle(node)
	pv.refreshContent()
}

// TappedSecondary shows the context menu at the click. This is the standard Fyne
// pop-up menu — the same themed overlay Entry and read-only selectable text use
// for their right-click menus (Fyne draws its own UI on the GL canvas, so there is
// no native OS menu to invoke). A right-click never disturbs the selection
// (MouseDown returns early for the secondary button), so "Copy" acts on whatever
// the user had already highlighted.
func (pv *PrettyView) TappedSecondary(e *fyne.PointEvent) {
	pv.requestFocus()
	app := fyne.CurrentApp()
	if app == nil || app.Driver() == nil {
		return
	}
	c := app.Driver().CanvasForObject(pv)
	if c == nil {
		return
	}
	widget.ShowPopUpMenuAtPosition(pv.contextMenu(pv.nodeAtPosition(e.Position)), c, e.AbsolutePosition)
}

// nodeAtPosition resolves the structural node owning the display line under a local
// pointer position, or NoNode if the click is below the content or on an empty
// document. The context menu uses it to offer "Copy subtree" for the clicked node;
// keying off the line's Owner (not a byte offset) makes it work for every format.
func (pv *PrettyView) nodeAtPosition(local fyne.Position) model.NodeID {
	if pv.doc == nil {
		return model.NoNode
	}
	cx, cy := pv.contentPos(local)
	pos := pv.hitTest(cx, cy)
	if pos.line < 0 || int(pos.line) >= len(pv.doc.Lines) {
		return model.NoNode
	}
	return pv.doc.Lines[pos.line].Owner
}

// contextMenu builds the right-click menu: Copy (greyed out unless there is a
// selection) and Select all (greyed out on an empty document). Both carry their
// keyboard accelerator so the menu reads like a native one. hasSelection is the
// cheap ordered() predicate, not SelectedText, so opening the menu over a
// select-all of a multi-megabyte document does not materialize the whole string.
func (pv *PrettyView) contextMenu(node model.NodeID) *fyne.Menu {
	_, _, hasSelection := pv.ordered()
	empty := pv.doc == nil || pv.doc.TotalVisibleRows() == 0

	copyItem := fyne.NewMenuItem("Copy", pv.CopySelection)
	copyItem.Disabled = !hasSelection
	copyItem.Shortcut = &fyne.ShortcutCopy{}

	items := []*fyne.MenuItem{copyItem}

	// "Copy subtree" copies the node owning the right-clicked line — a value, or a
	// whole {…}/[…]/<tag>…</tag> container — regardless of fold state. It keys off the
	// line's Owner, so unlike CopySubtree(byteOffset) it works for every format.
	if !empty && node != model.NoNode && node != 0 {
		n := node
		items = append(items, fyne.NewMenuItem("Copy subtree", func() {
			if app := fyne.CurrentApp(); app != nil {
				app.Clipboard().SetContent(pv.subtreeText(n))
			}
		}))
	}

	selectAll := fyne.NewMenuItem("Select all", pv.SelectAll)
	selectAll.Disabled = empty
	selectAll.Shortcut = &fyne.ShortcutSelectAll{}
	items = append(items, selectAll)

	return fyne.NewMenu("", items...)
}

// refreshContent re-sizes the scroll content (row count / width may have changed)
// and reflows, without re-measuring the font or rebuilding the palette.
func (pv *PrettyView) refreshContent() {
	if pv.r == nil {
		return
	}
	pv.r.scroll.Content.Resize(pv.contentSize())
	pv.r.refreshBars() // bars only; reflow() below rebuilds the visible rows once
	pv.r.reflow()
}

// Cursor reports the pointer shape: a pointer over a fold triangle, the text
// I-beam elsewhere. The over-triangle flag is updated by hover tracking.
func (pv *PrettyView) Cursor() desktop.Cursor {
	if pv.overTriangle {
		return desktop.PointerCursor
	}
	return desktop.TextCursor
}
