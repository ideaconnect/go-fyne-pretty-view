All facts confirmed. Here is the authoritative architecture and build plan.

---

# prettyview — Authoritative Architecture & Build Plan

**Module:** `github.com/ideaconnect/go-fyne-pretty-view` · **Fyne:** v2.7.4 · **Go:** 1.26 · **Status:** design-frozen, implementation-ready

A reusable Go+Fyne widget: a Bruno-like structured-data viewer for JSON / JSONC / XML / HTML / raw text with syntax highlighting, per-node expand/fold with collapse summaries, copy-subtree, true character-level free-text drag selection across rows, and incremental search with reveal-into-folds. Built for hard memory bounds via row virtualization.

All Fyne citations below are verified against `fyne.io/fyne/v2@v2.7.4/`.

---

## 1. Overview & non-negotiable constraints

### 1.1 The hard memory bound (stated as an object-count invariant)

> **INVARIANT M-1 (live CanvasObjects).** At any instant, the number of live `fyne.CanvasObject`s owned by a `PrettyView` is bounded by
> `V × (G + 1 + K) + V × (R_sel + R_match) + C_chrome`
> where `V = ceil(viewportHeight/rowHeight)+2` (visible rows incl. overscan), `G = max indent guides per row (capped 32)`, `K = max colored text runs emitted per row (capped 2·viewportCols)`, `R_sel`/`R_match` = selection/match rectangles per row (≤ a few), and `C_chrome` ≈ 12 (scroll bars/shadows). **This bound is independent of document size.** For a 900px viewport it is ≈ 1,000 objects (worst case ≈ 2,800). It must never scale with total rows, total tokens, line length, or selection span.

> **INVARIANT M-2 (per-object GPU/heap bytes).** No single `canvas.Text` may be wider than the viewport. Each text run is horizontally **culled to the visible column window** before its `.Text` is set, so its rasterized texture is `≤ viewportWidth × rowHeight × 4` bytes. This is **mandatory**, not an optimization (see Risk R-1).

> **INVARIANT M-3 (model size).** The parsed document is a struct-of-arrays of flat arenas: one shared `[]byte` source (zero-copy segments), one `[]Node` (32 B/node, no pointers), one `[]Line` (24 B), one `[]Segment` (12 B), plus the fold index. Target ≈ 5× source bytes. The 467 KiB `openapi.json` ⇒ ≈ 2.3 MB model (≈4.85×, the measured ratio guarded by `TestModelSizeRatio`). No per-node heap allocation; no per-token `color.Color`; no per-line `[]rune` stored for the whole document.

> **INVARIANT M-4 (selection/search/copy are model-based).** Selection state is four integers; matches are `(line, colStart, colEnd)` triples (line-keyed so they survive folding); copy reconstructs text from the displayed segments of the model. **No `CanvasObject` is ever read to produce clipboard text, and nothing per-character/per-token/per-off-screen-row is ever a widget.** This mirrors Fyne's own `widget/selectable.go` (state = 4 ints + flag, selectable.go:16-24; `SelectedText` slices `[]rune(provider.String())[start:stop]`, selectable.go:120-131).

### 1.2 Why these are reachable (and where naïve designs break)

- The glfw painter rasterizes each `canvas.Text` to a bitmap sized to the **entire string width** (`bounds := text.MinSize()` → `image.NewRGBA(Rect(0,0,width,height))`, `internal/painter/gl/texture.go:171-173`). One 2 MB single-line value would attempt a ~1.15 GB bitmap. → M-2 forces per-row horizontal culling.
- Text textures are cached **by content** `FontCacheEntry{Text,Size,Style,Source,Canvas,Color}` with a 1-minute expiry (`internal/cache/base.go:9` `ValidDuration = 1*time.Minute`; key in `cache/text.go`). Scrolling a large file deposits one retained texture per distinct token for up to 60 s. → M-2's narrow textures + a memory test that scrolls the whole file (§11) bound this.
- The scroller does **no** spatial culling: `Content.Move(-Offset.X,-Offset.Y)` moves the whole content (scroller.go:454) and the tree walk visits every child (`internal/driver/util.go` `walkObjectTree`, no spatial pruning). → only visible rows may ever be in the tree (M-1).
- Fyne's own `selectableRenderer.buildSelection` builds one rectangle **per selected row across the full span** (selectable.go:373-385) — copying it verbatim yields O(span) rects. → we intersect with the visible window first (M-1, §6.4).

---

## 2. Package layout

One module: a public package `prettyview` (the widget, renderer, input, controls, theme) plus three internal leaf packages — `internal/parse`, `internal/model`, `internal/geometry`. No `internal/` Fyne imports anywhere (A3 constraint C: `go vet` rejects `fyne.io/fyne/v2/internal/...`; we use `sync.Pool`, `container.Scroll`, `fyne.MeasureText` instead).

```
github.com/ideaconnect/go-fyne-pretty-view/
├── go.mod                       // module path, Go 1.26, require fyne.io/fyne/v2 v2.7.4, golang.org/x/net
│
│   # Root: the public package `prettyview`.
├── prettyview.go                // PrettyView widget: BaseWidget, CreateRenderer, public ctor + options
├── widget_input.go              // input interfaces (Tappable/SecondaryTappable/Mouseable/Draggable/Hoverable/Cursorable/Focusable/Shortcutable); Tapped (fold), TappedSecondary (context menu)
├── renderer.go                  // prettyViewRenderer: container.Scroll + manual visible-window virtualization
├── row.go                       // rowWidget + rowRenderer: per-row colored canvas.Text runs (horizontally culled), indent guides, fold triangle
├── highlight.go                 // pooled selection + search-match rectangles (visible-window only)
├── selection.go                 // selection state (anchor/focus modelPos), hit-test wiring, copy, select-all
├── selection_words.go           // token-aware word/line bounds for double/triple-click
├── search.go                    // SearchQuery/Match, debounced byte-scan, reveal-into-folds, nav, status
├── theme.go                     // Theme + SyntaxColors, default dark/light palettes, palette() builder, theme-color helpers
├── options.go                   // functional Options: WithFormat, WithWrap, WithSearchConfig, WithTheme, ...
├── format.go                    // public Format / WrapMode aliases of the model types
├── controls.go                  // OPTIONAL ready-made controls: NewToolbar (+ToolbarConfig), NewSearchBar, NewFormatSelect, NewFoldButtons, NewWrapToggle, ShowOpenDialog
├── icons.go                     // embedded Font Awesome Free toolbar glyphs (icons/fontawesome/*.svg, CC BY 4.0), recolored to the theme foreground
│
├── internal/geometry/
│   └── geometry.go              // Metrics: exact charWidth + rounded rowHeight, col<->x, pixel<->(row,col) hit-test, ColsForDepth (wrap), ONE origin convention
│
├── internal/model/              // Document SoA + fold index + soft-wrap projection (no Fyne imports)
│   ├── api.go                   // exported model API consumed by the view
│   ├── model.go                 // Document SoA: Node, Line, Segment, ColorRole, arenas; display/source-byte mapping
│   ├── foldindex.go             // Fenwick fold index: visible-row projection (per-line weight), fold/unfold
│   ├── wrap.go                  // soft word-wrap: per-line visual-row weights + break computation
│   ├── builder.go               // Builder: parser-facing arena construction (Open/AddLeaf/Close), subtree-size pass
│   ├── summary.go               // fold-summary text ("{ N items }", "[ N items ]", "<tag> N children")
│   └── format.go                // Format enum + String()
│
├── internal/parse/              // Parser interface + implementations (call into the model Builder)
│   ├── parse.go                 // Parser interface, AutoDetect
│   ├── parse_json.go            // hand-written zero-copy JSON/JSONC byte scanner
│   ├── parse_xml.go             // XML via encoding/xml.Decoder.Token
│   ├── parse_html.go            // HTML via golang.org/x/net/html tokenizer (tolerant)
│   └── parse_raw.go             // raw: split on \n, one line per physical line (fallback)
│
├── testdata/                    // small.json, openapi.json (~478KB), catalog.xml, page.html, big.json (~7.5MB)
│
├── .github/
│   ├── workflows/ci.yml         // tests + >90% coverage gate + codecov + cross-platform demo build
│   └── dependabot.yml           // weekly gomod + github-actions updates
│
└── cmd/prettyview-demo/
    └── main.go                  // demo app: fixture dropdown + NewToolbar (Open/format/expand/collapse/wrap/search)
```

Tests are co-located with the code they cover: headless `*_test.go` under each
`internal/` package (model arena sizes, fold/wrap projection, geometry golden
hit-test, parser output) and root `*_test.go` for the rendering, selection,
search, virtualization, and memory-ceiling guards. See
[STRUCTURE.md](../STRUCTURE.md) for the full test map.

---

## 3. Public API

The core public signatures. The widget is **read-only** (a viewer): it exposes `SelectedText() string` and routes `Ctrl/Cmd+C` to `CopySelection` through its own `TypedShortcut` (it implements `fyne.Shortcutable`). The once-considered "disabled widget so the driver auto-routes only Copy" trick was not needed and is **not** implemented — there is no `Disabled()` method.

