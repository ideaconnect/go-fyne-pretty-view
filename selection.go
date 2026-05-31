package prettyview

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// selection is the model-based selection state: four integers (two positions)
// plus flags. It is independent of which rows are on screen, so it survives
// scrolling and folding. Endpoints are stored as (line, col); a hidden line is
// snapped to its nearest visible ancestor at render/copy time.
type selection struct {
	anchor, focus modelPos
	active        bool // there is a non-empty selection
	dragging      bool

	grab         grabMode
	grabA, grabB modelPos // the word/line originally grabbed (for double/triple-drag)
}

type grabMode uint8

const (
	grabNone grabMode = iota
	grabWord
	grabLine
)

const multiClickWindow = 300 * time.Millisecond

func (pv *PrettyView) requestFocus() {
	app := fyne.CurrentApp()
	if app == nil || app.Driver() == nil {
		return
	}
	if c := app.Driver().CanvasForObject(pv); c != nil {
		c.Focus(pv)
	}
}

// --- mouse / drag ---

func (pv *PrettyView) MouseDown(ev *desktop.MouseEvent) {
	pv.requestFocus()
	if ev.Button == desktop.MouseButtonSecondary {
		return // reserved for a future context menu; leave selection intact
	}
	cx, cy := pv.contentPos(ev.Position)
	// A press on a fold triangle is a fold gesture, handled by Tapped.
	if pv.foldNodeAt(cx, cy) != NoNode {
		pv.r.dragArmed = false
		return
	}
	pos := pv.doc.hitTest(pv.met, cx, cy)
	if pos.line < 0 {
		return
	}
	pv.r.dragArmed = true

	now := time.Now()
	if now.Sub(pv.lastClickAt) < multiClickWindow && near(pv.lastClickPos, ev.Position) {
		pv.clickCount++
	} else {
		pv.clickCount = 1
	}
	pv.lastClickAt = now
	pv.lastClickPos = ev.Position

	shift := ev.Modifier&fyne.KeyModifierShift != 0
	switch {
	case shift:
		pv.sel.focus = pos
		pv.sel.active = true
		pv.sel.grab = grabNone
	case pv.clickCount >= 3:
		a, b := pv.lineBounds(pos.line)
		pv.sel.anchor, pv.sel.focus = a, b
		pv.sel.grab, pv.sel.grabA, pv.sel.grabB = grabLine, a, b
		pv.sel.active = true
	case pv.clickCount == 2:
		a, b := pv.wordBounds(pos.line, pos.col)
		pv.sel.anchor, pv.sel.focus = a, b
		pv.sel.grab, pv.sel.grabA, pv.sel.grabB = grabWord, a, b
		pv.sel.active = true
	default:
		pv.sel.anchor, pv.sel.focus = pos, pos
		pv.sel.grab = grabNone
		pv.sel.active = false
	}
	pv.refreshSelectionView()
}

func (pv *PrettyView) MouseUp(*desktop.MouseEvent) {}

func (pv *PrettyView) Dragged(ev *fyne.DragEvent) {
	if pv.r == nil || !pv.r.dragArmed {
		return
	}
	cx, cy := pv.contentPos(ev.Position)
	pos := pv.doc.hitTest(pv.met, cx, cy)
	if pos.line < 0 {
		return
	}
	switch pv.sel.grab {
	case grabWord:
		a, b := pv.wordBounds(pos.line, pos.col)
		pv.extendGrab(a, b)
	case grabLine:
		a, b := pv.lineBounds(pos.line)
		pv.extendGrab(a, b)
	default:
		// Anchor is set authoritatively in MouseDown and never recomputed here.
		pv.sel.focus = pos
	}
	pv.sel.active = true
	pv.sel.dragging = true
	pv.autoscrollEdge(ev.Position)
	pv.refreshSelectionView()
}

func (pv *PrettyView) DragEnd() {
	if pv.r != nil {
		pv.r.dragArmed = false
	}
	pv.sel.dragging = false
	if pv.sel.anchor == pv.sel.focus {
		pv.sel.active = false
	}
	pv.refreshSelectionView()
}

