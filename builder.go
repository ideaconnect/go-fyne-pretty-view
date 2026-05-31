package prettyview

import "unicode/utf8"

// Builder is the arena-construction API that parsers drive. A parser never
// builds a tree of pointers; it calls Open/Leaf/Close and the Builder appends
// flat records into the Document's arenas.
//
// Segment text comes in two flavors (see Seg): a zero-copy byte range into the
// source, used for the actual data tokens (keys, string/number literals), and a
// synthesized literal, used for structural punctuation and summaries. Literals
// are interned once into Document.Aux and deduplicated, so repeated ": " / ","
// / "{" cost a few bytes total across the whole document.
type Builder struct {
	doc      *Document
	stack    []NodeID
	litCache map[string][2]uint32

	collapseDepth int // auto-collapse containers at this depth or deeper (0 = never)
}

// Seg is a parser-facing description of one colored run on a line. Provide Lit
// for synthesized text, or Start/End (a byte range into the source) for a
// zero-copy run.
type Seg struct {
	Role  ColorRole
	Lit   string // synthesized text; if "", Start/End name a zero-copy Src range
	Start uint32
	End   uint32
}

// srcSeg builds a zero-copy segment spanning src[start:end].
func srcSeg(role ColorRole, start, end int) Seg {
	return Seg{Role: role, Start: uint32(start), End: uint32(end)}
}

// litSeg builds a synthesized-text segment.
func litSeg(role ColorRole, lit string) Seg {
	return Seg{Role: role, Lit: lit}
}

func newBuilder(src []byte, format Format, collapseDepth int) *Builder {
	d := &Document{Src: src, Format: format}
	d.Nodes = append(d.Nodes, Node{
		Parent: NoNode, Subtree: 1, ChildCount: 0,
		HeadLine: -1, CloseLine: -1, Kind: KindRoot, Depth: 0,
	})
	b := &Builder{doc: d, litCache: map[string][2]uint32{}, collapseDepth: collapseDepth}
	b.stack = append(b.stack, 0)
	return b
}

// curDepth is the indentation depth for a node added now. The root sits at
// stack[0], so top-level nodes get depth 0.
func (b *Builder) curDepth() uint8 {
	d := len(b.stack) - 1
	if d > 255 {
		d = 255
	}
	return uint8(d)
}

// intern adds s to Aux (deduplicated) and returns its byte range.
func (b *Builder) intern(s string) (uint32, uint32) {
	if r, ok := b.litCache[s]; ok {
		return r[0], r[1]
	}
	start := uint32(len(b.doc.Aux))
	b.doc.Aux = append(b.doc.Aux, s...)
	end := uint32(len(b.doc.Aux))
	b.litCache[s] = [2]uint32{start, end}
	return start, end
}

// appendSegs appends a line's segments to the arena and returns their range.
func (b *Builder) appendSegs(segs []Seg) (first uint32, count uint16) {
	first = uint32(len(b.doc.Segs))
	for _, sg := range segs {
		seg := Segment{Role: sg.Role}
		if sg.Lit != "" {
			seg.Start, seg.End = b.intern(sg.Lit)
			seg.Buf = bufAux
		} else {
			seg.Start, seg.End, seg.Buf = sg.Start, sg.End, bufSrc
		}
		b.doc.Segs = append(b.doc.Segs, seg)
	}
	count = uint16(uint32(len(b.doc.Segs)) - first)
	return
}

func (b *Builder) top() NodeID { return b.stack[len(b.stack)-1] }

// linkChild records a new child on its parent.
func (b *Builder) linkChild(parent NodeID) {
	b.doc.Nodes[parent].ChildCount++
}

// Open begins a foldable container. head are the segments of its opening line
// (e.g. `"paths": {` or `<product id="0">`). srcStart is the byte offset in Src
// where the node begins, for copy-subtree.
func (b *Builder) Open(k Kind, srcStart int, head []Seg) NodeID {
	parent := b.top()
	id := NodeID(len(b.doc.Nodes))
	depth := b.curDepth()

	headLine := int32(len(b.doc.Lines))
	sf, sc := b.appendSegs(head)
	b.doc.Lines = append(b.doc.Lines, Line{Owner: id, Fold: id, SegFirst: sf, SegCount: sc, Depth: depth})

	n := Node{
		Parent: parent, Subtree: 1, ChildCount: 0,
		HeadLine: headLine, CloseLine: headLine,
		SrcStart: uint32(srcStart), Kind: k, Depth: depth,
	}
	if b.collapseDepth > 0 && int(depth) >= b.collapseDepth {
		n.Flags |= flagDefaultCollapsed
	}
	b.doc.Nodes = append(b.doc.Nodes, n)
	b.linkChild(parent)
	b.stack = append(b.stack, id)
	return id
}

// Leaf adds a single-line, non-foldable node (a key/value, scalar, text, comment
// or raw line). srcStart/srcEnd bound its source bytes for copy-subtree.
func (b *Builder) Leaf(k Kind, srcStart, srcEnd int, segs []Seg) NodeID {
	parent := b.top()
	id := NodeID(len(b.doc.Nodes))
	depth := b.curDepth()

	line := int32(len(b.doc.Lines))
	sf, sc := b.appendSegs(segs)
	b.doc.Lines = append(b.doc.Lines, Line{Owner: id, Fold: NoNode, SegFirst: sf, SegCount: sc, Depth: depth})

	b.doc.Nodes = append(b.doc.Nodes, Node{
		Parent: parent, Subtree: 1, ChildCount: 0,
		HeadLine: line, CloseLine: line,
		SrcStart: uint32(srcStart), SrcEnd: uint32(srcEnd), Kind: k, Depth: depth,
	})
	b.linkChild(parent)
	return id
}

