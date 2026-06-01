package prettyview

import (
	"image/color"
	"math"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// prettyViewRenderer implements the manual visible-window virtualization. It owns
// a container.Scroll over a contentBox that is sized to the full document extent
// but only ever contains ~viewport-many live row widgets (recycled via a pool),
// plus the selection and match highlight layers.
type prettyViewRenderer struct {
	pv *PrettyView

	scroll  *container.Scroll
	content *fyne.Container // the scroll content (sized to the whole document)

	selLayer   *fyne.Container // lowest z: selection rectangles
	matchLayer *fyne.Container // search-match rectangles
	rowLayer   *fyne.Container // highest z: row text

	rowPool sync.Pool
	live    map[int]*rowWidget // visible row index -> widget

	firstRow, lastRow int  // current visible row range
	dragArmed         bool // a selection drag is in progress

	selRects   []*canvas.Rectangle // pooled selection highlight rects
	matchRects []*canvas.Rectangle // pooled search-match highlight rects

	// reusable Objects backing slices, one per layer (Fyne holds Objects by
	// reference, so these must not be shared between layers)
	rowObjs   []fyne.CanvasObject
	selObjs   []fyne.CanvasObject
	matchObjs []fyne.CanvasObject
}

// CreateRenderer implements fyne.Widget. It builds the scroll + layered content
// and wires scrolling to the visible-window reflow.
func (pv *PrettyView) CreateRenderer() fyne.WidgetRenderer {
	pv.ExtendBaseWidget(pv)
	r := &prettyViewRenderer{pv: pv, live: map[int]*rowWidget{}}
	r.rowPool.New = func() any { return newRowWidget(pv) }

	r.selLayer = container.NewWithoutLayout()
	r.matchLayer = container.NewWithoutLayout()
	r.rowLayer = container.NewWithoutLayout()
	r.content = container.New(&contentLayout{pv: pv}, r.selLayer, r.matchLayer, r.rowLayer)

	r.scroll = container.NewScroll(r.content)
	r.scroll.Direction = container.ScrollBoth
	r.scroll.OnScrolled = func(fyne.Position) { r.reflow() }

	pv.r = r
	pv.recomputeMetrics()
	return r
}

func (r *prettyViewRenderer) Destroy()                     {}
func (r *prettyViewRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.scroll} }
func (r *prettyViewRenderer) MinSize() fyne.Size           { return fyne.NewSize(120, 80) }

func (r *prettyViewRenderer) Layout(size fyne.Size) {
	r.scroll.Resize(size)
	r.scroll.Move(fyne.NewPos(0, 0))
	r.reflow()
}

func (r *prettyViewRenderer) Refresh() {
	r.pv.recomputeMetrics()
	r.scroll.Content.Resize(r.pv.contentSize())
	r.scroll.Refresh()
	r.reflow()
	canvas.Refresh(r.pv)
}

// reflow recomputes the visible window from the scroll offset and recycles row
// widgets so only the on-screen rows are live. Transcribed from widget.List's
// fixed-height fast path.
func (r *prettyViewRenderer) reflow() {
	pv := r.pv
	if pv.doc == nil || pv.met.rowH <= 0 {
		return
	}
	pv.viewOffX = r.scroll.Offset.X
	pv.viewW = r.scroll.Size().Width
	vpH := r.scroll.Size().Height
	m := pv.met

	total := int(pv.doc.TotalVisibleRows())
	if total == 0 {
		r.clearRows()
		r.rowLayer.Objects = nil
		r.rowLayer.Refresh()
		return
	}

	offY := r.scroll.Offset.Y
	first := int(math.Floor(float64(offY / m.rowH)))
	if first < 0 {
		first = 0
	}
	last := int(math.Ceil(float64((offY + vpH) / m.rowH)))
	if last >= total {
		last = total - 1
	}
	r.firstRow, r.lastRow = first, last

	// Recycle rows that scrolled out of view.
	for idx, rw := range r.live {
		if idx < first || idx > last {
			rw.Hide()
			rw.line = -1
			r.rowPool.Put(rw)
			delete(r.live, idx)
		}
	}

	cw := pv.contentSize().Width
	size := fyne.NewSize(cw, m.rowH)
	for idx := first; idx <= last; idx++ {
		rw, existed := r.live[idx]
		if !existed {
			rw = r.rowPool.Get().(*rowWidget)
			r.live[idx] = rw
		}
		// Set the line and geometry WITHOUT refreshing, then trigger exactly one
		// build per row. Show/Resize/Refresh each funnel into the renderer's
		// build(), so we let only one of them fire: Show for a newly-shown row,
		// Refresh for a reused one. Resize is skipped unless the size truly changed
		// (all rows share one size, so this is normally a no-op).
		rw.line = pv.doc.LineAtRow(int32(idx))
		rw.Move(fyne.NewPos(0, float32(idx)*m.rowH))
		if rw.Size() != size {
			rw.Resize(size)
		}
		if existed {
			rw.Refresh()
		} else {
			rw.Show()
		}
	}

	// The loop above already (re)built and repainted each row exactly once. Use
	// canvas.Refresh here (not rowLayer.Refresh, whose Container.Refresh would
	// re-build every child a second time) to pick up the new Objects list.
	r.rowLayer.Objects = r.liveObjects()
	canvas.Refresh(r.rowLayer)

	r.rebuildSelection(first, last)
	r.rebuildMatches(first, last)
}

