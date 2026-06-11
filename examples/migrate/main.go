// Command migrate is the v1 → v2 migration example referenced by MIGRATION.md. It is
// compiled by `go build ./...` (and `make check`), so the documented snippet is real,
// not aspirational. It is built, not run.
package main

import (
	"fyne.io/fyne/v2/app"
	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"
)

func main() {
	a := app.New()
	w := a.NewWindow("viewer")

	// Read-only: identical to v1 — only the import path changed to .../go-fyne-pretty-view/v2.
	pv := prettyview.New()
	pv.SetData([]byte(`{"a":1}`), prettyview.FormatAuto)

	// New in v2 (opt-in, additive): an editor that pretty-formats on the fly.
	ed := prettyview.New(
		prettyview.WithEditable(),
		prettyview.WithInputConfig(prettyview.InputConfig{AutoFormat: prettyview.AutoFormatOnPause}),
		prettyview.WithUndoLimit(500),
	)
	ed.SetOnChanged(func(text string) { _ = text })
	ed.SetOnValidationChanged(func(s prettyview.ParseStatus) { _ = s })
	_ = ed.Editable()
	_, _ = ed.Caret()
	_ = ed.Text() // the displayed/pretty text, distinct from raw Source()

	w.SetContent(pv)
	_ = ed
	// ShowAndRun is intentionally omitted: this example is built, not run.
}
