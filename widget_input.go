package prettyview

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// Input interfaces implemented by PrettyView. Fold toggling rides on Tapped;
// character-level selection rides on the mouse/drag/focus interfaces handled in
// selection.go.
var (
	_ fyne.Tappable      = (*PrettyView)(nil)
	_ desktop.Cursorable = (*PrettyView)(nil)
	_ desktop.Mouseable  = (*PrettyView)(nil)
	_ desktop.Hoverable  = (*PrettyView)(nil)
	_ fyne.Draggable     = (*PrettyView)(nil)
	_ fyne.Focusable     = (*PrettyView)(nil)
	_ fyne.Shortcutable  = (*PrettyView)(nil)
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
	li := pv.doc.LineAtRow(int32(row))
	line := &pv.doc.Lines[li]
	if line.Fold == model.NoNode {
		return model.NoNode
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
