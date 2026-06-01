package prettyview

import (
	"os"
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
	bound := int(600/pv.met.rowH) + 4
	t.Logf("total visible rows=%d, live row widgets=%d, viewport bound=%d, rowH=%.1f",
		total, live, bound, pv.met.rowH)

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

	bound := int(600/pv.met.rowH) + 4

	// Scroll through the whole document in viewport-sized steps; the live row
	// count must stay bounded the entire time.
	total := int(pv.doc.TotalVisibleRows())
	maxLive := 0
	for y := float32(0); y < float32(total)*pv.met.rowH; y += 500 {
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
