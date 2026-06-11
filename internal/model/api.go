package model

import "unicode/utf8"

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

// FirstVisualRowOfLine is RowOfLine under its wrap-aware name: the top visual row
// of a (possibly multi-row) display line.
func (d *Document) FirstVisualRowOfLine(line int32) int32 { return d.fold.rowOfLine(line) }

// LineAndSubRowAtRow maps a visible visual row to its display line and the 0-based
// sub-row within that line (0 unless the line is soft-wrapped). The renderer uses
// the sub-row to pick which column slice of the line a given screen row shows.
func (d *Document) LineAndSubRowAtRow(row int32) (line, sub int32) {
	return d.fold.lineAndSubRow(row)
}

// LineAtSourceOffset maps a byte offset into Src back to the display line whose source
// bytes cover it: the last line whose first source-backed segment starts at or before
// off (display lines are emitted in source order, so the first line that starts past off
// ends the search). It is a coarse, line-granular map — enough to re-place an edit caret
// on the right line after a raw->structured projection swap. Returns 0 for an empty
// document. A rune-precise column map is the caret semantic anchor (#41).
func (d *Document) LineAtSourceOffset(off int) int32 {
	best := int32(0)
	for li := 0; li < len(d.Lines); li++ {
		start, ok := d.lineFirstSrcStart(int32(li))
		if !ok {
			continue
		}
		if start > off {
			break
		}
		best = int32(li)
	}
	return best
}

// lineFirstSrcStart returns the Src start offset of a line's first source-backed
// (BufSrc) segment, or false for a synthesized-only line.
func (d *Document) lineFirstSrcStart(li int32) (int, bool) {
	for _, s := range d.LineSegs(li) {
		if s.Buf == BufSrc {
			return int(s.Start), true
		}
	}
	return 0, false
}

// LineColAtSourceOffset maps a Src byte offset to a rune-precise display (line, col):
// it finds the line covering off (LineAtSourceOffset) and then walks that line's
// segments, returning the displayed-rune column where off falls. An offset inside a
// source-backed segment lands exactly on its rune; one in inter-token whitespace (not
// displayed) clamps to the nearest segment boundary. This is the caret anchor across a
// non-destructive reformat: the buffer bytes don't move, so a stable byte offset maps
// straight to the new structured position (issue #41). Returns (0,0) for an empty doc.
func (d *Document) LineColAtSourceOffset(off int) (line int32, col int) {
	if len(d.Lines) == 0 {
		return 0, 0
	}
	line = d.LineAtSourceOffset(off)
	runesBefore := 0
	for _, s := range d.LineSegs(line) {
		if s.Buf == BufSrc {
			if off <= int(s.Start) {
				return line, runesBefore // before this token (inter-token gap) -> its start
			}
			if off < int(s.End) {
				return line, runesBefore + utf8.RuneCount(d.Src[s.Start:off])
			}
		}
		runesBefore += utf8.RuneCount(d.SegBytes(s))
	}
	return line, runesBefore // past the last source token on the line -> end of its text
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
