package prettyview

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestCaptureScreenshots renders the widget on several fixtures to PNGs under
// /tmp for visual inspection. It is a development aid, not an assertion.
func TestCaptureScreenshots(t *testing.T) {
	if os.Getenv("PV_SHOTS") == "" {
		t.Skip("set PV_SHOTS=1 to render screenshots")
	}
	shots := []struct{ file, out string }{
		{"testdata/openapi.json", "/tmp/pv_openapi.png"},
		{"testdata/catalog.xml", "/tmp/pv_xml.png"},
		{"testdata/page.html", "/tmp/pv_html.png"},
	}
	for _, s := range shots {
		src, err := os.ReadFile(s.file)
		if err != nil {
			t.Fatal(err)
		}
		test.NewApp()
		pv := NewWithData(src, FormatAuto)
		win := test.NewWindow(pv)
		win.Resize(fyne.NewSize(900, 600))
		pv.Refresh()

		// Save a clean hero shot for the README before adding decorations.
		if s.file == "testdata/openapi.json" {
			if f, err := os.Create("docs/shot-json.png"); err == nil {
				_ = png.Encode(f, win.Canvas().Capture())
				f.Close()
			}
		}

		// Select a few mid-document rows and run a search to visualize highlights.
		if total := pv.doc.TotalVisibleRows(); total > 8 {
			la := pv.doc.LineAtRow(3)
			lb := pv.doc.LineAtRow(6)
			pv.sel = selection{anchor: modelPos{la, 2}, focus: modelPos{lb, 8}, active: true}
			pv.refreshSelectionView()
		}
		pv.Search(SearchQuery{Text: "type"})

		img := win.Canvas().Capture()
		f, err := os.Create(s.out)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
		win.Close()
		t.Logf("wrote %s (%v)", s.out, img.Bounds())
	}
}
