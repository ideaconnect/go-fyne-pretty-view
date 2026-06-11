// Command prettyview-editor demonstrates the v2 EDITABLE mode of the prettyview
// widget: type or paste data with live syntax highlighting, and pretty-print on demand.
//
// It is the companion to prettyview-demo (the read-only viewer). The same widget,
// constructed with prettyview.WithEditable(), becomes a light editor — with a rendered
// caret, undo/redo, cut/paste, a live validity status, real-time syntax coloring while
// typing, and a Reformat button that pretty-prints the buffer in place (the caret stays
// put). Formatting is on demand: typing never reflows the text out from under you.
package main

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/fonttheme"
)

const defaultSample = "messy JSON"

// sampleNames is the dropdown order; samples are deliberately minified/messy so the
// on-pause pretty-formatting is visible.
var sampleNames = []string{"messy JSON", "minified JSON", "messy XML", "empty"}

var samples = map[string]string{
	"messy JSON":    `{"name":"prettyview","editable":true,"nested":{"a":1,"b":[1,2,3],"deep":{"x":true,"y":null}},"items":["one","two","three"]}`,
	"minified JSON": `{"id":42,"tags":["go","fyne","editor"],"ok":true,"nested":{"k":"v"}}`,
	"messy XML":     `<catalog><book id="1"><title>Go</title><author>X</author></book><book id="2"><title>Fyne</title></book></catalog>`,
	"empty":         "",
}

func main() {
	a := app.New()
	// Install the bundled typefaces (JetBrains Mono for the body, Inter for the UI).
	a.Settings().SetTheme(fonttheme.New(theme.DefaultTheme()))
	w := a.NewWindow("prettyview editor demo")
	w.Resize(fyne.NewSize(1000, 720))
	w.SetContent(buildUI(w, startArg()))
	w.ShowAndRun()
}

// startArg is the optional initial input: the first CLI argument (a sample name or a
// file path), or "" to load the default sample.
func startArg() string {
	if len(os.Args) > 1 {
		return os.Args[1]
	}
	return ""
}

// buildUI assembles the editor demo bound to window w and loads the initial content. It
// is separated from main (which only adds ShowAndRun) so the wiring is testable.
func buildUI(w fyne.Window, start string) fyne.CanvasObject {
	// The one line that turns the viewer into an editor: WithEditable(). The default
	// AutoFormatOff keeps typing snappy with live syntax colors and no surprise reflow;
	// the Reformat button pretty-prints on demand. Line numbers make the validation gutter
	// marker visible.
	ed := prettyview.New(
		prettyview.WithEditable(),
		prettyview.WithLineNumbers(),
	)

	status := widget.NewLabel("status: —")
	status.TextStyle = fyne.TextStyle{Monospace: true}
	refreshStatus := func() {
		st := ed.ParseStatus()
		v := "✓ valid"
		if !st.OK {
			v = fmt.Sprintf("✗ parse error near line %d", st.ErrorLine+1)
		}
		status.SetText(fmt.Sprintf("status: %s   (%d bytes)", v, len(ed.Source())))
	}
	// SetOnChanged fires (debounced) after the edited text settles — i.e. just after each
	// auto-reformat — so the status always reflects the current parse.
	ed.SetOnChanged(func(string) { refreshStatus() })

	load := func(name string) {
		ed.SetData([]byte(samples[name]), prettyview.FormatAuto)
		w.SetTitle("prettyview editor demo — " + name)
		refreshStatus() // SetData sets validity synchronously; mirror it into the status bar
	}
	sampleSel := widget.NewSelect(sampleNames, load)

	controls := container.NewHBox(
		widget.NewLabel("Sample:"), sampleSel,
		widget.NewButton("Reformat", ed.Reformat),
		widget.NewButton("Undo", ed.Undo),
		widget.NewButton("Redo", ed.Redo),
	)

	hint := widget.NewLabel("Click in, then type or paste — syntax highlighting is live as you type. " +
		"Reformat pretty-prints in place (the caret stays put); Undo/Redo (or Ctrl/Cmd+Z / Shift+Z) walk the history.")
	hint.Wrapping = fyne.TextWrapWord

	top := container.NewVBox(controls, hint)
	content := container.NewBorder(top, status, nil, nil, ed)

	switch {
	case start == "":
		sampleSel.SetSelected(defaultSample) // triggers load
	case isSample(start):
		sampleSel.SetSelected(start)
	default:
		if data, err := os.ReadFile(start); err == nil {
			ed.SetData(data, prettyview.FormatAuto)
			w.SetTitle("prettyview editor demo — " + start)
		} else {
			ed.SetText("error reading " + start + ": " + err.Error())
		}
	}
	refreshStatus()
	return content
}

func isSample(name string) bool {
	_, ok := samples[name]
	return ok
}
