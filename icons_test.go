package prettyview

import (
	"bytes"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/fyne-io/oksvg"
)

// iconGetters is every embedded toolbar glyph's accessor.
var iconGetters = []func(color.Color) fyne.Resource{
	iconSearch, iconFolder, iconWrapText, iconExpand, iconCollapse, iconArrowUp, iconArrowDown,
}

// TestIconResourcesThemed verifies every embedded Font Awesome glyph, built with no
// explicit color, is a fyne.ThemedResource on the theme foreground: that is what lets
// Fyne recolor it at draw time (a runtime theme switch, and widget.Button's contrast
// recolor on a HighImportance fill). Its colorized content must still be a real SVG
// whose fill is the theme foreground, so a theme change reaches the pixels.
func TestIconResourcesThemed(t *testing.T) {
	test.NewApp()
	variant := fyne.CurrentApp().Settings().ThemeVariant()
	wantFill := []byte(`fill="` + colorToHex(themeColor(theme.ColorNameForeground, variant)) + `"`)

	for _, get := range iconGetters {
		res := get(nil)
		if res == nil || len(res.Content()) == 0 {
			t.Fatal("nil/empty icon resource")
		}
		themed, ok := res.(fyne.ThemedResource)
		if !ok {
			t.Errorf("%s is not a fyne.ThemedResource (Fyne will not recolor it)", res.Name())
			continue
		}
		if themed.ThemeColorName() != theme.ColorNameForeground {
			t.Errorf("%s colors from %q, want the theme foreground", res.Name(), themed.ThemeColorName())
		}
		if bytes.Contains(res.Content(), []byte("currentColor")) {
			t.Errorf("%s colorized content still contains currentColor", res.Name())
		}
		if !bytes.Contains(res.Content(), wantFill) {
			t.Errorf("%s colorized content lacks the foreground fill %q", res.Name(), wantFill)
		}
	}
}

// TestIconResourcesExplicitColor verifies a glyph built with an explicit color is a plain
// static resource (so Fyne leaves it alone on any button importance) with exactly that
// color baked in for currentColor, checked against an independently derived hex so a
// wrong-color-source regression (theme foreground, a constant) fails.
func TestIconResourcesExplicitColor(t *testing.T) {
	test.NewApp()
	want := color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}
	wantFill := []byte(`fill="#123456"`)
	for _, get := range iconGetters {
		res := get(want)
		if _, themed := res.(fyne.ThemedResource); themed {
			t.Errorf("%s with an explicit color is still themed (Fyne would recolor it)", res.Name())
		}
		if bytes.Contains(res.Content(), []byte("currentColor")) {
			t.Errorf("%s still contains currentColor (not recolored)", res.Name())
		}
		if !bytes.Contains(res.Content(), wantFill) {
			t.Errorf("%s was not recolored to the explicit color %q", res.Name(), wantFill)
		}
		if _, err := oksvg.ReadIconStream(bytes.NewReader(res.Content())); err != nil {
			t.Errorf("%s no longer parses as SVG: %v", res.Name(), err)
		}
	}
}

// TestIconResourcesParse decodes every recolored glyph through the same SVG parser
// Fyne renders with, guarding the one thing the icon swap must guarantee: that the
// vendored (and recolored) Font Awesome SVGs are still well-formed and renderable.
// The substring checks above assert intent; this asserts the bytes actually parse.
// TestColorToHexStraightAlpha pins the premultiplied-alpha fix: colorToHex must
// read straight (NRGBA) channels, so a non-opaque themed foreground bakes its true
// color rather than a darkened/premultiplied one. Reading c.RGBA() directly would
// scale each channel by alpha (here ×0.5) and yield #643219 instead of #c86432.
func TestColorToHexStraightAlpha(t *testing.T) {
	got := colorToHex(color.NRGBA{R: 200, G: 100, B: 50, A: 128})
	if got != "#c86432" {
		t.Errorf("colorToHex(half-transparent) = %s, want #c86432 (straight alpha)", got)
	}
}

func TestIconResourcesParse(t *testing.T) {
	test.NewApp()
	for _, get := range iconGetters {
		res := get(nil)
		if _, err := oksvg.ReadIconStream(bytes.NewReader(res.Content())); err != nil {
			t.Errorf("%s no longer parses as SVG: %v", res.Name(), err)
		}
	}
}
