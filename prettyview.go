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
package prettyview

import (
	"image/color"
	"time"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/parse"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// PrettyView is the virtualized structured-data viewer widget.
//
// All state is unexported. Construct one with New or NewWithData and feed it
// content with SetData / SetText.
type PrettyView struct {
	widget.BaseWidget

	cfg config
	doc *model.Document

	// view state, owned by the Fyne goroutine
	r          *prettyViewRenderer
	met        metrics
	palette    []color.Color
	guideColor color.Color
	selColor   color.Color
	viewOffX   float32 // current horizontal scroll offset (content space)
	viewW      float32 // current viewport width

	// selection state (4 positions + flags, all model-based)
	sel          selection
	focused      bool
	overTriangle bool

	// search state
	search            searchState
	searchTimer       *time.Timer // debounce timer for keystroke-driven search
	matchColor        color.Color
	activeMatchColor  color.Color
	onSearchRequested func() // invoked on Ctrl+F (e.g. to focus a search box)
	onSearchChanged   func() // invoked after the match set / active match changes
	onDataChanged     func() // invoked after SetData/Reparse swaps the document

	// multi-click tracking for word/line selection
	lastClickAt  time.Time
	lastClickPos fyne.Position
	clickCount   int
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
	pv.doc = parse.Parse(src, format, pv.cfg.collapseDepth)
	pv.ClearSearch()
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
func (pv *PrettyView) SetDefaultCollapseDepth(depth int) { pv.cfg.collapseDepth = depth }

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

// SetSyntaxColors overrides the syntax palette for a theme variant and refreshes.
func (pv *PrettyView) SetSyntaxColors(variant fyne.ThemeVariant, c SyntaxColors) {
	if pv.cfg.syntaxOverrides == nil {
		pv.cfg.syntaxOverrides = map[fyne.ThemeVariant]SyntaxColors{}
	}
	pv.cfg.syntaxOverrides[variant] = c
	pv.Refresh()
}

// centerOnLine scrolls so that (line, col) is centered in the viewport.
func (pv *PrettyView) centerOnLine(line int32, col int) {
	if pv.r == nil || line < 0 {
		return
	}
	row := int(pv.doc.RowOfLine(line))
	depth := pv.doc.Lines[line].Depth
	vp := pv.r.scroll.Size()
	cs := pv.contentSize()
	y := clampf(float32(row)*pv.met.rowH-(vp.Height-pv.met.rowH)/2, 0, maxf(0, cs.Height-vp.Height))
	x := clampf(pv.met.colX(depth, col)-vp.Width/2, 0, maxf(0, cs.Width-vp.Width))
	pv.r.scrollToOffset(fyne.NewPos(x, y))
}
