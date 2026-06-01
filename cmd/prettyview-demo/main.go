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

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	prettyview "github.com/ideaconnect/go-fyne-pretty-view"
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
	w := a.NewWindow("prettyview demo")
	w.Resize(fyne.NewSize(1000, 720))

	pv := prettyview.New()

	// (a) The optional built-in control bar, used as-is. Flip any Show* field to
	// false to omit a control and supply your own.
	toolbar := prettyview.NewToolbar(pv, prettyview.ToolbarConfig{
		ShowOpen:           true,
		ShowFormat:         true,
		ShowExpandCollapse: true,
		ShowSearch:         true,
		Window:             w, // enables the Open dialog and Ctrl/Cmd+F focus
	})

	// (b) An app-supplied control that drives the public API directly.
	loadFixture := func(path string) {
		data, err := os.ReadFile(path)
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
	w.SetContent(container.NewBorder(top, nil, nil, nil, pv))

	start := "testdata/openapi.json"
	if len(os.Args) > 1 {
		start = os.Args[1]
	}
	if isFixture(start) {
		fixtureSel.SetSelected(start)
	} else {
		loadFixture(start)
	}

	w.ShowAndRun()
}

func isFixture(path string) bool {
	for _, f := range fixtures {
		if f == path {
			return true
		}
	}
	return false
}
