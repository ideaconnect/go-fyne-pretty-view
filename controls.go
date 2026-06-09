package prettyview

import (
	"fmt"
	"io"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
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
		left.Add(iconBtn(iconFolder(), func() {
			if cfg.OnOpen != nil {
				cfg.OnOpen()
				return
			}
			ShowOpenDialog(pv, cfg.Window)
		}))
	}
	if cfg.ShowFormat {
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

// iconBtn is a compact, flat (low-importance) icon-only button.
func iconBtn(icon fyne.Resource, tapped func()) *widget.Button {
	b := widget.NewButtonWithIcon("", icon, tapped)
	b.Importance = widget.LowImportance
	return b
}

// NewFoldButtons returns an expand-all / collapse-all icon pair bound to pv.
func NewFoldButtons(pv *PrettyView) fyne.CanvasObject {
	return container.NewHBox(
		iconBtn(iconExpand(), pv.ExpandAll),
		iconBtn(iconCollapse(), pv.CollapseAll),
	)
}

// NewWrapToggle returns a wrap-text icon toggle bound to pv: it flips between
// soft-wrap (WrapWord) and horizontal scroll (WrapNone), and is highlighted
// (HighImportance) while wrapping is on so the state is visible.
func NewWrapToggle(pv *PrettyView) fyne.CanvasObject {
	var btn *widget.Button
	btn = iconBtn(iconWrapText(), func() {
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
	// A canvas.Text (not a widget.Label) for the match counter: a Label's inner
	// padding would make the entry→counter gap wider than the gap between the nav
	// buttons. Centered so it lines up vertically with the buttons.
	count := canvas.NewText("", theme.Color(theme.ColorNameForeground))

	update := func() {
		active, total, capped := pv.SearchStatus()
		switch {
		case total == 0 && entry.Text == "":
			count.Text = ""
		case total == 0:
			count.Text = "0/0"
		case capped:
			count.Text = fmt.Sprintf("%d/%d+", active, total)
		default:
			count.Text = fmt.Sprintf("%d/%d", active, total)
		}
		count.Refresh()
	}
	entry.OnChanged = func(s string) { pv.searchDebounced(SearchQuery{Text: s}) }
	entry.OnSubmitted = func(string) {
		// Enter applies immediately, bypassing the debounce. If the query is already
		// applied (the debounced scan ran), Enter means find-next, so advance. But on
		// a fresh query that beat the debounce, Search reveals match #1 — don't jump
		// straight past it to #2; only advance when the text is unchanged.
		advance := pv.search.query.Text == entry.Text
		pv.Search(SearchQuery{Text: entry.Text})
		if advance {
			pv.SearchNext()
		}
	}
	pv.SetOnSearchChanged(update)
	pv.SetOnSearchRequested(func() { focusObject(entry) })

	prev := iconBtn(iconArrowUp(), pv.SearchPrev)
	next := iconBtn(iconArrowDown(), pv.SearchNext)

	// The entry expands (Border center); the counter and nav buttons sit in one
	// HBox so entry→counter, counter→prev and prev→next are all one padding wide.
	return container.NewBorder(nil, nil, widget.NewIcon(iconSearch()),
		container.NewHBox(container.NewCenter(count), prev, next), entry)
}

// ShowOpenDialog opens Fyne's built-in file-open dialog (an in-canvas widget, not
// the OS-native picker — Fyne draws its own UI) and loads the chosen file into pv,
// auto-detecting the format. Exposed so hosts can trigger it from their own
// menu/button. A host that wants the platform-native dialog should instead set
// ToolbarConfig.OnOpen (or call its own picker) and feed the bytes to pv.SetData.
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