```go
package prettyview

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// Format selects (or, with FormatAuto, detects) the input grammar.
type Format uint8

const (
	FormatAuto Format = iota // run AutoDetect heuristics
	FormatRaw
	FormatJSON
	FormatJSONC
	FormatXML
	FormatHTML
)

// WrapMode controls long-line handling. Default WrapNone (horizontal scroll), per Bruno.
type WrapMode uint8

const (
	WrapNone WrapMode = iota // long lines overflow; horizontal scroll (default)
	WrapWord                 // soft-wrap to viewport width (recomputed on resize, debounced)
)

// PrettyView is the virtualized structured-data viewer widget.
type PrettyView struct {
	widget.BaseWidget
	// all state unexported
}

// New constructs an empty PrettyView. Apply zero or more Options.
func New(opts ...Option) *PrettyView

// NewWithData constructs a PrettyView and immediately parses src with the given format.
func NewWithData(src []byte, format Format, opts ...Option) *PrettyView

// --- Content ---

// SetData parses src under format (FormatAuto detects) and refreshes the view.
// Parsing is SYNCHRONOUS on the calling goroutine; the compact model (~5x source)
// builds fast even for multi-MB input. PrettyView is NOT safe for concurrent use —
// call on the Fyne goroutine (marshal with fyne.Do from a worker). src is retained
// zero-copy: do not mutate it after the call.
func (pv *PrettyView) SetData(src []byte, format Format)

// SetText is shorthand for SetData([]byte(s), FormatAuto).
func (pv *PrettyView) SetText(s string)

// Format reports the format actually used for the current document.
func (pv *PrettyView) Format() Format

// --- Folding ---

func (pv *PrettyView) ExpandAll()
func (pv *PrettyView) CollapseAll()

// ExpandTo expands every collapsed ancestor of the node owning byte offset off,
// then scrolls it into view. No-op if offset is out of range.
func (pv *PrettyView) ExpandTo(byteOffset int)

// SetDefaultCollapseDepth collapses, on load, every container deeper than depth
// (depth 0 = never auto-collapse). Applies to subsequent SetData calls.
func (pv *PrettyView) SetDefaultCollapseDepth(depth int)

// --- Selection / clipboard ---

// SelectedText returns the exact displayed text currently selected, or "".
func (pv *PrettyView) SelectedText() string

func (pv *PrettyView) SelectAll()
func (pv *PrettyView) ClearSelection()

// CopySelection copies SelectedText() to the clipboard (no-op if empty).
func (pv *PrettyView) CopySelection()

// CopySubtree copies the serialized source bytes of the node under byteOffset
// (the whole {…}/[…]/<tag>…</tag> span), regardless of fold state.
func (pv *PrettyView) CopySubtree(byteOffset int)

// --- Search ---

// Search starts/replaces an incremental search (debounced, synchronous, capped).
func (pv *PrettyView) Search(q SearchQuery)
func (pv *PrettyView) SearchNext()
func (pv *PrettyView) SearchPrev()
func (pv *PrettyView) ClearSearch()

// SearchStatus returns (active 1-based index, total, capped) for a count label "3/27" / "3/10000+".
func (pv *PrettyView) SearchStatus() (active, total int, capped bool)

// --- Wrap / theming ---

// SetWrap switches long-line handling at runtime; Wrap reports the current mode.
func (pv *PrettyView) SetWrap(m WrapMode)
func (pv *PrettyView) Wrap() WrapMode

// SetTheme overrides any colors for a variant; SetSyntaxColors is the token-only
// shorthand. Both compose with prior overrides.
func (pv *PrettyView) SetTheme(variant fyne.ThemeVariant, t Theme)
func (pv *PrettyView) SetSyntaxColors(variant fyne.ThemeVariant, c SyntaxColors)
```

### Options (functional options)

```go
type Option func(*config)

func WithFormat(f Format) Option              // force a format (skip auto-detect)
func WithWrap(m WrapMode) Option              // WrapNone (default) | WrapWord
func WithSearchConfig(c SearchConfig) Option  // MaxMatches, DebounceFor, MinQueryLen
func WithDefaultCollapseDepth(d int) Option   // auto-collapse below depth d on load
func WithTabWidth(n int) Option               // tab expansion width for display (default 4)
func WithIndentStep(px float32) Option        // pixels per indent level
func WithTheme(v fyne.ThemeVariant, t Theme) Option              // override any colors for a variant
func WithSyntaxColors(v fyne.ThemeVariant, c SyntaxColors) Option // token-only shorthand
```

`MinSize`/`Refresh` come from the embedded `widget.BaseWidget`; `CreateRenderer` is implemented in `renderer.go`. Copy and Select-all are handled directly by the widget's `TypedShortcut` (it implements `fyne.Shortcutable`); `Ctrl/Cmd+F` is additionally registered on the window canvas by `registerFindShortcut` (controls.go) and routed to the host via `SetOnSearchRequested`.

---

## 4. Data model & parsers

### 4.1 Decision: struct-of-arrays, `int32` ids, no pointers

A tree-of-pointers needs one alloc per node + GC scan pressure; a 151 KB JSON parses to ~6–9k nodes (§4.7), i.e. an allocation storm. **SoA** stores nodes in one `[]Node` arena; children are contiguous index ranges; per-node text is byte-offset slices into one shared `Src []byte`. `int32` `NodeID` halves id memory and caps at 2.1B nodes.

### 4.2 Core types

```go
type NodeID = int32

const NoNode NodeID = -1

type Kind uint8

const (
	KindRoot Kind = iota
	KindObject       // JSON {} (foldable)
	KindArray        // JSON [] (foldable)
	KindKeyValue     // "key": <scalar>  (single line, not foldable)
	KindScalar       // bare scalar array element
	KindElement      // XML/HTML <tag>…</tag> (foldable)
	KindEmptyElement // <tag/> or void element (single line)
	KindText         // XML/HTML text / CDATA
	KindComment      // JSONC/XML/HTML comment
	KindRawLine      // raw mode: one physical source line
	KindError        // a recovered parse-error marker line
)

// ColorRole: 1 byte per segment; resolved to color.Color at draw time. NOT a color value.
type ColorRole uint8

const (
	RolePlain ColorRole = iota
	RoleKey
	RoleString
	RoleNumber
	RoleBool
	RoleNull
	RolePunct
	RoleTag
	RoleAttr
	RoleComment
	RoleMuted // fold-summary text
)

// Segment is one contiguous same-color run on a display line. 12 bytes.
// Text is a byte slice of Src (Buf==0) or Aux (Buf==1, synthesized).
type Segment struct {
	Start uint32 // byte offset into the buffer named by Buf
	End   uint32 // exclusive
	Role  ColorRole
	Buf   uint8  // 0 = Document.Src, 1 = Document.Aux
}

// Node is the SoA structural record. 32 bytes, no pointers. A subtree is the
// contiguous id range [id, id+Subtree).
type Node struct {
	Parent     NodeID // -1 for the root
	Subtree    int32  // nodes in this subtree incl. self (>= 1)
	ChildCount int32  // direct children
	HeadLine   int32  // index into Lines of this node's own/opening line
	CloseLine  int32  // index into Lines of its closing line (== HeadLine for a leaf)
	SrcStart   uint32 // byte span into Src covering the whole node (copy-subtree)
	SrcEnd     uint32
	Kind       Kind
	Depth      uint8
	Flags      uint8 // bit0 DefaultCollapsed
	_          uint8 // padding
}

// Line is the SoA display record — the unit the projection and renderer work in.
// 24 bytes. Foldability and the collapsed ("{ N items }") rendering live per LINE
// (Fold + CollFirst/CollCount), which is why the projection (§4.4) is line-granular.
type Line struct {
	Owner     NodeID // structural node this line belongs to
	Fold      NodeID // node whose fold triangle sits on this line, or NoNode
	SegFirst  uint32 // first segment of the expanded rendering
	CollFirst uint32 // first segment of the collapsed (folded-head) rendering
	SegCount  uint16
	CollCount uint16
	Depth     uint8
	_         [3]uint8
}

type Document struct {
	Src   []byte    // original bytes, retained once for zero-copy segments
	Aux   []byte    // synthesized text: summaries, decoded entities, tab-expansion spaces
	Nodes []Node    // structural arena; Nodes[0] == synthetic root
	Lines []Line    // display arena (the projection/render unit)
	Segs  []Segment // segment arena

	Format Format

	fold *foldIndex // visible-line projection over Lines (§4.4)

	MaxLineRunes int   // widest expanded line, in runes (content-width upper bound)
	MaxDepth     uint8 // deepest indentation level present

	lineRunes []int32 // per-line expanded rune-count cache (extent)

	// Soft-wrap projection (§4.4). rowsOf[line] is the visual-row count of the line's
	// currently-displayed text at the active wrap width; nil ⇒ WrapNone fast path.
	// colsByDepth is the per-depth column budget the view supplies (the model cannot
	// import geometry), retained so one fold/unfold can re-weight a head without a full
	// reprojection.
	rowsOf      []int32
	colsByDepth []int
}
```

