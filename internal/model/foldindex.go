package model

import "math/bits"

// foldIndex is the visible-line projection. It answers, in O(log n):
//
//   - TotalVisibleRows()        — how many rows are currently on screen-able
//   - lineAtRow(row) -> line     — which display line is at a given visible row
//   - rowOfLine(line) -> row     — which visible row a line occupies
//
// and supports fold/unfold of a node in O(k), where k is the number of that
// node's currently-visible descendant lines, with an O(log n) Fenwick update.
//
// Visibility rule: a line is visible iff none of its ancestor containers is
// collapsed. We track, per line, hiddenBy = the nearest collapsed ancestor node
// (or NoNode). vis[line] = 1 iff hiddenBy == NoNode. The Fenwick is built over
// vis, so rank/select over visible lines is O(log n).
//
// Incremental-correctness invariant: fold/unfold is only ever invoked on a node
// that is currently visible (its triangle is on screen). Hidden triangles can't
// be clicked, and bulk operations (ExpandAll/CollapseAll/defaults) rebuild the
// whole projection instead of stepping incrementally. Under this invariant, when
// we fold X every line inside X that is still visible has hiddenBy == NoNode, so
// claiming exactly those lines (and restoring exactly those on unfold) is sound
// even with arbitrarily nested folds.

type bitset struct{ words []uint64 }

func newBitset(n int) bitset { return bitset{words: make([]uint64, (n+63)/64)} }

func (b bitset) get(i NodeID) bool { return b.words[i>>6]&(1<<uint(i&63)) != 0 }
func (b *bitset) set(i NodeID)     { b.words[i>>6] |= 1 << uint(i&63) }
func (b *bitset) clear(i NodeID)   { b.words[i>>6] &^= 1 << uint(i&63) }

// fenwick is a 1-indexed binary indexed tree over per-line visibility counts.
type fenwick struct {
	tree   []int32 // len = n+1; external indices are 0-based lines
	maxLog int
}

func newFenwick(n int) fenwick {
	ml := 0
	if n > 0 {
		ml = bits.Len(uint(n)) - 1
	}
	return fenwick{tree: make([]int32, n+1), maxLog: ml}
}

// add adds delta to line i (0-based).
func (f *fenwick) add(i int, delta int32) {
	for i++; i < len(f.tree); i += i & -i {
		f.tree[i] += delta
	}
}

// prefix returns the sum of vis over lines [0, i).
func (f *fenwick) prefix(i int) int32 {
	var s int32
	for ; i > 0; i -= i & -i {
		s += f.tree[i]
	}
	return s
}

func (f *fenwick) total() int32 { return f.prefix(len(f.tree) - 1) }

// kth returns the 0-based line index of the k-th visible line (k is 1-based).
// Caller must ensure 1 <= k <= total().
func (f *fenwick) kth(k int32) int {
	pos := 0
	for b := f.maxLog; b >= 0; b-- {
		next := pos + (1 << b)
		if next < len(f.tree) && f.tree[next] < k {
			pos = next
			k -= f.tree[next]
		}
	}
	return pos // 1-based position-1 == 0-based line index of the k-th visible line
}

type foldIndex struct {
	collapsed bitset   // over NodeID
	hiddenBy  []NodeID // per line: nearest collapsed ancestor, or NoNode
	vis       []int32  // per line: 1 if visible, else 0
	bit       fenwick  // over vis
}

// newFoldIndex constructs the all-visible projection ARRAYS for d (every line
// visible, no collapsed ancestors). It deliberately does NOT build the Fenwick — the
// caller builds it exactly once: a default-collapse parse builds it via applyDefaults'
// rebuild, every other caller calls buildFenwick directly. Building here as well would
// rebuild the Fenwick twice whenever collapseDepth > 0.
func newFoldIndex(d *Document) *foldIndex {
	n := len(d.Lines)
	fi := &foldIndex{
		collapsed: newBitset(len(d.Nodes)),
		hiddenBy:  make([]NodeID, n),
		vis:       make([]int32, n),
	}
	for i := range fi.hiddenBy {
		fi.hiddenBy[i] = NoNode
		fi.vis[i] = fi.weightOf(d, int32(i))
	}
	return fi
}

