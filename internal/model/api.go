package model

// This file is the public projection/fold surface of Document. The fold index
// itself stays unexported; the view drives folding through these delegators and
// never touches projection internals.

// TotalVisibleRows reports how many display rows are currently visible.
func (d *Document) TotalVisibleRows() int32 { return d.fold.TotalVisibleRows() }

// LineAtRow maps a 0-based visible row to its display-line index.
func (d *Document) LineAtRow(row int32) int32 { return d.fold.lineAtRow(row) }

// RowOfLine maps a display line to the visible row it occupies.
func (d *Document) RowOfLine(line int32) int32 { return d.fold.rowOfLine(line) }

// Fold collapses node; Unfold expands it; Toggle flips it.
func (d *Document) Fold(node NodeID)   { d.fold.fold(d, node) }
func (d *Document) Unfold(node NodeID) { d.fold.unfold(d, node) }
func (d *Document) Toggle(node NodeID) { d.fold.toggle(d, node) }

// RevealLine expands every collapsed ancestor hiding line. Returns true if
// anything changed.
func (d *Document) RevealLine(line int32) bool { return d.fold.revealLine(d, line) }

// ExpandAncestors expands every collapsed ancestor of node.
func (d *Document) ExpandAncestors(node NodeID) bool { return d.fold.expandAncestors(d, node) }

// ExpandAll expands every node; CollapseAll collapses below the top level.
func (d *Document) ExpandAll()   { d.fold.expandAll(d) }
func (d *Document) CollapseAll() { d.fold.collapseAll(d) }

// Collapsed reports whether node is collapsed.
func (d *Document) Collapsed(node NodeID) bool { return d.fold.collapsed.get(node) }

// Visible reports whether a display line is currently visible. An out-of-range
// line reports false (matching the guards on its sibling accessors).
func (d *Document) Visible(line int32) bool {
	if line < 0 || int(line) >= len(d.fold.vis) {
		return false
	}
	return d.fold.vis[line] == 1
}

// Rebuild recomputes the projection from the collapsed bitset (used by tests and
// after bulk fold changes).
func (d *Document) Rebuild() { d.fold.rebuild(d) }

// ProjectionBytes estimates the heap footprint of the fold/visibility index
// (used by memory accounting/tests).
func (d *Document) ProjectionBytes() int {
	return len(d.fold.vis)*4 + len(d.fold.hiddenBy)*4 + len(d.fold.bit.tree)*4 + len(d.fold.collapsed.words)*8
}

// VisibleLine returns line itself if visible, else the head line of its nearest
// collapsed ancestor (the row actually shown for it).
func (d *Document) VisibleLine(line int32) int32 {
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

// AssembleLine appends a line's expanded display bytes into buf (reused; pass
// buf[:0]).
func (d *Document) AssembleLine(li int32, buf []byte) []byte {
	for _, s := range d.LineSegs(li) {
		buf = append(buf, d.SegBytes(s)...)
	}
	return buf
}