> **Shipped model is line-granularity.** A separate `Lines` arena is the
> projection/render unit, and the fold index (§4.4) maps visible rows to **lines**
> via `lineAtRow`/`rowOfLine`. The original design vocabulary in the rest of this
> section was node-granularity (`nodeAtRow`/`rowOfNode`, `ExtraRows`,
> `SegFirst`/`SummarySeg` on `Node`, `Foldable()`/`HasSummary()`); where those names
> still appear below, read them as their `Lines`-arena equivalents. The size
> invariants hold: `Node` 32 B, `Line` 24 B, `Segment` 12 B (guarded by
> `internal/model/sizes_test.go`). The palette lives on the widget, not the model.

**Why segments are offsets, not strings:** a JSON `"key": 42` line is 3–4 segments all pointing into `Src` at existing byte ranges → zero string allocation. Only synthesized text (summaries, entity-decoded runs, tab-expansion) lands in `Aux` once. This is RichText's per-segment-color idea (richtext.go `TextSegment`→`canvas.Text`) without RichText's per-segment live objects (R2 §4 rejects RichText for exactly this: no virtualization, O(doc) refresh).

### 4.3 Display text and tab handling (resolves A2 Issue #4)

The renderer/selection/search consume one **display string per line**, derived on
demand from the line's segments. A display line *is* its segment runs; there is **no
`colMap`** (the originally-envisaged display-col→source-byte map was never needed).

Raw/fallback lines **expand tabs to stops** at parse time (`WithTabWidth`, default 4,
now live): each tab becomes an interned run of spaces in `Aux` (a `RolePlain` pad
segment), so the uniform monospace grid (§6.3) stays exact. The original tab byte
stays in `Src`.

**Copy round-trips the original bytes.** `selectedText` walks the visible lines of the
span and appends each whole line's displayed bytes directly (no per-line `[]rune`);
for raw documents it rewrites each pad segment back to a single `\t` via
`Document.AppendDisplayLine(restoreTabs)`, so a copy reproduces the source tabs rather
than the expanded spaces. Only a partial endpoint line is rune-sliced for the column
cut. (Structured JSON/XML/HTML lines have no pad segments; in-string `\t` escapes are
preserved as source bytes.)

### 4.4 Visible-row projection — Fenwick over per-line visibility

Maps `visibleRow ↔ line` and supports fold/unfold without O(n) work per toggle. A
subtree's lines are the contiguous range `[HeadLine, CloseLine]`.

```go
type fenwick struct {
	tree   []int32 // 1-indexed BIT over Lines (prefix sums of vis)
	maxLog int     // for the binary-lift in kth()
}

type foldIndex struct {
	collapsed bitset   // over NodeID
	hiddenBy  []NodeID // per line: nearest collapsed ancestor, or NoNode
	vis       []int32  // per line: visual-row weight if visible (1 without wrap), else 0
	bit       fenwick  // prefix sums over vis
}
```

**Complexities (the contract):**

| Operation | Cost | How |
|---|---|---|
| `TotalVisibleRows()` | **O(1)** | `bit.total()` |
| `lineAtRow(row)` → line | **O(log n)** | Fenwick binary-lift (`kth`) |
| `rowOfLine(line)` → row | **O(log n)** | `bit.prefix(line)` |
| **fold(id) / unfold(id)** | **O(k lines touched)**, with an O(log n) row-count delta | set/clear the `collapsed` bit, then point-update `vis`/`hiddenBy` over the affected line range |
| `ExpandAll` / `CollapseAll` / `applyDefaults` | **O(n)** single pass | set the bitset, then one `rebuild` (the Fenwick is rebuilt once) |
| `RevealLine(line)` | O(depth + k) | `unfoldAncestors` — unfold each collapsed ancestor, outermost-first |

