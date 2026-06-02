package prettyview

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/geometry"
)

// Selection and search-match highlight rectangles are drawn per visible row, on
// dedicated layers beneath the text. Both intersect their span with the visible
// window first, so the rectangle count is bounded by the number of visible rows
// regardless of how large the selection or match set is (invariant M-1). Under
// soft-wrap a logical line spans several visual rows; each rect is clipped to its
// visual row's column slice, so the bound still holds.

// placeSpanRect positions pooled rect n to cover displayed columns [lo,hi) of the
// visual row `row`, drawing text-relative to colBase (the row's first column under
// soft-wrap, else 0 for the absolute, horizontally-scrolled layout). extra adds
// trailing width (the selection's trailing-newline bleed). Returns the next free
// index — unchanged if the span collapses to nothing.
func (r *prettyViewRenderer) placeSpanRect(pool *[]*canvas.Rectangle, n int, m geometry.Metrics,
	depth uint8, lo, hi, colBase, row int, fill color.Color, extra float32) int {
	x1 := m.ColX(depth, lo-colBase)
	x2 := m.ColX(depth, hi-colBase) + extra
	if x2 <= x1 {
		return n
	}
	rect := poolRect(pool, n)
	rect.FillColor = fill
	rect.Move(fyne.NewPos(x1, m.RowY(row)))
	rect.Resize(fyne.NewSize(x2-x1, m.RowH))
	rect.Show()
	return n + 1
}

// subSpan returns the displayed-column window [w0,w1) and text base for visual row
// `row` showing (li, sub). Under WrapNone it is the whole line [0,runeLen) at base
// 0 (the absolute layout). The breaks slice is cached by line across calls.
func (r *prettyViewRenderer) subSpan(li, sub int32, runeLen int, wrapOn bool,
	breaks *[]int32, breaksLine *int32) (w0, w1, colBase int) {
	if !wrapOn {
		return 0, runeLen, 0
	}
	if li != *breaksLine {
		*breaks = r.pv.doc.WrapBreaks(li, (*breaks)[:0])
		*breaksLine = li
	}
	s := int(sub)
	if s > len(*breaks)-2 {
		s = len(*breaks) - 2
	}
	return int((*breaks)[s]), int((*breaks)[s+1]), int((*breaks)[s])
}

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
	wrapOn := pv.doc.WrapActive()

	n := 0
	breaks := []int32(nil)
	breaksLine := int32(-1)
	for row := first; row <= last; row++ {
		if row < 0 {
			continue
		}
		li := int32(row)
		var sub int32
		if wrapOn {
			li, sub = pv.doc.LineAndSubRowAtRow(int32(row))
		} else {
			li = pv.doc.LineAtRow(int32(row))
		}
		if li < a.line || li > b.line {
			continue // line outside the selection span
		}
		depth := pv.doc.Lines[li].Depth
		runeLen := pv.doc.LineRuneLen(li)

		// Selection's column range on this logical line.
		selS, selE := 0, runeLen
		if li == a.line {
			selS = clampInt(a.col, 0, runeLen)
		}
		if li == b.line {
			selE = clampInt(b.col, 0, runeLen)
		}

		w0, w1, colBase := r.subSpan(li, sub, runeLen, wrapOn, &breaks, &breaksLine)
		lo, hi := max(selS, w0), min(selE, w1)
		// Trailing-newline bleed: shown when the line is selected through its end,
		// another selected line follows, and this is the line's last visual row.
		bleed := li < b.line && selE >= runeLen && w1 >= runeLen
		if lo >= hi && !bleed {
			continue
		}
		extra := float32(0)
		if bleed {
			extra = m.CharWidth * 0.6
		}
		n = r.placeSpanRect(&r.selRects, n, m, depth, lo, max(hi, lo), colBase, row, pv.selColor, extra)
	}
	r.applyRects(r.selLayer, &r.selRects, &r.selObjs, n)
}

// rebuildMatches draws search-match highlights for the rows in [first, last].
// Matches whose line is currently a collapsed fold-head are skipped (their text
// is not shown until expanded); matches on hidden lines are simply never in the
// visible range. Under soft-wrap a match is clipped to each visual row it crosses.
func (r *prettyViewRenderer) rebuildMatches(first, last int) {
	pv := r.pv
	n := 0
	if pv.doc != nil && len(pv.search.matches) > 0 {
		m := pv.met
		wrapOn := pv.doc.WrapActive()
		total := pv.doc.TotalVisibleRows()
		breaks := []int32(nil)
		breaksLine := int32(-1)
		for row := first; row <= last; row++ {
			if row < 0 || int32(row) >= total {
				continue
			}
			li := int32(row)
			var sub int32
			if wrapOn {
				li, sub = pv.doc.LineAndSubRowAtRow(int32(row))
			} else {
				li = pv.doc.LineAtRow(int32(row))
			}
			idxs := pv.search.byLine[li]
			if len(idxs) == 0 || pv.doc.IsCollapsed(li) {
				continue
			}
			depth := pv.doc.Lines[li].Depth
			runeLen := pv.doc.LineRuneLen(li)
			w0, w1, colBase := r.subSpan(li, sub, runeLen, wrapOn, &breaks, &breaksLine)
			for _, mi := range idxs {
				mt := pv.search.matches[mi]
				lo := max(clampInt(mt.ColStart, 0, runeLen), w0)
				hi := min(clampInt(mt.ColEnd, 0, runeLen), w1)
				if lo >= hi {
					continue // match does not touch this visual row
				}
				fill := pv.matchColor
				if mi == pv.search.active {
					fill = pv.activeMatchColor
				}
				n = r.placeSpanRect(&r.matchRects, n, m, depth, lo, hi, colBase, row, fill, 0)
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