// extendGrab sets anchor/focus so the selection spans the originally grabbed
// word/line and the newly grabbed one (for double/triple-click drag).
func (pv *PrettyView) extendGrab(a, b modelPos) {
	if pv.posLess(b, pv.sel.grabA) {
		pv.sel.anchor, pv.sel.focus = pv.sel.grabB, a
	} else {
		pv.sel.anchor, pv.sel.focus = pv.sel.grabA, b
	}
}

// autoscrollEdge nudges the viewport when the pointer is dragged near an edge.
func (pv *PrettyView) autoscrollEdge(local fyne.Position) {
	if pv.r == nil {
		return
	}
	const edge, step = 24, 40
	h := pv.r.scroll.Size().Height
	w := pv.r.scroll.Size().Width
	var dx, dy float32
	switch {
	case local.Y < edge:
		dy = -step
	case local.Y > h-edge:
		dy = step
	}
	switch {
	case local.X < edge:
		dx = -step
	case local.X > w-edge:
		dx = step
	}
	if dx != 0 || dy != 0 {
		pv.r.scrollBy(dx, dy)
		cx, cy := pv.contentPos(local)
		if pos := pv.doc.hitTest(pv.met, cx, cy); pos.line >= 0 {
			pv.sel.focus = pos
		}
	}
}

// --- hover / cursor ---

func (pv *PrettyView) MouseIn(*desktop.MouseEvent) {}
func (pv *PrettyView) MouseOut()                   {}
func (pv *PrettyView) MouseMoved(ev *desktop.MouseEvent) {
	cx, cy := pv.contentPos(ev.Position)
	pv.overTriangle = pv.foldNodeAt(cx, cy) != NoNode
}

// --- focus / keyboard / shortcuts ---

func (pv *PrettyView) FocusGained() { pv.focused = true; pv.refreshSelectionView() }
func (pv *PrettyView) FocusLost() {
	pv.focused = false
	if pv.r != nil {
		pv.r.dragArmed = false
	}
	pv.refreshSelectionView()
}
func (pv *PrettyView) TypedRune(rune) {}
func (pv *PrettyView) TypedKey(ev *fyne.KeyEvent) {
	if ev.Name == fyne.KeyEscape {
		pv.ClearSelection()
	}
}

func (pv *PrettyView) TypedShortcut(s fyne.Shortcut) {
	switch sc := s.(type) {
	case *fyne.ShortcutCopy:
		pv.CopySelection()
	case *fyne.ShortcutSelectAll:
		pv.SelectAll()
	case *desktop.CustomShortcut:
		if sc.KeyName == fyne.KeyF && sc.Modifier == fyne.KeyModifierShortcutDefault && pv.onSearchRequested != nil {
			pv.onSearchRequested()
		}
	}
}

// --- public selection / clipboard API ---

// SelectedText returns the exact text currently selected, or "".
func (pv *PrettyView) SelectedText() string { return pv.selectedText() }

// SelectAll selects the entire (currently visible) document.
func (pv *PrettyView) SelectAll() {
	if pv.doc == nil {
		return
	}
	total := pv.doc.fold.TotalVisibleRows()
	if total == 0 {
		return
	}
	first := pv.doc.fold.lineAtRow(0)
	last := pv.doc.fold.lineAtRow(total - 1)
	pv.sel.anchor = modelPos{line: first, col: 0}
	pv.sel.focus = modelPos{line: last, col: pv.doc.lineRuneLen(last)}
	pv.sel.active = true
	pv.sel.grab = grabNone
	pv.refreshSelectionView()
}

// ClearSelection drops any selection.
func (pv *PrettyView) ClearSelection() {
	pv.sel = selection{}
	pv.refreshSelectionView()
}

// CopySelection copies SelectedText to the clipboard (no-op if empty).
func (pv *PrettyView) CopySelection() {
	txt := pv.selectedText()
	if txt == "" {
		return
	}
	if app := fyne.CurrentApp(); app != nil {
		app.Clipboard().SetContent(txt)
	}
}

// CopySubtree copies the serialized text of the node owning byteOffset (the
// whole {…}/[…]/<tag>…</tag> span), regardless of fold state.
func (pv *PrettyView) CopySubtree(byteOffset int) {
	node := pv.nodeAtByteOffset(byteOffset)
	if node == NoNode {
		return
	}
	if app := fyne.CurrentApp(); app != nil {
		app.Clipboard().SetContent(pv.subtreeText(node))
	}
}

// --- selection geometry / text helpers ---

