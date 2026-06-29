package model

import "testing"

// commentLineOf returns the display line owned by the first KindComment node, or -1.
func commentLineOf(d *Document) int32 {
	for li := int32(0); li < int32(len(d.Lines)); li++ {
		if o := d.Lines[li].Owner; o != NoNode && int(o) < len(d.Nodes) && d.Nodes[o].Kind == KindComment {
			return li
		}
	}
	return -1
}

// containerByDepth returns the first foldable container node at the given nesting depth (the
// nestedDoc fixture has one KindObject at depth 0 — "root" — and one at depth 1 — "inner").
func containerByDepth(d *Document, depth uint8) NodeID {
	for id := NodeID(0); int(id) < len(d.Nodes); id++ {
		if d.Nodes[id].Kind == KindObject && d.Nodes[id].Depth == depth {
			return id
		}
	}
	return NoNode
}

// TestFoldProjectionAPI gives the model's public fold/projection API direct in-package teeth. These
// delegators (Fold/Unfold/Toggle/Collapsed/ExpandAll/CollapseAll/CollapseToDepth/ExpandToDepth/
// RevealLine/VisibleLine) were exercised only transitively by the root widget's tests, so their
// mutants landed in gremlins' "Not covered" bucket — high cross-package coverage with no direct
// teeth. This asserts the exact projection behavior so a mutation (a swapped fold/unfold, a wrong
// depth comparison, a dropped reveal) is caught here, in the package that owns the logic.
func TestFoldProjectionAPI(t *testing.T) {
	d := nestedDoc()
	total := int(d.TotalVisibleRows())
	if total != d.TotalLines() {
		t.Fatalf("fresh doc must be fully expanded: TotalVisibleRows %d != TotalLines %d", total, d.TotalLines())
	}
	outer := containerByDepth(d, 0) // "root"  (depth 0)
	inner := containerByDepth(d, 1) // "inner" (depth 1)
	if outer == NoNode || inner == NoNode {
		t.Fatalf("fixture missing outer/inner containers (outer=%d inner=%d)", outer, inner)
	}

	// Fold / Collapsed / Unfold / Toggle round-trip.
	if d.Collapsed(outer) {
		t.Error("outer must start expanded")
	}
	d.Fold(outer)
	if !d.Collapsed(outer) {
		t.Error("Fold(outer) must mark it collapsed")
	}
	if folded := int(d.TotalVisibleRows()); folded >= total {
		t.Errorf("folding outer must hide rows: %d not < %d", folded, total)
	}
	d.Unfold(outer)
	if d.Collapsed(outer) || int(d.TotalVisibleRows()) != total {
		t.Errorf("Unfold(outer) must fully restore (collapsed=%v rows=%d want %d)", d.Collapsed(outer), d.TotalVisibleRows(), total)
	}
	d.Toggle(outer)
	if !d.Collapsed(outer) {
		t.Error("Toggle on an expanded node must collapse it")
	}
	d.Toggle(outer)
	if d.Collapsed(outer) {
		t.Error("Toggle on a collapsed node must expand it")
	}

	// CollapseAll collapses foldable nodes at depth >= 1 only (leaves the depth-0 outer open).
	d.CollapseAll()
	if d.Collapsed(outer) {
		t.Error("CollapseAll must not collapse the depth-0 container")
	}
	if !d.Collapsed(inner) {
		t.Error("CollapseAll must collapse the depth-1 container")
	}
	innerChild := d.Nodes[inner].HeadLine + 1 // inner's first child line, hidden while inner is collapsed
	if d.Visible(innerChild) {
		t.Errorf("inner child line %d must be hidden after CollapseAll", innerChild)
	}
	if got, want := d.VisibleLine(innerChild), d.Nodes[inner].HeadLine; got != want {
		t.Errorf("VisibleLine(hidden %d) = %d, want the collapsed inner head %d", innerChild, got, want)
	}
	if !d.RevealLine(innerChild) {
		t.Error("RevealLine of a hidden line must expand its ancestor and return true")
	}
	if !d.Visible(innerChild) {
		t.Error("after RevealLine the line must be visible")
	}
	if d.RevealLine(innerChild) {
		t.Error("RevealLine of an already-visible line must return false")
	}

	// ExpandAll clears every collapse.
	d.ExpandAll()
	if d.Collapsed(inner) || d.Collapsed(outer) || int(d.TotalVisibleRows()) != total {
		t.Errorf("ExpandAll must restore all %d rows with nothing collapsed", total)
	}
	// VisibleLine of an already-visible line is the line itself; an out-of-range line is returned
	// unchanged (the guard), matching its sibling accessors.
	if got := d.VisibleLine(innerChild); got != innerChild {
		t.Errorf("VisibleLine(visible %d) = %d, want the line itself", innerChild, got)
	}
	if got := d.VisibleLine(-1); got != -1 {
		t.Errorf("VisibleLine(out-of-range -1) = %d, want -1 unchanged", got)
	}

	// Depth-scoped fold/expand: collapse >= depth, expand < depth.
	d.CollapseToDepth(1)
	if !d.Collapsed(inner) || d.Collapsed(outer) {
		t.Error("CollapseToDepth(1) must collapse the depth-1 inner and leave the depth-0 outer open")
	}
	d.Fold(outer)      // now both collapsed
	d.ExpandToDepth(1) // expands foldables at depth < 1 → only the depth-0 outer
	if d.Collapsed(outer) || !d.Collapsed(inner) {
		t.Error("ExpandToDepth(1) must expand the depth-0 outer and leave the depth-1 inner collapsed")
	}

	if d.ProjectionBytes() <= 0 {
		t.Error("ProjectionBytes must report a positive footprint for a non-empty projection")
	}
}

