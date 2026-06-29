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
