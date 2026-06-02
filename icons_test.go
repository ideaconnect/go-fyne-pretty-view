package prettyview

import (
	"bytes"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

// TestIconResourcesRecolored verifies every embedded Iconoir glyph loads and has
// its stroke recolored from currentColor to a concrete theme-foreground hex (Fyne
// does not theme stroke icons, so this baking is what makes them visible).
func TestIconResourcesRecolored(t *testing.T) {
	test.NewApp()
	for _, get := range []func() fyne.Resource{
		iconSearch, iconFolder, iconWrapText, iconExpand, iconCollapse, iconArrowUp, iconArrowDown,
	} {
		res := get()
		if res == nil || len(res.Content()) == 0 {
			t.Fatal("nil/empty icon resource")
		}
		if bytes.Contains(res.Content(), []byte("currentColor")) {
			t.Errorf("%s still contains currentColor (not recolored)", res.Name())
		}
		if !bytes.Contains(res.Content(), []byte(`stroke="#`)) {
			t.Errorf("%s stroke was not set to a hex color", res.Name())
		}
	}
}

// TestIconButtonTooltip exercises the hover-tooltip icon button: show on MouseIn,
// hide on MouseOut, and the tap action; an empty tip pops nothing.
func TestIconButtonTooltip(t *testing.T) {
	test.NewApp()
	tapped := false
	btn := newIconButton(iconSearch(), "Search", func() { tapped = true })
	win := test.NewWindow(btn)
	defer win.Close()
	win.Resize(fyne.NewSize(120, 80))

	btn.MouseIn(&desktop.MouseEvent{}) // -> showTip (creates the popup)
	btn.MouseIn(&desktop.MouseEvent{}) // -> reuses the popup
	btn.MouseOut()                     // -> hideTip
	btn.OnTapped()
	if !tapped {
		t.Error("icon button tap did not fire its action")
	}

	newIconButton(iconFolder(), "", nil).showTip() // empty tip: no-op, no panic
}
