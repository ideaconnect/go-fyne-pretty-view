package prettyview_test

import (
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"
)

// ExampleNew builds a viewer, loads JSON with auto-detection, adds the optional
// toolbar, and shows it.
func ExampleNew() {
	a := app.New()
	w := a.NewWindow("viewer")

	pv := prettyview.New(prettyview.WithWrap(prettyview.WrapWord))
	pv.SetData([]byte(`{"hello":"world","items":[1,2,3]}`), prettyview.FormatAuto)

	bar := prettyview.NewToolbar(pv, prettyview.ToolbarConfig{
		ShowSearch:         true,
		ShowExpandCollapse: true,
		ShowWrap:           true,
	})
	w.SetContent(container.NewBorder(bar, nil, nil, nil, pv))
	w.ShowAndRun()
}

// ExamplePrettyView_Search runs a regular-expression search, reads the live match
// counter, and steps to the next match.
func ExamplePrettyView_Search() {
	pv := prettyview.NewWithData([]byte(`{"a":"x1","b":"x2","c":"yy"}`), prettyview.FormatJSON)

	pv.Search(prettyview.SearchQuery{Text: `x\d`, Mode: prettyview.SearchRegex})
	pv.SearchStatus() // -> (active, total, capped); total is 2 here
	pv.SearchNext()   // advance to the next match (wraps at the end)
}

// ExamplePrettyView_CopySubtree copies a node's whole subtree to the clipboard by its
// source byte offset (JSON/JSONC; XML/HTML carry no per-node offset and return false).
// The right-click "Copy subtree" menu item does the same for any format.
func ExamplePrettyView_CopySubtree() {
	pv := prettyview.NewWithData([]byte(`{"user":{"name":"ada","id":1}}`), prettyview.FormatJSON)

	// Byte offset of the '{' opening the "user" object in the source.
	pv.CopySubtree(8) // -> true; the "user" subtree is now on the clipboard
}