// visibleLine returns line itself if visible, else the head line of its nearest
// collapsed ancestor (the row actually shown for it).
func (d *Document) visibleLine(line int32) int32 {
	if line < 0 {
		return line
	}
	if d.fold.vis[line] == 1 {
		return line
	}
	if hb := d.fold.hiddenBy[line]; hb != NoNode {
		return d.Nodes[hb].HeadLine
	}
	return line
}

// snap maps a selection endpoint to a visible position: a visible line and a
// column clamped to that line's displayed length.
func (pv *PrettyView) snap(p modelPos) modelPos {
	if p.line < 0 {
		return p
	}
	vl := pv.doc.visibleLine(p.line)
	col := p.col
	if n := pv.doc.lineRuneLen(vl); col > n {
		col = n
	}
	if col < 0 {
		col = 0
	}
	return modelPos{line: vl, col: col}
}

// posLess reports whether p precedes q in visible (row, col) order.
func (pv *PrettyView) posLess(p, q modelPos) bool {
	pr := pv.doc.fold.rowOfLine(pv.doc.visibleLine(p.line))
	qr := pv.doc.fold.rowOfLine(pv.doc.visibleLine(q.line))
	if pr != qr {
		return pr < qr
	}
	return p.col < q.col
}

// ordered returns the selection endpoints in document order plus whether the
// selection is non-empty.
func (pv *PrettyView) ordered() (modelPos, modelPos, bool) {
	if !pv.sel.active {
		return modelPos{}, modelPos{}, false
	}
	a := pv.snap(pv.sel.anchor)
	b := pv.snap(pv.sel.focus)
	if pv.posLess(b, a) {
		a, b = b, a
	}
	if a.line == b.line && a.col == b.col {
		return a, b, false
	}
	return a, b, true
}

func (pv *PrettyView) selectedText() string {
	a, b, ok := pv.ordered()
	if !ok {
		return ""
	}
	ra := int(pv.doc.fold.rowOfLine(a.line))
	rb := int(pv.doc.fold.rowOfLine(b.line))
	if ra == rb {
		runes := []rune(pv.doc.displayString(a.line))
		return string(runes[clampInt(a.col, 0, len(runes)):clampInt(b.col, 0, len(runes))])
	}
	var sb strings.Builder
	for row := ra; row <= rb; row++ {
		li := pv.doc.fold.lineAtRow(int32(row))
		runes := []rune(pv.doc.displayString(li))
		start, end := 0, len(runes)
		if row == ra {
			start = clampInt(a.col, 0, len(runes))
		}
		if row == rb {
			end = clampInt(b.col, 0, len(runes))
		}
		sb.WriteString(string(runes[start:end]))
		if row < rb {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// subtreeText reconstructs the displayed text of a node's whole subtree (all
// lines HeadLine..CloseLine), indented by depth, regardless of fold state.
func (pv *PrettyView) subtreeText(node NodeID) string {
	n := &pv.doc.Nodes[node]
	if n.HeadLine < 0 {
		return ""
	}
	var sb strings.Builder
	for li := n.HeadLine; li <= n.CloseLine; li++ {
		l := &pv.doc.Lines[li]
		sb.WriteString(strings.Repeat("  ", int(l.Depth)-int(n.Depth)))
		sb.WriteString(pv.doc.lineString(li))
		if li < n.CloseLine {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// nodeAtByteOffset returns the deepest node whose source span contains offset
// (JSON only; XML/HTML lack offsets and return NoNode).
func (pv *PrettyView) nodeAtByteOffset(offset int) NodeID {
	best := NoNode
	off := uint32(offset)
	for id := range pv.doc.Nodes {
		n := &pv.doc.Nodes[id]
		if n.SrcEnd == 0 && n.SrcStart == 0 {
			continue
		}
		if off >= n.SrcStart && off < n.SrcEnd {
			if best == NoNode || n.Depth > pv.doc.Nodes[best].Depth {
				best = NodeID(id)
			}
		}
	}
	return best
}

func (pv *PrettyView) refreshSelectionView() {
	if pv.r == nil {
		return
	}
	pv.r.rebuildSelection(pv.r.firstRow, pv.r.lastRow)
}

func near(a, b fyne.Position) bool {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx < 4 && dx > -4 && dy < 4 && dy > -4
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampf(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
