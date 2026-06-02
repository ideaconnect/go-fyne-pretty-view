All facts confirmed. Here is the authoritative architecture and build plan.

---

# prettyview — Authoritative Architecture & Build Plan

**Module:** `github.com/ideaconnect/go-fyne-pretty-view` · **Fyne:** v2.7.4 · **Go:** 1.26 · **Status:** design-frozen, implementation-ready

A reusable Go+Fyne widget: a Bruno-like structured-data viewer for JSON / JSONC / XML / HTML / raw text with syntax highlighting, per-node expand/fold with collapse summaries, copy-subtree, true character-level free-text drag selection across rows, and incremental search with reveal-into-folds. Built for hard memory bounds via row virtualization.

All Fyne citations below are verified against `/home/bartosz/go/pkg/mod/fyne.io/fyne/v2@v2.7.4/`.

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

Single module, one public package `prettyview`, with small unexported helper files. No `internal/` Fyne imports anywhere (A3 constraint C: `go vet` rejects `fyne.io/fyne/v2/internal/...`; we use `sync.Pool`, `container.Scroll`, `fyne.MeasureText` instead).

```
github.com/ideaconnect/go-fyne-pretty-view/
├── go.mod                       // module path, Go 1.26, require fyne.io/fyne/v2 v2.7.4, golang.org/x/net
│
├── prettyview.go                // PrettyView widget: BaseWidget, CreateRenderer, public ctor + options
├── widget_input.go              // input interfaces: Mouseable/Draggable/DoubleTappable/Cursorable/Focusable/Keyable/Shortcutable
├── renderer.go                  // prettyViewRenderer: container.Scroll + manual visible-window virtualization
├── contentbox.go                // contentBox: sized spacer + 3 layers (sel/match/rows); MinSize=full doc extent
├── row.go                       // rowWidget + rowRenderer: per-row colored canvas.Text runs, indent guides, fold triangle
├── geometry.go                  // metrics: charWidth/rowHeight (rounded), col<->x, pixel<->(row,col) hit-test, ONE origin convention
├── pool.go                      // sync.Pool wrappers for rowWidget and *canvas.Rectangle
│
├── model.go                     // Document SoA: Node, Segment, ColorRole, arenas; LineText/source-byte mapping
├── foldindex.go                 // fenwick + foldIndex: visible-row projection, nodeAtRow, rowOfNode, fold/unfold
├── builder.go                   // Builder: parser-facing arena construction (Open/AddLeaf/Close), subtree-size pass
├── parse.go                     // Parser interface, AutoDetect, Format
├── parse_json.go                // hand-written zero-copy JSON/JSONC byte scanner
├── parse_xml.go                 // XML via encoding/xml.Decoder.Token
├── parse_html.go                // HTML via golang.org/x/net/html tokenizer (tolerant)
├── parse_raw.go                 // raw: split on \n, one KindRawLine per physical line (fallback)
├── summary.go                   // fold-summary text generation ("{ N items }", "[ N items ]")
│
├── selection.go                 // Selection state (anchor/focus Pos), hit-test wiring, normalize, copy, selectAll
├── selection_words.go           // token-aware word/line bounds for double/triple-click
├── search.go                    // SearchController, SearchQuery/Result/Match, matcher, chunked scan, nav, reveal
│
├── theme.go                     // syntax ColorRole -> theme color names; wrapping theme; palette build
├── options.go                   // Option funcs (functional options): WithFormat, WithWrap, WithSearchConfig, ...
│
├── testdata/                    // small.json, openapi.json (~478KB), catalog.xml, page.html, big.json (~7.5MB)
│
├── internal_math_test.go        // geometry round-trip, hit-test off-by-one, charWidth integer
├── foldindex_test.go            // projection complexity + correctness
├── model_test.go                // parser -> node counts, segments, summaries, zero-copy assertions
├── selection_test.go            // selection math, copy substring, tabs round-trip, copy-after-collapse
├── search_test.go               // scan offsets (byte->rune), reveal-ancestors, nav wrap
├── memory_test.go               // OBJECT-COUNT + heap-ceiling assertions (scroll + select-all on big.json)
│
└── cmd/prettyview-demo/
    └── main.go                  // demo app: file picker / load testdata, format toggle, search bar, ExpandAll/CollapseAll
```

---

## 3. Public API

All real Fyne signatures. The widget is **read-only** (a viewer); it exposes `Disabled()==true` and a `SelectedText() string`, mirroring how the glfw driver auto-routes only the Copy shortcut to a disabled widget exposing `SelectedText` (window.go:830-836; R3 §8).

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

// SetData parses src under format (FormatAuto detects). Parsing runs off the Fyne
// goroutine; the model is swapped and the view refreshed via fyne.Do when done.
// Safe to call from any goroutine.
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

// SelectedText returns the exact source substring currently selected, or "".
// Also the method the disabled-widget Copy shortcut path reads.
func (pv *PrettyView) SelectedText() string

func (pv *PrettyView) SelectAll()
func (pv *PrettyView) ClearSelection()

// CopySelection copies SelectedText() to the clipboard (no-op if empty).
func (pv *PrettyView) CopySelection()

