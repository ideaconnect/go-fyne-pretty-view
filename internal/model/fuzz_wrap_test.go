package model

import (
	"strings"
	"testing"
)

// FuzzWrapPartition generalizes TestWrapProjection (#72): for arbitrary content and a random
// column budget, the soft-wrap projection must keep its invariants — Σ RowsOfLine ==
// TotalVisibleRows, the visualRow<->(line,sub) bijection holds with sub in range, and no
// visual row's column span exceeds the budget.
func FuzzWrapPartition(f *testing.F) {
	f.Add("hello world foo bar baz", 8)
	f.Add("a\nbb\nccc\n", 3)
	f.Add("0123456789012345678901234567890", 7)
	f.Fuzz(func(t *testing.T, content string, cols int) {
		if len(content) > 4000 {
			return
		}
		if cols < 0 {
			cols = -cols
		}
		cols = cols%20 + 2 // a sane positive budget, 2..21
		d := rawDoc(strings.Split(content, "\n")...)
		budgets := make([]int, int(d.MaxDepth)+1)
		for i := range budgets {
			budgets[i] = cols
		}
		d.SetWrapColumns(budgets)
		if !d.WrapActive() {
			return
		}
		var sum int32
		for li := int32(0); li < int32(d.TotalLines()); li++ {
			sum += d.RowsOfLine(li)
		}
		if sum != d.TotalVisibleRows() {
			t.Fatalf("Σ rowsOf=%d != TotalVisibleRows=%d (cols=%d)", sum, d.TotalVisibleRows(), cols)
		}
		for r := int32(0); r < d.TotalVisibleRows(); r++ {
			line, sub := d.LineAndSubRowAtRow(r)
			if got := d.RowOfLine(line) + sub; got != r {
				t.Fatalf("row %d -> (line %d, sub %d) -> row %d (cols=%d)", r, line, sub, got, cols)
			}
			if sub < 0 || sub >= d.RowsOfLine(line) {
				t.Fatalf("row %d: sub %d out of [0,%d) (cols=%d)", r, sub, d.RowsOfLine(line), cols)
			}
		}
		for li := int32(0); li < int32(d.TotalLines()); li++ {
			breaks := d.WrapBreaks(li, nil)
			for k := 0; k+1 < len(breaks); k++ {
				if span := breaks[k+1] - breaks[k]; span > int32(cols) {
					t.Fatalf("line %d row %d span %d > budget %d", li, k, span, cols)
				}
			}
		}
	})
}
