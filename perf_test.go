package prettyview

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/parse"

	"fyne.io/fyne/v2"
)

// TestReflowGridSkipsPrefixWalk is the deterministic, renderer-attached guard for
// issue #4: a reflow scrolled deep into a wrapped byte-grid (ASCII) line must walk
// O(visible window) bytes, not O(scroll position). The grid fast path walks zero, so
// debugBytesWalked stays tiny no matter how far down the line is scrolled; before the
// fast path the deep build walked the full start column (hundreds of KB per row).
func TestReflowGridSkipsPrefixWalk(t *testing.T) {
	long := strings.Repeat("abcdefghij", 200_000) // 2 MB single ASCII (grid) line
	pv, win := renderInWindow(t, []byte(`["`+long+`"]`), FormatJSON, 400, 300)
	defer win.Close()
	pv.SetWrap(WrapWord)

	li := pv.doc.LineAtRow(1)
	if !pv.doc.LineIsByteGrid(li) {
		t.Fatal("precondition: the long ASCII line should be a byte grid")
	}
	deep := int(pv.doc.RowOfLine(li)) + int(pv.doc.TotalVisibleRows())/2

	atomic.StoreInt64(&debugBytesWalked, 0)
	pv.r.scrollToOffset(fyne.NewPos(0, pv.met.RowY(deep))) // exactly one reflow
	walked := atomic.LoadInt64(&debugBytesWalked)

	t.Logf("bytes walked one reflow deep into the wrapped grid line = %d", walked)
	if walked > 100_000 {
		t.Errorf("reflow walked %d bytes deep into a grid line — O(scroll position), not O(visible window)", walked)
	}
}

// TestRebuildMatchesSkipsOffWindowMatches is the #101 guard for the per-sub-row match scan:
// a single giant wrapped line carrying thousands of matches must, on one reflow, visit only
// the matches inside the visible sub-rows' windows (binary-search skip + early break), not all
// matches once per visible sub-row.
func TestRebuildMatchesSkipsOffWindowMatches(t *testing.T) {
	long := strings.Repeat("needle ", 5000) // ~5000 matches on ONE logical (string) line
	pv, win := renderInWindow(t, []byte(`["`+long+`"]`), FormatJSON, 400, 300)
	defer win.Close()
	pv.SetWrap(WrapWord)
	pv.Search(SearchQuery{Text: "needle"})
	if _, total, _ := pv.SearchStatus(); total < 1000 {
		t.Fatalf("precondition: expected many matches on the wrapped line, got %d", total)
	}

	atomic.StoreInt64(&debugMatchVisits, 0)
	pv.r.reflow() // exactly one reflow over the few currently-visible sub-rows
	visits := atomic.LoadInt64(&debugMatchVisits)

	t.Logf("rebuildMatches inner-loop visits for one reflow = %d (of %d matches)", visits, len(pv.search.matches))
	if visits > 500 {
		t.Errorf("rebuildMatches visited %d matches in one reflow — O(all matches) per sub-row, not O(visible window)", visits)
	}
}

// TestReflowNonASCIIWalkIsScrollProportional characterizes the documented limitation behind
// #101 (8a): the O(visible-window) reflow fast path requires a byte==column grid, so a line
// containing a multibyte rune takes the decode path and walks O(FirstVisibleCol) per reflow —
// proportional to the horizontal scroll offset, NOT the visible window. This locks the known
// behavior (and DESIGN.md's caveat); a future rune-prefix index that fixes it should update
// this test rather than leave the regression silent.
func TestReflowNonASCIIWalkIsScrollProportional(t *testing.T) {
	long := strings.Repeat("é", 200_000) // 2-byte rune throughout -> NOT a byte grid
	pv, win := renderInWindow(t, []byte(`["`+long+`"]`), FormatJSON, 400, 300)
	defer win.Close()

	li := pv.doc.LineAtRow(1)
	if pv.doc.LineIsByteGrid(li) {
		t.Fatal("precondition: a multibyte line must NOT be a byte grid")
	}
	cs := pv.contentSize()
	atomic.StoreInt64(&debugBytesWalked, 0)
	pv.r.scrollToOffset(fyne.NewPos(cs.Width/2, 0)) // scroll far right on the long line
	walked := atomic.LoadInt64(&debugBytesWalked)
	t.Logf("bytes walked for a non-ASCII line scrolled to mid-width = %d (decode path, O(FirstVisibleCol))", walked)
	if walked == 0 {
		t.Error("expected the non-grid decode path to walk bytes proportional to the scroll offset")
	}
}