// CopySubtree copies the serialized source bytes of the node under byteOffset
// (the whole {…}/[…]/<tag>…</tag> span), regardless of fold state.
func (pv *PrettyView) CopySubtree(byteOffset int)

// Disabled always reports true (read-only viewer); enables Copy-only shortcut routing.
func (pv *PrettyView) Disabled() bool

// --- Search ---

// Search starts/replaces an incremental search (debounced, synchronous, capped).
func (pv *PrettyView) Search(q SearchQuery)
func (pv *PrettyView) SearchNext()
func (pv *PrettyView) SearchPrev()
func (pv *PrettyView) ClearSearch()

// SearchStatus returns (active 1-based index, total, capped) for a count label "3/27" / "3/10000+".
func (pv *PrettyView) SearchStatus() (active, total int, capped bool)

// --- Theming hook ---

// SetSyntaxColors overrides the syntax palette for a given variant.
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
func WithSyntaxColors(v fyne.ThemeVariant, c SyntaxColors) Option
```

`CreateRenderer`, `MinSize`, `Refresh` come from embedded `widget.BaseWidget` (widget.go:71,119,134). The widget registers shortcuts (Copy, SelectAll, Ctrl+F) through an embedded `fyne.ShortcutHandler` in `ExtendBaseWidget` (pattern at entry.go:298-301,1042-1135; `fyne.ShortcutHandler` at shortcut.go:5-30).

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
	Src    []byte    // original bytes, retained once for zero-copy segments
	Aux    []byte    // synthesized text: summaries, punctuation, tab-expansion spaces
	Nodes  []Node    // structural arena; Nodes[0] == synthetic root
	Lines  []Line    // display arena (the projection/render unit)
	Segs   []Segment // segment arena
	Format Format

	lineRunes []int32    // per-line expanded rune-count cache (extent)
	fold      *foldIndex // visible-row projection over Lines (§4.4)
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
	vis       []int32  // per line: 1 if visible, else 0
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

### 4.5 Parser interface and Builder

The parser never builds a tree; it drives a `Builder` that appends into arenas.

```go
type Parser interface {
	Format() Format
	Detect(src []byte) int          // 0..100 confidence for AutoDetect
	Parse(src []byte, b *Builder) error // tolerant; emits partial structure on malformed input
}

type Builder struct { /* doc *Document; stack []NodeID; unexported */ }

func (b *Builder) OpenContainer(k Kind, head []Seg) NodeID // foldable; head segs e.g. `"key": {` or `<product id="0">`
func (b *Builder) AddLeaf(k Kind, segs []Seg) NodeID       // single-line node
func (b *Builder) CloseContainer() NodeID                  // computes ChildCount, caches summary, pops stack

