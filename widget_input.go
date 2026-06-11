package prettyview

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
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
	_ desktop.Keyable        = (*PrettyView)(nil) // KeyDown/KeyUp track Shift for keyboard selection
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
// pointer position, or NoNode on an empty document. A click below the content clamps
// to the last visible line (HitTest), so it resolves to that line's owner. The
// context menu uses this to offer "Copy subtree" for the clicked node; keying off the
// line's Owner (not a byte offset) makes it work for every format.
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

// keyPath returns the JSONPath-style accessor of node from the document root, e.g.
// $.users[2].name — object members contribute .key, array elements [index]. It walks
// Parent links in the node arena; meaningful for JSON/JSONC.
func (pv *PrettyView) keyPath(node model.NodeID) string {
	d := pv.doc
	var accessors []string
	for n := node; n > 0; {
		p := d.Nodes[n].Parent
		if p <= 0 {
			break // parent is the synthetic root: the top-level value is just "$"
		}
		if d.Nodes[p].Kind == model.KindArray {
			accessors = append(accessors, "["+strconv.Itoa(pv.childIndex(p, n))+"]")
		} else {
			accessors = append(accessors, "."+pv.nodeKeyText(n))
		}
		n = p
	}
	var sb strings.Builder
	sb.WriteByte('$')
	for i := len(accessors) - 1; i >= 0; i-- {
		sb.WriteString(accessors[i])
	}
	return sb.String()
}

// childIndex returns the position of child among parent's direct children. Direct
// children are not contiguous in the arena, so it steps sibling-by-sibling using each
// node's Subtree span.
func (pv *PrettyView) childIndex(parent, child model.NodeID) int {
	d := pv.doc
	end := parent + d.Nodes[parent].Subtree
	idx := 0
	for c := parent + 1; c < end; c += d.Nodes[c].Subtree {
		if c == child {
			return idx
		}
		idx++
	}
	return idx
}

// nodeKeyText extracts an object member's key (unquoted) from its head line's RoleKey
// segment, or "" if the node has no key.
func (pv *PrettyView) nodeKeyText(n model.NodeID) string {
	d := pv.doc
	head := d.Nodes[n].HeadLine
	if head < 0 {
		return ""
	}
	for _, seg := range d.LineSegs(head) {
		if seg.Role == model.RoleKey {
			// The key path is gated to JSON/JSONC, so a RoleKey segment is a valid JSON
			// string literal — Unquote recovers the true key (handling an embedded or
			// escaped quote) instead of blindly trimming quote bytes.
			s := string(d.SegBytes(seg))
			if unq, err := strconv.Unquote(s); err == nil {
				return unq
			}
			return strings.Trim(s, `"`)
		}
	}
	return ""
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
		// "Copy key path" (JSON/JSONC) gives the JSONPath-style accessor of the node,
		// e.g. $.users[2].name, walking Parent links in the node arena.
		if pv.Format() == FormatJSON || pv.Format() == FormatJSONC {
			items = append(items, fyne.NewMenuItem("Copy key path", func() {
				if app := fyne.CurrentApp(); app != nil {
					app.Clipboard().SetContent(pv.keyPath(n))
				}
			}))
		}
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
