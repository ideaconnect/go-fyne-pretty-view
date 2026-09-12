package prettyview

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/fonttheme"
)

// TestCaptureScreenshots renders the README's image set under docs/ (and a copy under
// /tmp for inspection) with Fyne's software painter, so it needs no display. It is a
// development aid, not an assertion: run it with `make shots` (PV_SHOTS=1) after a
// rendering change and read the PNGs.
//
// Every shot is rendered at 2x scale with the bundled JetBrains Mono / Inter faces
// (fonttheme) on Fyne's default theme, so the images are crisp on high-DPI displays and
// look like the demo binaries rather than the test theme.
func TestCaptureScreenshots(t *testing.T) {
	if os.Getenv("PV_SHOTS") == "" {
		t.Skip("set PV_SHOTS=1 to render screenshots")
	}
	captureHero(t)
	captureXML(t)
	captureHTML(t)
	captureLight(t)
	captureEditor(t)
	captureLiveTyping(t)
}

// shotApp installs a fresh test app with the bundled fonts on top of base (the default
// theme, or theme.LightTheme() for the light shot).
func shotApp(base fyne.Theme) {
	a := test.NewApp()
	a.Settings().SetTheme(fonttheme.New(base))
}

// shotWindow opens content in a 2x window of the given logical size and refreshes pv.
func shotWindow(content fyne.CanvasObject, pv *PrettyView, w, h float32) fyne.Window {
	win := test.NewWindow(content)
	win.Canvas().(software.WindowlessCanvas).SetScale(2)
	win.Resize(fyne.NewSize(w, h))
	pv.Refresh()
	return win
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	src, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

// captureHero is the README's opening image: the full built-in toolbar over a JSON
// document with line numbers, a few nodes folded to their summaries, and a search
// with its match counter and highlights.
func captureHero(t *testing.T) {
	shotApp(theme.DefaultTheme())
	pv := NewWithData(fixture(t, "openapi.json"), FormatJSON, WithLineNumbers())
	bar := NewToolbar(pv, ToolbarConfig{ShowFormat: true, ShowExpandCollapse: true, ShowWrap: true, ShowSearch: true})
	win := shotWindow(container.NewBorder(bar, nil, nil, nil, pv), pv, 960, 600)
	pv.CollapseToDepth(3)
	pv.Search(SearchQuery{Text: "type"})
	pv.SearchNext()
	pv.SearchNext()
	// Drive the search bar's entry so the counter and the query match what the viewer
	// shows (the bar registers the search-changed hook, so the counter updates itself).
	if entry := findSearchEntry(bar); entry != nil {
		entry.SetText("type")
	}
	// The counter's text grew after the bar was laid out; a real driver re-lays out
	// containers whose min size changed before each paint, the headless canvas does not.
	bar.Refresh()
	pv.Refresh()
	writePNG(t, win, "docs/hero.png")
	win.Close()
}

// captureXML shows the XML view with fold summaries ("<tag> N children") and a selection.
func captureXML(t *testing.T) {
	shotApp(theme.DefaultTheme())
	pv := NewWithData(fixture(t, "catalog.xml"), FormatXML, WithLineNumbers())
	win := shotWindow(pv, pv, 880, 412)
	pv.CollapseToDepth(3)
	if total := pv.doc.TotalVisibleRows(); total > 8 {
		la, lb := pv.doc.LineAtRow(3), pv.doc.LineAtRow(5)
		pv.sel = selection{anchor: modelPos{la, 4}, focus: modelPos{lb, 10}, active: true}
		pv.refreshSelectionView()
	}
	pv.Refresh()
	writePNG(t, win, "docs/xml.png")
	win.Close()
}

// captureHTML shows the HTML view: tags, attributes, and a folded <script> body.
func captureHTML(t *testing.T) {
	shotApp(theme.DefaultTheme())
	pv := NewWithData(fixture(t, "page.html"), FormatHTML, WithLineNumbers(), WithWrap(WrapWord))
	win := shotWindow(pv, pv, 880, 412)
	pv.CollapseToDepth(4)
	pv.Refresh()
	writePNG(t, win, "docs/html.png")
	win.Close()
}

// captureLight renders the JSON view under the light variant. The test app reports no
// variant preference, so the light syntax palette is installed explicitly the way a host
// would with WithSyntaxColors; the structural colors follow theme.LightTheme().
func captureLight(t *testing.T) {
	shotApp(theme.LightTheme())
	variant := fyne.CurrentApp().Settings().ThemeVariant()
	pv := NewWithData(fixture(t, "small.json"), FormatJSON, WithLineNumbers(),
		WithSyntaxColors(variant, defaultSyntaxColors(theme.VariantLight)))
	win := shotWindow(pv, pv, 880, 360)
	pv.Search(SearchQuery{Text: "title"})
	pv.Refresh()
	writePNG(t, win, "docs/light.png")
	win.Close()
}

// captureEditor renders the editable widget: line-number gutter, live syntax colors, and
// a rendered caret on a buffer that Reformat pretty-printed in place.
func captureEditor(t *testing.T) {
	shotApp(theme.DefaultTheme())
	ed := New(WithEditable(), WithLineNumbers())
	ed.SetData([]byte(`{"name":"prettyview","editable":true,"nested":{"a":1,"b":[1,2,3],"ok":true},"items":["one","two","three"],"count":42}`), FormatJSON)
	win := shotWindow(ed, ed, 880, 430)
	ed.FocusGained()
	ed.Reformat()
	ed.SetCaret(5, 9)
	ed.Refresh()
	writePNG(t, win, "docs/editor.png")
	win.Close()
}

// captureLiveTyping renders the editor mid-edit: a minified buffer colored as typed, not
// reformatted, with the caret. The contrast to docs/editor.png.
func captureLiveTyping(t *testing.T) {
	shotApp(theme.DefaultTheme())
	ed := New(WithEditable(), WithLineNumbers())
	ed.SetData([]byte(`{"id":42,"tags":["go","fyne","editor"],"ok":true,"nested":{"k":"v"}}`), FormatJSON)
	win := shotWindow(ed, ed, 880, 92)
	ed.FocusGained()
	ed.SetCaret(0, 24)
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

// filepathBase is the trailing file name of a /-separated path (avoids importing
// path/filepath for this dev-only helper).
func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}
