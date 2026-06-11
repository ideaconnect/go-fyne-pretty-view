package prettyview

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/geometry"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// modelPos is a position in the document: a stable display-line index and a rune
// column into that line's displayed text. Line indices never change after parse
// (only visibility does), so a modelPos survives folding; a hidden line is
// snapped to the nearest visible ancestor at use sites.
type modelPos struct {
	line int32
	col  int
}

// hitTest maps a content-space pixel to a model position (via the geometry leaf).
func (pv *PrettyView) hitTest(contentX, contentY float32) modelPos {
	line, col := geometry.HitTest(pv.doc, pv.met, contentX, contentY)
	return modelPos{line: line, col: col}
}

// selection is the model-based selection state: four integers (two positions)
// plus flags. It is independent of which rows are on screen, so it survives
// scrolling and folding. Endpoints are stored as (line, col); a hidden line is
// snapped to its nearest visible ancestor at render/copy time.
type selection struct {
	anchor, focus modelPos
	active        bool // there is a non-empty selection
	dragging      bool
	placed        bool // a caret/anchor has been established by user interaction

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
		return // the right-click context menu is shown by TappedSecondary; leave the selection intact
	}
	cx, cy := pv.contentPos(ev.Position)
	// A press on a fold triangle is a fold gesture, handled by Tapped.
	if pv.foldNodeAt(cx, cy) != model.NoNode {
		if pv.r != nil {
			pv.r.dragArmed = false
		}
		return
	}
	pos := pv.hitTest(cx, cy)
	if pos.line < 0 {
		return
	}
	if pv.r != nil {
		pv.r.dragArmed = true
	}

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
		// Extend the selection from the established caret/anchor. If no caret has
		// been placed yet (fresh widget, or just after ClearSelection), a stray
		// shift-click must not select from the document origin — place the caret
		// at the click instead. A real prior click at (0,0) sets placed, so
		// shift-selecting from the top still works.
		if pv.sel.placed {
			pv.sel.focus = pos
			pv.sel.active = true
		} else {
			pv.sel.anchor, pv.sel.focus = pos, pos
			pv.sel.active = false
		}
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
	pv.sel.placed = true // a caret/anchor now exists for subsequent shift-clicks
	pv.refreshSelectionView()
}

func (pv *PrettyView) MouseUp(*desktop.MouseEvent) {}

func (pv *PrettyView) Dragged(ev *fyne.DragEvent) {
	if pv.r == nil || !pv.r.dragArmed {
		return
	}
	cx, cy := pv.contentPos(ev.Position)
	pos := pv.hitTest(cx, cy)
	if pos.line < 0 {
		return
	}
	pv.applyHit(pos)
	pv.sel.active = true
	pv.sel.dragging = true
	pv.autoscrollEdge(ev.Position)
	pv.refreshSelectionView()
}

// applyHit moves the drag focus to pos, honoring the active grab mode: a word- or
// line-grab (double/triple-click drag) extends the selection to the whole word or
// line under pos via extendGrab, while a plain drag just moves the focus. Both the
// pointer-driven Dragged and the edge auto-scroll go through here so a drag that
// reaches the viewport edge keeps extending by word/line instead of collapsing to
// a single character. The anchor is set authoritatively in MouseDown.
func (pv *PrettyView) applyHit(pos modelPos) {
	switch pv.sel.grab {
	case grabWord:
		a, b := pv.wordBounds(pos.line, pos.col)
		pv.extendGrab(a, b)
	case grabLine:
		a, b := pv.lineBounds(pos.line)
		pv.extendGrab(a, b)
	default:
		pv.sel.focus = pos
	}
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
		if pos := pv.hitTest(cx, cy); pos.line >= 0 {
			pv.applyHit(pos) // honor word/line grab during edge auto-scroll, not just plain focus
		}
	}
}

// --- hover / cursor ---