**Honest fold cost (A2 Issue #6 / D1).** A fold cannot be truly O(log n) when it must
hide `k` previously-visible descendant lines — each needs its `vis` zeroed. So it is
**O(k lines touched)** with an O(log n) row-count delta; bounded by currently-visible
lines, never total document size when most is already collapsed. `ExpandAll`/`CollapseAll`
and the load-time default-collapse use the O(n) `rebuild` (one Fenwick build).

**No `line→row` map** (resolves A2 Issue #7): `rowOfLine` is the O(log n) Fenwick prefix
query, not a materialized map that would need an O(n) rebuild per fold.

**Soft word-wrap — the same Fenwick, weighted by visual rows** (`wrap.go`). Wrapping
makes one display line occupy several *visual* rows whose count depends on the
viewport width. Rather than a second projection, we **generalize the per-line Fenwick
weight from `{0,1}` to `rowsOf[line]`** — the number of wrapped rows the line's
currently-displayed text occupies (`0` when hidden). `kth`/`prefix` already sum
arbitrary weights, so `TotalVisibleRows`, `lineAtRow`, and `rowOfLine` are unchanged;
`lineAndSubRow(visualRow)` adds one extra `prefix` to recover the sub-row. Fold/unfold
add `±rowsOf[line]` and re-weight the toggled head (its collapsed summary can wrap to a
different count). `rowsOf` is a side array (nil ⇒ WrapNone, a single nil check on every
projection path — the non-wrap fast path pays nothing); per-line break columns are
recomputed on demand for the ~viewport-many visible lines, never stored. The per-depth
column budget is supplied by the view (`geometry.ColsForDepth`) because the model
cannot import geometry. A resize reprojects (O(n)) only when the integer column count
changes. Because each visual row paints at most one column budget of text, no
`canvas.Text` can exceed the viewport (M-2) under wrap automatically — an unbreakable
run char-breaks. Selection/search/copy are unchanged: they operate on logical lines,
so a wrapped line still copies without soft-break newlines.

### 4.5 Parser interface and Builder

The parser never builds a tree; it drives a `Builder` that appends into arenas.

```go
type Parser interface {
	Format() Format
	Detect(src []byte) int          // 0..100 confidence for AutoDetect
	Parse(src []byte, b *Builder) error // tolerant; emits partial structure on malformed input
}

type Builder struct { /* doc *Document; stack []NodeID; unexported */ }

func (b *Builder) Open(k Kind, srcStart int, head []Seg) NodeID        // foldable; head segs e.g. `"key": {` or `<product id="0">`
func (b *Builder) Leaf(k Kind, srcStart, srcEnd int, segs []Seg) NodeID // single-line node
func (b *Builder) Close(srcEnd int, closeSegs []Seg) NodeID             // computes Subtree, pops the stack

type Seg struct {
	Role  ColorRole
	Lit   string // if non-empty, interned into Aux (synthesized)
	Start uint32 // else byte range into Src (zero-copy)
	End   uint32
}
```

`Open`/`Close` keep an explicit `stack []NodeID` so children land contiguously after the parent's slot. `ChildCount` is bumped in `linkChild` (called from `Open` and `Leaf`); `Close` computes the node's `Subtree` size and pops the stack. There is no separate post-order pass: the fold index is built once by `newFoldIndex` after parse (all-visible), and a final `finish` pass interns each fold-head's collapsed (summary) rendering once trailing commas are placed.

`AutoDetect(src)` runs each `Detect` and picks the max: leading `{`/`[` → JSON; `<?xml`/leading `<root>` → XML; `<!doctype html`/`<html` → HTML; else Raw.

### 4.6 The four parsers

- **JSON / JSONC** (`parse_json.go`): a **hand-written byte scanner** over `Src` yielding tokens with `(start,end)` offsets — zero-copy, preserves key order and comment positions for Bruno fidelity. `encoding/json.Decoder.Token` is the reference but is **not** the path: it allocates per-token strings and loses offsets (A1: that re-allocates ≈ the whole file). Mapping: `{…}`→`KindObject`, `[…]`→`KindArray`, `"key": scalar`→`KindKeyValue`, bare scalar→`KindScalar`, value-container carries its `"key":` prefix in the container's head segs (saves a node/member). The closing brace is the container's own close line (`Node.CloseLine`), owned by the same node — not a separate node.
- **XML** (`parse_xml.go`): `encoding/xml.Decoder.Token` (present in Go 1.26 stdlib). `StartElement`+children→`KindElement` with attributes inline in head segs; self-closing/empty→`KindEmptyElement`; `CharData`→`KindText` (entity-decoded into `Aux`); `Comment`→`KindComment`. Token offsets aren't exposed by `xml.Decoder`, so for XML/HTML the head/text segments use **interned `Aux` literals**, not zero-copy `Src` ranges — copy uses the display text directly (acceptable: XML copy fidelity is the reconstructed canonical form).
- **HTML** (`parse_html.go`): `golang.org/x/net/html` tokenizer (`x/net` already a transitive Fyne dep; `go get golang.org/x/net/html`). Tolerant of unclosed tags; void elements→`KindEmptyElement`; `<!DOCTYPE …>`→a muted comment-style leaf.
- **Raw** (`parse_raw.go`): split `Src` on `\n`; each line→`KindRawLine` with one `RolePlain` segment spanning the line's `Src` byte range (zero-copy). The universal fallback when a structured parse fails mid-stream.

### 4.7 Summaries

Built in the post-parse `finish` pass as a `Segment` with `Role==RoleMuted`, interned into `Aux`, forming the fold-head's collapsed rendering (`Line.CollFirst`/`CollCount`): `KindObject`→`{ N items }` (`{ }`, `{ 1 item }` special-cased), `KindArray`→`[ N items ]`, `KindElement`→`<tag> N children`. A collapsed container renders head segs + summary + close on one row; toggling flips the `collapsed` bit + the incremental Fenwick update only — no re-parse, no full re-flatten.

### 4.8 Memory budget — measured on openapi.json (~478 KB / 467 KiB)

The model-vs-source multiple is the only ratio a test asserts (`TestModelSizeRatio`,
bounded well under budget). Measured: a ≈467 KiB JSON parses to a ≈2.3 MB model
(≈4.85×). The arenas are flat and pointer-free:

| Arena | Notes |
|---|---|
| `Src` (retained zero-copy) | = source size |
| `Nodes` (32 B) + `Lines` (24 B) | one record per structural node / display line |
| `Segs` (12 B) | a few per line, offsets into `Src`/`Aux` |
| `Aux` | synthesized text (summaries, punctuation, tab pads) |
| `foldIndex` (`vis` + `hiddenBy` + Fenwick + collapsed bitset) | the projection |
| **Total** | **≈ 4.85× source** — no per-node alloc, no per-token color, no whole-doc `[]rune` |

`big.json` (7.5 MB) → a flat-arena model in the same proportion, **no allocation
storm**, live `CanvasObject` count unchanged (M-1).

---

## 5. Rendering engine

### 5.1 DECISION: manual `container.Scroll` virtualization (NOT `widget.List`)

**Verdict, grounded in A3.** A3 proved (window.go:460-471 single-predicate deepest-match dispatch + util.go:57-61 deepest-wins hit-test) that a custom interactive child *can* own its input inside a `widget.List` row — so List is not disqualified on input grounds. We still choose **manual `container.Scroll`**, on two independent grounds A3 explicitly endorses:

1. **Single content-space selection/match overlay.** `widget.List` owns and rebuilds `scroller.Content.Objects` every layout pass (list.go:758-763) and boxes each row in a `listItem` with its own background rect (list.go:512-543). A free-text selection that spans mid-row N→mid-row M as one continuous layer must be a **sibling of the rows in content space**; List won't allow that without fighting the layout. With `container.Scroll` we own a content container (`contentLayout`) with three stacked layers.
2. **Horizontal scroll of long unwrapped lines.** `widget.List` is vertical-only. `container.Scroll` with `Direction = container.ScrollBoth` (= `fyne.ScrollBoth`, container/scroll.go:22) gives free horizontal scroll + bar.

Cost accepted: we re-implement the fixed-height visible-window math (transcribed from list.go:413-435) and the recycle pool (a plain `sync.Pool`; List's `internal/async.Pool` is just a wrapper and is unimportable anyway, A3 constraint C). ~120 LOC we fully control.

### 5.2 Object tree

```
PrettyView (BaseWidget; implements input interfaces in §6.1)
└─ prettyViewRenderer (fyne.WidgetRenderer)
   └─ scroll *container.Scroll          // Direction = container.ScrollBoth
      └─ content *fyne.Container        // layout = contentLayout; MinSize = (maxLineRunes*charWidth+pad, totalRows*rowH)
         Objects() (low→high z):
         ├─ selLayer   *fyne.Container   // pooled selection rects (translucent, A=0x40)
         ├─ matchLayer *fyne.Container   // pooled search-match rects
         └─ rowLayer   *fyne.Container   // ~V pooled rowWidgets (the only document text objects)
```

Z-order by slice position — earlier = drawn first = lower (entry.go:1813-1819: "selection rectangles to appear underneath the text"). The content is a plain `*fyne.Container` whose layout manager is `contentLayout`; `contentLayout.MinSize()` returns the **full document extent** (pure arithmetic, never walks children — A1 case (d)), so the scrollbar geometry is correct, while the layers hold only visible children.

### 5.3 ONE coordinate origin (resolves A2 Issues #2, #3, #9)

This is load-bearing and stated once, used everywhere. `container.Scroll` translates its single `Content` child by `Move(-Offset.X, -Offset.Y)` on **both axes** (scroller.go:454, verified). Therefore **all layer children (rows, selection rects, match rects) live in raw content space and subtract NO offset on either axis.** The scroll does the translation.

```
// Content-space conventions (top padding = 0; rows butt against content origin):
rowH      = round(MeasureText("M", textSize, Mono).Height) + rowPad   // integer
charWidth = MeasureText("M", textSize, Mono).Width                    // EXACT font advance, NOT rounded
indentX(depth) = innerPad + depth*indentStep

// Placement (content space, no offset):
row y      = row * rowH
col x      = indentX(depth) + col * charWidth

// Hit-test (viewport pixel -> model), add BOTH offsets to enter content space:
contentY = local.Y + scroll.Offset.Y
contentX = local.X + scroll.Offset.X
row      = floor(contentY / rowH)
col      = round((contentX - indentX(depth)) / charWidth)   // half-glyph rounding; clamp [0, runeLen]
```

`rowH` is **rounded** to a whole pixel; `charWidth` is kept at the font's **exact (possibly fractional) advance** — Fyne draws `canvas.Text` at that natural advance, so rounding the grid cell would let a long run drift past its column and overlap the next segment (and selection rects drift off the glyphs). Keeping it exact holds the grid, the text, and the rects in lockstep on arbitrarily long lines. The `floor(contentY/rowH)` form with top-padding=0 is the single convention used by `reflow`, `hitTest`, and the rect builders — identical, so no off-by-one (A2 Issue #2). Adding `Offset.X` to `contentX` fixes the silent wrong-column copy under horizontal scroll (A2 Issue #3). Rects subtracting no offset fixes the 2× selection drift (A2 Issue #9).

### 5.4 Per-row primitive: N × `canvas.Text` (one per same-color run)

`canvas.Text` is strictly single-color (canvas/text.go:16-31; `DrawString` takes one `color.Color`, font.go:180). TextGrid is rejected (3 objects/cell + one Text per char, textgrid.go:687-699,719). RichText is rejected (no virtualization, O(doc) refresh, richtext.go:617-691). So each visible row renders **one `canvas.Text` per contiguous same-color segment**, at `x = indentX + col*charWidth`. K ≈ 3–10 for JSON, not char count.

```go
type rowWidget struct {
	widget.BaseWidget
	pv   *PrettyView
	line int32 // display-line index this row shows (-1 = unused)
	sub  int32 // wrapped sub-row of `line` (0 unless soft-wrapped)
	// Under soft-wrap the renderer supplies the sub-row's column span; startCol < 0
	// is the WrapNone sentinel (cull to the horizontal visible window instead).
	startCol, endCol int32 // endCol exclusive
	rr               *rowRenderer
}

type rowRenderer struct {
	row      *rowWidget
	guides   []*canvas.Line // pooled indent rules, ≤32, surplus Hidden
	triangle *canvas.Text   // "▶"/"▼"; Hidden unless line is a fold head on sub-row 0
	texts    []*canvas.Text // pooled colored runs; surplus Hidden
	objects  []fyne.CanvasObject
}
```

**`rowRenderer.build()` (recycle hot path, no steady-state alloc):** there is no `VisibleRow` argument — `reflow` sets the row's `(line, sub, startCol, endCol)` fields, then triggers exactly one `build` via `Show` (new row) or `Refresh` (reused row).

1. indent guides: ensure `len==depth` (cap 32), reuse, `Move`/`Resize`/`Show`, `Hide` surplus.
2. triangle: only on a fold head's first visual row (`line.Fold != NoNode && sub == 0`) — set `"▶"`/`"▼"` from the collapsed bit and Show, else Hide.
3. **Horizontal cull (MANDATORY, M-2):** the column window is the sub-row's `[startCol,endCol)` under soft-wrap (drawn from the left edge, `colBase = startCol`), else `[FirstVisibleCol, LastVisibleCol]` of the horizontal viewport. Each `DisplaySeg` is walked **once, never past `lastCol`** (so a multi-MB single-segment line costs O(window), not O(line length)); the intersecting byte slice becomes one `canvas.Text` at `ColX(depth, a-colBase)`, colored `pv.palette[seg.Role]` and sized `width*CharWidth × RowH` **directly** (no `MeasureText` — the grid is uniform). A `2*windowCols` hard cap bounds emitted runes, so every texture ≤ viewport width.
4. trim surplus `texts`→`Hide()`; rebuild `objects` = visible guides + triangle + visible texts; `canvas.Refresh(r.row)`.

Pooling discipline (grow with `append` only when `len<=i`, trim by `Hide()`) mirrors selectable.go:382-385.

### 5.5 Visible-window reflow (transcribed from list.go:413-435)

```go
func (r *prettyViewRenderer) reflow() {
	pv := r.pv
	if pv.doc == nil || pv.met.RowH <= 0 { return }
	pv.viewOffX, pv.viewW = r.scroll.Offset.X, r.scroll.Size().Width
	vpH, m := r.scroll.Size().Height, pv.met

	pv.syncWrap()                            // a resize crossing a column boundary reprojects here
	total := int(pv.doc.TotalVisibleRows())  // O(1)
	if total == 0 { /* clear rows + sel/match rects */ return }

	first := max(0, int(math.Floor(float64(r.scroll.Offset.Y/m.RowH))))
	last  := min(total-1, int(math.Ceil(float64((r.scroll.Offset.Y+vpH)/m.RowH))))

	for idx, rw := range r.live { // recycle rows out of [first,last]
		if idx < first || idx > last { rw.Hide(); rw.line = -1; r.rowPool.Put(rw); delete(r.live, idx) }
	}
	wrapOn := pv.doc.WrapActive()
	for idx := first; idx <= last; idx++ {
		rw, existed := r.live[idx]
		if !existed { rw = r.rowPool.Get().(*rowWidget); r.live[idx] = rw }
		if wrapOn {
			rw.line, rw.sub = pv.doc.LineAndSubRowAtRow(int32(idx)) // O(log n)
			// WrapBreaks computed once per distinct line (cache), sliced by sub →
			rw.startCol, rw.endCol = breaks[sub], breaks[sub+1]
		} else {
			rw.line, rw.sub = pv.doc.LineAtRow(int32(idx)), 0
			rw.startCol, rw.endCol = -1, -1 // WrapNone sentinel: row culls horizontally
		}
		rw.Move(fyne.NewPos(0, float32(idx)*m.RowH))     // CONTENT space, no offset
		if rw.Size() != size { rw.Resize(size) }
		if existed { rw.Refresh() } else { rw.Show() }   // exactly ONE build per row
	}
	r.rowLayer.Objects = r.liveObjects()
	canvas.Refresh(r.rowLayer)                // NOT rowLayer.Refresh (that rebuilds every child again)
	r.rebuildSelection(first, last)           // §6.7
	r.rebuildMatches(first, last)             // §7.5
}
```

Wiring: `scroll.OnScrolled = func(fyne.Position){ r.reflow() }` (scroller.go:495 — fires on wheel/bar/page-tap). `ScrollToOffset` does **not** fire `OnScrolled` (scroller.go:572), so after any programmatic scroll (search reveal, autoscroll) we call `reflow()` explicitly. `Refresh()` recomputes metrics+palette+content size, then `reflow()`. Never `Refresh` from `Layout` (WidgetRenderer contract, widget.go:17-33).

### 5.6 Object-count proof (M-1)

`V = ceil(900/18)+2 ≈ 52`. Per row ≈ 8 guides + 1 triangle + ~8 texts ≈ 17 (cap ~50). Rows ≈ 52×17 ≈ 884. Selection rects ≤ V. Match rects ≤ ~2V. Chrome ≈ 12. **Total ≈ 1,000, worst ≈ 2,800 — independent of document size, selection span, line length, total rows.** The only document-size-dependent storage is the §4 model. ∎

### 5.7 Fold toggle (tap, model-space hit-test, no per-triangle widget)

The root `PrettyView` (not each triangle) handles `Tapped`; it hit-tests in content space. If the tap is inside a foldable row's triangle hot-zone `[indentX-triangleSlot, indentX]`, it calls `doc.fold.toggle(nodeID)` (§4.4), recomputes content size (rows/maxLineRunes changed), then `Refresh()`. `Cursor()` returns `desktop.PointerCursor` over a triangle, else `desktop.TextCursor` (entry.go:248-250 pattern).

---

## 6. Char-level selection (Bruno fidelity)

### 6.1 Interfaces (static asserts, entry.go:28-37 pattern)

```go
var (
	_ fyne.Tappable          = (*PrettyView)(nil) // Tapped           (fold-triangle toggle)
	_ fyne.SecondaryTappable = (*PrettyView)(nil) // TappedSecondary  (right-click context menu)
	_ desktop.Cursorable     = (*PrettyView)(nil) // Cursor           (pointer over triangle / I-beam)
	_ desktop.Mouseable      = (*PrettyView)(nil) // MouseDown/MouseUp (selection press)
	_ desktop.Hoverable      = (*PrettyView)(nil) // MouseIn/Moved/Out (triangle hover tracking)
	_ fyne.Draggable         = (*PrettyView)(nil) // Dragged/DragEnd   (drag-select + edge autoscroll)
	_ fyne.Focusable         = (*PrettyView)(nil) // FocusGained/Lost, TypedKey (keyboard nav)
	_ fyne.Shortcutable      = (*PrettyView)(nil) // TypedShortcut     (Ctrl/Cmd+C / +A / +F)
)
```

(`PrettyView` implements neither `DoubleTappable` nor `desktop.Keyable`:
double/triple-click is timed in `MouseDown`, and keyboard navigation rides on
`fyne.Focusable`'s `TypedKey`.)

### 6.2 State (4 ints + flags, mirroring selectable.go:16-24)

```go
type modelPos struct {
	line int32 // STABLE display-line index (survives fold/unfold)
	col  int   // rune column into the line's DISPLAY text, [0, runeLen]
}

type selection struct {
	anchor, focus modelPos
	active        bool     // there is a non-empty selection
	dragging      bool
	placed        bool     // a caret/anchor has been established by user interaction
	grab          grabMode // grabNone | grabWord | grabLine
	grabA, grabB  modelPos // the word/line originally grabbed (double/triple-drag)
}
```

Endpoints persist as a stable `line` index, so they survive folding with **no stored row** and **no `line→row` map** (A2 Issue #7): the row is recovered on demand by the O(log n) Fenwick prefix query (`rowOfLine`, via `snap`/`ordered`). If an endpoint's line is now hidden it is snapped to the nearest visible ancestor, and `col` is **clamped to that line's runeLen**, at render/copy time.

### 6.3 Hit-test (O(1) monospace, no MeasureText in handlers)

```go
// The widget delegates all pixel<->model mapping to the geometry leaf, which owns
// the single coordinate convention (§5.3): resolve the row from contentY, the
// (line, sub-row) under soft-wrap, then the rune column from contentX on the
// uniform monospace grid — clamping to the line's runeLen and to end-of-document.
// Handlers pass already-content-space coords (Offset.X/Y added by contentPos).
func (pv *PrettyView) hitTest(contentX, contentY float32) modelPos {
	line, col := geometry.HitTest(pv.doc, pv.met, contentX, contentY)
	return modelPos{line: line, col: col}
}
```

`round((x-indentX)/charWidth)` is algebraically identical to `selectable.cursorColAt`'s `pos.X < indentX + col*charWidth + charWidth/2` (selectable.go:190) for uniform `charWidth` — we close-form it, avoiding the O(n) per-prefix MeasureText thrash (R2 §2). The shipped code (`internal/geometry`'s `ColX`/`ColAtX`) computes columns on a **uniform monospace grid only**: one rune = one `CharWidth` cell. **Not implemented (known limitation):** the once-envisaged proportional/CJK/combining-glyph fallback (a per-`Line` `uniformGrid bool` tag plus a lazy cached `prefixW []float32` binary-searched per hit-test, O(log n) per hit-test and one O(n) MeasureText per such line). The target content is ASCII/BMP monospace JSON/XML/HTML, where the uniform grid is exact; wide (CJK), zero-width (combining), and proportional glyphs render and hit-test on the same single-cell advance, so they can mis-align and mis-hit-test. The prefix-width scheme above is the intended escalation if that becomes a goal.

### 6.4 Event wiring

- **MouseDown** sets the anchor authoritatively at the true press position (`hitTest(m.Position)`); detects triple-tap first via `isTripleTap(doubleTappedAtMs, now)` vs `DoubleTapDelay()` (300 ms, selectable.go:413-415); shift extends `focus` keeping `anchor`. Secondary button never starts drag/clears selection.
- **Dragged** — **anchor is NEVER recomputed here** (resolves A2 Issue #1: the first `DragEvent.Dragged` delta is relative to the previous mouse-move sample, not the press point, because `mouseDragPos` updates every move at window.go:417 — so `d.Position.Subtract(d.Dragged)` mis-anchors by up to one sample). We delete that idiom from `selectable.go:84` deliberately. `Dragged` only moves `focus = hitTest(d.Position)`, applies word/line grab extension, then autoscrolls (§6.6).
- **DragEnd** drops empty selections (selectable.go:63-73 analog).
- **Double/triple-click** is detected inside `MouseDown` by timing successive presses (a `clickCount` within `multiClickWindow` ≈ 300 ms), **not** via a `DoubleTappable` handler: 2 = word-select (token-aware, §6.5), 3 = whole-line. A word/line grab is remembered (`grabA`/`grabB`) so a subsequent drag extends by word/line.
- **Right-click** (`TappedSecondary`) opens the context menu (Copy / Select all) and never disturbs the selection.
- **Cursor/Focus/Shift**: `TextCursor` over text, `PointerCursor` over triangles (Hoverable-driven on `MouseMoved`); selection drawn only when focused; shift-extend reads the **press event's modifier** (`ev.Modifier & fyne.KeyModifierShift`) in `MouseDown`, not a separate key handler (the widget implements no `desktop.Keyable`).
- **Shortcuts**: handled directly by the widget's `TypedShortcut` (`fyne.Shortcutable`) — `*fyne.ShortcutCopy`→`CopySelection`, `*fyne.ShortcutSelectAll`→`SelectAll`, and a `*desktop.CustomShortcut` for `Ctrl/Cmd+F`→`onSearchRequested`. The `Ctrl/Cmd+F` accelerator is also registered on the window canvas by `registerFindShortcut` (controls.go).

### 6.5 Word/line bounds (token-aware)

Default mirrors `getTextWhitespaceRegion`/`isWordSeparator` (entry.go:1924-1987), but a **token-aware override** consults the line's color-run metadata: double-click inside a token-run selects the whole run `[runStart,runEnd]` (so `"quoted string"` or `-12.5e3` selects as a unit). **Guard (A2 lower-severity):** synthetic summary rows (`{ 6 items }`) have no token runs — if `line.runs == nil` fall back to the whitespace-class heuristic. Triple-click selects the whole row (`col 0 … runeLen`, mirrors `selectCurrentRow`, selectable.go:218-225).

### 6.6 Autoscroll while dragging past edge (resolves A2 Issue #8 — data race)

Edge autoscroll is a cursor-driven nudge (`ScrollToOffset(Offset.Add(move))`) computed inside `Dragged` and followed by `reflow()`. **It is not shipped as the originally-designed `time.Ticker`.** Because `Dragged` fires only on pointer motion (window.go:411-419), dragging to the viewport edge and then **holding the pointer stationary stops the scroll** — a known limitation (backlogged; the ticker would close it). The upside: all autoscroll reads/writes happen inside the `Dragged` handler on the Fyne goroutine, so there is no off-thread clock and no data race (cf. R-13).

### 6.7 Selection rectangles (visible-window only — resolves A1 Break #3 / A2 Issue #9)

```go
func (r *prettyViewRenderer) rebuildSelection(first, last int) {
	a, b, ok := r.pv.ordered() // snapped endpoints in document order; ok == non-empty
	if !ok { r.applyRects(r.selLayer, &r.selRects, &r.selObjs, 0); return }
	m, wrapOn := r.pv.met, r.pv.doc.WrapActive()
	n := 0
	for row := first; row <= last; row++ {
		// Map this VISUAL ROW to its logical line (and sub-row under wrap). The
		// selection is (line, col) with no row field, so we go row -> line, never the
		// reverse; lines outside [a.line, b.line] are skipped.
		li := int32(row); var sub int32
		if wrapOn { li, sub = r.pv.doc.LineAndSubRowAtRow(int32(row)) } else { li = r.pv.doc.LineAtRow(int32(row)) }
		if li < a.line || li > b.line { continue }
		depth, runeLen := r.pv.doc.Lines[li].Depth, r.pv.doc.LineRuneLen(li)
		selS, selE := 0, runeLen
		if li == a.line { selS = clampInt(a.col, 0, runeLen) }
		if li == b.line { selE = clampInt(b.col, 0, runeLen) }
		// Intersect with this visual row's displayed-column window [w0,w1) (the whole
		// line at base 0 under WrapNone). A selected line break "bleeds" trailing width.
		w0, w1, colBase := r.subSpan(li, sub, runeLen, wrapOn, &breaks, &breaksLine)
		lo, hi := max(selS, w0), min(selE, w1)
		n = r.placeSpanRect(&r.selRects, n, m, depth, lo, max(hi, lo), colBase, row, r.pv.selColor, bleed)
	}
	r.applyRects(r.selLayer, &r.selRects, &r.selObjs, n) // hide surplus, publish first n (≤ V, M-1)
}
```

**Do NOT copy `buildSelection`'s `rowCount = selectEndRow - selectStartRow + 1` loop (selectable.go:373-385)** — that builds one rect per selected row across the whole span (O(span)). We iterate the **intersection with `[first,last]`**, so ≤ V rects for any selection (M-1). `FillColor = th.Color(theme.ColorNameSelection, v)` (translucent A=0x40, R5 §3, shows text through).

### 6.8 Copy (model-based, source-byte accurate)

`selectedText()` walks the visible lines of the span and appends each whole line's **displayed bytes** via `AppendDisplayLine` (rewriting tab pads back to `\t` for raw docs, §4.3), rune-slicing only a partial endpoint, joined with `\n`. `SelectedText()` slices model bytes, never reads a `CanvasObject` (selectable.go:120-131 analog). `CopySelection` → `fyne.CurrentApp().Clipboard().SetContent(txt)` (app-level clipboard, app.go:88; `Window.Clipboard()` is deprecated, window.go:104). **Folded-region semantics:** default WYSIWYG — a collapsed node contributes its summary string. `CopySubtree(byteOffset)` re-serializes the node's full `[id, id+subtree[id])` source range regardless of fold. **Copy-after-collapse contract (A2 Issue #7):** if a node inside an active selection is collapsed, copy then returns the summary for that node, not the hidden children — asserted by a test.

---

## 7. Search / seek

### 7.1 Types (fold-stable coordinates)

```go
type SearchMode uint8
const ( SearchPlain SearchMode = iota; SearchRegex )

type SearchQuery struct {
	Text          string
	Mode          SearchMode
	CaseSensitive bool
}

// Match is in MODEL coordinates: a stable display-line index + rune columns into
// the line's expanded display text. Keying by LINE (not visible row) makes matches
// survive fold/unfold; the visible row is an O(log n) Fenwick lookup at draw time.
type Match struct { Line int32; ColStart, ColEnd int }

// Search state is INTERNAL — there is no exported SearchResult. byLine indexes the
// matches falling on each line for O(1) per-row highlight intersection.
type searchState struct {
	query   SearchQuery
	matches []Match         // ordered by (line, then ColStart)
	byLine  map[int32][]int // line -> indices into matches
	active  int             // -1 if none
	capped  bool
	err     error
}

type SearchConfig struct {
	MaxMatches  int           // default 10_000
	DebounceFor time.Duration // default 150ms
	MinQueryLen int           // default 1
}
```

Keying by **line index** (not visible row) makes matches survive fold/unfold; `match→visibleRow` is the O(log n) Fenwick lookup, recomputed per projection change. This is the single most important search decision.

### 7.2 Scan (RE2, single-pass byte→rune — resolves A2 Issue #5)

`regexp` (RE2) is linear-time and safe for live-typed patterns; case-insensitive uses inline `(?i)`. Plain mode: lower-case fast path only when the line is pure-ASCII; if the line has any byte ≥ 0x80, do a **rune-wise `unicode.ToLower` fold** (the `len(ToLower)!=len` guard alone is unsound — e.g. U+212A). **Byte→rune offsets are converted in ONE forward pass per line** maintaining `(bytesConsumed, runesConsumed)` — O(L), never O(K·L) (A2 Issue #5: per-match `utf8.RuneCountInString(s[:b])` is quadratic on a long single-line minified doc). *(Shipped: `colCursor` in search.go.)* Search ignores synthetic summary text; it scans real `LineText`.

### 7.3 Threading & supersession

**Shipped: the scan is synchronous on the Fyne goroutine, not a worker.** With the single-pass byte→rune conversion (§7.2) and a hard `MaxMatches` cap, a full scan is O(total bytes) and stays under a frame — ~5 ms over the 7.5 MB / 440k-line fixture — so keystroke debouncing (`searchDebounced`, 150 ms, via `time.AfterFunc`+`fyne.Do`) is enough to keep typing smooth. A `searchGen` counter gives last-query-wins: a debounce callback that fired before a newer keystroke / `ClearSearch` / `SetData` checks the generation and skips itself.

The originally-designed **off-thread chunked scan** (a worker producing `ChunkBytes` slices, publishing snapshots via `fyne.Do`, with a `gen`+cancel channel) is **not implemented and not needed** at the in-memory sizes this viewer targets — and keeping the scan on the Fyne goroutine is what makes search/​reflow access to `pv.search` race-free without a mutex (running it off-thread would reintroduce that race). The ceiling is a single multi-gigabyte document, which is out of scope. **Search reads only immutable arenas** (`Src`, `Nodes`, `Segs`); only `foldIndex` and the search state mutate, always on the Fyne goroutine.

### 7.4 Navigation & reveal (resolves A2 Issue #6)

`Next`/`Prev` are index arithmetic with **unconditional** wrap (`((active+dir)%n+n)%n`, in `step`). The scan is synchronous and complete before navigation, so there is no `Complete`/"searching…" partial state. Count label `"3/27"` (or `"3/10000+"` when capped).

**`revealActive`:** (1) `RevealLine(line)` — unfold each collapsed ancestor (outermost-first, `unfoldAncestors`); (2) `row := rowOfLine(line)`; (3) center: `y := clamp(row*rowH - (vpH-rowH)/2, 0, maxOffsetY)`; (4) `scroll.ScrollToOffset(NewPos(matchX, y))` then `reflow()` (ScrollToOffset doesn't fire OnScrolled, scroller.go:572). **Order is load-bearing: expand → recompute total → resolve row → scroll.** **Stay on the fixed-height fast path** (never `SetItemHeight`-equivalent) so offset math is O(1). Because the scan is synchronous (no streamed chunks), there is no `userHasInteracted` gate and no separate scroll debounce: `revealActive` runs right after each `Search`/`SearchNext`/`SearchPrev`. Keystroke coalescing happens upstream, at the debounced scan (`searchDebounced`, §7.3).

### 7.5 Highlight (reuses §6.7 mechanism, separate pools)

Per visible row, resolve the row's line and intersect that line's matches via `search.byLine[line]` (a `map[int32][]int` built once per scan for O(1) lookup). Z-order low→high: `selection → other-match → active-match → text`. The active match fills with `pv.activeMatchColor` (from `Theme.ActiveMatch`, strong orange), others with `pv.matchColor` (`Theme.Match`, soft yellow) — both translucent, applied directly in `placeSpanRect`. Same pooled-rect, visible-window-only discipline as §6.7 (≤ V rects, M-1).

---

## 8. Theming / colors

Colors live in a plain `Theme` struct (theme.go), **not** as custom `fyne.ThemeColorName` registrations (the originally-designed `prettyview.*` color names and "wrapping theme" were not shipped). `Theme` carries the nine syntax-token colors plus the structural/UI colors; `SyntaxColors` is the token-only subset.

```go
type Theme struct {
	Key, String, Number, Bool, Null, Punct, Tag, Attr, Comment color.Color // syntax tokens
	Foreground, Summary, IndentGuide, Selection, Match, ActiveMatch color.Color // structural / UI
}
```

Each segment stores a 1-byte `model.ColorRole`. `recomputeMetrics` (renderer.go) resolves the effective theme for the active variant — `defaultTheme(variant)` with any per-variant `WithTheme`/`SetTheme` override merged in — then builds `pv.palette = theme.palette()`, a `[]color.Color` indexed by `ColorRole`, **once per metrics rebuild** (i.e. once per `Refresh` when the text size or variant changed). The structural defaults are pulled from the host Fyne theme via `themeColor(name, variant)` using **builtin** names (`ColorNameForeground` for plain/punct fallback, `ColorNameDisabled` for the summary, `ColorNameSelection` for the selection fill), so an un-themed viewer tracks the app. The syntax tokens default to a Bruno-ish, VS Code Dark+/Light palette (dark: key `#9CDCFE`, string `#CE9178`, number `#B5CEA8`, bool/null/tag `#569CD6`, attr `#9CDCFE`, comment `#6A9955`, punct `#D4D4D4`; the light variant uses darker equivalents). `Match` (soft yellow) and `ActiveMatch` (strong orange) are literals. The monospace style is `fyne.TextStyle{Monospace:true}`; the variant is read via `app.Settings().ThemeVariant()`, so a dark/light toggle recolors on the next `Refresh`.

---

## 9. Threading & large-file handling

- **Parse synchronously on the Fyne goroutine.** `SetData` calls `parse.Parse(src, …)` directly (prettyview.go), then `ClearSearch`/`ClearSelection`/`Refresh` — no worker, no model swap. The compact model (~5× source) builds fast enough that a multi-MB document parses within a frame or two. `PrettyView` is **not** safe for concurrent use, so a caller loading data from another goroutine marshals the whole `SetData` with `fyne.Do`. `src` is retained zero-copy.
- **Search** as §7.3 (synchronous on the Fyne goroutine, debounced, `searchGen` last-query-wins).
- **Drag autoscroll** as §6.6 — a cursor-driven nudge inside the `Dragged` handler on the Fyne goroutine, **not** a ticker, so there is no off-thread clock and no data race.
- **Invariant:** post-parse arenas (`Src`, `Nodes`, `Lines`, `Segs`, `Aux`, the `lineRunes` extent cache) are read-only; only `foldIndex` (visibility + wrap weights) and selection/search state mutate, always on the Fyne goroutine.

---

## 10. Risks & mitigations (every adversary finding carried forward)

| # | Finding (source) | Severity | Resolution adopted |
|---|---|---|---|
| R-1 | **Single long-line `canvas.Text` → ~1 GB bitmap** (`texture.go:171-173` sizes texture to full `MinSize().Width`) — A1 Break #1 | Blocker | **Mandatory per-row horizontal culling (M-2):** emit only the visible column sub-range; hard-cap text at `2·viewportCols`. §5.4. |
| R-2 | **Content-keyed text-texture cache, 60 s expiry** (`cache/base.go:9`) → content-proportional transient GPU/heap on scroll — A1 Break #2 | High | Narrow textures (R-1) cap per-entry bytes; memory test scrolls all of `big.json` and asserts a heap ceiling (§11 M11). Optionally shorten `FYNE_CACHE`. Documented. |
| R-3 | **Copying `selectable.buildSelection` span loop → O(span) rects** (selectable.go:373-385) — A1 Break #3 | High | Intersect selection/match with visible window first → ≤ V rects. §6.7. Test asserts rect count ≤ V on select-all of `big.json`. |
| R-4 | `[]rune` per model line → 4× source blow-up — A1 smaller finding | Med | Lines stored as byte-offset segments; `[]rune` materialized **only per visible row**, never for the whole doc. M-3, §6.3. |
| R-5 | `json/xml.Decoder.Token` fallback allocates per-token strings (≈ whole file) — A1 | Med | JSON path is a hand-written zero-copy byte scanner; Token is reference-only. §4.6. |
| R-6 | **Wrong drag anchor** from `d.Position.Subtract(d.Dragged)` (delta is vs previous sample, window.go:417) — A2 Issue #1 | Med | Anchor set authoritatively in `MouseDown`; never recomputed in `Dragged`. §6.4. |
| R-7 | **Hit-test off-by-one** (missing origin term) — A2 Issue #2 | Med | One origin convention (top-pad 0, integer `rowH`, `floor(contentY/rowH)`) across reflow/hitTest/rects. §5.3. Golden round-trip test. |
| R-8 | **Wrong columns copied under horizontal scroll** (Offset.X dropped) — A2 Issue #3 | Blocker (data corruption) | `contentX = local.X + Offset.X` in hit-test. §5.3/§6.3. |
| R-9 | **Tabs → clipboard ≠ source** — A2 Issue #4 | Med | **Resolved (no colMap):** raw lines expand tabs to interned space pads; copy rewrites each pad back to a real `\t` via `AppendDisplayLine(restoreTabs)`. §4.3. Test: `TestSelectedTextRawTabsRoundTrip`. |
| R-10 | **O(K·L) byte→rune in search** — A2 Issue #5 | High (freeze) | **Resolved:** single forward pass per line (`colCursor`, §7.2); ~5 ms full scan of the 7.5 MB fixture, so the synchronous scan needs no chunking (§7.3). |
| R-11 | **Reveal frame-drops + mid-scan viewport yank** — A2 Issue #6 | High | Fixed-height fast path; batched ancestor expand; debounced reveal scroll; auto-reveal only on user intent. §7.4. |
| R-12 | **`lineID→row` map → O(n) rebuild per fold; "O(log n) fold" overclaim** — A2 Issue #7, D1 open risk | High | the stable `line` index *is* the id; `rowOfLine` is an O(log n) Fenwick prefix (no map). Fold honestly O(k visible descendants) with O(log n) row delta; `hiddenBy` array keeps lookups O(log n). §4.4/§6.2. |
| R-13 | **Autoscroll ticker data race** (reads UI fields off-thread) — A2 Issue #8 | n/a | **No ticker shipped.** Drag-edge autoscroll runs inside `Dragged` (pointer-motion only), entirely on the Fyne goroutine, so there is no off-thread race to begin with. Held-stationary edge autoscroll (which a ticker would add) is a known limitation on the backlog, not shipped. §6.6. |
| R-14 | **Selection rects drift 2×** (subtracting Offset on a scrolled-content child; scroller.go:454 already translates both axes) — A2 Issue #9 | Blocker (visible) | Rects in raw content space, **no** offset subtraction either axis. §5.3/§6.7. Round-trip test. |
| R-15 | Fractional `charWidth` drift on long lines — A2 minor | Low | Keep `charWidth` at the font's **exact** advance so the grid matches the rendered text (rounding it is what *causes* the drift — a long run overruns its column and overlaps the next segment). `rowH` is still rounded. §5.3. |
| R-16 | Next-wrap during incomplete scan jumps backward — A2 minor | Low | While `!Complete`, clamp at last known match, show "searching…". §7.4. |
| R-17 | Double-click on summary row indexes nil run slice — A2 minor | Low | Guard: `if line.runs==nil` use whitespace heuristic. §6.5. |
| R-18 | **`internal/...` packages unimportable** (`go vet`) — A3 constraint C | Blocker (build) | Use `sync.Pool`, `container.Scroll`, `fyne.MeasureText` — never any `fyne.io/fyne/v2/internal/...`. §2, §5.1. |
| R-19 | Deep fully-expanded tree min-size cost — A1 case (d) | Low | `contentLayout.MinSize()` is pure arithmetic, never walks children; indent guides capped at 32. §5.2/§5.4. |

---

## 11. Build plan (ordered, milestone-based, each independently testable)

Each milestone ends with green tests and (from M7) a runnable demo. `go test ./...` and `go vet ./...` (the latter enforces R-18) must pass at every milestone.

> **Historical.** This is the original ordered plan; the file and helper names below reflect the *intended* milestones, not the shipped layout (see §2 for the actual package tree — e.g. the planned `contentbox.go`/`pool.go` were folded into `renderer.go`/`row.go`, and the model/parse/geometry code lives under `internal/`).

**M0 — Repo skeleton.**
`go.mod` (`module github.com/ideaconnect/go-fyne-pretty-view`, `go 1.26`, `require fyne.io/fyne/v2 v2.7.4`, `golang.org/x/net`). Empty `prettyview.go` with `PrettyView` embedding `widget.BaseWidget` + a stub `CreateRenderer` returning `widget.NewSimpleRenderer(canvas.NewRectangle(...))` (widget.go:203). Add testdata fixtures incl. `tabs.json`. *Test:* `go build ./...`, `go vet ./...` clean.

**M1 — Model arenas + JSON scanner + summaries.**
`model.go`, `builder.go`, `parse.go`, `parse_json.go`, `summary.go`. Hand-written zero-copy JSON/JSONC scanner → SoA. *Tests (model_test.go):* node counts for `small.json`; segment roles correct; `unsafe.Sizeof(Node)==32`; **zero-copy assertion** (segment byte ranges are sub-slices of `Src`, no per-node `[]byte` alloc — check `&Src[seg.Start]` aliasing); summary strings (`{ }`/`{ 1 item }`/`{ 6 items }`); `colMap` identity for tab-free lines and correct for `tabs.json`.

**M2 — Fold index / projection.**
`foldindex.go` (fenwick + post-order subtree pass + `hiddenBy`). *Tests (foldindex_test.go):* `TotalVisibleRows` O(1) correctness; `nodeAtRow`/`rowOfNode` round-trip for every visible row; fold/unfold changes counts correctly; `ExpandAll`/`CollapseAll`; complexity probe (collapse near top of a synthetic 100k-node doc updates total in one prefix query; lookups stay O(log n)). Assert no `[]rune`/string alloc per fold (`testing.AllocsPerRun`).

**M3 — XML / HTML / Raw parsers.**
`parse_xml.go`, `parse_html.go`, `parse_raw.go`, `AutoDetect`. *Tests:* `catalog.xml`/`page.html` node mapping (elements/attrs/text/comments, void elements, tolerant unclosed tags); raw line count; AutoDetect picks the right format for each fixture.

**M4 — Geometry + hit-test math (pure, no widgets).**
`geometry.go`. Exact `charWidth` + rounded `rowH`; the one origin convention; `hitTest` and col↔x. *Tests (geometry_test.go):* **golden round-trip** — for rows {0,1,deep} at non-trivial `Offset`, `hitTest(rectScreenPos(row,col)) == (row,col)`; off-by-one guards at row top edge and `+rowH-0.5`; a fractional `charWidth` keeps a long run aligned to its column (no segment overlap); `contentX` includes `Offset.X`.

**M5 — Renderer + contentBox + row widget (read-only display).**
`renderer.go`, `contentbox.go`, `row.go`, `pool.go`. `container.Scroll` (ScrollBoth) + manual `reflow`; per-row culled `canvas.Text`; indent guides; fold triangle; `OnScrolled→reflow`. *Tests:* with a synthetic 6k-row model and a fixed viewport, after `reflow` the live row count ≤ V; `contentBox.MinSize()` equals `(maxLineRunes*charWidth+pad, total*rowH)` and allocates 0 per call.

**M6 — Fold toggle via tap.**
Root `Tapped`/`Cursor` model-space hit-test; triangle hot-zone; `toggle`→`Refresh`. *Tests:* simulate a `PointEvent` at a triangle, assert the projection total changes and selection (if any) re-resolves.

**M7 — Demo app (first runnable).**
`cmd/prettyview-demo/main.go`: load a testdata file, format toggle, ExpandAll/CollapseAll buttons. *Manual:* loads `small.json` then `openapi.json` (478 KB), folds work, scroll is smooth.

**M8 — Selection + copy.**
`selection.go`, `selection_words.go`, input interfaces in `widget_input.go`. MouseDown anchor, Dragged focus (no re-anchor), DragEnd, double/triple-click, shift-extend, autoscroll ticker, `rebuildSelection`, `selectedText`, `CopySelection`/`CopySubtree`, `SelectAll`. *Tests (selection_test.go):* normalize/swap; single-row and multi-row copy substring exact; **tabs round-trip** (clipboard contains `\t` from `tabs.json`); **copy-after-collapse** returns summary (R-12); shift-extend; word/line bounds incl. summary-row nil-run guard. *Race:* `go test -race` exercising the autoscroll ticker (R-13).

**M9 — Memory / object-count assertions.**
`memory_test.go`. *Tests:* load `big.json` (7.5 MB); assert live `CanvasObject` count after `reflow` ≤ M-1 bound; **scroll the entire document** in steps and assert a heap ceiling well under 1 GB (R-2 — `runtime.ReadMemStats`, with a settle/GC between samples); **select-all** and assert selection-rect count ≤ V (R-3); a single 2 MB minified line asserts no `canvas.Text` wider than viewport and bounded heap (R-1). Also assert model size for the 151 KB fixture ≈ 5× (M-3).

**M10 — Search + reveal.**
`search.go`, theme `ColorNameMatchHighlight`. Synchronous RE2 scan, single-pass byte→rune (`colCursor`), `searchGen` supersession, debounce, `revealActive` (batched expand, centered scroll, user-intent gating), Next/Prev wrap rules, highlight pools, Ctrl+F focus. *Tests (search_test.go):* plain + regex matches with correct rune offsets incl. a multibyte fixture; non-overlapping; reveal expands ancestors and `rowOfNode` resolves; nav wrap; cap → `Capped`; bad regex → `Err`; debounce timer lifecycle + generation supersession.

**M11 — Theming, options, polish, final demo.**
`theme.go`, `options.go`. Wrapping theme + palette rebuild on `Refresh`; dark/light variant; functional options (`WithFormat`/`WithWrap`/`WithSearchConfig`/`WithDefaultCollapseDepth`/`WithSyntaxColors`/...). Demo gains a search bar with `3/27` count, format auto-detect, and loads a **151 KB JSON** end-to-end. *Manual acceptance:* 151 KB JSON loads; fold/copy/select/search all work; dark↔light recolors; `go test -race ./...` and `go vet ./...` green.

### Test strategy summary

- **Pure-model unit tests** (no Fyne canvas needed): M1–M4, M8 (copy), M10 (scan offsets/reveal) — run headless, fast, deterministic.
- **Geometry golden tests** (M4): the single most important correctness guard — `hitTest`↔`rectScreenPos` round-trip through `Offset` on both axes (catches R-7, R-8, R-14 in one test).
- **Memory/object-count test** (M9): the headline-constraint guard — must **scroll and select-all the whole `big.json`**, asserting both an object-count bound *and* a heap ceiling, because an object-count-only test passes while the texture-cache leak (R-2) and long-line texture (R-1) silently breach 1 GB.
- **Race test** (M8/M11): `go test -race` over selection drag + autoscroll ticker (R-13).
- **`go vet ./...` in CI** at every milestone enforces the no-`internal/`-imports constraint (R-18).

---

Relevant Fyne source files this design is grounded on (absolute paths):
- `fyne.io/fyne/v2@v2.7.4/widget/list.go` (fast-path window math 413-435; recycle pool 649-754; Content.Objects rebuild 758-763)
- `fyne.io/fyne/v2@v2.7.4/internal/widget/scroller.go` (Offset 490, OnScrolled 495, ScrollToOffset 572 no-OnScrolled, both-axes Content.Move 454, canvas.Refresh idiom 477)
- `fyne.io/fyne/v2@v2.7.4/widget/selectable.go` (state 16-24, getRowCol 197-215, selection/normalize 235-263, SelectedText 120-131, buildSelection pooled rects 329-405, isTripleTap 413-415)
- `fyne.io/fyne/v2@v2.7.4/widget/entry.go` (z-order under text 1813-1819, TextCursor 248-250, shift/word/autoscroll 346-372/1852-1922/1924-1987, shortcuts 1042-1135, disabled-Copy routing context)
- `fyne.io/fyne/v2@v2.7.4/internal/painter/gl/texture.go` (full-line-width text bitmap 171-173 — drives M-2)
- `fyne.io/fyne/v2@v2.7.4/internal/cache/base.go` (content-keyed text textures, 60 s ValidDuration 9 — drives R-2)
- `fyne.io/fyne/v2@v2.7.4/internal/driver/glfw/window.go` (drag threshold + incremental Dragged delta + mouseDragPos per-move 405-424; deepest-match dispatch 460-471)
- `fyne.io/fyne/v2@v2.7.4/widget/textgrid.go` (rounded monospace cell size 646-649)
- `fyne.io/fyne/v2@v2.7.4/canvas/text.go` (single-color Text 16-31), `text.go` (MeasureText 71), `thread.go` (Do/DoAndWait 8/18), `clipboard.go` (4-9), `app.go` (Clipboard 88), `theme.go` (Color 28-33), `widget.go` (WidgetRenderer 17-33; NewSimpleRenderer 203)