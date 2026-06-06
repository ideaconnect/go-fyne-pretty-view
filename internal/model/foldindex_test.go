package model

import "testing"

// TestLineAndSubRowAllHidden pins the clamp contract for the (currently
// unreachable through the public fold projection) state where lines exist but
// every line is hidden, total() == 0. lineAndSubRow must agree with lineAtRow —
// the last line, sub-row 0 — and must never return a negative sub-row.
func TestLineAndSubRowAllHidden(t *testing.T) {
	d := &Document{Lines: make([]Line, 3)}
	fi := newFoldIndex(d)
	fi.buildFenwick()

	// Sanity: all lines visible -> row 0 maps to line 0, sub 0.
	if line, sub := fi.lineAndSubRow(0); line != 0 || sub != 0 {
		t.Fatalf("all-visible lineAndSubRow(0) = (%d,%d), want (0,0)", line, sub)
	}

	// Force the all-hidden state directly (the public API can't produce it).
	for i := range fi.vis {
		fi.vis[i] = 0
	}
	fi.buildFenwick()
	if total := fi.bit.total(); total != 0 {
		t.Fatalf("precondition not met: total = %d, want 0", total)
	}

	gotLine, gotSub := fi.lineAndSubRow(0)
	wantLine := fi.lineAtRow(0) // the accessor we must match
	if gotSub < 0 {
		t.Errorf("all-hidden lineAndSubRow(0) sub = %d, want non-negative", gotSub)
	}
	if gotLine != wantLine || gotSub != 0 {
		t.Errorf("all-hidden lineAndSubRow(0) = (%d,%d), want (%d,0) to match lineAtRow", gotLine, gotSub, wantLine)
	}
}
