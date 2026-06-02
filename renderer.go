package prettyview

import (
	"math"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/geometry"
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
	pv.destroyed.Store(false) // re-enable if the widget is being re-created after a Destroy
	pv.searchGen++            // invalidate any debounce scan queued before the Destroy/re-create
	r := &prettyViewRenderer{pv: pv, live: map[int]*rowWidget{}}
	// Pooled rows start hidden so the reflow's Show() reliably fires the row
	// renderer's build(): a row that is already visible (Fyne's default) would make
	// Show() a no-op and the row would render blank on its first appearance. The
	// recycle path Hide()s rows before returning them to the pool, so this only
	// matters for freshly-created ones.
	r.rowPool.New = func() any {
		rw := newRowWidget(pv)
		rw.Hide()
		return rw
	}

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

// Destroy tears the widget down. It cancels any pending debounced search so the
// timer can't fire after teardown (a stale scan against freed state / off the Fyne
// thread). The destroyed flag also makes an already-fired-but-not-yet-run callback
// a no-op, closing the window Timer.Stop alone can't.
func (r *prettyViewRenderer) Destroy() {
	r.pv.destroyed.Store(true)
	r.pv.stopSearchTimer()
}
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
	r.refreshBars()
	r.reflow()
	canvas.Refresh(r.pv)
}

// refreshBars updates the scrollbars for the current content extent WITHOUT
// scroll.Refresh()'s Content.Refresh() cascade, which would rebuild every live row
// a second time on top of reflow() (which rebuilds the visible window exactly once).
// container.Scroll exposes no bars-only refresh and ScrollToOffset early-returns on
// an unchanged offset, so we detach the rows first: scroll.Refresh()'s cascade then
// finds an empty rowLayer (no row builds) and reflow() repopulates it immediately
// after, within the same synchronous call (no intervening paint).
func (r *prettyViewRenderer) refreshBars() {
	r.rowLayer.Objects = nil
	r.scroll.Refresh()
}

// reflow recomputes the visible window from the scroll offset and recycles row
// widgets so only the on-screen rows are live. Transcribed from widget.List's
// fixed-height fast path.
func (r *prettyViewRenderer) reflow() {
	pv := r.pv
	if pv.doc == nil || pv.met.RowH <= 0 {
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
		// Drop any selection/match rectangles left over from a previous, non-empty
		// document. The normal path clears these via rebuildSelection/rebuildMatches,
		// which we skip here, so do it explicitly (and reset the visible range).
		r.firstRow, r.lastRow = 0, -1
		r.applyRects(r.selLayer, &r.selRects, &r.selObjs, 0)
		r.applyRects(r.matchLayer, &r.matchRects, &r.matchObjs, 0)
		return
	}

	offY := r.scroll.Offset.Y
	first := int(math.Floor(float64(offY / m.RowH)))
	if first < 0 {
		first = 0
	}
	last := int(math.Ceil(float64((offY + vpH) / m.RowH)))
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
	size := fyne.NewSize(cw, m.RowH)
	for idx := first; idx <= last; idx++ {
		rw, existed := r.live[idx]
		if !existed {
			rw = r.rowPool.Get().(*rowWidget)
			r.live[idx] = rw
		}
		// Set the line and geometry WITHOUT refreshing, then trigger exactly one
		// build per row. Move and Resize do NOT build (Resize only calls the row
		// renderer's no-op Layout); the single build comes from Show for a newly-
		// shown (hidden) row, or Refresh for a reused one. Resize is skipped unless
		// the size truly changed (all rows share one size, so this is normally a
		// no-op). Pooled rows start hidden (see CreateRenderer) so Show reliably
		// fires the build rather than no-opping on an already-visible row.
		rw.line = pv.doc.LineAtRow(int32(idx))
		rw.Move(fyne.NewPos(0, float32(idx)*m.RowH))
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
	if pv.doc == nil || pv.met.RowH <= 0 {
		return fyne.NewSize(0, 0)
	}
	rows := pv.doc.TotalVisibleRows()
	h := float32(rows) * pv.met.RowH
	w := pv.met.TextOriginX(pv.doc.MaxDepth) + float32(pv.doc.MaxLineRunes)*pv.met.CharWidth + pv.met.CharWidth*2
	return fyne.NewSize(w, h)
}

// recomputeMetrics measures the monospace cell, builds the metrics, and resolves
// the effective theme (palette + selection/match/guide colors) for the active
// variant, applying any per-variant override.
func (pv *PrettyView) recomputeMetrics() {
	// Reading the text size and variant is cheap; do it first so the memo can skip
	// the expensive MeasureText + palette rebuild when nothing relevant changed.
	// SetTheme/SetSyntaxColors clear metricsReady to force a rebuild on override.
	var ts float32
	variant := fyne.ThemeVariant(theme.VariantDark)
	app := fyne.CurrentApp()
	haveApp := app != nil && app.Settings() != nil
	if haveApp {
		ts = theme.TextSize()
		variant = app.Settings().ThemeVariant()
	} else {
		ts = theme.DefaultTheme().Size(theme.SizeNameText)
	}
	if pv.metricsReady && ts == pv.lastTextSize && variant == pv.lastVariant {
		return // measured cell + palette unchanged — skip MeasureText and the palette alloc
	}

	// Measuring text needs a live app/driver. Built before an app exists (e.g.
	// SetData before app.New()), fall back to default-theme estimates so construction
	// never panics; the memo mismatch above remeasures once a canvas exists.
	var cw, glyphH float32
	if haveApp {
		sz := fyne.MeasureText("MMMMMMMMMM", ts, fyne.TextStyle{Monospace: true})
		cw, glyphH = sz.Width/10, sz.Height
	} else {
		cw, glyphH = ts*0.6, ts*1.3
	}
	pv.met = geometry.NewMetrics(cw, glyphH, pv.cfg.indentStep)
	pv.met.TextSize = ts

	t := pv.resolveTheme(variant)
	pv.palette = t.palette()
	pv.guideColor = t.IndentGuide
	pv.selColor = t.Selection
	pv.matchColor = t.Match
	pv.activeMatchColor = t.ActiveMatch

	pv.lastTextSize, pv.lastVariant, pv.metricsReady = ts, variant, true
}