// TestAssembleAndDisplayLine covers the model's two line-byte accessors directly. AssembleLine
// returns a line's expanded segment bytes (== LineString); AppendDisplayLine returns the fold-aware
// displayed bytes and, with restoreTabs, rewrites each interned space-pad (a raw line's tab stop)
// back to a single '\t' so a copy round-trips the source tab instead of the expanded spaces.
func TestAssembleAndDisplayLine(t *testing.T) {
	d := nestedDoc()
	for li := int32(0); li < int32(d.TotalLines()); li++ {
		if got, want := string(d.AssembleLine(li, nil)), d.LineString(li); got != want {
			t.Errorf("AssembleLine(%d) = %q, want LineString %q", li, got, want)
		}
		// With wrap off and nothing collapsed, the displayed bytes equal the expanded bytes.
		if got, want := string(d.AppendDisplayLine(li, nil, false)), d.DisplayString(li); got != want {
			t.Errorf("AppendDisplayLine(%d) = %q, want DisplayString %q", li, got, want)
		}
	}

	// restoreTabs: build a raw line "a" + a 4-space interned pad (BufAux RolePlain) + "b". With
	// restoreTabs the pad is emitted as one '\t'; without it, the expanded spaces are kept.
	b := NewBuilder([]byte("a\tb"), FormatRaw, 0)
	b.Leaf(KindRawLine, 0, 3, []Seg{SrcSeg(RolePlain, 0, 1), LitSeg(RolePlain, "    "), SrcSeg(RolePlain, 2, 3)})
	rd := b.Finish()
	if got, want := string(rd.AppendDisplayLine(0, nil, false)), "a    b"; got != want {
		t.Errorf("AppendDisplayLine(restoreTabs=false) = %q, want %q", got, want)
	}
	if got, want := string(rd.AppendDisplayLine(0, nil, true)), "a\tb"; got != want {
		t.Errorf("AppendDisplayLine(restoreTabs=true) = %q, want the tab restored %q", got, want)
	}
}