// weightOf is the Fenwick weight a *visible* line contributes: its visual-row
// count. Under WrapNone (d.rowsOf == nil) that is always 1 — a single nil check,
// so the non-wrap path pays nothing. Under WrapWord it is the line's wrapped-row
// count for its currently-displayed rendering (fold-aware, maintained by the wrap
// projection pass and the fold/unfold weight updates).
func (fi *foldIndex) weightOf(d *Document, li int32) int32 {
	if d.rowsOf == nil {
		return 1
	}
	return d.rowsOf[li]
}

// buildFenwick rebuilds the Fenwick tree from vis in O(n). It reuses the existing
// backing array when the size is unchanged (the common case for fold-state
// rebuilds), avoiding a fresh allocation per rebuild.
func (fi *foldIndex) buildFenwick() {
	n := len(fi.vis)
	if len(fi.bit.tree) == n+1 {
		for i := range fi.bit.tree {
			fi.bit.tree[i] = 0
		}
	} else {
		fi.bit = newFenwick(n)
	}
	for i := 0; i < n; i++ {
		fi.bit.tree[i+1] += fi.vis[i]
		j := (i + 1) + ((i + 1) & -(i + 1))
		if j <= n {
			fi.bit.tree[j] += fi.bit.tree[i+1]
		}
	}
}

// rebuild recomputes hiddenBy/vis from the collapsed bitset in one O(n·depth)
// pass, then rebuilds the Fenwick. Used by defaults / ExpandAll / CollapseAll.
func (fi *foldIndex) rebuild(d *Document) {
	type frame struct {
		node      NodeID
		closeLine int32
		collapsed bool
	}
	var stack []frame
	innermostCollapsed := func() NodeID {
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].collapsed {
				return stack[i].node
			}
		}
		return NoNode
	}
	for li := int32(0); li < int32(len(d.Lines)); li++ {
		for len(stack) > 0 && stack[len(stack)-1].closeLine < li {
			stack = stack[:len(stack)-1]
		}
		hb := innermostCollapsed()
		fi.hiddenBy[li] = hb
		if hb == NoNode {
			fi.vis[li] = fi.weightOf(d, li)
		} else {
			fi.vis[li] = 0
		}
		if f := d.Lines[li].Fold; f != NoNode {
			stack = append(stack, frame{node: f, closeLine: d.Nodes[f].CloseLine, collapsed: fi.collapsed.get(f)})
		}
	}
	fi.buildFenwick()
}

// TotalVisibleRows reports how many display rows are currently visible.
func (fi *foldIndex) TotalVisibleRows() int32 { return fi.bit.total() }

// lineAtRow maps a 0-based visible row to its display-line index. An out-of-range
// row is clamped (row < 0 -> first line, row >= total -> last line) so a misuse
// returns a valid index rather than a fake one-past-last that panics downstream.
func (fi *foldIndex) lineAtRow(row int32) int32 {
	n := int32(len(fi.vis))
	if n == 0 {
		return 0
	}
	if row < 0 {
		row = 0
	}
	li := int32(fi.bit.kth(row + 1))
	if li >= n {
		li = n - 1
	}
	return li
}

// lineAndSubRow maps a 0-based visible visual row to its display line and the
// 0-based sub-row within that line (0 unless the line is soft-wrapped). O(log n).
// Out-of-range rows clamp, matching lineAtRow.
func (fi *foldIndex) lineAndSubRow(visualRow int32) (line, sub int32) {
	n := int32(len(fi.vis))
	if n == 0 {
		return 0, 0
	}
	if visualRow < 0 {
		visualRow = 0
	}
	if total := fi.bit.total(); visualRow >= total {
		visualRow = total - 1
	}
	line = int32(fi.bit.kth(visualRow + 1))
	if line >= n {
		line = n - 1
	}
	return line, visualRow - fi.bit.prefix(int(line))
}

// rowOfLine maps a display line to the visible row it occupies (or, if hidden,
// the row it would occupy). O(log n). An out-of-range line is clamped to the
// valid prefix range rather than indexing the Fenwick tree out of bounds.
func (fi *foldIndex) rowOfLine(line int32) int32 {
	if line < 0 {
		return 0
	}
	if int(line) >= len(fi.vis) {
		return fi.bit.total()
	}
	return fi.bit.prefix(int(line))
}

