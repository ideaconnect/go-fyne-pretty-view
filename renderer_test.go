package prettyview

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// renderInWindow puts a PrettyView in a test window of the given size and forces
// a layout pass, returning the widget ready for inspection.
func renderInWindow(t *testing.T, src []byte, format Format, w, h float32) (*PrettyView, fyne.Window) {
	t.Helper()
	test.NewApp()
	pv := NewWithData(src, format)
	win := test.NewWindow(pv)
	win.Resize(fyne.NewSize(w, h))
	pv.Refresh()
	if pv.r == nil {
		t.Fatal("renderer was not created")
	}
	return pv, win
}

func TestVirtualizationRowCount(t *testing.T) {
	src, err := os.ReadFile("testdata/big.json")
	if err != nil {
		t.Fatal(err)
	}
	pv, win := renderInWindow(t, src, FormatJSON, 800, 600)
	defer win.Close()

	total := int(pv.doc.TotalVisibleRows())
	live := len(pv.r.live)
	bound := int(600/pv.met.RowH) + 4
	t.Logf("total visible rows=%d, live row widgets=%d, viewport bound=%d, rowH=%.1f",
		total, live, bound, pv.met.RowH)

	if total < 1000 {
		t.Fatalf("big.json should have many rows, got %d", total)
	}
	if live == 0 {
		t.Fatal("no rows were rendered")
	}
	if live > bound {
		t.Errorf("live rows %d exceed viewport bound %d — virtualization is broken", live, bound)
	}
}

func TestScrollRecyclesRows(t *testing.T) {
	src, err := os.ReadFile("testdata/big.json")
	if err != nil {
		t.Fatal(err)
	}
	pv, win := renderInWindow(t, src, FormatJSON, 800, 600)
	defer win.Close()

	bound := int(600/pv.met.RowH) + 4

	// Scroll through the whole document in viewport-sized steps; the live row
	// count must stay bounded the entire time.
	total := int(pv.doc.TotalVisibleRows())
	maxLive := 0
	for y := float32(0); y < float32(total)*pv.met.RowH; y += 500 {
		pv.r.scrollToOffset(fyne.NewPos(0, y))
		if n := len(pv.r.live); n > maxLive {
			maxLive = n
		}
	}
	t.Logf("max live rows while scrolling entire 7.5MB doc = %d (bound %d)", maxLive, bound)
	if maxLive > bound {
		t.Errorf("live rows peaked at %d, exceeding bound %d", maxLive, bound)
	}
}

// TestPooledRowStartsHidden is the unit-level guard for the "blank first reflow"
// bug: a row taken fresh from the pool must start hidden, so the reflow's Show()
// transitions it visible and fires the row renderer's build(). A visible-by-default
// row would make Show() a no-op and render blank on first appearance.
func TestPooledRowStartsHidden(t *testing.T) {
	test.NewApp()
	pv := New()
	r := pv.CreateRenderer().(*prettyViewRenderer)
	rw := r.rowPool.Get().(*rowWidget)
	if rw.Visible() {
		t.Error("a fresh pooled row must start hidden so reflow's Show() triggers build()")
	}
}

// TestFirstReflowBuildsFreshRows drives exactly one sized reflow against a brand-
// new renderer (fresh pool) and asserts every visible row was actually built —
// counted via debugRowBuilds, which is robust even for a legitimately empty line
// (build() runs but emits no objects). Without the hidden-on-create fix, Show() is
// a no-op on the visible-by-default rows and the build count is 0.
func TestFirstReflowBuildsFreshRows(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"alpha":1,"beta":2,"gamma":3}`), FormatJSON)
	r := pv.CreateRenderer().(*prettyViewRenderer) // brand-new renderer + pool
	atomic.StoreInt64(&debugRowBuilds, 0)
	r.Layout(fyne.NewSize(400, 300)) // single sized reflow against the fresh pool
	if len(r.live) == 0 {
		t.Fatal("expected visible rows after the first layout")
	}
	if got := atomic.LoadInt64(&debugRowBuilds); got != int64(len(r.live)) {
		t.Errorf("first reflow built %d rows, want %d (one build per fresh visible row)", got, len(r.live))
	}
}

// TestRefreshBuildsEachRowOnce guards the Refresh()/refreshContent() paths against
// the double-build regression: scroll.Refresh()'s Content.Refresh() cascade must not
// rebuild rows that reflow() already builds. After settling, one Refresh() must build
// each live row exactly once — not twice (the bug was 2x).
func TestRefreshBuildsEachRowOnce(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"alpha":1,"beta":2,"gamma":3,"delta":4}`), FormatJSON)
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))
	pv.Refresh() // settle metrics + initial reflow
	if len(pv.r.live) == 0 {
		t.Fatal("expected visible rows")
	}
	atomic.StoreInt64(&debugRowBuilds, 0)
	pv.Refresh()
	if got, want := atomic.LoadInt64(&debugRowBuilds), int64(len(pv.r.live)); got != want {
		t.Errorf("Refresh built %d rows, want %d (one build per visible row; double-build regression)", got, want)
	}
}

