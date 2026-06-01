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
