// Command prettyview-demo exercises the prettyview widget. It shows both ways a
// host can drive the viewer:
//
//   - the built-in, optional control bar (prettyview.NewToolbar) — format
//     selector, expand/collapse, search, and Open — used as-is; and
//   - the app's own control (a fixture dropdown) calling the public API directly.
//
// Disable any built-in control via ToolbarConfig and supply your own instead.
package main

import (
	"os"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fynetooltip "github.com/dweymouth/fyne-tooltip"

	prettyview "github.com/ideaconnect/go-fyne-pretty-view"
	"github.com/ideaconnect/go-fyne-pretty-view/fonttheme"
)

var fixtures = []string{
	"testdata/small.json",
	"testdata/openapi.json",
	"testdata/big.json",
	"testdata/catalog.xml",
	"testdata/page.html",
}

func main() {
	a := app.New()
	// Install the bundled typefaces (JetBrains Mono for the viewer body, Inter for
	// the UI). Pass fonttheme.WithFonts(...) to override individual faces.
	a.Settings().SetTheme(fonttheme.New(theme.DefaultTheme()))
	w := a.NewWindow("prettyview demo")
	w.Resize(fyne.NewSize(1000, 720))
	// Wrap the content in a tooltip layer (fyne-tooltip) so the toolbar/search bar's
	// icon-only buttons show hover labels — Fyne core has no tooltip support.
	w.SetContent(fynetooltip.AddWindowToolTipLayer(buildUI(w, startPath()), w.Canvas()))
	w.ShowAndRun()
}

// startPath is the initial fixture/file: the first CLI argument, or openapi.json.
func startPath() string {
	if len(os.Args) > 1 {
		return os.Args[1]
	}
	return "testdata/openapi.json"
}

// buildUI assembles the demo UI bound to window w and loads the initial path. It is
// separated from main (which only adds ShowAndRun) so the wiring is testable.
func buildUI(w fyne.Window, start string) fyne.CanvasObject {
	pv := prettyview.New()

	// (a) The optional built-in control bar, used as-is. Flip any Show* field to
	// false to omit a control and supply your own.
	toolbar := prettyview.NewToolbar(pv, prettyview.ToolbarConfig{
		ShowOpen:           true,
		ShowFormat:         true,
		ShowExpandCollapse: true,
		ShowWrap:           true,
		ShowSearch:         true,
		Window:             w, // enables the Open dialog and Ctrl/Cmd+F focus
	})

	// (b) An app-supplied control that drives the public API directly.
	loadFixture := func(path string) {
		data, err := readFixture(path)
		if err != nil {
			pv.SetText("error reading " + path + ": " + err.Error())
			return
		}
		pv.SetData(data, prettyview.FormatAuto)
		w.SetTitle("prettyview demo — " + filepath.Base(path))
	}
	fixtureSel := widget.NewSelect(fixtures, loadFixture)

	top := container.NewVBox(
		container.NewHBox(widget.NewLabel("Fixture:"), fixtureSel),
		toolbar,
	)
	content := container.NewBorder(top, nil, nil, nil, pv)

	if isFixture(start) {
		fixtureSel.SetSelected(start)
	} else {
		loadFixture(start)
	}
	return content
}

func isFixture(path string) bool {
	for _, f := range fixtures {
		if f == path {
			return true
		}
	}
	return false
}

// readFixture reads one of the demo's bundled fixtures by its testdata/ path. It
// looks in the current directory, next to the executable (the layout of the
// released zip — the binary alongside testdata/), and the source tree during
// development, so the demo works however it was launched. Anything not in the
// fixture set is read as a plain path (e.g. a user-supplied CLI argument).
func readFixture(path string) ([]byte, error) {
	if isFixture(path) {
		for _, base := range fixtureBaseDirs() {
			if data, err := os.ReadFile(filepath.Join(base, path)); err == nil {
				return data, nil
			}
		}
	}
	return os.ReadFile(path)
}

// fixtureBaseDirs lists the directories the bundled testdata/ may live under, in
// priority order: the working directory, the executable's directory (the released
// zip), and the module root during development.
func fixtureBaseDirs() []string {
	dirs := []string{"."}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		dirs = append(dirs, filepath.Join(filepath.Dir(file), "..", ".."))
	}
	return dirs
}
