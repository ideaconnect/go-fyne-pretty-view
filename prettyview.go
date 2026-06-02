// Package prettyview provides a memory-efficient, virtualized Fyne widget for
// viewing structured data — JSON, JSONC, XML, HTML, and raw text — in the style
// of Bruno's response viewer: syntax highlighting, per-node expand/fold with
// collapse summaries, copy-subtree, true character-level free-text selection,
// and incremental search.
//
// The widget is built around a hard memory bound: only the rows currently
// visible in the viewport ever exist as live fyne.CanvasObjects. Everything
// else lives in a compact struct-of-arrays document model, and selection,
// search, and copy all operate on that model rather than on widgets. As a
// result a multi-megabyte document occupies a small, predictable number of
// canvas objects regardless of its size.
//
// # Threading
//
// PrettyView follows the standard Fyne widget threading model: it is NOT safe for
// concurrent use. Every method — including the data, fold, selection, and search
// mutators (SetData, SetText, Reparse, ExpandAll, CollapseAll, ExpandTo, Search,
// SearchNext/Prev, ClearSearch, SelectAll, ClearSelection, SetTheme, …) — must be
// called on the goroutine that runs the Fyne event loop (the "main"/Fyne
// goroutine), exactly like any other Fyne widget. To drive the widget from another
// goroutine (e.g. after a network fetch), marshal the call with fyne.Do:
//
//	go func() {
//	    data := fetch()
//	    fyne.Do(func() { pv.SetData(data, prettyview.FormatAuto) })
//	}()
//
// The widget intentionally holds no locks: all of its state (the document, the
// fold index, and the selection/search state) is owned by the Fyne goroutine, so
// single-threaded access is a precondition rather than something guarded at
// runtime. The only work that runs off that goroutine is the search-debounce timer
// (time.AfterFunc); it marshals its scan back via fyne.Do and drops superseded
// scans via a generation counter, so it never touches widget state concurrently.
package prettyview