// TestAppendPrettyLineCommentVerbatim gives the #121/#123 comment branch of AppendPrettyLine direct
// in-package coverage (the root-package Text/copy tests don't count toward this package's mutation
// efficacy). A JSONC comment's display segment escapes its interior newline to a one-row "\n", but
// AppendPrettyLine must emit the node's SOURCE bytes verbatim (real newline) and record NO span.
func TestAppendPrettyLineCommentVerbatim(t *testing.T) {
	src := []byte("/* a\nb */") // a two-line block comment
	b := NewBuilder(src, FormatJSONC, 0)
	b.Leaf(KindComment, 0, len(src), []Seg{LitSeg(RoleComment, `/* a\nb */`)}) // display-escaped segment
	d := b.Finish()

	cli := commentLineOf(d)
	if cli < 0 {
		t.Fatal("fixture has no comment line")
	}
	spanCalls := 0
	out := d.AppendPrettyLine(cli, 2, []byte("X"), func(uint32, uint32, int) { spanCalls++ })
	if got, want := string(out), "X  "+string(src); got != want {
		t.Errorf("comment line: got %q, want %q (existing buf + indent + verbatim source)", got, want)
	}
	if spanCalls != 0 {
		t.Errorf("comment line recorded %d caret spans, want 0 (keeps Reformat spans ascending)", spanCalls)
	}
}

// TestAppendPrettyLineMarkupCommentNotVerbatim pins the #123-review format guard: a non-JSONC (e.g.
// XML) comment line must NOT take the verbatim-from-source branch. Its display segment is the
// whitespace-collapsed canonical form, which AppendPrettyLine must emit (matching Reformat and the
// on-screen display) — emitting the raw source instead would diverge Text/copy from both.
func TestAppendPrettyLineMarkupCommentNotVerbatim(t *testing.T) {
	src := []byte("<!--  x  y  -->") // raw source has non-canonical inner whitespace
	b := NewBuilder(src, FormatXML, 0)
	b.Leaf(KindComment, 0, len(src), []Seg{LitSeg(RoleComment, "<!-- x y -->")}) // collapsed display seg
	d := b.Finish()
	cli := commentLineOf(d)
	if cli < 0 {
		t.Fatal("fixture has no comment line")
	}
	if got, want := string(d.AppendPrettyLine(cli, 0, nil, nil)), "<!-- x y -->"; got != want {
		t.Errorf("markup comment: got %q, want the collapsed display segment %q (not the raw source)", got, want)
	}
}

// TestAppendPrettyLineNormalLine covers the non-comment branch directly: the indent is emitted, each
// segment's bytes are appended in order, and spanCb fires once per BufSrc segment with offsets that
// point into the appended region (the caret-remap contract).
func TestAppendPrettyLineNormalLine(t *testing.T) {
	src := []byte(`"k":1`)
	b := NewBuilder(src, FormatJSON, 0)
	// One leaf line: a BufSrc key segment + a BufSrc number segment, plus an interned BufAux ": ".
	b.Leaf(KindKeyValue, 0, len(src), []Seg{
		SrcSeg(RoleKey, 0, 3),    // "k"
		LitSeg(RolePunct, ": "),  // synthesized (BufAux) — must NOT trigger spanCb
		SrcSeg(RoleNumber, 4, 5), // 1
	})
	d := b.Finish()

	var got [][3]int // (srcStart, srcEnd, outStart) per BufSrc span
	out := d.AppendPrettyLine(0, 2, nil, func(s, e uint32, o int) {
		got = append(got, [3]int{int(s), int(e), o})
	})
	if want := `  "k": 1`; string(out) != want {
		t.Errorf("normal line: got %q, want %q", string(out), want)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 BufSrc spans (the BufAux punct must not callback), got %d: %v", len(got), got)
	}
	if got[0] != [3]int{0, 3, 2} { // "k" at out offset 2 (after the 2-space indent)
		t.Errorf("first span = %v, want [0 3 2]", got[0])
	}
	if got[1][0] != 4 || got[1][1] != 5 { // the number's source range
		t.Errorf("second span src range = [%d %d], want [4 5]", got[1][0], got[1][1])
	}
}