func (pv *PrettyView) MouseIn(*desktop.MouseEvent) {}
func (pv *PrettyView) MouseOut()                   {}
func (pv *PrettyView) MouseMoved(ev *desktop.MouseEvent) {
	cx, cy := pv.contentPos(ev.Position)
	pv.overTriangle = pv.foldNodeAt(cx, cy) != model.NoNode
}

// --- focus / keyboard / shortcuts ---

func (pv *PrettyView) FocusGained() { pv.focused = true; pv.refreshSelectionView() }
func (pv *PrettyView) FocusLost() {
	pv.focused = false
	pv.shiftHeld = false // a Shift released off-widget must not stick
	if pv.r != nil {
		pv.r.dragArmed = false
	}
	if pv.cfg.editable && pv.cfg.input.AutoFormat == AutoFormatOnBlur {
		pv.reformatNow() // reformat-on-blur (#40)
	}
	pv.refreshSelectionView()
}

// TypedRune inserts a typed character at the caret when the widget is editable
// (replacing any active selection); it is a no-op for a read-only viewer, exactly
// as in v1.
func (pv *PrettyView) TypedRune(r rune) {
	if pv.cfg.editable {
		pv.editInsert([]byte(string(r)))
	}
}

// TypedKey handles Escape (clear selection) and keyboard scrolling/navigation:
// Up/Down scroll one row, PageUp/PageDown one viewport, Home/End jump to the top/
// bottom. A multi-megabyte viewer should be navigable without the mouse.
// KeyDown / KeyUp track the Shift modifier (fyne.KeyEvent carries none), so TypedKey
// can tell Shift+arrow (extend the keyboard selection) from a plain arrow (scroll).
func (pv *PrettyView) KeyDown(key *fyne.KeyEvent) {
	if key.Name == desktop.KeyShiftLeft || key.Name == desktop.KeyShiftRight {
		pv.shiftHeld = true
	}
}

func (pv *PrettyView) KeyUp(key *fyne.KeyEvent) {
	if key.Name == desktop.KeyShiftLeft || key.Name == desktop.KeyShiftRight {
		pv.shiftHeld = false
	}
}

func (pv *PrettyView) TypedKey(ev *fyne.KeyEvent) {
	if pv.r == nil {
		if ev.Name == fyne.KeyEscape {
			pv.ClearSelection()
		}
		return
	}
	vpH := pv.r.scroll.Size().Height

	// Shift+arrow / Shift+Home/End extend the keyboard selection from the caret.
	if pv.shiftHeld {
		switch ev.Name {
		case fyne.KeyDown:
			pv.keyExtend(1, keepCol, false, false)
			return
		case fyne.KeyUp:
			pv.keyExtend(-1, keepCol, false, false)
			return
		case fyne.KeyHome:
			pv.keyExtend(0, 0, true, false)
			return
		case fyne.KeyEnd:
			pv.keyExtend(0, 0, false, true)
			return
		}
	}

	// Edit mode claims the printable/navigation/deletion keys (arrows move the caret,
	// Enter inserts a newline, Backspace/Delete edit) before the read-only handlers, so
	// none of v1's Enter=fold / arrow=scroll meanings collide with typing. Keys it does
	// not claim (Escape, PageUp/Down) fall through to the read-only behavior below.
	if pv.cfg.editable && pv.editKey(ev) {
		return
	}

	switch ev.Name {
	case fyne.KeyEscape:
		pv.ClearSelection()
	case fyne.KeyReturn, fyne.KeyEnter:
		pv.keyToggleFold() // toggle the fold on the caret line, if it is a fold head
	case fyne.KeyDown:
		pv.r.scrollBy(0, pv.met.RowH)
	case fyne.KeyUp:
		pv.r.scrollBy(0, -pv.met.RowH)
	case fyne.KeyLeft:
		pv.r.scrollBy(-4*pv.met.CharWidth, 0)
	case fyne.KeyRight:
		pv.r.scrollBy(4*pv.met.CharWidth, 0)
	case fyne.KeyPageDown, fyne.KeySpace:
		pv.r.scrollBy(0, vpH)
	case fyne.KeyPageUp:
		pv.r.scrollBy(0, -vpH)
	case fyne.KeyHome:
		pv.r.scrollToOffset(fyne.NewPos(0, 0))
	case fyne.KeyEnd:
		cs := pv.contentSize()
		pv.r.scrollToOffset(fyne.NewPos(pv.r.scroll.Offset.X, max(0, cs.Height-vpH)))
	}
}

