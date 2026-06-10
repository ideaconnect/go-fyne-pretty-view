package model

import "unicode/utf8"

// Soft-wrap projection. The viewer wraps long display lines to the viewport width
// instead of scrolling them horizontally. Wrapping is purely presentational: the
// model still has one logical Line per row of structure; a wrapped line simply
// occupies several VISUAL rows. We absorb that into the existing Fenwick fold
// index by letting a visible line carry a weight > 1 (its visual-row count,
// rowsOf[line]) instead of a flat 1. See foldindex.go's weightOf.
//
// The column budget per depth is supplied by the view (the model cannot import
// geometry), recomputed only when the integer column count changes, so a resize
// reprojects at most once per crossed column boundary — not once per pixel. Each
// reprojection is one O(total displayed bytes) pass on the Fyne goroutine (single-
// digit milliseconds on a multi-MB document, hardware-dependent), so dragging a
// resize across many column boundaries can briefly cost one such pass per boundary.

// SetWrapColumns enables soft-wrap with the given per-depth text-column budget
// (colsByDepth[d] = columns available to a line at depth d, as the view derives it
// from the metrics + viewport width). It recomputes every line's visual-row count
// and reprojects in one O(total displayed bytes) pass. A nil/empty table disables
// wrap (WrapNone): rowsOf is dropped and every visible line reverts to one row, the
// zero-cost fast path. Must be called on the Fyne goroutine like the other mutators.
func (d *Document) SetWrapColumns(colsByDepth []int) {
	if len(colsByDepth) == 0 {
		d.colsByDepth = nil
		d.rowsOf = nil
		d.fold.rebuild(d) // weightOf now returns 1 everywhere → all-visible weights restored
		return
	}
	d.colsByDepth = append(d.colsByDepth[:0], colsByDepth...) // copy; caller may reuse its slice
	if cap(d.rowsOf) < len(d.Lines) {
		d.rowsOf = make([]int32, len(d.Lines))
	} else {
		d.rowsOf = d.rowsOf[:len(d.Lines)]
	}
	d.fold.rebuild(d) // rowsOf is now non-nil, so rebuild refreshes it and projects
}

// computeWrapRows fills rowsOf[line] with each line's visual-row count for its
// currently-displayed (fold-aware) rendering at the active per-depth budget.
func (d *Document) computeWrapRows() {
	if cap(d.rowsOf) >= len(d.Lines) {
		d.rowsOf = d.rowsOf[:len(d.Lines)]
	} else {
		d.rowsOf = make([]int32, len(d.Lines))
	}
	for li := range d.Lines {
		cols := wrapColsAt(d.colsByDepth, d.Lines[li].Depth)
		d.rowsOf[li] = d.wrapWalk(int32(li), cols, nil)
	}
}

// reweightLine recomputes one line's visual-row count in place (used when a fold
// toggle changes a head line's displayed rendering). No-op under WrapNone.
func (d *Document) reweightLine(li int32) {
	if d.rowsOf == nil {
		return
	}
	cols := wrapColsAt(d.colsByDepth, d.Lines[li].Depth)
	d.rowsOf[li] = d.wrapWalk(li, cols, nil)
}

// wrapColsAt returns the column budget for depth from the per-depth table: deeper
// lines past the table reuse the deepest entry, and the value is floored at 1 so
// the walker always advances even at a pathological indentation.
func wrapColsAt(colsByDepth []int, depth uint8) int {
	if len(colsByDepth) == 0 {
		return 1 << 30 // no wrap configured: effectively unbounded
	}
	i := int(depth)
	if i >= len(colsByDepth) {
		i = len(colsByDepth) - 1
	}
	if c := colsByDepth[i]; c >= 1 {
		return c
	}
	return 1
}

// wrapWalk walks line li's currently-displayed runes and reports its soft-wrap
// structure for a budget of cols columns per visual row. For every soft-break it
// calls emit with the column (rune index) at which a new visual row begins, and it
// returns the number of visual rows (>= 1). A nil emit just counts. Word
// boundaries (the column right after a space) are preferred; an unbreakable run
// wider than cols falls back to a char-break so a visual row is never wider than
// cols — this is what preserves the no-texture-wider-than-the-viewport invariant.
func (d *Document) wrapWalk(li int32, cols int, emit func(col int32)) int32 {
	rows := int32(1)
	col := 0      // current rune column
	rowStart := 0 // column where the current visual row begins
	lastOpp := -1 // latest break opportunity (column just after a space) in this row
	flush := func(at int) {
		brk := at
		if lastOpp > rowStart {
			brk = lastOpp // break after the last space rather than mid-word
		}
		if emit != nil {
			emit(int32(brk))
		}
		rows++
		rowStart = brk
		lastOpp = -1
	}
	for _, s := range d.DisplaySegs(li) {
		b := d.SegBytes(s)
		i := 0
		for i < len(b) {
			for col-rowStart >= cols { // row full: break before placing this rune
				flush(col)
			}
			var r rune
			if b[i] < utf8.RuneSelf {
				r = rune(b[i])
				i++
			} else {
				rr, sz := utf8.DecodeRune(b[i:])
				r, i = rr, i+sz
			}
			if r == ' ' {
				lastOpp = col + 1
			}
			col++
		}
	}
	return rows
}

// WrapBreaks appends line li's visual-row boundaries to dst (reused; pass dst[:0])
// and returns it: a sorted list [0, b1, …, lineLen] whose consecutive pairs are
// each visual row's [startCol, endCol) in displayed rune columns. With wrap off it
// is simply [0, lineLen] (one row). The budget is the retained per-depth table.
func (d *Document) WrapBreaks(li int32, dst []int32) []int32 {
	dst = append(dst, 0)
	n := int32(d.LineRuneLen(li))
	if d.colsByDepth == nil {
		return append(dst, n)
	}
	cols := wrapColsAt(d.colsByDepth, d.Lines[li].Depth)
	d.wrapWalk(li, cols, func(c int32) { dst = append(dst, c) })
	return append(dst, n)
}

// RowsOfLine reports how many visual rows line li currently occupies (1 under
// WrapNone, or for any line whose displayed text fits one row).
func (d *Document) RowsOfLine(li int32) int32 {
	if d.rowsOf == nil || li < 0 || int(li) >= len(d.rowsOf) {
		return 1
	}
	return d.rowsOf[li]
}

// WrapActive reports whether soft-wrap is currently on.
func (d *Document) WrapActive() bool { return d.rowsOf != nil }
