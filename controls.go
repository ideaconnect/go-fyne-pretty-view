package prettyview

import (
	"fmt"
	"io"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Optional, ready-made controls bound to a PrettyView.
//
// The PrettyView widget has no built-in chrome — it is purely the viewer. These
// helpers let a host application choose, per control, between:
//
//   (a) using a provided control as-is (drop NewToolbar / NewSearchBar / … into
//       your layout), or
//   (b) omitting it and driving PrettyView's public API from your own widgets
//       (SetData, Reparse, ExpandAll, CollapseAll, Search, SearchNext, …).
//
// You can also mix the two: show the built-in search + fold buttons but supply
// your own file picker, for example. Everything here is plain Fyne; nothing is
// required to use the viewer.

// ToolbarConfig selects which built-in controls NewToolbar includes. Each is
// optional; leave a field false to omit that control and provide your own.
type ToolbarConfig struct {
	ShowOpen           bool        // an "Open…" button (needs Window or OnOpen)
	ShowFormat         bool        // a format selector (auto/json/xml/html/raw)
	ShowExpandCollapse bool        // Expand all / Collapse all buttons
	ShowWrap           bool        // a wrap-text icon toggle (soft-wrap on/off)
	ShowSearch         bool        // a find box with prev/next and a match counter
	Window             fyne.Window // enables the built-in Open dialog and Ctrl/Cmd+F focus
	OnOpen             func()      // overrides the built-in Open behavior, if set
}

// DefaultToolbarConfig enables every control.
func DefaultToolbarConfig() ToolbarConfig {
	return ToolbarConfig{ShowOpen: true, ShowFormat: true, ShowExpandCollapse: true, ShowWrap: true, ShowSearch: true}
}

// NewToolbar builds an optional control bar bound to pv from cfg. Disabled
// controls are omitted. The result is a plain fyne.CanvasObject; place it where
// you like (typically the top of a container.NewBorder around the PrettyView).
func NewToolbar(pv *PrettyView, cfg ToolbarConfig) fyne.CanvasObject {
	left := container.NewHBox()
	if cfg.ShowOpen && (cfg.OnOpen != nil || cfg.Window != nil) {
		left.Add(newIconButton(iconFolder(), "Open…", func() {
			if cfg.OnOpen != nil {
				cfg.OnOpen()
				return
			}
			ShowOpenDialog(pv, cfg.Window)
		}))
	}
	if cfg.ShowFormat {
		left.Add(widget.NewLabel("Format:"))
		left.Add(NewFormatSelect(pv))
	}
	if cfg.ShowExpandCollapse {
		left.Add(NewFoldButtons(pv))
	}
	if cfg.ShowWrap {
		left.Add(NewWrapToggle(pv))
	}

	if cfg.ShowSearch {
		bar := NewSearchBar(pv)
		if cfg.Window != nil {
			registerFindShortcut(cfg.Window, pv)
		}
		if len(left.Objects) == 0 {
			return bar
		}
		return container.NewBorder(nil, nil, left, nil, bar)
	}
	return left
}

// NewFoldButtons returns an expand-all / collapse-all icon pair (with hover
// tooltips) bound to pv.
func NewFoldButtons(pv *PrettyView) fyne.CanvasObject {
	return container.NewHBox(
		newIconButton(iconExpand(), "Expand all", pv.ExpandAll),
		newIconButton(iconCollapse(), "Collapse all", pv.CollapseAll),
	)
}

// NewWrapToggle returns a wrap-text icon toggle (with a hover tooltip) bound to pv:
// it flips between soft-wrap (WrapWord) and horizontal scroll (WrapNone), and is
// highlighted (HighImportance) while wrapping is on so the state is visible.
func NewWrapToggle(pv *PrettyView) fyne.CanvasObject {
	var btn *iconButton
	btn = newIconButton(iconWrapText(), "Wrap text", func() {
		if pv.Wrap() == WrapWord {
			pv.SetWrap(WrapNone)
		} else {
			pv.SetWrap(WrapWord)
		}
		btn.Importance = wrapImportance(pv)
		btn.Refresh()
	})
	btn.Importance = wrapImportance(pv)
	return btn
}

func wrapImportance(pv *PrettyView) widget.Importance {
	if pv.Wrap() == WrapWord {
		return widget.HighImportance
	}
	return widget.LowImportance
}

// NewFormatSelect returns a format selector bound to pv. Choosing a format
// re-parses the current source; the selection follows the document when content
// is loaded elsewhere (it registers PrettyView's data-changed hook).
func NewFormatSelect(pv *PrettyView) fyne.CanvasObject {
	names := []string{"auto", "json", "jsonc", "xml", "html", "raw"}
	sel := widget.NewSelect(names, nil)
	sel.Selected = pv.Format().String()
	sel.OnChanged = func(s string) {
		if parseFormatName(s) != pv.Format() {
			pv.Reparse(parseFormatName(s))
		}
	}
	pv.SetOnDataChanged(func() {
		sel.Selected = pv.Format().String()
		sel.Refresh()
	})
	return sel
}

// NewSearchBar returns a find box (with prev/next and a self-updating match
// counter) bound to pv. It registers PrettyView's search-changed and
// search-requested hooks so the counter stays in sync and Ctrl/Cmd+F focuses it.
func NewSearchBar(pv *PrettyView) fyne.CanvasObject {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("search…")
	count := widget.NewLabel("")

	update := func() {
		active, total, capped := pv.SearchStatus()
		switch {
		case total == 0 && entry.Text == "":
			count.SetText("")
		case total == 0:
			count.SetText("0/0")
		case capped:
			count.SetText(fmt.Sprintf("%d/%d+", active, total))
		default:
			count.SetText(fmt.Sprintf("%d/%d", active, total))
		}
	}
	entry.OnChanged = func(s string) { pv.searchDebounced(SearchQuery{Text: s}) }
	entry.OnSubmitted = func(string) {
		// Enter applies immediately, bypassing the debounce, then advances.
		pv.Search(SearchQuery{Text: entry.Text})
		pv.SearchNext()
	}
	pv.SetOnSearchChanged(update)
	pv.SetOnSearchRequested(func() { focusObject(entry) })

	prev := newIconButton(iconArrowUp(), "Previous match", pv.SearchPrev)
	next := newIconButton(iconArrowDown(), "Next match", pv.SearchNext)

	return container.NewBorder(nil, nil, widget.NewIcon(iconSearch()),
		container.NewHBox(count, prev, next), entry)
}

// ShowOpenDialog opens a native file-open dialog and loads the chosen file into
// pv (auto-detecting the format). Exposed so hosts can trigger it from their own
// menu/button.
func ShowOpenDialog(pv *PrettyView, win fyne.Window) {
	if win == nil {
		return
	}
	dialog.ShowFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil || rc == nil {
			return
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		pv.SetData(data, FormatAuto)
	}, win)
}

func registerFindShortcut(win fyne.Window, pv *PrettyView) {
	win.Canvas().AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault},
		func(fyne.Shortcut) {
			if pv.onSearchRequested != nil {
				pv.onSearchRequested()
			}
		},
	)
}

func focusObject(o fyne.CanvasObject) {
	app := fyne.CurrentApp()
	if app == nil || app.Driver() == nil {
		return
	}
	f, ok := o.(fyne.Focusable)
	if !ok {
		return
	}
	if c := app.Driver().CanvasForObject(o); c != nil {
		c.Focus(f)
	}
}

func parseFormatName(s string) Format {
	switch s {
	case "json":
		return FormatJSON
	case "jsonc":
		return FormatJSONC
	case "xml":
		return FormatXML
	case "html":
		return FormatHTML
	case "raw":
		return FormatRaw
	default:
		return FormatAuto
	}
}
