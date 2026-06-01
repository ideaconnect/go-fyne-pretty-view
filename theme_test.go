package prettyview

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

func TestPaletteVariantsDiffer(t *testing.T) {
	test.NewApp()
	dark := defaultTheme(theme.VariantDark).palette()
	light := defaultTheme(theme.VariantLight).palette()
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

// TestThemeOverrideApplied checks that non-syntax colors (selection, match,
// guide, summary) are overridable via WithTheme, and that overrides compose with
// WithSyntaxColors without clobbering each other.
func TestThemeOverrideApplied(t *testing.T) {
	test.NewApp()
	variant := fyne.CurrentApp().Settings().ThemeVariant()
	sel := testColor{10, 20, 30, 200}
	match := testColor{40, 50, 60, 200}
	str := testColor{70, 80, 90, 255}

	pv := New(
		WithTheme(variant, Theme{Selection: sel, Match: match}),
		WithSyntaxColors(variant, SyntaxColors{String: str}), // must compose, not clobber
	)
	win := test.NewWindow(pv)
	defer win.Close()
	pv.Refresh()

	if pv.selColor != sel {
		t.Errorf("selection override not applied: got %v", pv.selColor)
	}
	if pv.matchColor != match {
		t.Errorf("match override not applied: got %v", pv.matchColor)
	}
	if pv.palette[model.RoleString] != str {
		t.Errorf("syntax override clobbered by theme override: got %v", pv.palette[model.RoleString])
	}
	// A field left unset keeps its default (active match unchanged).
	if pv.activeMatchColor == nil {
		t.Error("unset field should keep its default, not become nil")
	}

	// Runtime SetTheme also works and composes.
	pv.SetTheme(variant, Theme{ActiveMatch: testColor{1, 1, 1, 255}})
	if pv.matchColor != match {
		t.Error("SetTheme clobbered an earlier override")
	}
}

type testColor struct{ r, g, b, a uint8 }

func (c testColor) RGBA() (r, g, b, a uint32) {
	return uint32(c.r) << 8, uint32(c.g) << 8, uint32(c.b) << 8, uint32(c.a) << 8
}