// TestReflowReusesWrapBreaks guards the per-frame wrap-break allocation fix: the soft-wrap
// row-build loop must fill the PERSISTENT r.reflowBreaks scratch (reused across reflows),
// not a fresh local that re-allocates the whole line's break list every scroll tick. If it
// regresses to a local, the field is never written and stays empty; the capacity must also
// stay stable across steady reflows.
func TestReflowReusesWrapBreaks(t *testing.T) {
	src := []byte(`["` + strings.Repeat("alpha bravo ", 400) + `"]`) // wraps into many sub-rows
	pv, win := renderInWindow(t, src, FormatJSON, 300, 200)
	defer win.Close()
	pv.SetWrap(WrapWord)
	pv.Refresh()
	if !pv.doc.WrapActive() {
		t.Fatal("precondition: wrap should be active")
	}
	if len(pv.r.reflowBreaks) == 0 {
		t.Fatal("reflow did not populate the persistent reflowBreaks scratch — per-frame wrap-break alloc regressed")
	}
	c0 := cap(pv.r.reflowBreaks)
	for i := 0; i < 10; i++ {
		pv.r.reflow()
	}
	if cap(pv.r.reflowBreaks) != c0 {
		t.Errorf("reflowBreaks capacity changed %d -> %d across steady reflows (re-allocating per frame)", c0, cap(pv.r.reflowBreaks))
	}
}

// TestSearchNextZeroAlloc guards that SearchNext does no allocation on the model-only
// navigation path (pv.r == nil, so reveal/reflow is skipped): wrapping the active
// index over the existing match slice is pure arithmetic.
func TestSearchNextZeroAlloc(t *testing.T) {
	pv := New() // no renderer
	pv.doc = parse.Parse([]byte(`{"a":"x","b":"x","c":"x","d":"x"}`), FormatJSON, 0)
	pv.runSearch(SearchQuery{Text: `"x"`})
	if _, total, _ := pv.SearchStatus(); total < 2 {
		t.Fatalf("need multiple matches, got %d", total)
	}
	if n := testing.AllocsPerRun(100, func() { pv.SearchNext() }); n != 0 {
		t.Errorf("SearchNext allocated %.1f times on the model-only path, want 0", n)
	}
}

// TestRevealLineMatchesRebuild verifies the incremental reveal leaves the
// projection identical to a full rebuild and does not disturb unrelated folds.
func TestRevealLineMatchesRebuild(t *testing.T) {
	d := parse.Parse([]byte(`{"a":{"b":{"c":"deep"}},"sib":{"x":1,"y":2}}`), FormatJSON, 0)
	sib := findFoldHead(d, `"sib"`)
	b := findFoldHead(d, `"b"`)
	a := findFoldHead(d, `"a"`)
	// Collapse innermost-first so each node is visible when collapsed.
	d.Fold(sib)
	d.Fold(b)
	d.Fold(a)

	deep := lineContaining(d, "deep")
	if deep < 0 || d.Visible(deep) {
		t.Fatal("precondition: deep line should be hidden")
	}

	d.RevealLine(deep)
	got := visSnapshot(d)

	if !d.Visible(deep) {
		t.Error("deep line not revealed")
	}
	if !d.Collapsed(sib) {
		t.Error("revealing a deep line wrongly expanded an unrelated sibling")
	}

	// A full rebuild from the same collapsed bitset must yield identical visibility.
	d.Rebuild()
	for i, v := range got {
		if v != d.Visible(int32(i)) {
			t.Fatalf("incremental reveal diverged from rebuild at line %d", i)
		}
	}
}

// TestReflowBuildsEachRowOnce guards against the double-build regression: one
// reflow must build each visible row exactly once, not twice.
func TestReflowBuildsEachRowOnce(t *testing.T) {
	src, err := os.ReadFile("testdata/big.json")
	if err != nil {
		t.Fatal(err)
	}
	pv, win := renderInWindow(t, src, FormatJSON, 800, 600)
	defer win.Close()

	visible := pv.r.lastRow - pv.r.firstRow + 1
	atomic.StoreInt64(&debugRowBuilds, 0)
	pv.r.scrollToOffset(fyne.NewPos(0, 4000)) // one reflow at a fresh position
	builds := atomic.LoadInt64(&debugRowBuilds)

	t.Logf("builds=%d for ~%d visible rows", builds, visible)
	// Allow a small margin (recycled vs newly-shown rows), but never ~2x.
	if builds > int64(visible)+2 {
		t.Errorf("reflow built rows %d times for %d visible rows — double-build regressed", builds, visible)
	}
}