// keepCol is the sentinel for keyExtend meaning "preserve the focus column".
const keepCol = -1

// subRowOfCol returns the sub-row index whose [breaks[k],breaks[k+1]) span holds col.
// breaks is the WrapBreaks slice [0, …, lineLen]; under WrapNone it is [0, lineLen],
// so the result is always 0.
func subRowOfCol(breaks []int32, col int) int {
	for k := 0; k+1 < len(breaks); k++ {
		if int32(col) < breaks[k+1] {
			return k
		}
	}
	if len(breaks) >= 2 {
		return len(breaks) - 2
	}
	return 0
}

// keyExtend moves the keyboard caret (the selection focus) by dRows visible rows
// and/or to a column, keeping the anchor, so a Shift+arrow extends the selection.
// The first move establishes a caret at the top visible line if none exists.
func (pv *PrettyView) keyExtend(dRows, col int, toLineStart, toLineEnd bool) {
	if pv.doc == nil || pv.doc.TotalVisibleRows() == 0 {
		return
	}
	if !pv.sel.placed {
		li := pv.doc.LineAtRow(int32(max(pv.r.firstRow, 0)))
		pv.sel.anchor = modelPos{line: li, col: 0}
		pv.sel.focus = pv.sel.anchor
		pv.sel.placed = true
	}
	f := pv.sel.focus
	vl := pv.doc.VisibleLine(f.line)
	if dRows != 0 {
		// Baseline is the caret's CURRENT visual row — the line's first row plus the
		// sub-row holding f.col — so repeated moves advance under soft-wrap instead of
		// snapping back to the line's first sub-row. Under WrapNone every line is one
		// row (breaks == [0, lineLen]) and this reduces to RowOfLine(vl)+dRows.
		breaks := pv.doc.WrapBreaks(vl, nil)
		sub := subRowOfCol(breaks, f.col)
		visualCol := f.col - int(breaks[sub]) // horizontal offset within the sub-row
		row := clampInt(int(pv.doc.RowOfLine(vl))+sub+dRows, 0, int(pv.doc.TotalVisibleRows())-1)
		nl, nsub := pv.doc.LineAndSubRowAtRow(int32(row))
		f.line, vl = nl, nl
		nbreaks := pv.doc.WrapBreaks(nl, nil)
		if int(nsub) > len(nbreaks)-2 {
			nsub = int32(len(nbreaks) - 2)
		}
		// Preserve the horizontal position, clamped into the destination sub-row.
		f.col = clampInt(int(nbreaks[nsub])+visualCol, int(nbreaks[nsub]), int(nbreaks[nsub+1]))
	}
	switch {
	case toLineStart:
		f.col = 0
	case toLineEnd:
		f.col = pv.doc.LineRuneLen(vl)
	case col != keepCol:
		f.col = clampInt(col, 0, pv.doc.LineRuneLen(vl))
	}
	pv.sel.focus = f
	pv.sel.active = pv.sel.anchor != pv.sel.focus
	pv.sel.grab = grabNone
	pv.centerOnLine(vl, f.col) // keep the moving caret in view
	pv.refreshSelectionView()
}