// TestKeyboardScrolling guards keyboard navigation (P13): PageDown/End/Home/Down must
// move the scroll offset on a document taller than the viewport.
func TestKeyboardScrolling(t *testing.T) {
	test.NewApp()
	src := "[" + strings.Repeat(`"x",`, 300) + `"end"]` // ~300 rows, one element per line
	pv := NewWithData([]byte(src), FormatJSON)
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(300, 200))
	pv.Refresh()

	top := pv.r.scroll.Offset.Y
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyPageDown})
	pgdn := pv.r.scroll.Offset.Y
	if pgdn <= top {
		t.Errorf("PageDown did not scroll down (%.1f -> %.1f)", top, pgdn)
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	end := pv.r.scroll.Offset.Y
	if end < pgdn {
		t.Errorf("End did not reach the bottom (%.1f -> %.1f)", pgdn, end)
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	if home := pv.r.scroll.Offset.Y; home != 0 {
		t.Errorf("Home did not return to the top, y=%.1f", home)
	}
	pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	if down := pv.r.scroll.Offset.Y; down <= 0 {
		t.Errorf("Down did not scroll, y=%.1f", down)
	}
}

// rowText concatenates the visible text runs of the live row showing line li,
// left to right — i.e. exactly what the user sees on that row.
func rowText(r *prettyViewRenderer, li int32) (string, bool) {
	for _, rw := range r.live {
		if rw.line == li && rw.rr != nil {
			var sb strings.Builder
			for i := 0; i < len(rw.rr.texts); i++ {
				if rw.rr.texts[i].Visible() {
					sb.WriteString(rw.rr.texts[i].Text)
				}
			}
			return sb.String(), true
		}
	}
	return "", false
}

// TestRowRendersVisibleWindowWhenScrolled validates the rewritten build() culling:
// when a long line is scrolled horizontally, the row must render exactly the slice
// of its display text in the visible column window — computed here independently by
// []rune slicing as a cross-check of the byte-walking cull.
func TestRowRendersVisibleWindowWhenScrolled(t *testing.T) {
	long := strings.Repeat("abcdefghij", 80) // 800 ASCII chars
	pv, win := renderInWindow(t, []byte(`["`+long+`"]`), FormatJSON, 300, 200)
	defer win.Close()

	li := pv.doc.LineAtRow(1) // the long string element
	depth := pv.doc.Lines[li].Depth
	row := int(pv.doc.RowOfLine(li))
	// Scroll right so the visible window starts around column 137.
	pv.r.scrollToOffset(fyne.NewPos(pv.met.ColX(depth, 137), pv.met.RowY(row)))
	pv.r.reflow()

	got, ok := rowText(pv.r, li)
	if !ok {
		t.Fatal("the long-line row is not live")
	}

	disp := []rune(pv.doc.DisplayString(li))
	first := pv.met.FirstVisibleCol(depth, pv.viewOffX)
	last := pv.met.LastVisibleCol(depth, pv.viewOffX+pv.viewW)
	if last > len(disp) {
		last = len(disp)
	}
	if first > len(disp) {
		first = len(disp)
	}
	want := string(disp[first:last])
	if got != want {
		t.Errorf("visible row text mismatch (first=%d last=%d):\n got=%q\nwant=%q", first, last, got, want)
	}
	// Culling invariant: the rendered slice must be viewport-bounded, never the whole line.
	if len(got) >= len(long) {
		t.Errorf("row rendered %d runes — culling failed (line has %d)", len(got), len(long))
	}
}

// TestRowRendersMultibyteUnscrolled checks the build() rune walk handles multibyte
// text correctly at the left edge (no horizontal scroll).
// TestRowRendersMultibyteWhenScrolled guards the decode (non-grid) branch of build()
// after the byte==column-grid fast path was added: a horizontally-scrolled line with
// multi-byte runes (LineIsByteGrid == false) must still render exactly the visible
// column window via the rune-walk path, not the arithmetic shortcut.
func TestRowRendersMultibyteWhenScrolled(t *testing.T) {
	long := strings.Repeat("héllo wörld ", 80) // multi-byte runes force the decode path
	pv, win := renderInWindow(t, []byte(`["`+long+`"]`), FormatJSON, 300, 200)
	defer win.Close()

	li := pv.doc.LineAtRow(1)
	if pv.doc.LineIsByteGrid(li) {
		t.Fatal("precondition: a multi-byte line must not be a byte grid")
	}
	depth := pv.doc.Lines[li].Depth
	row := int(pv.doc.RowOfLine(li))
	pv.r.scrollToOffset(fyne.NewPos(pv.met.ColX(depth, 137), pv.met.RowY(row)))
	pv.r.reflow()

	got, ok := rowText(pv.r, li)
	if !ok {
		t.Fatal("the long-line row is not live")
	}
	disp := []rune(pv.doc.DisplayString(li))
	first := pv.met.FirstVisibleCol(depth, pv.viewOffX)
	last := pv.met.LastVisibleCol(depth, pv.viewOffX+pv.viewW)
	if last > len(disp) {
		last = len(disp)
	}
	if first > len(disp) {
		first = len(disp)
	}
	if want := string(disp[first:last]); got != want {
		t.Errorf("multibyte scrolled row text mismatch (first=%d last=%d):\n got=%q\nwant=%q", first, last, got, want)
	}
	if len(got) >= len([]rune(long)) {
		t.Errorf("row rendered %d runes — culling failed on the multibyte decode path", len(got))
	}
}

func TestRowRendersMultibyteUnscrolled(t *testing.T) {
	pv, win := renderInWindow(t, []byte(`{"k":"héllo wörld ☃"}`), FormatJSON, 600, 200)
	defer win.Close()
	pv.r.reflow()
	li := pv.doc.LineAtRow(1) // `"k": "héllo wörld ☃"`
	got, ok := rowText(pv.r, li)
	if !ok {
		t.Fatal("row not live")
	}
	if want := pv.doc.DisplayString(li); got != want {
		t.Errorf("multibyte row text = %q, want %q", got, want)
	}
}
