package model

import "testing"

// rawDoc builds a small raw document (one Leaf per line) for projection tests.
func rawDoc(lines ...string) *Document {
	var src []byte
	offs := make([][2]int, len(lines))
	for i, s := range lines {
		start := len(src)
		src = append(src, s...)
		offs[i] = [2]int{start, len(src)}
		src = append(src, '\n')
	}
	b := NewBuilder(src, FormatRaw, 0)
	for _, o := range offs {
		b.Leaf(KindRawLine, o[0], o[1], []Seg{SrcSeg(RolePlain, o[0], o[1])})
	}
	return b.Finish()
}

// TestWeightGeneralizationNoOp pins the M-W0 invariant: under WrapNone the fold
// index keeps rowsOf nil and projects every visible line to exactly one row, so
// generalizing the Fenwick weight from {0,1} to a per-line count is a true no-op
// for the (default) non-wrap path.
func TestWeightGeneralizationNoOp(t *testing.T) {
	d := rawDoc("alpha", "bravo", "charlie", "delta")
	if d.rowsOf != nil {
		t.Fatalf("WrapNone must leave rowsOf nil, got len %d", len(d.rowsOf))
	}
	if got := d.TotalVisibleRows(); got != 4 {
		t.Fatalf("TotalVisibleRows = %d, want 4", got)
	}
	for r := int32(0); r < d.TotalVisibleRows(); r++ {
		li := d.LineAtRow(r)
		if back := d.RowOfLine(li); back != r {
			t.Errorf("row %d -> line %d -> row %d (round-trip broken)", r, li, back)
		}
		if !d.Visible(li) {
			t.Errorf("line %d at row %d reports not visible", li, r)
		}
	}
}
