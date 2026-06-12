package prettyview

import (
	"os"
	"runtime"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/parse"

	"fyne.io/fyne/v2"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// modelBytes estimates the heap footprint of a parsed document's arenas.
func modelBytes(d *model.Document) int {
	return len(d.Src) +
		len(d.Aux) +
		len(d.Nodes)*32 +
		len(d.Lines)*24 +
		len(d.Segs)*12 +
		d.ProjectionBytes()
}

// TestModelSizeRatio locks the memory budget — the widget's whole reason to exist. The
// bounds sit ~15-25% above the measured ratio so an accidental SoA regression (an extra
// per-node field, a stray per-line string materialization) trips the guard, while ordinary
// fixture drift does not; the old <=12x bar was loose enough to hide a >2x bloat. A lower
// floor also fails an accidental under-count (arenas not populated). Measured 2026-06:
// openapi.json 4.85x, big.json ~7.1x (the documented ≈5-7x band, AGENTS invariant #3).
func TestModelSizeRatio(t *testing.T) {
	cases := []struct {
		file     string
		maxRatio float64
	}{
		{"testdata/openapi.json", 6.0},
		{"testdata/big.json", 8.5},
	}
	for _, c := range cases {
		src, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		d := parse.Parse(src, FormatJSON, 0)
		model := modelBytes(d)
		ratio := float64(model) / float64(len(src))
		t.Logf("%s: source=%d KB, model=%d KB, ratio=%.2fx, nodes=%d lines=%d segs=%d",
			c.file, len(src)/1024, model/1024, ratio, len(d.Nodes), len(d.Lines), len(d.Segs))
		if ratio > c.maxRatio {
			t.Errorf("%s: model is %.2fx source (want <= %.1fx) — SoA layout regressed", c.file, ratio, c.maxRatio)
		}
		if ratio < 2 {
			t.Errorf("%s: model is only %.2fx source (< 2x) — arenas not populated / under-count?", c.file, ratio)
		}
	}
}

// TestLongLineTextureCulled feeds a single multi-megabyte string value and checks
// that, when scrolled to it, no row emits a canvas.Text wider than the viewport.
// Without culling, Fyne would try to rasterize a ~1 GB bitmap for the line.
func TestLongLineTextureCulled(t *testing.T) {
	huge := make([]byte, 0, 2<<20+32)
	huge = append(huge, `{"x":"`...)
	for i := 0; i < 2<<20; i++ {
		huge = append(huge, 'a')
	}
	huge = append(huge, `","y":1}`...)

	pv, win := renderInWindow(t, huge, FormatJSON, 800, 600)
	defer win.Close()

	// Scroll horizontally into the middle of the long line.
	pv.r.scrollToOffset(fyne.NewPos(50000, pv.met.RowH)) // row 1 is the long value

	viewport := pv.r.scroll.Size().Width
	var worst float32
	for _, rw := range pv.r.live {
		if w := rw.maxTextWidth(); w > worst {
			worst = w
		}
	}
	t.Logf("widest emitted canvas.Text = %.0f px, viewport = %.0f px", worst, viewport)
	if worst > viewport*3 {
		t.Errorf("a text run is %.0f px wide (viewport %.0f) — long-line culling is broken", worst, viewport)
	}
}

// TestHeapCeilingScrollingBigJSON loads the 7.5 MB fixture, scrolls the entire
// document, and asserts the live heap stays far below 1 GB.
func TestHeapCeilingScrollingBigJSON(t *testing.T) {
	src, err := os.ReadFile("testdata/big.json")
	if err != nil {
		t.Fatal(err)
	}
	pv, win := renderInWindow(t, src, FormatJSON, 1000, 800)
	defer win.Close()

	total := int(pv.doc.TotalVisibleRows())
	step := pv.met.RowH * 200
	for y := float32(0); y < float32(total)*pv.met.RowH; y += step {
		pv.r.scrollToOffset(fyne.NewPos(0, y))
	}

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	heapMB := float64(ms.HeapAlloc) / (1 << 20)
	t.Logf("model=%d MB, heap after scrolling all %d rows = %.0f MB",
		modelBytes(pv.doc)/(1<<20), total, heapMB)

	if heapMB > 600 {
		t.Errorf("heap reached %.0f MB scrolling a 7.5 MB file — virtualization/culling leaked", heapMB)
	}
}
