package model

import (
	"strings"
	"testing"
)

// These give the model's construction API direct in-package teeth. The Builder is heavily exercised
// by the parse package's tests, but parse mutation does not mutate model code — so these methods
// (AppendComma, Depth, LastLine, closeDangling/closeBraceFor, the pooled NewPooledBuilder/
// ResetBuilder, MarkRawText/IsRawText, childrenSummary) sat in gremlins' "Not covered" bucket: real
// logic with no direct mutation measurement. The assertions below pin their exact behavior.

func TestBuilderConstruction(t *testing.T) {
	b := NewBuilder([]byte(`{"a":1,"b":2}`), FormatJSON, 0)
	if got := b.Depth(); got != 0 {
		t.Errorf("Depth at the root = %d, want 0", got)
	}
	obj := b.Open(KindObject, 0, []Seg{LitSeg(RolePunct, "{")})
	if got := b.Depth(); got != 1 {
		t.Errorf("Depth inside the open object = %d, want 1", got)
	}
	m1 := b.Leaf(KindKeyValue, 1, 6, []Seg{LitSeg(RoleKey, `"a"`), LitSeg(RolePunct, ": "), LitSeg(RoleNumber, "1")})
	b.AppendComma(b.LastLine(m1)) // trailing comma on the first member's line
	b.Leaf(KindKeyValue, 7, 12, []Seg{LitSeg(RoleKey, `"b"`), LitSeg(RolePunct, ": "), LitSeg(RoleNumber, "2")})
	b.Close(13, []Seg{LitSeg(RolePunct, "}")})
	if got := b.Depth(); got != 0 {
		t.Errorf("Depth after Close = %d, want 0", got)
	}
	if got, want := b.LastLine(obj), int32(3); got != want { // root has no line; obj head=0, m1=1, m2=2, close=3
		t.Errorf("LastLine(obj) = %d, want its close line %d", got, want)
	}

	d := b.Finish()
	if len(d.Nodes) != 4 {
		t.Fatalf("node count = %d, want 4 (root, object, 2 members)", len(d.Nodes))
	}
	if got := d.Nodes[obj].ChildCount; got != 2 {
		t.Errorf("object ChildCount = %d, want 2", got)
	}
	// AppendComma must have placed the "," on member 1's line, contiguous with its tokens.
	if got := d.LineString(d.Nodes[m1].HeadLine); !strings.HasSuffix(got, ",") {
		t.Errorf("member 1 line = %q, want a trailing comma from AppendComma", got)
	}
	if got := d.LineString(d.Nodes[m1].HeadLine + 1); strings.HasSuffix(got, ",") {
		t.Errorf("member 2 line = %q, must NOT have a comma (none was appended)", got)
	}
}

func TestBuilderClosesDangling(t *testing.T) {
	// A tolerant parser may stop with a container still open; Finish must force-close it
	// (closeDangling -> closeBraceFor) so the model stays well-formed with a real close line.
	b := NewBuilder([]byte("{"), FormatJSON, 0)
	obj := b.Open(KindObject, 0, []Seg{LitSeg(RolePunct, "{")})
	d := b.Finish() // object never explicitly closed
	if d.Nodes[obj].CloseLine == d.Nodes[obj].HeadLine {
		t.Fatal("a dangling object must get a synthesized close line (closeDangling)")
	}
	if got := d.LineString(d.Nodes[obj].CloseLine); got != "}" {
		t.Errorf("dangling object's synthesized close = %q, want closeBraceFor(KindObject) %q", got, "}")
	}

	// closeBraceFor varies by kind: array -> "]", element -> "…" (truncation marker), else "}".
	for kind, want := range map[Kind]string{KindArray: "]", KindElement: "…", KindObject: "}"} {
		if got := closeBraceFor(kind); got != want {
			t.Errorf("closeBraceFor(%v) = %q, want %q", kind, got, want)
		}
	}
}

func TestChildrenSummary(t *testing.T) {
	cases := map[int]string{0: "0 children", 1: "1 child", 2: "2 children", 6: "6 children"}
	for n, want := range cases {
		if got := childrenSummary(n); got != want {
			t.Errorf("childrenSummary(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPooledBuilderReuse(t *testing.T) {
	// NewPooledBuilder + ResetBuilder back the editable-mode EditPool: a second build into the same
	// Builder (after Reset) must produce a correct, independent Document from reused arenas.
	b := NewPooledBuilder([]byte("x"), FormatRaw, 0)
	b.Leaf(KindRawLine, 0, 1, []Seg{SrcSeg(RolePlain, 0, 1)})
	if d := b.Finish(); d.TotalLines() != 1 || d.LineString(0) != "x" {
		t.Fatalf("first pooled build: lines=%d line0=%q, want 1 line %q", d.TotalLines(), d.LineString(0), "x")
	}

	b.ResetBuilder([]byte("yz"), FormatRaw, 0)
	b.Leaf(KindRawLine, 0, 2, []Seg{SrcSeg(RolePlain, 0, 2)})
	d := b.Finish()
	if d.TotalLines() != 1 || d.LineString(0) != "yz" {
		t.Errorf("after ResetBuilder: lines=%d line0=%q, want 1 line %q", d.TotalLines(), d.LineString(0), "yz")
	}
	if len(d.Nodes) != 2 { // root + the one raw leaf; the prior build's nodes must be cleared
		t.Errorf("after ResetBuilder node count = %d, want 2 (root + leaf), prior build not cleared?", len(d.Nodes))
	}
}

func TestMarkRawTextRoundTrip(t *testing.T) {
	b := NewBuilder([]byte("x"), FormatHTML, 0)
	raw := b.Leaf(KindRawLine, 0, 1, []Seg{SrcSeg(RolePlain, 0, 1)})
	b.MarkRawText(raw)
	d := b.Finish()
	if !d.IsRawText(raw) {
		t.Error("IsRawText must report a node flagged by MarkRawText")
	}
	if d.IsRawText(NoNode) || d.IsRawText(NodeID(len(d.Nodes))) {
		t.Error("IsRawText must be false for NoNode and out-of-range ids")
	}
}