// liveObjects returns the live rows as a CanvasObject slice, reusing one backing
// array across reflows. The slice is published as rowLayer.Objects, so it is not
// shared with any other layer.
func (r *prettyViewRenderer) liveObjects() []fyne.CanvasObject {
	r.rowObjs = r.rowObjs[:0]
	for _, rw := range r.live {
		r.rowObjs = append(r.rowObjs, rw)
	}
	return r.rowObjs
}

func (r *prettyViewRenderer) clearRows() {
	for idx, rw := range r.live {
		rw.Hide()
		rw.line = -1
		r.rowPool.Put(rw)
		delete(r.live, idx)
	}
}

// scrollToOffset programmatically scrolls and reflows. ScrollToOffset updates the
// offset and scrollbar thumbs WITHOUT the scroll.Refresh()->Content.Refresh()
// cascade (which would rebuild every layer redundantly with reflow), and does not
// fire OnScrolled, so reflow is called explicitly.
func (r *prettyViewRenderer) scrollToOffset(p fyne.Position) {
	r.scroll.ScrollToOffset(p)
	r.reflow()
}

// scrollBy scrolls by a delta, clamped to the valid range.
func (r *prettyViewRenderer) scrollBy(dx, dy float32) {
	cs := r.pv.contentSize()
	vp := r.scroll.Size()
	off := r.scroll.Offset
	nx := clampf(off.X+dx, 0, maxf(0, cs.Width-vp.Width))
	ny := clampf(off.Y+dy, 0, maxf(0, cs.Height-vp.Height))
	r.scrollToOffset(fyne.NewPos(nx, ny))
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// contentLayout reports the full document extent as the scroll content's MinSize
// and stretches the three layers to fill it. It never walks children for sizing,
// so a deep/large document costs only arithmetic here.
type contentLayout struct{ pv *PrettyView }

func (cl *contentLayout) MinSize([]fyne.CanvasObject) fyne.Size { return cl.pv.contentSize() }

func (cl *contentLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
}

// contentSize is the full scrollable extent for the current document and fold
// state. Width is an upper bound (widest line's runes at the deepest indent).
func (pv *PrettyView) contentSize() fyne.Size {
	if pv.doc == nil || pv.met.rowH <= 0 {
		return fyne.NewSize(0, 0)
	}
	rows := pv.doc.TotalVisibleRows()
	h := float32(rows) * pv.met.rowH
	w := pv.met.textOriginX(pv.doc.MaxDepth) + float32(pv.doc.MaxLineRunes)*pv.met.charWidth + pv.met.charWidth*2
	return fyne.NewSize(w, h)
}

// recomputeMetrics measures the monospace cell, builds the metrics, and rebuilds
// the syntax palette for the active theme variant.
func (pv *PrettyView) recomputeMetrics() {
	ts := float32(theme.TextSize())
	style := fyne.TextStyle{Monospace: true}
	sz := fyne.MeasureText("MMMMMMMMMM", ts, style)
	cw := sz.Width / 10
	pv.met = newMetrics(pv.cfg, cw, sz.Height)
	pv.met.textSize = ts

	variant := fyne.CurrentApp().Settings().ThemeVariant()
	var override *SyntaxColors
	if pv.cfg.syntaxOverrides != nil {
		if c, ok := pv.cfg.syntaxOverrides[variant]; ok {
			override = &c
		}
	}
	pv.palette = buildPalette(variant, override)
	pv.guideColor = guideColorFor(variant)
	pv.selColor = selectionColorFor(variant)
	pv.matchColor = color.NRGBA{0xff, 0xd5, 0x4f, 0x55}       // soft yellow
	pv.activeMatchColor = color.NRGBA{0xff, 0x8c, 0x1a, 0xaa} // strong orange
}

func selectionColorFor(variant fyne.ThemeVariant) color.Color {
	c := themeColor(theme.ColorNameSelection, variant)
	rr, gg, bb, _ := c.RGBA()
	return color.NRGBA{uint8(rr >> 8), uint8(gg >> 8), uint8(bb >> 8), 0x66}
}

func guideColorFor(variant fyne.ThemeVariant) color.Color {
	fg := themeColor(theme.ColorNameForeground, variant)
	rr, gg, bb, _ := fg.RGBA()
	return color.NRGBA{uint8(rr >> 8), uint8(gg >> 8), uint8(bb >> 8), 0x22}
}
