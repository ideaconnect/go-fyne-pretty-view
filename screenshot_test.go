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

	// Editor (v2): the editable widget with the line-number gutter, live syntax colors,
	// and a rendered caret — pretty-printed in place by Reformat. Saved for the README's
	// Editing section.
	captureEditor(t)
	captureFoldSearch(t)
	captureLiveTyping(t)
}

// captureFoldSearch renders the read-only viewer with deeper nodes folded (collapse
// summaries showing) and a search highlight — for the README's feature section.
func captureFoldSearch(t *testing.T) {
	src, err := os.ReadFile("testdata/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	test.NewApp()
	pv := NewWithData(src, FormatJSON, WithLineNumbers())
	win := test.NewWindow(pv)
	win.Resize(fyne.NewSize(880, 460))
	pv.Refresh()
	pv.CollapseToDepth(3) // leave the top levels open; fold deeper ones to their summaries
	pv.Search(SearchQuery{Text: "type"})
	pv.Refresh()

	writePNG(t, win, "docs/fold-search.png")
	win.Close()
}

// captureLiveTyping renders the editor mid-edit: a minified buffer that is colored as typed
// (NOT reformatted), with the caret — the contrast to the prettified docs/editor.png.
func captureLiveTyping(t *testing.T) {
	test.NewApp()
	ed := New(WithEditable(), WithLineNumbers())
	ed.SetData([]byte(`{"id":42,"tags":["go","fyne","editor"],"ok":true,"nested":{"k":"v"}}`), FormatJSON)
	win := test.NewWindow(ed)
	win.Resize(fyne.NewSize(880, 92)) // a tight band: one minified, colored line
	ed.Refresh()
	ed.FocusGained()
	ed.SetCaret(0, 24) // caret mid-line, no reformat -> stays minified but colored
	ed.Refresh()

	writePNG(t, win, "docs/editor-live.png")
	win.Close()
}

// writePNG encodes win's current canvas to path (and /tmp for inspection).
func writePNG(t *testing.T, win fyne.Window, path string) {
	t.Helper()
	img := win.Canvas().Capture()
	for _, out := range []string{path, "/tmp/pv_" + filepathBase(path)} {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	t.Logf("wrote %s (%v)", path, img.Bounds())
}

// filepathBase is the trailing file name of a /-separated path (avoids importing path/filepath
// for this dev-only helper).
func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

// captureEditor renders the v2 editable widget on a JSON sample and writes a hero shot for
// the README to docs/editor.png (and /tmp for inspection).
func captureEditor(t *testing.T) {
	test.NewApp()
	ed := New(WithEditable(), WithLineNumbers())
	ed.SetData([]byte(`{"name":"prettyview","editable":true,"nested":{"a":1,"b":[1,2,3],"ok":true},"items":["one","two","three"],"count":42}`), FormatJSON)
	win := test.NewWindow(ed)
	win.Resize(fyne.NewSize(880, 430)) // tight to the ~19 reformatted lines, little dead space
	ed.Refresh()
	ed.FocusGained()
	ed.Reformat() // pretty-print the buffer in place (colored, caret preserved)
	ed.SetCaret(5, 9)
	ed.Refresh()

	img := win.Canvas().Capture()
	for _, out := range []string{"docs/editor.png", "/tmp/pv_editor.png"} {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	win.Close()
	t.Logf("wrote docs/editor.png (%v)", img.Bounds())
}