// keyToggleFold toggles the fold on the caret's line when it is a fold head.
func (pv *PrettyView) keyToggleFold() {
	if pv.doc == nil || !pv.sel.placed {
		return
	}
	li := pv.sel.focus.line
	if int(li) < 0 || int(li) >= len(pv.doc.Lines) {
		return
	}
	if node := pv.doc.Lines[li].Fold; node != model.NoNode {
		pv.toggleFold(node)
	}
}

func (pv *PrettyView) TypedShortcut(s fyne.Shortcut) {
	switch sc := s.(type) {
	case *fyne.ShortcutCopy:
		pv.CopySelection()
	case *fyne.ShortcutSelectAll:
		pv.SelectAll()
	case *fyne.ShortcutUndo:
		pv.Undo() // no-op unless editable
	case *fyne.ShortcutRedo:
		pv.Redo() // no-op unless editable
	case *fyne.ShortcutPaste:
		pv.Paste() // no-op unless editable
	case *fyne.ShortcutCut:
		pv.Cut() // copy-only (no-op) unless editable
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
	total := pv.doc.TotalVisibleRows()
	if total == 0 {
		return
	}
	first := pv.doc.LineAtRow(0)
	last := pv.doc.LineAtRow(total - 1)
	pv.sel.anchor = modelPos{line: first, col: 0}
	pv.sel.focus = modelPos{line: last, col: pv.doc.LineRuneLen(last)}
	pv.sel.active = true
	pv.sel.grab = grabNone
	pv.sel.placed = true
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

// CopySubtree copies the displayed text of the node owning byteOffset (its whole
// {…}/[…]/<tag>…</tag> span, regardless of fold state) to the clipboard, reporting
// whether a node was found and copied. Source byte offsets are populated for every
// structured format (JSON/JSONC/XML/HTML); an out-of-range offset returns false. The
// copied text is the viewer's pretty-printed rendering of the subtree, not the
// original bytes. (The right-click "Copy subtree" menu item does the same without an
// offset, for any format.)
func (pv *PrettyView) CopySubtree(byteOffset int) bool {
	node := pv.nodeAtByteOffset(byteOffset)
	if node == model.NoNode {
		return false
	}
	if app := fyne.CurrentApp(); app != nil {
		app.Clipboard().SetContent(pv.subtreeText(node))
		return true
	}
	return false
}

// --- selection geometry / text helpers ---

// snap maps a selection endpoint to a visible position: a visible line and a
// column clamped to that line's displayed length.
func (pv *PrettyView) snap(p modelPos) modelPos {
	if p.line < 0 {
		return p
	}
	vl := pv.doc.VisibleLine(p.line)
	col := p.col
	if n := pv.doc.LineRuneLen(vl); col > n {
		col = n
	}
	if col < 0 {
		col = 0
	}
	return modelPos{line: vl, col: col}
}

// posLess reports whether p precedes q in visible (row, col) order.
func (pv *PrettyView) posLess(p, q modelPos) bool {
	pr := pv.doc.RowOfLine(pv.doc.VisibleLine(p.line))
	qr := pv.doc.RowOfLine(pv.doc.VisibleLine(q.line))
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
	// Walk the visible display lines of the span directly (a.line..b.line, skipping
	// folded-away lines) instead of resolving every row through the O(log n) Fenwick
	// projection. A whole line — every interior line, and either endpoint when its
	// column cut is the full line — is appended byte-for-byte into a reused buffer
	// with no per-line string/[]rune allocation; only a genuinely partial endpoint
	// is decoded to runes for the column slice. raw documents restore real tabs.
	restoreTabs := pv.Format() == FormatRaw
	var sb strings.Builder
	var buf []byte
	first := true
	for li := a.line; li <= b.line; li++ {
		if !pv.doc.Visible(li) {
			continue
		}
		if !first {
			sb.WriteByte('\n')
		}
		first = false
		runeLen := pv.doc.LineRuneLen(li)
		start, end := 0, runeLen
		if li == a.line {
			start = clampInt(a.col, 0, runeLen)
		}
		if li == b.line {
			end = clampInt(b.col, 0, runeLen)
		}
		if start == 0 && end == runeLen {
			buf = pv.doc.AppendDisplayLine(li, buf[:0], restoreTabs)
			sb.Write(buf)
			continue
		}
		pv.appendDisplayRange(li, start, end, restoreTabs, &sb)
	}
	return sb.String()
}

// appendDisplayRange writes line li's displayed text for the rune-column range
// [start,end) to sb. It mirrors AppendDisplayLine's tab handling (R-9) for the
// partial-endpoint case: with restoreTabs, a raw tab pad fully inside the range is
// written as a single '\t' so a copy round-trips the source tab; a pad the range
// only partly covers is written as its covered spaces. Columns are display runes,
// matching LineRuneLen and the [start,end) clamp in selectedText, so the slice can
// never desync from a tab that restores to one byte but spans several columns.
func (pv *PrettyView) appendDisplayRange(li int32, start, end int, restoreTabs bool, sb *strings.Builder) {
	if start >= end {
		return
	}
	col := 0
	for _, s := range pv.doc.DisplaySegs(li) {
		r := []rune(string(pv.doc.SegBytes(s)))
		segStart, segEnd := col, col+len(r)
		col = segEnd
		if segEnd <= start {
			continue
		}
		if segStart >= end {
			break
		}
		lo, hi := 0, len(r)
		if start > segStart {
			lo = start - segStart
		}
		if end < segEnd {
			hi = end - segStart
		}
		if restoreTabs && s.Buf == model.BufAux && s.Role == model.RolePlain && lo == 0 && hi == len(r) {
			sb.WriteByte('\t') // whole tab pad selected -> round-trip the source tab
			continue
		}
		sb.WriteString(string(r[lo:hi]))
	}
}

// subtreeText reconstructs the displayed text of a node's whole subtree (all
// lines HeadLine..CloseLine), indented by depth, regardless of fold state.
func (pv *PrettyView) subtreeText(node model.NodeID) string {
	n := &pv.doc.Nodes[node]
	if n.HeadLine < 0 {
		return ""
	}
	var sb strings.Builder
	for li := n.HeadLine; li <= n.CloseLine; li++ {
		l := &pv.doc.Lines[li]
		if d := int(l.Depth) - int(n.Depth); d > 0 { // guard: a shallower descendant would panic strings.Repeat
			sb.WriteString(strings.Repeat("  ", d))
		}
		sb.WriteString(pv.doc.LineString(li))
		if li < n.CloseLine {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// nodeAtByteOffset returns the deepest node whose source span contains offset
// (JSON only; XML/HTML lack offsets and return model.NoNode).
//
// Nodes are emitted in depth-first preorder, so SrcStart is non-decreasing in id
// order and a subtree occupies a contiguous id range. We binary-search the last
// node starting at or before off, then walk up ancestors to the first that
// actually spans off — O(log n + depth) instead of an O(n) scan of every node.
func (pv *PrettyView) nodeAtByteOffset(offset int) model.NodeID {
	nodes := pv.doc.Nodes
	off := uint32(offset)
	lo, hi := 0, len(nodes)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if nodes[mid].SrcStart <= off {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	// nodes[lo-1] has the greatest SrcStart <= off; the deepest container is it or
	// an ancestor (nodes whose subtree ended before off are skipped by the span test).
	for id := lo - 1; id >= 0; id = int(nodes[id].Parent) {
		n := &nodes[id]
		if n.SrcEnd > n.SrcStart && off >= n.SrcStart && off < n.SrcEnd {
			return model.NodeID(id)
		}
	}
	return model.NoNode
}

func (pv *PrettyView) refreshSelectionView() {
	if pv.r == nil {
		return
	}
	pv.r.rebuildSelection(pv.r.firstRow, pv.r.lastRow)
	pv.r.rebuildCaret() // focus/caret moves repaint without a full reflow
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
