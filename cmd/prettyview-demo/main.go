// Command prettyview-demo is a small harness that exercises the prettyview
// widget: it loads JSON/XML/HTML/raw fixtures (or a file given on the command
// line), switches format, expands/collapses, and searches.
package main

import (
	"os"
	"path/filepath"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
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

	load := func(path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			pv.SetText("error reading " + path + ": " + err.Error())
			return
		}
		pv.SetData(data, prettyview.FormatAuto)
		w.SetTitle("prettyview demo — " + filepath.Base(path) + " [" + pv.Format().String() + "]")
	}

	fileSel := widget.NewSelect(fixtures, func(p string) { load(p) })

	formatSel := widget.NewSelect(
		[]string{"auto", "json", "xml", "html", "raw"},
		func(s string) {
			// Re-apply the current file under the chosen format.
			if fileSel.Selected == "" {
				return
			}
			data, err := os.ReadFile(fileSel.Selected)
			if err != nil {
				return
			}
			pv.SetData(data, formatFromString(s))
		},
	)
	formatSel.SetSelected("auto")

	expandBtn := widget.NewButton("Expand all", pv.ExpandAll)
	collapseBtn := widget.NewButton("Collapse all", pv.CollapseAll)

	countLabel := widget.NewLabel("")
	updateCount := func() {
		active, total, capped := pv.SearchStatus()
		switch {
		case total == 0:
			countLabel.SetText("")
		case capped:
			countLabel.SetText(formatCount(active, total) + "+")
		default:
			countLabel.SetText(formatCount(active, total))
		}
	}
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("search…")
	searchEntry.OnChanged = func(s string) { pv.Search(prettyview.SearchQuery{Text: s}); updateCount() }
	searchEntry.OnSubmitted = func(string) { pv.SearchNext(); updateCount() }
	nextBtn := widget.NewButton("▼", func() { pv.SearchNext(); updateCount() })
	prevBtn := widget.NewButton("▲", func() { pv.SearchPrev(); updateCount() })
	focusSearch := func() { w.Canvas().Focus(searchEntry) }
	pv.SetOnSearchRequested(focusSearch)
	w.Canvas().AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault},
		func(fyne.Shortcut) { focusSearch() },
	)

	top := container.NewVBox(
		container.NewHBox(widget.NewLabel("File:"), fileSel, widget.NewLabel("Format:"), formatSel, expandBtn, collapseBtn),
		container.NewBorder(nil, nil, widget.NewLabel("Find:"),
			container.NewHBox(countLabel, prevBtn, nextBtn), searchEntry),
	)

	w.SetContent(container.NewBorder(top, nil, nil, nil, pv))

	// Initial content: a CLI argument, else the OpenAPI fixture.
	start := "testdata/openapi.json"
	if len(os.Args) > 1 {
		start = os.Args[1]
	}
	for _, f := range fixtures {
		if f == start {
			fileSel.SetSelected(f)
		}
	}
	if fileSel.Selected == "" {
		load(start)
	}

	w.ShowAndRun()
}

func formatCount(active, total int) string {
	return strconv.Itoa(active) + "/" + strconv.Itoa(total)
}

func formatFromString(s string) prettyview.Format {
	switch s {
	case "json":
		return prettyview.FormatJSON
	case "xml":
		return prettyview.FormatXML
	case "html":
		return prettyview.FormatHTML
	case "raw":
		return prettyview.FormatRaw
	default:
		return prettyview.FormatAuto
	}
}
