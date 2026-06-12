package model

import "testing"

// TestFoldIndexClampsOutOfRange covers the defensive clamps in lineAtRow / lineAndSubRow /
// rowOfLine: a negative or past-the-end argument returns a valid in-range index rather
// than indexing the Fenwick tree out of bounds.
func TestFoldIndexClampsOutOfRange(t *testing.T) {
	d := &Document{Lines: make([]Line, 4)}
	fi := newFoldIndex(d)
	fi.buildFenwick()
	last := int32(len(fi.vis)) - 1
	total := fi.bit.total()

	if got := fi.lineAtRow(-1); got != 0 {
		t.Errorf("lineAtRow(-1) = %d, want 0 (clamped)", got)
	}
	if got := fi.lineAtRow(total + 100); got != last {
		t.Errorf("lineAtRow past end = %d, want last line %d", got, last)
	}
	if l, s := fi.lineAndSubRow(-5); l != 0 || s != 0 {
		t.Errorf("lineAndSubRow(-5) = (%d,%d), want (0,0)", l, s)
	}
	if l, _ := fi.lineAndSubRow(total + 100); l != last {
		t.Errorf("lineAndSubRow past end -> line %d, want last %d", l, last)
	}
	if got := fi.rowOfLine(-1); got != 0 {
		t.Errorf("rowOfLine(-1) = %d, want 0", got)
	}
	if got := fi.rowOfLine(9999); got != total {
		t.Errorf("rowOfLine past end = %d, want total %d", got, total)
	}
}

// TestFoldIndexEmptyDocument covers the n==0 guards: every accessor on a line-less
// document returns the zero position instead of panicking.
func TestFoldIndexEmptyDocument(t *testing.T) {
	d := &Document{} // no lines at all
	fi := newFoldIndex(d)
	fi.buildFenwick()

	if got := fi.lineAtRow(0); got != 0 {
		t.Errorf("empty lineAtRow(0) = %d, want 0", got)
	}
	if l, s := fi.lineAndSubRow(0); l != 0 || s != 0 {
		t.Errorf("empty lineAndSubRow(0) = (%d,%d), want (0,0)", l, s)
	}
}
