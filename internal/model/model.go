package model

import "unicode/utf8"

// This file defines the compact, struct-of-arrays document model that backs the
// viewer. The design goals (see docs/DESIGN.md) are:
//
//   - No per-node heap allocation: nodes and display lines live in flat arenas
//     (Nodes, Lines, Segs). A document occupies roughly 5x its source size.
//   - Zero-copy text: a segment is a byte range into the original source (or, for
//     synthesized text such as summaries and decoded entities, into a single
//     shared Aux buffer). No per-token string is allocated.
//   - Model-based everything: rendering, selection, search, and copy all read
//     from this model; only the rows currently on screen are ever widgets.
//
// Projection granularity. We project at the *line* level rather than the *node*
// level. A foldable container owns two display lines — its head ("key": { ) and
// its close ( } ) — which are not adjacent in document order (the children sit
// between them). Line-granularity makes hiding a folded subtree a simple
// contiguous line range, and makes the close brace fall naturally after the
// children. Every leaf node owns exactly one line (its HeadLine == CloseLine); a
// foldable container owns a distinct head line and close line. An empty container
// ({}, [], <tag/>) is not foldable — the parsers emit it as a single-line Leaf, so
// it falls under the leaf case (head == close), never as an Open/Close container.

// NodeID indexes into Document.Nodes. The synthetic root is node 0.
type NodeID = int32

// NoNode is the sentinel for "no node" (e.g. a non-foldable line's Fold field,
// or the parent of the root).
const NoNode NodeID = -1

// Kind classifies a structural node.
type Kind uint8

const (
	KindRoot         Kind = iota // synthetic document root
	KindObject                   // JSON object {} (foldable)
	KindArray                    // JSON array [] (foldable)
	KindKeyValue                 // "key": <scalar> — single, non-foldable line
	KindScalar                   // bare scalar array element
	KindElement                  // XML/HTML <tag>…</tag> (foldable)
	KindEmptyElement             // <tag/> or void element — single line
	KindText                     // XML/HTML text / CDATA
	KindComment                  // JSONC/XML/HTML comment
	KindRawLine                  // raw mode: one physical source line
	KindError                    // a recovered parse-error marker line
)

// Foldable reports whether nodes of this kind can be collapsed.
func (k Kind) Foldable() bool {
	switch k {
	case KindObject, KindArray, KindElement, KindRoot:
		return true
	default:
		return false
	}
}

// ColorRole names a syntax color slot. It is one byte per segment and is resolved
// to a concrete color.Color only at draw time, via the active palette.
type ColorRole uint8

const (
	RolePlain   ColorRole = iota // default foreground (punctuation, structure)
	RoleKey                      // object key / attribute name target
	RoleString                   // string literal / text content
	RoleNumber                   // numeric literal
	RoleBool                     // true / false
	RoleNull                     // null
	RolePunct                    // braces, brackets, colons, commas
	RoleTag                      // XML/HTML element name
	RoleAttr                     // XML/HTML attribute name
	RoleComment                  // comment text
	RoleMuted                    // fold-summary text ("{ 6 items }")
	NumColorRoles
)

// Buffer selectors for a Segment's byte range.
const (
	BufSrc uint8 = 0 // range is into Document.Src (zero-copy)
	BufAux uint8 = 1 // range is into Document.Aux (synthesized text)
)

// Segment is one contiguous, single-color run of text on a display line.
// 12 bytes, no pointers.
//
// The uint32 offsets cap a single document's source at 4 GiB; parse.Parse
// truncates beyond that so the casts never wrap (multi-gigabyte documents are out
// of scope — see DESIGN §7.3).
type Segment struct {
	Start uint32    // byte offset into the buffer named by Buf
	End   uint32    // exclusive
	Role  ColorRole // color slot
	Buf   uint8     // BufSrc or BufAux
}

// Node flags.
const (
	flagDefaultCollapsed uint8 = 1 << iota // collapsed on load (depth policy)
	flagRawText                            // opaque text bytes (HTML <script>/<style>, XML CDATA): preserved verbatim on serialize, never entity-escaped (#85)
)

// MarkRawText flags a leaf node as opaque raw text (HTML <script>/<style> content or an XML
// CDATA section) so a markup Reformat re-emits its source bytes verbatim instead of the
// whitespace-collapsed, entity-escaped display text — escaping a script body's '<' to "&lt;"
// would be invalid JavaScript (#85). Display is unaffected (the node still renders its
// collapsed segment). Parsers call this right after Leaf.
func (b *Builder) MarkRawText(id NodeID) { b.doc.Nodes[id].Flags |= flagRawText }

// IsRawText reports whether node is opaque raw text marked by MarkRawText (#85).
func (d *Document) IsRawText(node NodeID) bool {
	return node != NoNode && int(node) < len(d.Nodes) && d.Nodes[node].Flags&flagRawText != 0
}

// Node is one structural element. 32 bytes, no pointers.
//
// Nodes are emitted depth-first, so a node's *subtree* occupies the contiguous
// id range [id, id+Subtree). Direct children are NOT contiguous (a child
// container's descendants sit between it and its next sibling); to iterate the
// ChildCount direct children, start at id+1 and advance by each child's Subtree.
type Node struct {
	Parent     NodeID // -1 for the root
	Subtree    int32  // number of nodes in this subtree, including self (>= 1)
	ChildCount int32  // number of direct children
	HeadLine   int32  // index into Lines of this node's own/opening line
	CloseLine  int32  // index into Lines of this node's closing line (== HeadLine for a leaf)
	SrcStart   uint32 // byte span into Src covering the whole node (for copy-subtree)
	SrcEnd     uint32
	Kind       Kind
	Depth      uint8
	Flags      uint8
	_          uint8 // padding
}

