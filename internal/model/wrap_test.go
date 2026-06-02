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

// TestWrapProjection drives the M-W1 soft-wrap math: enabling a column budget must
// make long lines occupy several visual rows while keeping the projection
// self-consistent — Σ rowsOf(visible) == TotalVisibleRows, visualRow↔(line,sub) is
// a bijection, and WrapBreaks partitions [0,lineLen] with every row ≤ the budget.
func TestWrapProjection(t *testing.T) {
	d := rawDoc(
		"short", // < budget, one row
		"alpha beta gamma delta epsilon zeta omega", // word-wraps
		"0123456789012345678901234567890123456789",  // 40 cols, unbreakable -> char-wrap
		"", // empty -> exactly one row
		"tail",
	)
	const cols = 10
	if d.WrapActive() {
		t.Fatal("WrapActive should be false before SetWrapColumns")
	}
	d.SetWrapColumns([]int{cols})
	if !d.WrapActive() {
		t.Fatal("WrapActive should be true after SetWrapColumns")
	}

	// 1. weights sum to the visual-row total.
	var sum int32
	for li := int32(0); li < int32(d.TotalLines()); li++ {
		if d.Visible(li) {
			sum += d.RowsOfLine(li)
		}
	}
	if sum != d.TotalVisibleRows() {
		t.Fatalf("Σ rowsOf(visible)=%d != TotalVisibleRows=%d", sum, d.TotalVisibleRows())
	}

	// 2. visualRow <-> (line, sub) bijection.
	for r := int32(0); r < d.TotalVisibleRows(); r++ {
		line, sub := d.LineAndSubRowAtRow(r)
		if got := d.FirstVisualRowOfLine(line) + sub; got != r {
			t.Errorf("row %d -> (line %d, sub %d) -> row %d", r, line, sub, got)
		}
		if sub < 0 || sub >= d.RowsOfLine(line) {
			t.Errorf("row %d: sub %d out of [0,%d)", r, sub, d.RowsOfLine(line))
		}
	}

	// 3. WrapBreaks partitions [0,lineLen]; each row ≤ cols; count == RowsOfLine.
	var dst []int32
	for li := int32(0); li < int32(d.TotalLines()); li++ {
		dst = d.WrapBreaks(li, dst[:0])
		n := int32(d.LineRuneLen(li))
		if dst[0] != 0 || dst[len(dst)-1] != n {
			t.Errorf("line %d breaks %v don't span [0,%d]", li, dst, n)
		}
		if int32(len(dst)-1) != d.RowsOfLine(li) {
			t.Errorf("line %d: %d row segments but RowsOfLine=%d", li, len(dst)-1, d.RowsOfLine(li))
		}
		for k := 1; k < len(dst); k++ {
			if dst[k] < dst[k-1] {
				t.Errorf("line %d breaks not monotonic: %v", li, dst)
			}
			if w := dst[k] - dst[k-1]; w > cols {
				t.Errorf("line %d row %d width %d > budget %d", li, k-1, w, cols)
			}
		}
	}

	// The empty line is exactly one visual row.
	if got := d.RowsOfLine(3); got != 1 {
		t.Errorf("empty line RowsOfLine = %d, want 1", got)
	}
	// The 40-col unbreakable line char-wraps into ceil(40/10) = 4 rows.
	if got := d.RowsOfLine(2); got != 4 {
		t.Errorf("40-col unbreakable line RowsOfLine = %d, want 4", got)
	}

	// Disabling wrap restores the one-row-per-line fast path.
	d.SetWrapColumns(nil)
	if d.WrapActive() {
		t.Fatal("WrapActive should be false after SetWrapColumns(nil)")
	}
	if got := d.TotalVisibleRows(); got != 5 {
		t.Errorf("after wrap off, TotalVisibleRows = %d, want 5", got)
	}
}
