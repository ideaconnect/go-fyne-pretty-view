package prettyview

import (
	"fmt"
	"io"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

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
	ShowFormat         bool        // a format selector (auto/json/jsonc/xml/html/raw)
	ShowExpandCollapse bool        // Expand all / Collapse all buttons
	ShowWrap           bool        // a wrap-text icon toggle (soft-wrap on/off)
	ShowSearch         bool        // a find box with prev/next and a match counter
	Window             fyne.Window // enables the built-in Open dialog and Ctrl/Cmd+F focus
	OnOpen             func()      // overrides the built-in Open behavior, if set
}

// DefaultToolbarConfig enables every control bound to win: the Open button (its
// built-in file dialog and Ctrl/Cmd+F search-focus both need a Window). Pass nil to
// omit those two (or set OnOpen yourself afterward to drive Open without a Window).
func DefaultToolbarConfig(win fyne.Window) ToolbarConfig {
	return ToolbarConfig{
		ShowOpen: true, ShowFormat: true, ShowExpandCollapse: true,
		ShowWrap: true, ShowSearch: true, Window: win,
	}
}

// NewToolbar builds an optional control bar bound to pv from cfg. Disabled
// controls are omitted. The result is a plain fyne.CanvasObject; place it where
// you like (typically the top of a container.NewBorder around the PrettyView).
func NewToolbar(pv *PrettyView, cfg ToolbarConfig) fyne.CanvasObject {
	left := container.NewHBox()
	if cfg.ShowOpen && (cfg.OnOpen != nil || cfg.Window != nil) {
		left.Add(iconBtn(iconFolder(), "Open file…", func() {
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

// iconBtn is a compact, flat (low-importance) icon-only button with a hover tooltip
// (tip) — icon-only buttons need a label affordance, and Fyne core has no tooltips, so
// this uses the fyne-tooltip add-on. The tooltip only renders if the host wraps its
// window content in fynetooltip.AddWindowToolTipLayer (see the README); otherwise it is
// simply absent.
func iconBtn(icon fyne.Resource, tip string, tapped func()) *ttwidget.Button {
	b := ttwidget.NewButtonWithIcon("", icon, tapped)
	b.Importance = widget.LowImportance
	b.SetToolTip(tip)
	return b
}

// iconLabel renders an icon with the SAME size and padding as iconBtn but inert: it is
// a disabled low-importance button, so it reads as a button-shaped affordance (aligned
// in the control row) without being clickable. Used for the search bar's leading
// magnifier glyph, which is a label, not an action.
func iconLabel(icon fyne.Resource, tip string) *ttwidget.Button {
	b := ttwidget.NewButtonWithIcon("", icon, func() {})
	b.Importance = widget.LowImportance
	b.SetToolTip(tip)
	b.Disable()
	return b
}

// NewFoldButtons returns an expand-all / collapse-all icon pair bound to pv.
func NewFoldButtons(pv *PrettyView) fyne.CanvasObject {
	return container.NewHBox(
		iconBtn(iconExpand(), "Expand all", pv.ExpandAll),
		iconBtn(iconCollapse(), "Collapse all", pv.CollapseAll),
	)
}

// NewWrapToggle returns a wrap-text icon toggle bound to pv: it flips between
// soft-wrap (WrapWord) and horizontal scroll (WrapNone), and is highlighted
// (HighImportance) while wrapping is on so the state is visible.
func NewWrapToggle(pv *PrettyView) fyne.CanvasObject {
	var btn *ttwidget.Button
	btn = iconBtn(iconWrapText(), "Toggle soft-wrap", func() {
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
	pv.addOnDataChangedHook(func() { // internal hook: does not clobber a host SetOnDataChanged (#99)
		sel.Selected = pv.Format().String()
		sel.Refresh()
	})
	return sel
}

// searchEntry is the find box: a single-line Entry that adds Esc-to-clear and
// Shift+Enter to find the previous match (plain Enter finds next via OnSubmitted).
// Shift state is tracked from KeyDown/KeyUp since TypedKey carries no modifier.
type searchEntry struct {
	widget.Entry
	shiftHeld bool
	onPrev    func() // Shift+Enter
	onEscape  func() // Esc
}

func newSearchEntry() *searchEntry {
	e := &searchEntry{}
	e.ExtendBaseWidget(e)
	e.SetPlaceHolder("search…")
	return e
}

func (e *searchEntry) KeyDown(key *fyne.KeyEvent) {
	if key.Name == desktop.KeyShiftLeft || key.Name == desktop.KeyShiftRight {
		e.shiftHeld = true
	}
	e.Entry.KeyDown(key)
}

func (e *searchEntry) KeyUp(key *fyne.KeyEvent) {
	if key.Name == desktop.KeyShiftLeft || key.Name == desktop.KeyShiftRight {
		e.shiftHeld = false
	}
	e.Entry.KeyUp(key)
}

// FocusLost clears the held-Shift flag: glfw delivers KeyUp only to the focused
// object, so a Shift released while the entry is unfocused would otherwise stick and
// mis-route the next Enter to find-previous.
func (e *searchEntry) FocusLost() {
	e.shiftHeld = false
	e.Entry.FocusLost()
}

func (e *searchEntry) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyEscape:
		if e.onEscape != nil {
			e.onEscape()
			return
		}
	case fyne.KeyReturn, fyne.KeyEnter:
		if e.shiftHeld && e.onPrev != nil {
			e.onPrev()
			return
		}
	}
	e.Entry.TypedKey(key)
}

// NewSearchBar returns a find box (with case-sensitive and regex toggles, prev/next,
// and a self-updating match counter) bound to pv. It registers PrettyView's
// search-changed and search-requested hooks so the counter stays in sync and
// Ctrl/Cmd+F focuses it. Enter finds next, Shift+Enter finds previous, Esc clears.
func NewSearchBar(pv *PrettyView) fyne.CanvasObject {
	entry := newSearchEntry()
	// A canvas.Text (not a widget.Label) for the match counter: a Label's inner
	// padding would make the entry→counter gap wider than the gap between the nav
	// buttons. Centered so it lines up vertically with the buttons.
	count := canvas.NewText("", theme.Color(theme.ColorNameForeground))

	caseSensitive, useRegex := false, false
	query := func() SearchQuery {
		mode := SearchPlain
		if useRegex {
			mode = SearchRegex
		}
		return SearchQuery{Text: entry.Text, CaseSensitive: caseSensitive, Mode: mode}
	}

	update := func() {
		if pv.SearchError() != nil {
			count.Text = "bad regex" // an invalid SearchRegex pattern (see SearchError)
			count.Refresh()
			return
		}
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
	entry.OnChanged = func(string) { pv.searchDebounced(query()) }
	entry.OnSubmitted = func(string) {
		// If the box's query is already the applied one, Enter means find-next, so just
		// advance — re-running Search would reset the active match to #1 (so every Enter
		// would snap back to #2). Otherwise Enter beat the debounce (or the query/toggles
		// changed): apply it, which reveals match #1 without jumping past it.
		if pv.search.query == query() {
			pv.SearchNext()
			return
		}
		pv.Search(query())
	}
	entry.onPrev = func() {
		// Same contract for Shift+Enter: step backward on the applied query, only
		// re-Search (revealing #1, then wrapping to the last) when the query changed.
		if pv.search.query == query() {
			pv.SearchPrev()
			return
		}
		pv.Search(query())
		pv.SearchPrev()
	}
	entry.onEscape = func() {
		entry.SetText("")
		pv.ClearSearch()
	}
	pv.addOnSearchChangedHook(update)                          // internal hooks: do not clobber a host's
	pv.addOnSearchRequestedHook(func() { focusObject(entry) }) // SetOnSearchChanged/Requested (#99)

	// Case-sensitive and regex toggles: HighImportance highlights the active state,
	// and toggling re-runs the current query so the result set updates immediately.
	toggleImportance := func(on bool) widget.Importance {
		if on {
			return widget.HighImportance
		}
		return widget.LowImportance
	}
	var caseBtn, regexBtn *ttwidget.Button
	caseBtn = ttwidget.NewButton("Aa", func() {
		caseSensitive = !caseSensitive
		caseBtn.Importance = toggleImportance(caseSensitive)
		caseBtn.Refresh()
		if entry.Text != "" {
			pv.Search(query())
		}
	})
	caseBtn.Importance = widget.LowImportance
	caseBtn.SetToolTip("Case-sensitive")
	regexBtn = ttwidget.NewButton(".*", func() {
		useRegex = !useRegex
		regexBtn.Importance = toggleImportance(useRegex)
		regexBtn.Refresh()
		if entry.Text != "" {
			pv.Search(query())
		}
	})
	regexBtn.Importance = widget.LowImportance
	regexBtn.SetToolTip("Regular expression")

	prev := iconBtn(iconArrowUp(), "Previous match", pv.SearchPrev)
	next := iconBtn(iconArrowDown(), "Find next", pv.SearchNext)

	// The entry expands (Border center); the counter, toggles and nav buttons sit in
	// one HBox so the inter-control gaps are all one padding wide.
	return container.NewBorder(nil, nil, iconLabel(iconSearch(), "Search"),
		container.NewHBox(container.NewCenter(count), caseBtn, regexBtn, prev, next), entry)
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
		loadFromReader(pv, win, rc, err)
	}, win)
}

// loadFromReader handles a file-open dialog result: a picker error is surfaced (not
// swallowed), a cancel (nil reader, no error) is silent, otherwise the file is read
// (surfacing any read error) and loaded with auto-detection. The read is bounded by the
// configured WithMaxInputBytes, so the bundled loader can never read a multi-gigabyte file
// whole into memory (#73): an over-cap file is refused with an error rather than silently
// truncated, since a file open is an explicit user action.
func loadFromReader(pv *PrettyView, win fyne.Window, rc fyne.URIReadCloser, err error) {
	if err != nil {
		dialog.ShowError(err, win)
		return
	}
	if rc == nil {
		return // user cancelled
	}
	defer rc.Close()
	data, tooLarge, err := readCapped(rc, pv.cfg.maxInputBytes)
	if err != nil {
		dialog.ShowError(err, win)
		return
	}
	if tooLarge {
		dialog.ShowError(fmt.Errorf("file exceeds the %d-byte limit set via WithMaxInputBytes", pv.cfg.maxInputBytes), win)
		return
	}
	pv.SetData(data, FormatAuto)
}

// readCapped reads all of r, but bounds the read (hence the allocation) to cap+1 bytes when
// cap > 0, so an oversized input is detected without ever reading the whole file. It returns
// the bytes (capped to cap) and whether the input exceeded cap. cap <= 0 means no cap.
func readCapped(r io.Reader, cap int) (data []byte, tooLarge bool, err error) {
	if cap > 0 {
		r = io.LimitReader(r, int64(cap)+1) // +1 byte lets ReadAll see the overflow
	}
	data, err = io.ReadAll(r)
	if err != nil {
		return nil, false, err
	}
	if cap > 0 && len(data) > cap {
		return data[:cap], true, nil
	}
	return data, false, nil
}

func registerFindShortcut(win fyne.Window, pv *PrettyView) {
	win.Canvas().AddShortcut(
		&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault},
		func(fyne.Shortcut) {
			pv.notifySearchRequested() // bundled search bar + host hook both fire (#99)
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
