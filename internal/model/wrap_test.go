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

// nestedDoc builds a small foldable document: an outer object holding a leaf with
// a long (wrapping) value and an inner foldable object. Used for fold/unfold-under-
// wrap tests where a collapsed head's summary wraps differently than its expanded
// head, and where folding a parent hides a multi-visual-row leaf.
func nestedDoc() *Document {
	b := NewBuilder(nil, FormatJSON, 0)
	b.Open(KindObject, 0, []Seg{LitSeg(RoleKey, `"root"`), LitSeg(RolePunct, ": {")})
	b.Leaf(KindKeyValue, 0, 0, []Seg{
		LitSeg(RoleKey, `"k"`), LitSeg(RolePunct, ": "),
		LitSeg(RoleString, `"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`), // 30 a's: wraps to several rows
	})
	b.Open(KindObject, 0, []Seg{LitSeg(RoleKey, `"inner"`), LitSeg(RolePunct, ": {")})
	b.Leaf(KindKeyValue, 0, 0, []Seg{LitSeg(RoleKey, `"x"`), LitSeg(RolePunct, ": "), LitSeg(RoleNumber, "1")})
	b.Leaf(KindKeyValue, 0, 0, []Seg{LitSeg(RoleKey, `"y"`), LitSeg(RolePunct, ": "), LitSeg(RoleNumber, "2")})
	b.Close(0, []Seg{LitSeg(RolePunct, "}")})
	b.Close(0, []Seg{LitSeg(RolePunct, "}")})
	return b.Finish()
}

func snapshotRows(d *Document) [][2]int32 {
	out := make([][2]int32, d.TotalVisibleRows())
	for r := int32(0); r < d.TotalVisibleRows(); r++ {
		line, sub := d.LineAndSubRowAtRow(r)
		out[r] = [2]int32{line, sub}
	}
	return out
}

func equalRows(a, b [][2]int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestWrapFoldUnfoldRoundTrip drives M-W2: under soft-wrap, incremental fold/unfold
// must apply per-line visual-row weight deltas (a hidden multi-row line removes all
// its rows) and re-weight the toggled head (its collapsed summary may wrap to a
// different row count). A full fold/unfold cycle must restore the projection
// exactly, and a single incremental fold must match a from-scratch rebuild.
func TestWrapFoldUnfoldRoundTrip(t *testing.T) {
	d := nestedDoc()
	const cols = 8
	d.SetWrapColumns([]int{cols})

	wantTotal := d.TotalVisibleRows()
	wantRows := snapshotRows(d)

	var dst []int32
	for id := int32(0); id < int32(len(d.Nodes)); id++ {
		if !foldable(d, id) {
			continue
		}
		d.Fold(id)
		// While collapsed, the head's weight must equal its collapsed WrapBreaks rows.
		head := d.Nodes[id].HeadLine
		dst = d.WrapBreaks(head, dst[:0])
		if got, want := d.RowsOfLine(head), int32(len(dst)-1); got != want {
			t.Errorf("collapsed head (node %d) weight %d != WrapBreaks rows %d", id, got, want)
		}
		d.Unfold(id)
	}
	if got := d.TotalVisibleRows(); got != wantTotal {
		t.Errorf("after fold/unfold cycle TotalVisibleRows = %d, want %d", got, wantTotal)
	}
	if !equalRows(snapshotRows(d), wantRows) {
		t.Errorf("projection not restored after fold/unfold cycle")
	}

	// Incremental fold of the inner (depth-1) object must equal a full rebuild.
	var inner int32 = -1
	for id := int32(0); id < int32(len(d.Nodes)); id++ {
		if foldable(d, id) && d.Nodes[id].Depth == 1 {
			inner = id
			break
		}
	}
	if inner < 0 {
		t.Fatal("no depth-1 foldable node in fixture")
	}
	d.Fold(inner)
	incTotal := d.TotalVisibleRows()
	incRows := snapshotRows(d)
	d.Rebuild() // from-scratch projection of the same collapsed state
	if d.TotalVisibleRows() != incTotal || !equalRows(snapshotRows(d), incRows) {
		t.Errorf("incremental fold projection != rebuild projection")
	}
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
		if got := d.RowOfLine(line) + sub; got != r {
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