type Seg struct {
	Role  ColorRole
	Lit   string // if non-empty, interned into Aux (synthesized)
	Start uint32 // else byte range into Src (zero-copy)
	End   uint32
}
```

`OpenContainer`/`CloseContainer` keep an explicit `stack []NodeID` so children land contiguously after the parent's slot; `CloseContainer` records `FirstChild`/`ChildCount`. A post-order pass after parse fills `foldIndex.subtree[]` and `ownRows[]` (one O(n) walk).

`AutoDetect(src)` runs each `Detect` and picks the max: leading `{`/`[` → JSON; `<?xml`/leading `<root>` → XML; `<!doctype html`/`<html` → HTML; else Raw.

### 4.6 The four parsers

- **JSON / JSONC** (`parse_json.go`): a **hand-written byte scanner** over `Src` yielding tokens with `(start,end)` offsets — zero-copy, preserves key order and comment positions for Bruno fidelity. `encoding/json.Decoder.Token` is the reference but is **not** the path: it allocates per-token strings and loses offsets (A1: that re-allocates ≈ the whole file). Mapping: `{…}`→`KindObject`, `[…]`→`KindArray`, `"key": scalar`→`KindKeyValue`, bare scalar→`KindScalar`, value-container carries its `"key":` prefix in the container's head segs (saves a node/member). Closing brace is rendered as a synthetic continuation row of the container (counted in `ExtraRows`), not a separate node.
- **XML** (`parse_xml.go`): `encoding/xml.Decoder.Token` (present in Go 1.26 stdlib). `StartElement`+children→`KindElement` with attributes inline in head segs; self-closing/empty→`KindEmptyElement`; `CharData`→`KindText` (entity-decoded into `Aux`); `Comment`→`KindComment`. Token offsets aren't exposed by `xml.Decoder`, so for XML/HTML the head/text segments use **interned `Aux` literals**, not zero-copy `Src` ranges — copy uses the display text directly (acceptable: XML copy fidelity is the reconstructed canonical form).
- **HTML** (`parse_html.go`): `golang.org/x/net/html` tokenizer (`x/net` already a transitive Fyne dep; `go get golang.org/x/net/html`). Tolerant of unclosed tags; void elements→`KindEmptyElement`; `<!DOCTYPE …>`→a muted comment-style leaf.
- **Raw** (`parse_raw.go`): split `Src` on `\n`; each line→`KindRawLine` with one `RolePlain` segment spanning the line's `Src` byte range (zero-copy). The universal fallback when a structured parse fails mid-stream.

### 4.7 Summaries

Cached once at `CloseContainer` into `Aux`, pointed at by `SummarySeg` (`RoleMuted`): `KindObject`→`{ N items }` (`{ }`, `{ 1 item }` special-cased), `KindArray`→`[ N items ]`, `KindElement`→`<tag> N children`. A collapsed container renders head segs + summary on one row; toggling flips the `collapsed` bit + the incremental Fenwick update only — no re-parse, no full re-flatten.

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

1. **Single content-space selection/match overlay.** `widget.List` owns and rebuilds `scroller.Content.Objects` every layout pass (list.go:758-763) and boxes each row in a `listItem` with its own background rect (list.go:512-543). A free-text selection that spans mid-row N→mid-row M as one continuous layer must be a **sibling of the rows in content space**; List won't allow that without fighting the layout. With `container.Scroll` we own a `contentBox` with three stacked layers.
2. **Horizontal scroll of long unwrapped lines.** `widget.List` is vertical-only. `container.Scroll` with `Direction = container.ScrollBoth` (= `fyne.ScrollBoth`, container/scroll.go:22) gives free horizontal scroll + bar.

Cost accepted: we re-implement the fixed-height visible-window math (transcribed from list.go:413-435) and the recycle pool (a plain `sync.Pool`; List's `internal/async.Pool` is just a wrapper and is unimportable anyway, A3 constraint C). ~120 LOC we fully control.

### 5.2 Object tree

```
PrettyView (BaseWidget; implements input interfaces in §6.1)
└─ prettyViewRenderer (fyne.WidgetRenderer)
   └─ scroll *container.Scroll          // Direction = container.ScrollBoth
      └─ content *contentBox            // MinSize = (maxLineRunes*charWidth+pad, totalRows*rowH)
         Objects() (low→high z):
         ├─ selLayer   *fyne.Container   // pooled selection rects (translucent, A=0x40)
         ├─ matchLayer *fyne.Container   // pooled search-match rects
         └─ rowLayer   *fyne.Container   // ~V pooled rowWidgets (the only document text objects)
```

Z-order by slice position — earlier = drawn first = lower (entry.go:1813-1819: "selection rectangles to appear underneath the text"). `contentBox.MinSize()` returns the **full document extent** (pure arithmetic, never walks children — A1 case (d)), so the scrollbar geometry is correct, but `contentBox` holds only visible children.

### 5.3 ONE coordinate origin (resolves A2 Issues #2, #3, #9)

This is load-bearing and stated once, used everywhere. `container.Scroll` translates its single `Content` child by `Move(-Offset.X, -Offset.Y)` on **both axes** (scroller.go:454, verified). Therefore **all layer children (rows, selection rects, match rects) live in raw content space and subtract NO offset on either axis.** The scroll does the translation.

```
// Content-space conventions (top padding = 0; rows butt against content origin):
rowH      = round(MeasureText("M", textSize, Mono).Height) + rowPad   // integer
charWidth = round(MeasureText("M", textSize, Mono).Width)             // integer (textgrid.go:646-649)
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

`charWidth`/`rowH` are **rounded to integers** at measure time (matching TextGrid, textgrid.go:646-649) so selection rects align with glyphs on long lines and don't drift sub-pixel (A2 lower-severity finding). The `floor(contentY/rowH)` form with top-padding=0 is the single convention used by `reflow`, `hitTest`, and the rect builders — identical, so no off-by-one (A2 Issue #2). Adding `Offset.X` to `contentX` fixes the silent wrong-column copy under horizontal scroll (A2 Issue #3). Rects subtracting no offset fixes the 2× selection drift (A2 Issue #9).

### 5.4 Per-row primitive: N × `canvas.Text` (one per same-color run)

`canvas.Text` is strictly single-color (canvas/text.go:16-31; `DrawString` takes one `color.Color`, font.go:180). TextGrid is rejected (3 objects/cell + one Text per char, textgrid.go:687-699,719). RichText is rejected (no virtualization, O(doc) refresh, richtext.go:617-691). So each visible row renders **one `canvas.Text` per contiguous same-color segment**, at `x = indentX + col*charWidth`. K ≈ 3–10 for JSON, not char count.

```go
type rowWidget struct {
	widget.BaseWidget
	pv *PrettyView
	// model snapshot for this row (set by Update):
	depth    uint16
	foldable bool
	folded   bool
	segs     []Segment // visible (culled) segments for this row
}

type rowRenderer struct {
	row          *rowWidget
	indentGuides []*canvas.Line // pooled, ≤32, surplus Hidden
	triangle     *canvas.Text   // "▶"/"▼"; Hidden when !foldable
	texts        []*canvas.Text // pooled colored runs; surplus Hidden
	objects      []fyne.CanvasObject
}
```

**`rowRenderer.Update(vr VisibleRow)` (recycle hot path, no steady-state alloc):**

1. indent guides: ensure len==depth (cap 32), reuse, `Move`/`Resize`/`Show`, `Hide` surplus.
2. triangle: if foldable set `"▶"`/`"▼"` and Show, else Hide.
3. **Horizontal cull (MANDATORY, M-2):** compute `firstCol = floor(Offset.X/charWidth)`, `lastCol = ceil((Offset.X+viewportW)/charWidth)`. For each segment intersecting `[firstCol,lastCol]`, set `text.Text = seg.Text[clipStart:clipEnd]` (sub-range of runes), `text.Move(indentX + clipFirstCol*charWidth, 0)`, `text.Color = pv.palette[seg.Role]`. **Hard-cap** emitted text length at `2*viewportCols` runes regardless. This guarantees every texture ≤ viewport-width (texture.go:171-173 sizes the bitmap to `text.MinSize().Width`).
4. trim surplus `texts`→`Hide()`; rebuild `objects` = visible guides + triangle + visible texts; `canvas.Refresh(r.row)` (scroller.go:477 idiom: "we have no Redraw()").

Pooling discipline (grow with `append` only when `len<=i`, trim by `Hide()`) mirrors selectable.go:382-385.

### 5.5 Visible-window reflow (transcribed from list.go:413-435)

```go
func (r *prettyViewRenderer) reflow() {
	off, vpH := r.scroll.Offset, r.scroll.Size().Height
	rowH := r.pv.rowH
	total := int(r.pv.doc.fold.TotalVisibleRows())

	first := max(0, int(math.Floor(float64(off.Y/rowH))))
	last  := min(total-1, int(math.Ceil(float64((off.Y+vpH)/rowH))))

	for idx, rw := range r.live { // recycle rows out of [first,last]
		if idx < first || idx > last { rw.Hide(); r.rowPool.Put(rw); delete(r.live, idx) }
	}
	for idx := first; idx <= last; idx++ {
		rw := r.live[idx]
		if rw == nil { rw = r.getRow(); r.live[idx] = rw }
		rw.Move(fyne.NewPos(0, float32(idx)*rowH))          // CONTENT space, no offset
		rw.Resize(fyne.NewSize(r.pv.contentWidth, rowH))
		id, rowInNode := r.pv.doc.fold.nodeAtRow(int32(idx)) // O(log n)
		rw.renderer().Update(r.pv.buildVisibleRow(id, rowInNode, off.X, vpH))
	}
	r.rowLayer.Objects = sortedLive(r.live)
	r.rebuildSelection(first, last) // §6.4
	r.rebuildMatches(first, last)   // §7.5
	canvas.Refresh(r.content)
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
	_ desktop.Mouseable   = (*PrettyView)(nil) // MouseDown/MouseUp  (driver/desktop/mouse.go:45-48)
	_ fyne.Draggable      = (*PrettyView)(nil) // Dragged/DragEnd    (canvasobject.go:54-57)
	_ fyne.DoubleTappable = (*PrettyView)(nil) // DoubleTapped       (canvasobject.go:48-50)
	_ desktop.Cursorable  = (*PrettyView)(nil) // Cursor             (driver/desktop/cursor.go:45-47)
	_ desktop.Hoverable   = (*PrettyView)(nil) // MouseIn/Moved/Out  (driver/desktop/mouse.go:51-58)
	_ fyne.Focusable      = (*PrettyView)(nil) // FocusGained/Lost…  (canvasobject.go:66-76)
	_ fyne.Shortcutable   = (*PrettyView)(nil) // TypedShortcut      (canvasobject.go:90-92)
	_ desktop.Keyable     = (*PrettyView)(nil) // KeyDown/KeyUp      (driver/desktop/key.go:61-66)
)
```

### 6.2 State (4 ints + flags, mirroring selectable.go:16-24)

```go
type Pos struct {
	node NodeID // STABLE logical-line id (survives fold/unfold)
	row  int    // resolved current visible row (cache; re-resolved on fold change)
	col  int    // rune column in the line's DISPLAY text, [0, runeLen]
}

type Selection struct {
	anchor, focus Pos
	active        bool // anchor != focus
	dragging      bool
	grabMode      grabMode // grabNone | grabWord | grabLine
	grabStart, grabEnd Pos
}
```

Endpoints persist as `node` (stable). After any fold change, `onFoldChanged` re-resolves `row` via the O(log n) Fenwick prefix query (`doc.fold.rowOfNode`) — **no `lineID→row` map** (A2 Issue #7). If an endpoint's node is now hidden, snap it to the nearest visible ancestor's summary row and **clamp `col` to that row's runeLen**.

### 6.3 Hit-test (O(1) monospace, no MeasureText in handlers)

```go
func (pv *PrettyView) hitTest(local fyne.Position) Pos {
	contentY := local.Y + pv.scroll.Offset.Y
	contentX := local.X + pv.scroll.Offset.X // ADD Offset.X — A2 Issue #3
	total := int(pv.doc.fold.TotalVisibleRows())
	row := int(math.Floor(float64(contentY / pv.rowH)))
	if row < 0 { row = 0 }
	if row >= total { // clamp to EOD: last row, end column (selectable.go:207-209)
		row = total - 1
		id, _ := pv.doc.fold.nodeAtRow(int32(row))
		ln := pv.lineRunes(id, 0)
		return Pos{node: id, row: row, col: len(ln)}
	}
	id, rin := pv.doc.fold.nodeAtRow(int32(row))
	ln := pv.lineRunes(id, rin)
	indentX := pv.innerPad + float32(pv.depthOf(id))*pv.indentStep
	rel := contentX - indentX
	col := 0
	if rel > 0 { col = int(math.Round(float64(rel / pv.charWidth))) } // ≡ selectable.go:190 half-glyph
	if col > len(ln) { col = len(ln) }
	return Pos{node: id, row: row, col: col}
}
```

`round((x-indentX)/charWidth)` is algebraically identical to `selectable.cursorColAt`'s `pos.X < indentX + col*charWidth + charWidth/2` (selectable.go:190) for uniform `charWidth` — we close-form it, avoiding the O(n) per-prefix MeasureText thrash (R2 §2). The shipped code (`internal/geometry`'s `ColX`/`ColAtX`) computes columns on a **uniform monospace grid only**: one rune = one `CharWidth` cell. **Not implemented (known limitation):** the once-envisaged proportional/CJK/combining-glyph fallback (a per-`Line` `uniformGrid bool` tag plus a lazy cached `prefixW []float32` binary-searched per hit-test, O(log n) per hit-test and one O(n) MeasureText per such line). The target content is ASCII/BMP monospace JSON/XML/HTML, where the uniform grid is exact; wide (CJK), zero-width (combining), and proportional glyphs render and hit-test on the same single-cell advance, so they can mis-align and mis-hit-test. The prefix-width scheme above is the intended escalation if that becomes a goal.

### 6.4 Event wiring

- **MouseDown** sets the anchor authoritatively at the true press position (`hitTest(m.Position)`); detects triple-tap first via `isTripleTap(doubleTappedAtMs, now)` vs `DoubleTapDelay()` (300 ms, selectable.go:413-415); shift extends `focus` keeping `anchor`. Secondary button never starts drag/clears selection.
- **Dragged** — **anchor is NEVER recomputed here** (resolves A2 Issue #1: the first `DragEvent.Dragged` delta is relative to the previous mouse-move sample, not the press point, because `mouseDragPos` updates every move at window.go:417 — so `d.Position.Subtract(d.Dragged)` mis-anchors by up to one sample). We delete that idiom from `selectable.go:84` deliberately. `Dragged` only moves `focus = hitTest(d.Position)`, applies word/line grab extension, then autoscrolls (§6.6).
- **DragEnd** drops empty selections (selectable.go:63-73 analog).
- **DoubleTapped** word-select (token-aware, §6.5); arms triple-tap timestamp.
- **Cursor/Focus/Shift**: `TextCursor` over text, `PointerCursor` over triangles (Hoverable-driven on MouseMoved); selection drawn only when focused (selectable.go:312-316); shift tracked via `KeyDown`/`KeyUp` watching `desktop.KeyShiftLeft/Right` (entry.go:346-372).
- **Shortcuts**: embedded `fyne.ShortcutHandler`; `AddShortcut(&fyne.ShortcutCopy{}, copySelection)`, `AddShortcut(&fyne.ShortcutSelectAll{}, selectAll)`, `AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault}, focusSearch)`.

### 6.5 Word/line bounds (token-aware)

Default mirrors `getTextWhitespaceRegion`/`isWordSeparator` (entry.go:1924-1987), but a **token-aware override** consults the line's color-run metadata: double-click inside a token-run selects the whole run `[runStart,runEnd]` (so `"quoted string"` or `-12.5e3` selects as a unit). **Guard (A2 lower-severity):** synthetic summary rows (`{ 6 items }`) have no token runs — if `line.runs == nil` fall back to the whitespace-class heuristic. Triple-click selects the whole row (`col 0 … runeLen`, mirrors `selectCurrentRow`, selectable.go:218-225).

### 6.6 Autoscroll while dragging past edge (resolves A2 Issue #8 — data race)

Edge autoscroll is a cursor-driven nudge (`ScrollToOffset(Offset.Add(move))`) computed inside `Dragged` and followed by `reflow()`. **It is not shipped as the originally-designed `time.Ticker`.** Because `Dragged` fires only on pointer motion (window.go:411-419), dragging to the viewport edge and then **holding the pointer stationary stops the scroll** — a known limitation (backlogged; the ticker would close it). The upside: all autoscroll reads/writes happen inside the `Dragged` handler on the Fyne goroutine, so there is no off-thread clock and no data race (cf. R-13).

### 6.7 Selection rectangles (visible-window only — resolves A1 Break #3 / A2 Issue #9)

```go
func (r *prettyViewRenderer) rebuildSelection(first, last int) {
	a, b, active := r.pv.sel.normalized() // swap so a precedes b (selectable.go:247-250)
	if !active { hideAll(r.selRects); r.selLayer.Objects = nil; return }
	n := 0
	for row := max(a.row, first); row <= min(b.row, last); row++ {
		id, rin := r.pv.doc.fold.nodeAtRow(int32(row))
		ln := r.pv.lineRunes(id, rin)
		s, e := 0, len(ln)
		if row == a.row { s = a.col }
		if row == b.row { e = b.col }
		if row < b.row { e = max(e, r.pv.visibleCols()) } // middle rows bleed to right edge (Bruno)
		if e <= s { continue }
		indentX := r.pv.innerPad + float32(r.pv.depthOf(id))*r.pv.indentStep
		x1, x2 := indentX+float32(s)*r.pv.charWidth, indentX+float32(e)*r.pv.charWidth
		rect := poolRect(&r.selRects, n, r.pv.selColor) // grow append-only, reuse (selectable.go:382-385)
		rect.Resize(fyne.NewSize(x2-x1+1, r.pv.rowH))   // +1 (selectable.go:402)
		rect.Move(fyne.NewPos(x1-1, float32(row)*r.pv.rowH)) // -1; CONTENT space, NO offset (scroller.go:454)
		if r.pv.focused { rect.Show() } else { rect.Hide() }
		n++
	}
	for ; n < len(r.selRects); n++ { r.selRects[n].Hide() } // trim by Hide, never destroy
	r.selLayer.Objects = r.selRects[:countVisible(r.selRects)]
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

// Match is in MODEL coordinates: stable NodeID + rune columns into the line's display text.
type Match struct { Node NodeID; ColStart, ColEnd int }

type SearchResult struct {
	Query    SearchQuery
	Matches  []Match // ordered by (doc-order of Node, then ColStart)
	Active   int     // -1 if none
	Capped   bool
	Complete bool
	Err      error
}

type SearchConfig struct {
	MaxMatches  int           // default 10_000
	DebounceFor time.Duration // default 150ms
	MinQueryLen int           // default 1
}
```

Keying by **`NodeID`** (not visible-row) makes matches survive fold/unfold; `match→visibleRow` is the O(log n) Fenwick lookup, recomputed per projection change. This is the single most important search decision.

### 7.2 Scan (RE2, single-pass byte→rune — resolves A2 Issue #5)

`regexp` (RE2) is linear-time and safe for live-typed patterns; case-insensitive uses inline `(?i)`. Plain mode: lower-case fast path only when the line is pure-ASCII; if the line has any byte ≥ 0x80, do a **rune-wise `unicode.ToLower` fold** (the `len(ToLower)!=len` guard alone is unsound — e.g. U+212A). **Byte→rune offsets are converted in ONE forward pass per line** maintaining `(bytesConsumed, runesConsumed)` — O(L), never O(K·L) (A2 Issue #5: per-match `utf8.RuneCountInString(s[:b])` is quadratic on a long single-line minified doc). *(Shipped: `colCursor` in search.go.)* Search ignores synthetic summary text; it scans real `LineText`.

### 7.3 Threading & supersession

**Shipped: the scan is synchronous on the Fyne goroutine, not a worker.** With the single-pass byte→rune conversion (§7.2) and a hard `MaxMatches` cap, a full scan is O(total bytes) and stays under a frame — ~5 ms over the 7.5 MB / 440k-line fixture — so keystroke debouncing (`searchDebounced`, 150 ms, via `time.AfterFunc`+`fyne.Do`) is enough to keep typing smooth. A `searchGen` counter gives last-query-wins: a debounce callback that fired before a newer keystroke / `ClearSearch` / `SetData` checks the generation and skips itself.

The originally-designed **off-thread chunked scan** (a worker producing `ChunkBytes` slices, publishing snapshots via `fyne.Do`, with a `gen`+cancel channel) is **not implemented and not needed** at the in-memory sizes this viewer targets — and keeping the scan on the Fyne goroutine is what makes search/​reflow access to `pv.search` race-free without a mutex (running it off-thread would reintroduce that race). The ceiling is a single multi-gigabyte document, which is out of scope. **Search reads only immutable arenas** (`Src`, `Nodes`, `Segs`); only `foldIndex` and the search state mutate, always on the Fyne goroutine.

### 7.4 Navigation & reveal (resolves A2 Issue #6)

`Next`/`Prev` are index arithmetic with wrap (`(a+dir+n)%n`); while `!Complete`, **do not wrap past the last known match** — clamp and show "searching…". Count label `"3/27"` (or `"3/10000+"` when capped).

**`revealActive`:** (1) `RevealLine(line)` — unfold each collapsed ancestor (outermost-first, `unfoldAncestors`); (2) `row := rowOfLine(line)`; (3) center: `y := clamp(row*rowH - (vpH-rowH)/2, 0, maxOffsetY)`; (4) `scroll.ScrollToOffset(NewPos(matchX, y))` then `reflow()` (ScrollToOffset doesn't fire OnScrolled, scroller.go:572). **Order is load-bearing: expand → recompute total → resolve row → scroll.** **Stay on the fixed-height fast path** (never `SetItemHeight`-equivalent) so offset math is O(1). **Auto-reveal only on explicit user intent** (typed query / pressed Enter), gated by a `userHasInteracted` flag — never yank the viewport on a later streamed chunk's arrival (A2 Issue #6 trigger 2). Holding Next/Prev: step the index every keystroke but **debounce the scroll+reflow** to the trailing ~16 ms.

### 7.5 Highlight (reuses §6.7 mechanism, separate pools)

Per visible row, intersect `Matches` with the row's `NodeID` (a `map[NodeID][]int` built once per published result for O(1) lookup; switch to binary search only if the 10k cap profiles hot). Z-order low→high: `selection → other-match → active-match → text`. Active match: `th.Color(ColorNameMatchHighlight, v)` at higher alpha; others lower; both translucent. Same pooled-rect, visible-window-only discipline as §6.7 (≤ V rects, M-1).

---

## 8. Theming / colors

Syntax roles are extra `fyne.ThemeColorName` strings (they coexist with builtins). A **wrapping theme** forwards everything to `theme.DefaultTheme()` except our names. The interface form `widget.Theme().Color(name, variant)` is used (theme.go:29; the package func `theme.Color(name)` is single-arg, A3 §D2 — both real).

```go
const (
	ColorNameSynKey   fyne.ThemeColorName = "prettyview.key"
	ColorNameSynString = "prettyview.string"
	ColorNameSynNumber = "prettyview.number"
	ColorNameSynBool   = "prettyview.bool"
	ColorNameSynNull   = "prettyview.null"
	ColorNameSynPunct  = "prettyview.punct"
	ColorNameSynTag    = "prettyview.tag"
	ColorNameSynAttr   = "prettyview.attr"
	ColorNameSynComment = "prettyview.comment"
	ColorNameMatchHighlight = "prettyview.match"
	// Selection reuses builtin theme.ColorNameSelection; Plain falls back to ColorNameForeground.
)
```

Tokens store a 1-byte `ColorRole`; a `palette []color.Color` is rebuilt **once per `Refresh()`** from `th.Color(name, v)` with `v := fyne.CurrentApp().Settings().ThemeVariant()` (settings.go:23). Dark defaults (Bruno-ish): key `#9CDCFE`, string `#CE9178`, number `#B5CEA8`, bool/null `#569CD6`, tag `#569CD6`, attr `#9CDCFE`, comment `#6A9955`, punct = `ColorNameForeground`; light variant derives darker equivalents. Monospace font is the bundled `DejaVuSansMono-Powerline.ttf` via `fyne.TextStyle{Monospace:true}` (theme/bundled-fonts.go:41-47) — zero setup. Dark/light toggle recolors on the next `Refresh` (selectable.go:305 re-reads `v` each pass).

---

## 9. Threading & large-file handling

- **Parse off-thread, swap on-thread.** `SetData` launches `go parse(...)`; on completion `fyne.Do(func(){ pv.swapModel(m); pv.recomputeContentSize(); pv.Refresh() })` (thread.go:18; R4 §4). The worker never touches widgets, `Offset`, pooled rows, or `Refresh`.
- **Search** as §7.3 (synchronous on the Fyne goroutine, debounced, `searchGen` last-query-wins).
- **Autoscroll ticker** as §6.6 (pure clock; all UI work inside `fyne.Do`).
- **Invariant:** post-parse arenas (`Src`, `Nodes`, `Segs`, `Aux`, precomputed `LineText`) are read-only; only `foldIndex` and selection/search state mutate, always on the Fyne goroutine. Workers read only the immutable arenas.

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
| R-12 | **`lineID→row` map → O(n) rebuild per fold; "O(log n) fold" overclaim** — A2 Issue #7, D1 open risk | High | `NodeID` *is* the line id; `rowOfNode` is O(log n) Fenwick prefix (no map). Fold honestly O(k visible descendants) with O(log n) row delta; `hiddenBy` array keeps lookups O(log n). §4.4/§6.2. |
| R-13 | **Autoscroll ticker data race** (reads UI fields off-thread) — A2 Issue #8 | n/a | **No ticker shipped.** Drag-edge autoscroll runs inside `Dragged` (pointer-motion only), entirely on the Fyne goroutine, so there is no off-thread race to begin with. Held-stationary edge autoscroll (which a ticker would add) is a known limitation on the backlog, not shipped. §6.6. |
| R-14 | **Selection rects drift 2×** (subtracting Offset on a scrolled-content child; scroller.go:454 already translates both axes) — A2 Issue #9 | Blocker (visible) | Rects in raw content space, **no** offset subtraction either axis. §5.3/§6.7. Round-trip test. |
| R-15 | Fractional `charWidth` drift on long lines — A2 minor | Low | Round `charWidth`/`rowH` to integers (textgrid.go:646-649). §5.3. |
| R-16 | Next-wrap during incomplete scan jumps backward — A2 minor | Low | While `!Complete`, clamp at last known match, show "searching…". §7.4. |
| R-17 | Double-click on summary row indexes nil run slice — A2 minor | Low | Guard: `if line.runs==nil` use whitespace heuristic. §6.5. |
| R-18 | **`internal/...` packages unimportable** (`go vet`) — A3 constraint C | Blocker (build) | Use `sync.Pool`, `container.Scroll`, `fyne.MeasureText` — never any `fyne.io/fyne/v2/internal/...`. §2, §5.1. |
| R-19 | Deep fully-expanded tree min-size cost — A1 case (d) | Low | `contentBox.MinSize()` is pure arithmetic, never walks children; indent guides capped at 32. §5.2/§5.4. |

---

## 11. Build plan (ordered, milestone-based, each independently testable)

Each milestone ends with green tests and (from M7) a runnable demo. `go test ./...` and `go vet ./...` (the latter enforces R-18) must pass at every milestone.

**M0 — Repo skeleton.**
`go.mod` (`module github.com/ideaconnect/go-fyne-pretty-view`, `go 1.26`, `require fyne.io/fyne/v2 v2.7.4`, `golang.org/x/net`). Empty `prettyview.go` with `PrettyView` embedding `widget.BaseWidget` + a stub `CreateRenderer` returning `widget.NewSimpleRenderer(canvas.NewRectangle(...))` (widget.go:203). Add testdata fixtures incl. `tabs.json`. *Test:* `go build ./...`, `go vet ./...` clean.

**M1 — Model arenas + JSON scanner + summaries.**
`model.go`, `builder.go`, `parse.go`, `parse_json.go`, `summary.go`. Hand-written zero-copy JSON/JSONC scanner → SoA. *Tests (model_test.go):* node counts for `small.json`; segment roles correct; `unsafe.Sizeof(Node)==32`; **zero-copy assertion** (segment byte ranges are sub-slices of `Src`, no per-node `[]byte` alloc — check `&Src[seg.Start]` aliasing); summary strings (`{ }`/`{ 1 item }`/`{ 6 items }`); `colMap` identity for tab-free lines and correct for `tabs.json`.

**M2 — Fold index / projection.**
`foldindex.go` (fenwick + post-order subtree pass + `hiddenBy`). *Tests (foldindex_test.go):* `TotalVisibleRows` O(1) correctness; `nodeAtRow`/`rowOfNode` round-trip for every visible row; fold/unfold changes counts correctly; `ExpandAll`/`CollapseAll`; complexity probe (collapse near top of a synthetic 100k-node doc updates total in one prefix query; lookups stay O(log n)). Assert no `[]rune`/string alloc per fold (`testing.AllocsPerRun`).

**M3 — XML / HTML / Raw parsers.**
`parse_xml.go`, `parse_html.go`, `parse_raw.go`, `AutoDetect`. *Tests:* `catalog.xml`/`page.html` node mapping (elements/attrs/text/comments, void elements, tolerant unclosed tags); raw line count; AutoDetect picks the right format for each fixture.

**M4 — Geometry + hit-test math (pure, no widgets).**
`geometry.go`. Integer `charWidth`/`rowH`; the one origin convention; `hitTest` and col↔x. *Tests (internal_math_test.go):* **golden round-trip** — for rows {0,1,deep} at non-trivial `Offset`, `hitTest(rectScreenPos(row,col)) == (row,col)`; off-by-one guards at row top edge and `+rowH-0.5`; `charWidth` is integer; `contentX` includes `Offset.X`.

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
- `/home/bartosz/go/pkg/mod/fyne.io/fyne/v2@v2.7.4/widget/list.go` (fast-path window math 413-435; recycle pool 649-754; Content.Objects rebuild 758-763)
- `/home/bartosz/go/pkg/mod/fyne.io/fyne/v2@v2.7.4/internal/widget/scroller.go` (Offset 490, OnScrolled 495, ScrollToOffset 572 no-OnScrolled, both-axes Content.Move 454, canvas.Refresh idiom 477)
- `/home/bartosz/go/pkg/mod/fyne.io/fyne/v2@v2.7.4/widget/selectable.go` (state 16-24, getRowCol 197-215, selection/normalize 235-263, SelectedText 120-131, buildSelection pooled rects 329-405, isTripleTap 413-415)
- `/home/bartosz/go/pkg/mod/fyne.io/fyne/v2@v2.7.4/widget/entry.go` (z-order under text 1813-1819, TextCursor 248-250, shift/word/autoscroll 346-372/1852-1922/1924-1987, shortcuts 1042-1135, disabled-Copy routing context)
- `/home/bartosz/go/pkg/mod/fyne.io/fyne/v2@v2.7.4/internal/painter/gl/texture.go` (full-line-width text bitmap 171-173 — drives M-2)
- `/home/bartosz/go/pkg/mod/fyne.io/fyne/v2@v2.7.4/internal/cache/base.go` (content-keyed text textures, 60 s ValidDuration 9 — drives R-2)
- `/home/bartosz/go/pkg/mod/fyne.io/fyne/v2@v2.7.4/internal/driver/glfw/window.go` (drag threshold + incremental Dragged delta + mouseDragPos per-move 405-424; deepest-match dispatch 460-471)
- `/home/bartosz/go/pkg/mod/fyne.io/fyne/v2@v2.7.4/widget/textgrid.go` (rounded monospace cell size 646-649)
- `/home/bartosz/go/pkg/mod/fyne.io/fyne/v2@v2.7.4/canvas/text.go` (single-color Text 16-31), `text.go` (MeasureText 71), `thread.go` (Do/DoAndWait 8/18), `clipboard.go` (4-9), `app.go` (Clipboard 88), `theme.go` (Color 28-33), `widget.go` (WidgetRenderer 17-33; NewSimpleRenderer 203)