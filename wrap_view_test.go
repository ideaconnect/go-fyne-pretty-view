package prettyview

import (
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
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
