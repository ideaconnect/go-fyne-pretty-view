package prettyview

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

func TestPaletteVariantsDiffer(t *testing.T) {
	dark := buildPalette(theme.VariantDark, nil)
	light := buildPalette(theme.VariantLight, nil)
	if len(dark) != int(model.NumColorRoles) || len(light) != int(model.NumColorRoles) {
		t.Fatalf("palette length: dark=%d light=%d want=%d", len(dark), len(light), model.NumColorRoles)
	}
	if dark[model.RoleString] == light[model.RoleString] {
		t.Error("string color should differ between dark and light variants")
	}
}

func TestSyntaxOverrideApplied(t *testing.T) {
	test.NewApp()
	variant := fyne.CurrentApp().Settings().ThemeVariant()
	custom := SyntaxColors{String: testColor{1, 2, 3, 255}}
	pv := New(WithSyntaxColors(variant, custom))
	win := test.NewWindow(pv)
	defer win.Close()
	pv.Refresh()
	if pv.palette[model.RoleString] != custom.String {
		t.Errorf("override not applied: got %v want %v", pv.palette[model.RoleString], custom.String)
	}
}

type testColor struct{ r, g, b, a uint8 }

func (c testColor) RGBA() (r, g, b, a uint32) {
	return uint32(c.r) << 8, uint32(c.g) << 8, uint32(c.b) << 8, uint32(c.a) << 8
}
