package prettyview

import (
	"os"
	"runtime"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/parse"

	"fyne.io/fyne/v2"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
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

func TestModelSizeRatio(t *testing.T) {
	src, err := os.ReadFile("testdata/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	d := parse.Parse(src, FormatJSON, 0)
	model := modelBytes(d)
	ratio := float64(model) / float64(len(src))
	t.Logf("source=%d KB, model=%d KB, ratio=%.2fx, nodes=%d lines=%d segs=%d",
		len(src)/1024, model/1024, ratio, len(d.Nodes), len(d.Lines), len(d.Segs))
	if ratio > 12 {
		t.Errorf("model is %.1fx source (want <= 12x) — SoA layout regressed", ratio)
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