// fold collapses node. Precondition: node currently visible (see invariant).
func (fi *foldIndex) fold(d *Document, node NodeID) {
	if fi.collapsed.get(node) {
		return
	}
	fi.collapsed.set(node)
	n := &d.Nodes[node]
	for li := n.HeadLine + 1; li <= n.CloseLine; li++ {
		if fi.hiddenBy[li] == NoNode {
			fi.hiddenBy[li] = node
			if fi.vis[li] == 1 {
				fi.vis[li] = 0
				fi.bit.add(int(li), -1)
			}
		}
	}
}

// unfold expands node. Precondition: node currently visible.
func (fi *foldIndex) unfold(d *Document, node NodeID) {
	if !fi.collapsed.get(node) {
		return
	}
	fi.collapsed.clear(node)
	n := &d.Nodes[node]
	for li := n.HeadLine + 1; li <= n.CloseLine; li++ {
		if fi.hiddenBy[li] == node {
			fi.hiddenBy[li] = NoNode
			fi.vis[li] = 1
			fi.bit.add(int(li), 1)
		}
	}
}

// toggle flips the fold state of node.
func (fi *foldIndex) toggle(d *Document, node NodeID) {
	if fi.collapsed.get(node) {
		fi.unfold(d, node)
	} else {
		fi.fold(d, node)
	}
}

// applyDefaults collapses nodes flagged default-collapsed and, if any were, rebuilds
// the projection once (rebuild also builds the Fenwick). It returns whether it
// rebuilt, so the caller can build the all-visible Fenwick itself when there were no
// defaults — keeping construction to a single Fenwick build either way.
func (fi *foldIndex) applyDefaults(d *Document) bool {
	any := false
	for id := range d.Nodes {
		if d.Nodes[id].Flags&flagDefaultCollapsed != 0 {
			fi.collapsed.set(NodeID(id))
			any = true
		}
	}
	if any {
		fi.rebuild(d)
	}
	return any
}

// foldable reports whether node id is a collapsible container.
func foldable(d *Document, id NodeID) bool {
	n := &d.Nodes[id]
	return n.HeadLine >= 0 && n.HeadLine != n.CloseLine && n.Kind.Foldable()
}

// collapseAll collapses every foldable node below the top level (depth >= 1),
// leaving the outermost container expanded so its first level stays visible.
func (fi *foldIndex) collapseAll(d *Document) {
	for id := range d.Nodes {
		nid := NodeID(id)
		if foldable(d, nid) && d.Nodes[id].Depth >= 1 {
			fi.collapsed.set(nid)
		}
	}
	fi.rebuild(d)
}

// expandAll expands every node.
func (fi *foldIndex) expandAll(d *Document) {
	for i := range fi.collapsed.words {
		fi.collapsed.words[i] = 0
	}
	fi.rebuild(d)
}

// revealLine expands every collapsed ancestor that hides line, making it visible.
// Returns true if anything changed. It unfolds incrementally (O(touched lines)
// per ancestor) rather than rebuilding the whole projection.
func (fi *foldIndex) revealLine(d *Document, line int32) bool {
	if line < 0 || int(line) >= len(fi.vis) || fi.vis[line] != 0 {
		return false
	}
	return fi.unfoldAncestors(d, d.Lines[line].Owner)
}

// unfoldAncestors unfolds every collapsed node on the chain from start up to the
// root, OUTERMOST-FIRST. Order is load-bearing: unfold's precondition is that the
// node is currently visible and it only restores lines whose nearest collapsed
// ancestor is that node, so the outermost collapsed ancestor must be unfolded
// before the ones nested inside it.
func (fi *foldIndex) unfoldAncestors(d *Document, start NodeID) bool {
	var collapsed []NodeID // innermost..outermost as we walk toward the root
	for a := start; a != NoNode; a = d.Nodes[a].Parent {
		if fi.collapsed.get(a) {
			collapsed = append(collapsed, a)
		}
	}
	if len(collapsed) == 0 {
		return false
	}
	for i := len(collapsed) - 1; i >= 0; i-- { // outermost first
		fi.unfold(d, collapsed[i])
	}
	return true
}
