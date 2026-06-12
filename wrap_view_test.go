package prettyview

import (
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// rowWidgetText concatenates a live row's visible text runs in emission
// (left-to-right) order — i.e. the text that row actually paints.
func rowWidgetText(rw *rowWidget) string {
	if rw.rr == nil {
		return ""
	}
	var sb strings.Builder
	for _, t := range rw.rr.texts {
		if t.Visible() {
			sb.WriteString(t.Text)
		}
	}
	return sb.String()
}

func runeSlice(s string, a, b int32) string {
	r := []rune(s)
	return string(r[a:b])
}

// widestWrappedLine returns the line occupying the most visual rows (and that count).
func widestWrappedLine(pv *PrettyView) (line, rows int32) {
	line, rows = -1, 0
	for li := int32(0); li < int32(pv.doc.TotalLines()); li++ {
		if r := pv.doc.RowsOfLine(li); r > rows {
			line, rows = li, r
		}
	}
	return
}

// TestWrapRendersContinuationRows drives M-W4: a long line wrapped to N visual rows
// must render as N rows whose concatenated text equals the logical line, with each
// row painting exactly its WrapBreaks column slice.
func TestWrapRendersContinuationRows(t *testing.T) {
	long := strings.Repeat("wxyz ", 60) // 300 chars, word-wraps cleanly
	src := []byte(`{"k":"` + long + `"}`)
	pv, win := renderInWindow(t, src, FormatJSON, 400, 600)
	defer win.Close()

	pv.SetWrap(WrapWord)

	target, rows := widestWrappedLine(pv)
	if rows < 2 {
		t.Fatalf("expected a wrapped line (>=2 visual rows), got max %d", rows)
	}

	parts := make([]string, rows)
	seen := make([]bool, rows)
	got := 0
	for _, rw := range pv.r.live {
		if rw.line == target {
			parts[rw.sub] = rowWidgetText(rw)
			if !seen[rw.sub] {
				seen[rw.sub] = true
				got++
			}
		}
	}
	if int32(got) != rows {
		t.Fatalf("rendered %d distinct sub-rows of the wrapped line, want %d", got, rows)
	}
	if joined, want := strings.Join(parts, ""), pv.doc.DisplayString(target); joined != want {
		t.Errorf("concatenated sub-rows != line text:\n got %q\nwant %q", joined, want)
	}

	var dst []int32
	dst = pv.doc.WrapBreaks(target, dst[:0])
	full := pv.doc.DisplayString(target)
	for sub := int32(0); sub < rows; sub++ {
		if want := runeSlice(full, dst[sub], dst[sub+1]); parts[sub] != want {
			t.Errorf("sub-row %d painted %q, want WrapBreaks slice %q", sub, parts[sub], want)
		}
	}
}

// TestWrapVirtualizationRowCount guards that soft-wrap does not break the
// virtualization invariant: even with every line wrapped, only ~viewport-many row
// widgets are ever live, including while scrolling the whole document.
func TestWrapVirtualizationRowCount(t *testing.T) {
	src, err := os.ReadFile("testdata/big.json")
	if err != nil {
		t.Fatal(err)
	}
	pv, win := renderInWindow(t, src, FormatJSON, 800, 600)
	defer win.Close()

	pv.SetWrap(WrapWord)
	if !pv.doc.WrapActive() {
		t.Fatal("wrap did not activate")
	}
	bound := int(600/pv.met.RowH) + 4
	total := int(pv.doc.TotalVisibleRows())
	if total < 1000 {
		t.Fatalf("expected many wrapped rows, got %d", total)
	}

	maxLive := len(pv.r.live)
	step := float32(total) * pv.met.RowH / 50
	for i := 0; i < 50; i++ {
		pv.r.scrollToOffset(fyne.NewPos(0, float32(i)*step))
		if n := len(pv.r.live); n > maxLive {
			maxLive = n
		}
	}
	t.Logf("wrapped big.json: %d visual rows, peak live widgets=%d (bound %d)", total, maxLive, bound)
	if maxLive > bound {
		t.Errorf("wrapped live rows peaked at %d, exceeding bound %d", maxLive, bound)
	}
}

// TestWrapLongLineTextureCulled is the M-2 invariant under soft-wrap: a 2 MB
// unbreakable value char-wraps, and every continuation row must paint a canvas.Text
// no wider than the viewport — wrapping makes culling automatic.
func TestWrapLongLineTextureCulled(t *testing.T) {
	huge := make([]byte, 0, 2<<20+32)
	huge = append(huge, `{"x":"`...)
	for i := 0; i < 2<<20; i++ {
		huge = append(huge, 'a')
	}
	huge = append(huge, `","y":1}`...)

	pv, win := renderInWindow(t, huge, FormatJSON, 800, 600)
	defer win.Close()
	pv.SetWrap(WrapWord)

	// The long value is line 1; scroll vertically well onto its continuation rows.
	pv.r.scrollToOffset(fyne.NewPos(0, 60*pv.met.RowH))

	viewport := pv.r.scroll.Size().Width
	var worst float32
	onContinuation := false
	for _, rw := range pv.r.live {
		if rw.sub > 0 {
			onContinuation = true
		}
		if w := rw.maxTextWidth(); w > worst {
			worst = w
		}
	}
	if !onContinuation {
		t.Fatal("expected to be scrolled onto wrapped continuation rows")
	}
	t.Logf("wrapped widest canvas.Text = %.0f px, viewport = %.0f px", worst, viewport)
	if worst > viewport {
		t.Errorf("wrapped text run %.0f px exceeds viewport %.0f — wrap culling broken", worst, viewport)
	}
}

// TestCopyAcrossWrappedLinesHasNoSoftBreaks is the headline correctness property:
// wrapping is presentational, so copying a selection spanning several wrapped lines
// must yield one newline per LOGICAL line boundary and none for soft-breaks.
func TestCopyAcrossWrappedLinesHasNoSoftBreaks(t *testing.T) {
	long := strings.Repeat("wxyz ", 60)
	src := []byte(`{"a":"` + long + `","b":"` + long + `","c":1}`)
	pv, win := renderInWindow(t, src, FormatJSON, 400, 600)
	defer win.Close()

	pv.SetWrap(WrapWord)
	// Confirm at least one selected line actually wraps (otherwise the test is moot).
	if _, rows := widestWrappedLine(pv); rows < 2 {
		t.Fatal("fixture did not wrap; cannot test soft-break exclusion")
	}
	pv.SelectAll()
	txt := pv.SelectedText()
	wantNL := pv.doc.TotalLines() - 1 // 5 logical lines -> 4 newlines
	if got := strings.Count(txt, "\n"); got != wantNL {
		t.Errorf("SelectedText has %d newlines, want %d (soft-breaks must not appear)", got, wantNL)
	}
}

// TestMatchHighlightOnContinuationRow checks a search match that falls past the
// first soft-break is highlighted on the correct continuation visual row.
func TestMatchHighlightOnContinuationRow(t *testing.T) {
	src := []byte(`{"k":"` + strings.Repeat("ab ", 90) + `ZZZ"}`) // ZZZ near the end
	pv, win := renderInWindow(t, src, FormatJSON, 400, 600)
	defer win.Close()
	pv.SetWrap(WrapWord)
	pv.Search(SearchQuery{Text: "ZZZ"})

	if len(pv.search.matches) == 0 {
		t.Fatal("no match found")
	}
	mt := pv.search.matches[0]
	var dst []int32
	dst = pv.doc.WrapBreaks(int32(mt.Line), dst[:0])
	if len(dst) < 3 {
		t.Fatal("match line did not wrap")
	}
	sub := 0
	for sub < len(dst)-2 && mt.ColStart >= int(dst[sub+1]) {
		sub++
	}
	if sub == 0 {
		t.Fatal("match landed on the first row; fixture too short to test continuation")
	}
	wantY := pv.met.RowY(int(pv.doc.RowOfLine(int32(mt.Line))) + sub)
	found := false
	for _, rc := range pv.r.matchRects {
		if rc.Visible() {
			if dy := rc.Position().Y - wantY; dy < 0.5 && dy > -0.5 {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("no visible match highlight at continuation row y=%.1f (sub %d)", wantY, sub)
	}
}

// TestWrapToggleRoundTrip checks SetWrap(WrapWord) then SetWrap(WrapNone) restores
// the projection, the scroll direction, and the visual-row total exactly.
func TestWrapToggleRoundTrip(t *testing.T) {
	src := []byte(`{"k":"` + strings.Repeat("wxyz ", 60) + `","n":2}`)
	pv, win := renderInWindow(t, src, FormatJSON, 400, 600)
	defer win.Close()

	noneTotal := pv.doc.TotalVisibleRows()
	noneDir := pv.r.scroll.Direction

	pv.SetWrap(WrapWord)
	if !pv.doc.WrapActive() {
		t.Fatal("wrap did not activate")
	}
	if pv.r.scroll.Direction != container.ScrollVerticalOnly {
		t.Errorf("scroll direction = %v under wrap, want vertical-only", pv.r.scroll.Direction)
	}
	if pv.doc.TotalVisibleRows() <= noneTotal {
		t.Errorf("wrap added no visual rows (%d <= %d)", pv.doc.TotalVisibleRows(), noneTotal)
	}

	pv.SetWrap(WrapNone)
	if pv.doc.WrapActive() {
		t.Fatal("wrap still active after SetWrap(WrapNone)")
	}
	if got := pv.doc.TotalVisibleRows(); got != noneTotal {
		t.Errorf("toggle back changed total: %d != %d", got, noneTotal)
	}
	if pv.r.scroll.Direction != noneDir {
		t.Errorf("scroll direction not restored: %v != %v", pv.r.scroll.Direction, noneDir)
	}
}