// Line is one display row. 24 bytes, no pointers.
type Line struct {
	Owner     NodeID // the structural node this line belongs to
	Fold      NodeID // node whose fold triangle sits on this line, or NoNode
	SegFirst  uint32 // first segment of the expanded rendering
	CollFirst uint32 // first segment of the collapsed rendering (valid iff Fold != NoNode)
	SegCount  uint16
	CollCount uint16
	Depth     uint8
	_         [3]uint8
}

// Document is the parsed, immutable-after-parse model. Only the fold index and
// (elsewhere) selection/search state mutate after construction, always on the
// Fyne goroutine.
type Document struct {
	Src   []byte    // original bytes, retained once for zero-copy segments
	Aux   []byte    // synthesized text: summaries, decoded entities, etc.
	Nodes []Node    // structural arena; Nodes[0] is the synthetic root
	Lines []Line    // display-line arena, in document order
	Segs  []Segment // segment arena

	Format Format

	fold *foldIndex // visible-line projection (built after parse)

	MaxLineRunes int   // widest expanded line, in runes (for content width)
	MaxDepth     uint8 // deepest indentation level present

	lineRunes []int32 // cached expanded displayed-rune-length per line (computeExtent)
	lineASCII []bool  // true iff the line's expanded text is a byte==column grid (no multi-byte rune)

	// Soft-wrap projection. rowsOf[line] is the number of visual rows a line's
	// currently-displayed text occupies at the active wrap width; nil means WrapNone
	// — every visible line is exactly one row (the fast path, with zero extra cost).
	// colsByDepth is the per-depth text-column budget the view supplies (the model
	// cannot import geometry, so the column math is passed in); it is retained so a
	// single fold/unfold can re-weight one head line without a full reprojection.
	rowsOf      []int32
	colsByDepth []int
}

// SegBytes returns the raw bytes a segment references.
func (d *Document) SegBytes(s Segment) []byte {
	if s.Buf == BufAux {
		return d.Aux[s.Start:s.End]
	}
	return d.Src[s.Start:s.End]
}

// LineSegs returns the expanded-rendering segments for a display line.
func (d *Document) LineSegs(li int32) []Segment {
	l := &d.Lines[li]
	return d.Segs[l.SegFirst : l.SegFirst+uint32(l.SegCount)]
}

// CollapsedSegs returns the collapsed-rendering segments for a fold-head line.
func (d *Document) CollapsedSegs(li int32) []Segment {
	l := &d.Lines[li]
	return d.Segs[l.CollFirst : l.CollFirst+uint32(l.CollCount)]
}

// LineString builds the full display text of a line's expanded rendering. It
// allocates and is intended for on-demand use (tests, a single visible/searched
// row) — never for materializing the whole document.
func (d *Document) LineString(li int32) string {
	return segsText(d, d.LineSegs(li))
}

// DisplaySegs returns the segments actually shown for a line, accounting for fold
// state: a collapsed fold-head shows its collapsed rendering, everything else its
// expanded segments.
func (d *Document) DisplaySegs(li int32) []Segment {
	if d.IsCollapsed(li) {
		return d.CollapsedSegs(li)
	}
	return d.LineSegs(li)
}

// DisplayString builds the text a line currently shows (fold-state aware).
func (d *Document) DisplayString(li int32) string {
	return segsText(d, d.DisplaySegs(li))
}

// LineRuneLen returns the number of runes in a line's currently-displayed text.
// The expanded length is cached at build time (computeExtent); a collapsed
// fold-head's rendering is short (head + summary + close), so it is counted
// directly rather than cached.
func (d *Document) LineRuneLen(li int32) int {
	if d.IsCollapsed(li) {
		return countRunes(d, d.CollapsedSegs(li))
	}
	if int(li) < len(d.lineRunes) {
		return int(d.lineRunes[li])
	}
	return countRunes(d, d.LineSegs(li)) // fallback for docs not built via Builder
}

// LineIsByteGrid reports whether line li's expanded text is a uniform byte==column
// grid (no multi-byte rune), so a column's byte offset within the line equals the
// column index. The renderer uses this to slice a visible column window by
// arithmetic instead of walking the prefix. Out-of-range or a document not built via
// Builder reports false (the safe, slow decode path).
func (d *Document) LineIsByteGrid(li int32) bool {
	return int(li) < len(d.lineASCII) && d.lineASCII[li]
}

func countRunes(d *Document, segs []Segment) int {
	n := 0
	for _, s := range segs {
		n += utf8.RuneCount(d.SegBytes(s))
	}
	return n
}

func segsText(d *Document, segs []Segment) string {
	n := 0
	for _, s := range segs {
		n += int(s.End - s.Start)
	}
	buf := make([]byte, 0, n)
	for _, s := range segs {
		buf = append(buf, d.SegBytes(s)...)
	}
	return string(buf)
}

// IsCollapsed reports whether the fold node owning line li is currently collapsed.
// Non-fold lines report false.
func (d *Document) IsCollapsed(li int32) bool {
	f := d.Lines[li].Fold
	if f == NoNode {
		return false
	}
	return d.fold.collapsed.get(f)
}

// TotalLines reports the number of display lines in the document (visible or not).
func (d *Document) TotalLines() int { return len(d.Lines) }

// EmptyDocument returns a valid, empty document (a lone root with no lines).
func EmptyDocument() *Document {
	d := &Document{
		Nodes: []Node{{Parent: NoNode, Subtree: 1, Kind: KindRoot, HeadLine: -1, CloseLine: -1}},
	}
	d.fold = newFoldIndex(d)
	d.fold.buildFenwick()
	return d
}
