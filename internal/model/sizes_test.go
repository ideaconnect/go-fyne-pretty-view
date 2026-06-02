package model

import (
	"testing"
	"unsafe"
)

// TestArenaSizes locks in the compact struct-of-arrays layout the memory budget
// depends on.
func TestArenaSizes(t *testing.T) {
	if got := unsafe.Sizeof(Node{}); got > 32 {
		t.Errorf("Node is %d bytes (want <= 32)", got)
	}
	if got := unsafe.Sizeof(Line{}); got > 24 {
		t.Errorf("Line is %d bytes (want <= 24)", got)
	}
	if got := unsafe.Sizeof(Segment{}); got > 12 {
		t.Errorf("Segment is %d bytes (want <= 12)", got)
	}
}

// TestSegCountSaturates checks that a pathological line with more segments than a
// uint16 can address saturates its SegCount instead of wrapping to a corrupt
// small value (which would make LineSegs slice an out-of-range / truncated range).
func TestSegCountSaturates(t *testing.T) {
	b := NewBuilder(nil, FormatRaw, 0)
	segs := make([]Seg, maxLineSegs+1000)
	for i := range segs {
		segs[i] = LitSeg(RolePlain, "x")
	}
	b.Leaf(KindRawLine, 0, 0, segs)
	d := b.Finish()

	li := int32(len(d.Lines) - 1) // the leaf we just added
	if d.Lines[li].SegCount != maxLineSegs {
		t.Errorf("SegCount = %d, want saturated %d", d.Lines[li].SegCount, maxLineSegs)
	}
	if got := len(d.LineSegs(li)); got != maxLineSegs {
		t.Errorf("LineSegs len = %d, want %d (must not panic or wrap)", got, maxLineSegs)
	}
}

// TestEmptyDocument constructs the zero document and checks the projection is
// consistent (no panics, no visible rows).
func TestEmptyDocument(t *testing.T) {
	d := EmptyDocument()
	if d.TotalVisibleRows() != 0 {
		t.Errorf("empty document has %d visible rows, want 0", d.TotalVisibleRows())
	}
	if d.TotalLines() != 0 {
		t.Errorf("empty document has %d lines, want 0", d.TotalLines())
	}
}
