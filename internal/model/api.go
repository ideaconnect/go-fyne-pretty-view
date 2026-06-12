package model

// This file is the public projection/fold surface of Document. The fold index
// itself stays unexported; the view drives folding through these delegators and
// never touches projection internals.

// TotalVisibleRows reports how many display rows are currently visible.
func (d *Document) TotalVisibleRows() int32 { return d.fold.TotalVisibleRows() }

// LineAtRow maps a 0-based visible row to its display-line index.
func (d *Document) LineAtRow(row int32) int32 { return d.fold.lineAtRow(row) }

// RowOfLine maps a display line to the visible row it occupies. Under soft-wrap a
// line spans several visual rows; this returns the FIRST (top) one.
func (d *Document) RowOfLine(line int32) int32 { return d.fold.rowOfLine(line) }

// LineAndSubRowAtRow maps a visible visual row to its display line and the 0-based
// sub-row within that line (0 unless the line is soft-wrapped). The renderer uses
// the sub-row to pick which column slice of the line a given screen row shows.
func (d *Document) LineAndSubRowAtRow(row int32) (line, sub int32) {
	return d.fold.lineAndSubRow(row)
}

// Fold collapses node; Unfold expands it; Toggle flips it.
func (d *Document) Fold(node NodeID)   { d.fold.fold(d, node) }
func (d *Document) Unfold(node NodeID) { d.fold.unfold(d, node) }
func (d *Document) Toggle(node NodeID) { d.fold.toggle(d, node) }

// RevealLine expands every collapsed ancestor hiding line. Returns true if
// anything changed.
func (d *Document) RevealLine(line int32) bool { return d.fold.revealLine(d, line) }

// ExpandAll expands every node; CollapseAll collapses below the top level.
func (d *Document) ExpandAll()   { d.fold.expandAll(d) }
func (d *Document) CollapseAll() { d.fold.collapseAll(d) }

// CollapseToDepth collapses every foldable node at nesting depth >= depth (leaving
// shallower nodes' state untouched); ExpandToDepth expands every foldable node at
// depth < depth. Top level is depth 0; the two compose.
func (d *Document) CollapseToDepth(depth int) { d.fold.setDepth(d, depth, true) }
func (d *Document) ExpandToDepth(depth int)   { d.fold.setDepth(d, depth, false) }

// Collapsed reports whether node is collapsed.
func (d *Document) Collapsed(node NodeID) bool { return d.fold.collapsed.get(node) }

// Visible reports whether a display line is currently visible. An out-of-range
// line reports false (matching the guards on its sibling accessors).
func (d *Document) Visible(line int32) bool {
	if line < 0 || int(line) >= len(d.fold.vis) {
		return false
	}
	return d.fold.vis[line] != 0
}

// Rebuild recomputes the projection from the collapsed bitset (used by tests and
// after bulk fold changes).
func (d *Document) Rebuild() { d.fold.rebuild(d) }

// ProjectionBytes estimates the heap footprint of the fold/visibility index
// (used by memory accounting/tests).
func (d *Document) ProjectionBytes() int {
	return len(d.fold.vis)*4 + len(d.fold.hiddenBy)*4 + len(d.fold.bit.tree)*4 +
		len(d.fold.collapsed.words)*8 + len(d.rowsOf)*4
}

// VisibleLine returns line itself if visible, else the head line of its nearest
// collapsed ancestor (the row actually shown for it).
func (d *Document) VisibleLine(line int32) int32 {
	if line < 0 || int(line) >= len(d.fold.vis) {
		return line
	}
	if d.fold.vis[line] != 0 {
		return line
	}
	if hb := d.fold.hiddenBy[line]; hb != NoNode {
		return d.Nodes[hb].HeadLine
	}
	return line
}

// AssembleLine appends a line's expanded display bytes into buf (reused; pass
// buf[:0]).
func (d *Document) AssembleLine(li int32, buf []byte) []byte {
	for _, s := range d.LineSegs(li) {
		buf = append(buf, d.SegBytes(s)...)
	}
	return buf
}

// AppendDisplayLine appends line li's currently-displayed, fold-aware bytes into
// buf (reused; pass buf[:0]) — the no-allocation equivalent of DisplayString, used
// by the copy path so a whole selected line costs no per-line string/[]rune
// allocation. When restoreTabs is true (raw documents, whose tab stops render as
// interned space pads), each pad segment is written as a single '\t' so a copy
// round-trips the original source tabs instead of the expanded spaces. A pad is the
// only RolePlain segment that lives in Aux on a raw line (real text is zero-copy
// from Src), so that pair identifies it unambiguously.
func (d *Document) AppendDisplayLine(li int32, buf []byte, restoreTabs bool) []byte {
	for _, s := range d.DisplaySegs(li) {
		if restoreTabs && s.Buf == BufAux && s.Role == RolePlain {
			buf = append(buf, '\t')
			continue
		}
		buf = append(buf, d.SegBytes(s)...)
	}
	return buf
}

// AppendPrettyLine appends line li's pretty serialization to buf and returns the grown
// slice: `indent` leading spaces, then the line's expanded (fold-independent) segment
// bytes in order. It writes no trailing newline — callers join lines and choose the indent
// (absolute Depth for the viewer text and Reformat, or Depth-relative for a copied subtree).
//
// For each source-backed (BufSrc) segment it invokes spanCb (when non-nil) with the
// segment's source [start,end) byte range and the segment's start offset within buf, so a
// caller rewriting the buffer (Reformat) can remap caret byte offsets. Synthesized (BufAux)
// segments carry no source range and trigger no callback.
//
// This is the single routine behind PrettyView.Text, the copy-subtree text, and the
// Reformat byte-serializer: the "indent two spaces per depth, then the expanded line text"
// convention lives here so those three can never drift.
func (d *Document) AppendPrettyLine(li int32, indent int, buf []byte, spanCb func(srcStart, srcEnd uint32, outStart int)) []byte {
	for k := 0; k < indent; k++ {
		buf = append(buf, ' ')
	}
	for _, s := range d.LineSegs(li) {
		if spanCb != nil && s.Buf == BufSrc {
			spanCb(s.Start, s.End, len(buf))
		}
		buf = append(buf, d.SegBytes(s)...)
	}
	return buf
}