// Close ends the current container. closeSegs are the segments of its closing
// line (e.g. `}`). A trailing comma, if any, is added afterwards by the parent
// via AppendComma. srcEnd is the byte offset just past the node's last source
// byte. The collapsed (folded) rendering is built later, in finish, once any
// trailing comma has been placed.
func (b *Builder) Close(srcEnd int, closeSegs []Seg) NodeID {
	id := b.top()
	b.stack = b.stack[:len(b.stack)-1]

	n := &b.doc.Nodes[id]
	n.SrcEnd = uint32(srcEnd)
	n.Subtree = NodeID(len(b.doc.Nodes)) - id

	closeLine := int32(len(b.doc.Lines))
	csf, csc := b.appendSegs(closeSegs)
	b.doc.Lines = append(b.doc.Lines, Line{Owner: id, Fold: NoNode, SegFirst: csf, SegCount: csc, Depth: n.Depth})
	n.CloseLine = closeLine
	return id
}

// AppendComma appends a "," punctuation segment to the given line. The line's
// segments must be the most recently appended block (i.e. this is called right
// after the node was emitted), so the comma stays contiguous with them.
func (b *Builder) AppendComma(lineIdx int32) {
	ss, se := b.intern(",")
	b.doc.Segs = append(b.doc.Segs, Segment{Start: ss, End: se, Role: RolePunct, Buf: bufAux})
	b.doc.Lines[lineIdx].SegCount++
}

// LastLine returns the final display line of a node (its close line for a
// container, its only line for a leaf).
func (b *Builder) LastLine(id NodeID) int32 { return b.doc.Nodes[id].CloseLine }

// closeDangling force-closes any containers left open by a tolerant parser that
// stopped early, so the model stays well-formed.
func (b *Builder) closeDangling() {
	for len(b.stack) > 1 {
		id := b.top()
		b.Close(int(b.doc.Nodes[id].SrcStart), []Seg{litSeg(RolePunct, closeBraceFor(b.doc.Nodes[id].Kind))})
	}
}

func closeBraceFor(k Kind) string {
	switch k {
	case KindArray:
		return "]"
	case KindElement:
		return "…"
	default:
		return "}"
	}
}

// buildCollapsedRenderings fills each foldable head line's collapsed segment run
// (head segs ++ muted summary ++ close segs) after all commas are placed.
func (b *Builder) buildCollapsedRenderings() {
	d := b.doc
	for id := range d.Nodes {
		nid := NodeID(id)
		if !foldable(d, nid) {
			continue
		}
		n := &d.Nodes[id]
		head := &d.Lines[n.HeadLine]
		clo := &d.Lines[n.CloseLine]

		collFirst := uint32(len(d.Segs))
		d.Segs = append(d.Segs, d.Segs[head.SegFirst:head.SegFirst+uint32(head.SegCount)]...)
		ss, se := b.intern(" " + summaryFor(n.Kind, int(n.ChildCount)) + " ")
		d.Segs = append(d.Segs, Segment{Start: ss, End: se, Role: RoleMuted, Buf: bufAux})
		d.Segs = append(d.Segs, d.Segs[clo.SegFirst:clo.SegFirst+uint32(clo.SegCount)]...)
		head.CollFirst = collFirst
		head.CollCount = uint16(uint32(len(d.Segs)) - collFirst)
	}
}

// finish completes construction: it closes any dangling containers, fills the
// root subtree size, builds collapsed renderings, builds the fold index, and
// applies the default-collapse policy.
func (b *Builder) finish() *Document {
	b.closeDangling()
	b.doc.Nodes[0].Subtree = NodeID(len(b.doc.Nodes))
	b.buildCollapsedRenderings()
	b.computeExtent()
	b.doc.fold = newFoldIndex(b.doc)
	b.doc.fold.applyDefaults(b.doc)
	return b.doc
}

// computeExtent records the widest line (in runes) and the deepest indent, used
// to size the horizontal/vertical scroll extent. One O(n) pass.
func (b *Builder) computeExtent() {
	d := b.doc
	for li := range d.Lines {
		l := &d.Lines[li]
		if l.Depth > d.maxDepth {
			d.maxDepth = l.Depth
		}
		runes := 0
		for _, s := range d.Segs[l.SegFirst : l.SegFirst+uint32(l.SegCount)] {
			runes += utf8.RuneCount(d.segBytes(s))
		}
		// A collapsed fold-head can be wider than its expanded head line.
		if l.Fold != NoNode {
			cr := 0
			for _, s := range d.Segs[l.CollFirst : l.CollFirst+uint32(l.CollCount)] {
				cr += utf8.RuneCount(d.segBytes(s))
			}
			if cr > runes {
				runes = cr
			}
		}
		if runes > d.maxLineRunes {
			d.maxLineRunes = runes
		}
	}
}
