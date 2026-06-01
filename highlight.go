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
		r.applyRects(r.selLayer, &r.selRects, &r.selObjs, 0)
		return
	}
	a, b, ok := pv.ordered()
	if !ok {
		r.applyRects(r.selLayer, &r.selRects, &r.selObjs, 0)
		return
	}
	m := pv.met
	ra := int(pv.doc.RowOfLine(a.line))
	rb := int(pv.doc.RowOfLine(b.line))

	n := 0
	for row := maxInt(ra, first); row <= minInt(rb, last); row++ {
		li := pv.doc.LineAtRow(int32(row))
		depth := pv.doc.Lines[li].Depth
		runeLen := pv.doc.LineRuneLen(li)

		s, e := 0, runeLen
		if row == ra {
			s = clampInt(a.col, 0, runeLen)
		}
		if row == rb {
			e = clampInt(b.col, 0, runeLen)
		}
		x1 := m.ColX(depth, s)
		var x2 float32
		if row < rb {
			// Rows fully inside the selection bleed a little past the line end to
			// signal that the trailing newline is included (Bruno behavior).
			x2 = m.ColX(depth, runeLen) + m.CharWidth*0.6
		} else {
			x2 = m.ColX(depth, e)
		}
		if x2 <= x1 {
			if row < rb {
				x2 = x1 + m.CharWidth*0.6
			} else {
				continue
			}
		}
		rect := poolRect(&r.selRects, n)
		rect.FillColor = pv.selColor
		rect.Move(fyne.NewPos(x1, m.RowY(row)))
		rect.Resize(fyne.NewSize(x2-x1, m.RowH))
		rect.Show()
		n++
	}
	r.applyRects(r.selLayer, &r.selRects, &r.selObjs, n)
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
		total := pv.doc.TotalVisibleRows()
		for row := first; row <= last; row++ {
			if row < 0 || int32(row) >= total {
				continue
			}
			li := pv.doc.LineAtRow(int32(row))
			idxs := pv.search.byLine[li]
			if len(idxs) == 0 || pv.doc.IsCollapsed(li) {
				continue
			}
			depth := pv.doc.Lines[li].Depth
			runeLen := pv.doc.LineRuneLen(li)
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
				rect.Move(fyne.NewPos(m.ColX(depth, s), m.RowY(row)))
				rect.Resize(fyne.NewSize(m.ColX(depth, e)-m.ColX(depth, s), m.RowH))
				rect.Show()
				n++
			}
		}
	}
	r.applyRects(r.matchLayer, &r.matchRects, &r.matchObjs, n)
}

// poolRect grows the slice on demand and returns the i-th rectangle.
func poolRect(pool *[]*canvas.Rectangle, i int) *canvas.Rectangle {
	for i >= len(*pool) {
		*pool = append(*pool, canvas.NewRectangle(color.Transparent))
	}
	return (*pool)[i]
}

// applyRects hides surplus pooled rects and publishes the first n on a layer,
// reusing the layer's backing Objects slice (objs) to avoid a per-call alloc.
func (r *prettyViewRenderer) applyRects(layer *fyne.Container, pool *[]*canvas.Rectangle, objs *[]fyne.CanvasObject, n int) {
	for i := n; i < len(*pool); i++ {
		(*pool)[i].Hide()
	}
	buf := (*objs)[:0]
	for i := 0; i < n; i++ {
		buf = append(buf, (*pool)[i])
	}
	*objs = buf
	layer.Objects = buf
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
