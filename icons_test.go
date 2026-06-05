package prettyview

import (
	"bytes"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/fyne-io/oksvg"
)

// iconGetters is every embedded toolbar glyph's accessor.
var iconGetters = []func() fyne.Resource{
	iconSearch, iconFolder, iconWrapText, iconExpand, iconCollapse, iconArrowUp, iconArrowDown,
}

// TestIconResourcesRecolored verifies every embedded Font Awesome glyph loads and
// has its fill recolored from currentColor to the active theme-foreground hex, so
// the icon tracks the installed theme regardless of how Fyne colorizes resources.
func TestIconResourcesRecolored(t *testing.T) {
	test.NewApp()

	// The exact hex the recolor should bake in: the foreground the icons resolve
	// through (themeColor → installed theme), for the active variant. Asserting
	// against this independently-derived value catches a wrong-color-source
	// regression (e.g. baking background or a constant) that a bare fill="#…"
	// substring check would miss.
	variant := fyne.CurrentApp().Settings().ThemeVariant()
	wantFill := []byte(`fill="` + colorToHex(themeColor(theme.ColorNameForeground, variant)) + `"`)

	for _, get := range iconGetters {
		res := get()
		if res == nil || len(res.Content()) == 0 {
			t.Fatal("nil/empty icon resource")
		}
		if bytes.Contains(res.Content(), []byte("currentColor")) {
			t.Errorf("%s still contains currentColor (not recolored)", res.Name())
		}
		if !bytes.Contains(res.Content(), wantFill) {
			t.Errorf("%s was not recolored to the foreground %q", res.Name(), wantFill)
		}
	}
}

// TestIconResourcesParse decodes every recolored glyph through the same SVG parser
// Fyne renders with, guarding the one thing the icon swap must guarantee: that the
// vendored (and recolored) Font Awesome SVGs are still well-formed and renderable.
// The substring checks above assert intent; this asserts the bytes actually parse.
func TestIconResourcesParse(t *testing.T) {
	test.NewApp()
	for _, get := range iconGetters {
		res := get()
		if _, err := oksvg.ReadIconStream(bytes.NewReader(res.Content())); err != nil {
			t.Errorf("%s no longer parses as SVG: %v", res.Name(), err)
		}
	}
}