import (
	"image/color"
	"sync/atomic"
	"time"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/geometry"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/parse"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// PrettyView is the virtualized structured-data viewer widget.
//
// All state is unexported. Construct one with New or NewWithData and feed it
// content with SetData / SetText.
//
// PrettyView is not safe for concurrent use: call its methods on the Fyne
// goroutine (see the package doc's Threading section).
type PrettyView struct {
	widget.BaseWidget

	cfg config
	doc *model.Document

	// view state, owned by the Fyne goroutine
	r          *prettyViewRenderer
	met        geometry.Metrics
	palette    []color.Color
	guideColor color.Color
	selColor   color.Color
	viewOffX   float32 // current horizontal scroll offset (content space)
	viewW      float32 // current viewport width

	// recomputeMetrics memo: the measured cell + palette only change with the text
	// size / theme variant / theme override, not on every data/fold/search refresh.
	metricsReady bool
	lastTextSize float32
	lastVariant  fyne.ThemeVariant

	// selection state (4 positions + flags, all model-based)
	sel          selection
	focused      bool
	overTriangle bool

	// search state
	search            searchState
	searchTimer       *time.Timer // debounce timer for keystroke-driven search
	searchGen         int         // bumped to invalidate a superseded/already-queued debounced scan
	destroyed         atomic.Bool // set on renderer teardown; guards the debounce callback
	matchColor        color.Color
	activeMatchColor  color.Color
	onSearchRequested func() // invoked on Ctrl+F (e.g. to focus a search box)
	onSearchChanged   func() // invoked after the match set / active match changes
	onDataChanged     func() // invoked after SetData/Reparse swaps the document

	// multi-click tracking for word/line selection
	lastClickAt  time.Time
	lastClickPos fyne.Position
	clickCount   int
	wordScratch  []rune // reused decode buffer for wordBounds (avoids per-drag alloc)

	// soft-wrap state (owned by the Fyne goroutine). wrapColsKey is the depth-0
	// column budget the model was last projected at; a resize reprojects only when
	// it changes. wrapCols is the reused per-depth budget table passed to the model.
	wrapColsKey int
	wrapCols    []int
}

// New constructs an empty PrettyView, applying zero or more Options.
func New(opts ...Option) *PrettyView {
	pv := &PrettyView{cfg: defaultConfig()}
	for _, o := range opts {
		o(&pv.cfg)
	}
	pv.doc = model.EmptyDocument()
	pv.ExtendBaseWidget(pv)
	return pv
}

// SetData parses src under format (FormatAuto detects) and refreshes the view.
// Parsing is synchronous; the model it builds is compact (~5x the source) so this
// is fast even for multi-megabyte input.
//
// The src slice is retained: the model holds zero-copy byte ranges into it, so
// callers must not mutate src after this call (copy it first if it may change).
func (pv *PrettyView) SetData(src []byte, format Format) {
	if format == FormatAuto {
		format = pv.cfg.format
	}
	pv.doc = parse.Parse(src, format, pv.cfg.collapseDepth, pv.cfg.tabWidth)
	pv.ClearSearch()
	pv.ClearSelection() // a selection from the old document is meaningless against the new one
	pv.Refresh()
	if pv.onDataChanged != nil {
		pv.onDataChanged()
	}
}

// SetText is shorthand for SetData([]byte(s), FormatAuto).
func (pv *PrettyView) SetText(s string) { pv.SetData([]byte(s), FormatAuto) }

// Reparse re-parses the current source under a different format (e.g. when a UI
// lets the user override auto-detection). No-op if no document is loaded.
func (pv *PrettyView) Reparse(format Format) {
	if pv.doc == nil {
		return
	}
	pv.SetData(pv.doc.Src, format)
}

// Source returns the bytes of the current document (the originally supplied
// input), or nil.
func (pv *PrettyView) Source() []byte {
	if pv.doc == nil {
		return nil
	}
	return pv.doc.Src
}

// Format reports the format actually used for the current document.
func (pv *PrettyView) Format() Format {
	if pv.doc == nil {
		return FormatRaw
	}
	return pv.doc.Format
}

// SetOnDataChanged registers a callback invoked whenever the document is
// replaced (SetData/SetText/Reparse). Use it to keep host controls (such as a
// format selector) in sync. Setting it replaces any previous callback.
func (pv *PrettyView) SetOnDataChanged(fn func()) { pv.onDataChanged = fn }

// SetOnSearchChanged registers a callback invoked whenever the search match set
// or active match changes. Use it to keep a host match counter in sync. Setting
// it replaces any previous callback.
func (pv *PrettyView) SetOnSearchChanged(fn func()) { pv.onSearchChanged = fn }

// NewWithData constructs a PrettyView and immediately parses src under format.
func NewWithData(src []byte, format Format, opts ...Option) *PrettyView {
	pv := New(opts...)
	pv.SetData(src, format)
	return pv
}

// ExpandAll expands every node.
func (pv *PrettyView) ExpandAll() {
	if pv.doc != nil {
		pv.doc.ExpandAll()
		pv.Refresh()
	}
}

// CollapseAll collapses every node below the top level.
func (pv *PrettyView) CollapseAll() {
	if pv.doc != nil {
		pv.doc.CollapseAll()
		pv.Refresh()
	}
}

// SetDefaultCollapseDepth sets the auto-collapse depth applied on subsequent
// SetData calls (0 disables).
func (pv *PrettyView) SetDefaultCollapseDepth(depth int) {
	if depth < 0 {
		depth = 0
	}
	pv.cfg.collapseDepth = depth
}

// ExpandTo expands every collapsed ancestor of the node owning byte offset off
// (JSON only; XML/HTML lack source offsets) and scrolls it into view.
func (pv *PrettyView) ExpandTo(off int) {
	if pv.doc == nil {
		return
	}
	node := pv.nodeAtByteOffset(off)
	if node == model.NoNode {
		return
	}
	line := pv.doc.Nodes[node].HeadLine
	pv.doc.RevealLine(line)
	pv.refreshContent()
	pv.centerOnLine(line, 0)
}

// SetWrap switches long-line handling between WrapNone (horizontal scroll) and
// WrapWord (soft-wrap to the viewport width) and refreshes. Wrapping is purely
// presentational: the model, selection, search, and copy are unchanged — a wrapped
// line still copies as one logical line.
func (pv *PrettyView) SetWrap(mode WrapMode) {
	pv.cfg.wrap = mode
	pv.refreshContent() // reflow -> syncWrap reconciles the projection + scroll direction
}

// Wrap reports the current long-line handling mode.
func (pv *PrettyView) Wrap() WrapMode { return pv.cfg.wrap }

// wrapColsTable rebuilds (into a reused buffer) the per-depth text-column budget for
// the current viewport width and metrics, which the model consumes to wrap lines.
func (pv *PrettyView) wrapColsTable() []int {
	n := int(pv.doc.MaxDepth) + 1
	if cap(pv.wrapCols) < n {
		pv.wrapCols = make([]int, n)
	} else {
		pv.wrapCols = pv.wrapCols[:n]
	}
	for d := 0; d < n; d++ {
		pv.wrapCols[d] = pv.met.ColsForDepth(uint8(d), pv.viewW)
	}
	return pv.wrapCols
}

// syncWrap reconciles the model's wrap projection with the configured mode and the
// current viewport width. It is cheap when already in sync (a single key compare);
// it reprojects (O(n)) only on the first enable, a mode change, or a resize that
// crosses a column boundary. Called from reflow once the viewport size is known.
func (pv *PrettyView) syncWrap() {
	if pv.r == nil || pv.doc == nil {
		return
	}
	wantWrap := pv.cfg.wrap == WrapWord && pv.viewW > 0
	if !wantWrap {
		if pv.doc.WrapActive() {
			pv.doc.SetWrapColumns(nil)
			pv.wrapColsKey = 0
			pv.r.scroll.Direction = container.ScrollBoth
			pv.r.scroll.Content.Resize(pv.contentSize())
		}
		return
	}
	key := pv.met.ColsForDepth(0, pv.viewW)
	if pv.doc.WrapActive() && key == pv.wrapColsKey {
		return // already projected at this width
	}
	pv.r.scroll.Direction = container.ScrollVerticalOnly
	pv.doc.SetWrapColumns(pv.wrapColsTable())
	pv.wrapColsKey = key
	pv.r.scroll.Content.Resize(pv.contentSize())
}

// SetTheme overrides any of the viewer's colors for a theme variant and
// refreshes. Nil fields keep their defaults; calls compose with earlier
// WithTheme/WithSyntaxColors/SetTheme overrides for that variant.
func (pv *PrettyView) SetTheme(variant fyne.ThemeVariant, t Theme) {
	pv.cfg.setThemeOverride(variant, t)
	pv.metricsReady = false // override changed: force the palette to rebuild even if the variant didn't
	pv.Refresh()
}

// SetSyntaxColors overrides just the syntax token colors for a theme variant and
// refreshes (shorthand for SetTheme with only the token fields set).
func (pv *PrettyView) SetSyntaxColors(variant fyne.ThemeVariant, c SyntaxColors) {
	pv.cfg.setThemeOverride(variant, c.asTheme())
	pv.metricsReady = false // override changed: force the palette to rebuild
	pv.Refresh()
}

// centerOnLine scrolls so that (line, col) is centered in the viewport. Under
// soft-wrap the target may be on a continuation sub-row, so the cell's exact
// content-space origin (via geometry.CellOrigin) drives the vertical centering and
// there is no horizontal scroll. The upper-bound guard keeps a stale line index
// (e.g. from a match recorded before a fold change) from indexing out of range.
func (pv *PrettyView) centerOnLine(line int32, col int) {
	if pv.r == nil || line < 0 || int(line) >= pv.doc.TotalLines() {
		return
	}
	vp := pv.r.scroll.Size()
	cs := pv.contentSize()
	cx, cy := geometry.CellOrigin(pv.doc, pv.met, line, col)
	y := clampf(cy-(vp.Height-pv.met.RowH)/2, 0, max(0, cs.Height-vp.Height))
	x := float32(0)
	if !pv.doc.WrapActive() {
		x = clampf(cx-vp.Width/2, 0, max(0, cs.Width-vp.Width))
	}
	pv.r.scrollToOffset(fyne.NewPos(x, y))
}
