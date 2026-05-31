package prettyview

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// Selection and search-match highlight rectangles are drawn per visible row, on
// dedicated layers beneath the text. Both intersect their span with the visible
// window first, so the rectangle count is bounded by the number of visible rows
// regardless of how large the selection or match set is (invariant M-1).

// rebuildSelection draws the selection highlight for the rows in [first, last].
func (r *prettyViewRenderer) rebuildSelection(first, last int) {
	pv := r.pv
	if pv.doc == nil {
		r.applyRects(r.selLayer, &r.selRects, 0)
		return
	}
	a, b, ok := pv.ordered()
	if !ok {
		r.applyRects(r.selLayer, &r.selRects, 0)
		return
	}
	m := pv.met
	ra := int(pv.doc.fold.rowOfLine(a.line))
	rb := int(pv.doc.fold.rowOfLine(b.line))

	n := 0
	for row := maxInt(ra, first); row <= minInt(rb, last); row++ {
		li := pv.doc.fold.lineAtRow(int32(row))
		depth := pv.doc.Lines[li].Depth
		runeLen := pv.doc.lineRuneLen(li)

		s, e := 0, runeLen
		if row == ra {
			s = clampInt(a.col, 0, runeLen)
		}
		if row == rb {
			e = clampInt(b.col, 0, runeLen)
		}
		x1 := m.colX(depth, s)
		var x2 float32
		if row < rb {
			// Rows fully inside the selection bleed a little past the line end to
			// signal that the trailing newline is included (Bruno behavior).
			x2 = m.colX(depth, runeLen) + m.charWidth*0.6
		} else {
			x2 = m.colX(depth, e)
		}
		if x2 <= x1 {
			if row < rb {
				x2 = x1 + m.charWidth*0.6
			} else {
				continue
			}
		}
		rect := poolRect(&r.selRects, n)
		rect.FillColor = pv.selColor
		rect.Move(fyne.NewPos(x1, m.rowY(row)))
		rect.Resize(fyne.NewSize(x2-x1, m.rowH))
		rect.Show()
		n++
	}
	r.applyRects(r.selLayer, &r.selRects, n)
}

// rebuildMatches draws search-match highlights for the rows in [first, last].
// Matches whose line is currently a collapsed fold-head are skipped (their text
// is not shown until expanded); matches on hidden lines are simply never in the
// visible range.
func (r *prettyViewRenderer) rebuildMatches(first, last int) {
	pv := r.pv
	n := 0
	if pv.doc != nil && len(pv.search.matches) > 0 {
		m := pv.met
		total := pv.doc.fold.TotalVisibleRows()
		for row := first; row <= last; row++ {
			if row < 0 || int32(row) >= total {
				continue
			}
			li := pv.doc.fold.lineAtRow(int32(row))
			idxs := pv.search.byLine[li]
			if len(idxs) == 0 || pv.doc.isCollapsed(li) {
				continue
			}
			depth := pv.doc.Lines[li].Depth
			runeLen := pv.doc.lineRuneLen(li)
			for _, mi := range idxs {
				mt := pv.search.matches[mi]
				s := clampInt(mt.ColStart, 0, runeLen)
				e := clampInt(mt.ColEnd, 0, runeLen)
				if e <= s {
					continue
				}
				rect := poolRect(&r.matchRects, n)
				if mi == pv.search.active {
					rect.FillColor = pv.activeMatchColor
				} else {
					rect.FillColor = pv.matchColor
				}
				rect.Move(fyne.NewPos(m.colX(depth, s), m.rowY(row)))
				rect.Resize(fyne.NewSize(m.colX(depth, e)-m.colX(depth, s), m.rowH))
				rect.Show()
				n++
			}
		}
	}
	r.applyRects(r.matchLayer, &r.matchRects, n)
}

// poolRect grows the slice on demand and returns the i-th rectangle.
func poolRect(pool *[]*canvas.Rectangle, i int) *canvas.Rectangle {
	for i >= len(*pool) {
		*pool = append(*pool, canvas.NewRectangle(color.Transparent))
	}
	return (*pool)[i]
}

// applyRects hides surplus pooled rects and publishes the first n on a layer.
func (r *prettyViewRenderer) applyRects(layer *fyne.Container, pool *[]*canvas.Rectangle, n int) {
	for i := n; i < len(*pool); i++ {
		(*pool)[i].Hide()
	}
	objs := make([]fyne.CanvasObject, n)
	for i := 0; i < n; i++ {
		objs[i] = (*pool)[i]
	}
	layer.Objects = objs
	layer.Refresh()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
