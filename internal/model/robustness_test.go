package model

import "testing"

// TestLineAtRowClampsToLastVisible is the #102 regression: an out-of-range row must clamp to
// the last VISIBLE line, not walk past the Fenwick total and return a hidden trailing line.
// Collapsing the root hides every line but its head, so a hidden trailing line exists to be
// wrongly returned.
func TestLineAtRowClampsToLastVisible(t *testing.T) {
	d := nestedDoc()
	d.Fold(0) // collapse the root container: only its head line stays visible
	total := d.TotalVisibleRows()
	if total < 1 {
		t.Fatalf("expected the head line visible after folding the root, total=%d", total)
	}
	last := d.LineAtRow(total - 1) // the last in-range row's line
	if !d.Visible(last) {
		t.Fatalf("last in-range line %d should be visible", last)
	}
	for _, row := range []int32{total, total + 1, total + 100} {
		got := d.LineAtRow(row)
		if got != last {
			t.Errorf("LineAtRow(%d) = %d, want %d (last visible line)", row, got, last)
		}
		if !d.Visible(got) {
			t.Errorf("LineAtRow(%d) returned hidden line %d", row, got)
		}
	}
}

// TestSetWrapColumnsEmptyDocActivatesWrap is the #102 regression for the nil-reslice trap: on
// a zero-line document, enabling wrap left rowsOf nil (cap(nil) < 0 is false → nil[:0] is nil),
// so WrapActive() stayed false despite the caller enabling wrap.
func TestSetWrapColumnsEmptyDocActivatesWrap(t *testing.T) {
	d := EmptyDocument()
	if d.WrapActive() {
		t.Fatal("a fresh empty document must not be wrapping")
	}
	d.SetWrapColumns([]int{40})
	if !d.WrapActive() {
		t.Error("SetWrapColumns(non-empty) must enable wrap even on a zero-line document (#102)")
	}
	d.SetWrapColumns(nil)
	if d.WrapActive() {
		t.Error("SetWrapColumns(nil) must disable wrap")
	}
}
