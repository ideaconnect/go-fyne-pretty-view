package prettyview

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// iconButton is a compact, icon-only, low-importance button that shows a small text
// tooltip on hover — the equivalent of an HTML title="…". Fyne 2.7 has no built-in
// widget tooltip, so we extend widget.Button (keeping its hover highlight, focus and
// tap behavior) and pop a label overlay on MouseIn / hide it on MouseOut.
type iconButton struct {
	widget.Button
	tip string
	pop *widget.PopUp
}

// newIconButton builds an icon-only button with a hover tooltip. tip is the hover
// title; tapped is the action.
func newIconButton(icon fyne.Resource, tip string, tapped func()) *iconButton {
	b := &iconButton{tip: tip}
	b.Icon = icon
	b.Importance = widget.LowImportance
	b.OnTapped = tapped
	b.ExtendBaseWidget(b)
	return b
}

func (b *iconButton) MouseIn(e *desktop.MouseEvent) {
	b.Button.MouseIn(e) // preserve the standard hover highlight
	b.showTip()
}

func (b *iconButton) MouseOut() {
	b.Button.MouseOut()
	b.hideTip()
}

func (b *iconButton) showTip() {
	if b.tip == "" {
		return
	}
	app := fyne.CurrentApp()
	if app == nil || app.Driver() == nil {
		return
	}
	c := app.Driver().CanvasForObject(b)
	if c == nil {
		return
	}
	if b.pop == nil {
		bg := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
		bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
		bg.StrokeWidth = 1
		b.pop = widget.NewPopUp(container.NewStack(bg, widget.NewLabel(b.tip)), c)
	}
	pos := app.Driver().AbsolutePositionForObject(b)
	b.pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+b.Size().Height+theme.Padding()))
}

func (b *iconButton) hideTip() {
	if b.pop != nil {
		b.pop.Hide()
	}
}
