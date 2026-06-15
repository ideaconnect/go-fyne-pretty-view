package prettyview

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// TestWithAlphaPreservesHue is the regression for the premultiplied-RGB bug: for a
// non-opaque source color, withAlpha must keep the straight R/G/B and only replace
// the alpha. Reading c.RGBA() directly (premultiplied) would darken/desaturate it.
func TestWithAlphaPreservesHue(t *testing.T) {
	src := color.NRGBA{R: 0x00, G: 0x6c, B: 0xff, A: 0x40} // Fyne-like semi-transparent blue
	got := withAlpha(src, 0x66)
	want := color.NRGBA{R: 0x00, G: 0x6c, B: 0xff, A: 0x66}
	if got != want {
		t.Errorf("withAlpha = %v, want %v (must preserve straight RGB, only set alpha)", got, want)
	}
}

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

// TestResetThemeClearsOverrides is the #103 regression: ResetTheme must drop every override
// for a variant and revert to the built-in defaults (which the additive SetTheme cannot undo,
// since a nil field keeps the prior override).
func TestResetThemeClearsOverrides(t *testing.T) {
	test.NewApp()
	variant := fyne.CurrentApp().Settings().ThemeVariant()
	sel := testColor{10, 20, 30, 200}
	str := testColor{70, 80, 90, 255}
	pv := New(WithTheme(variant, Theme{Selection: sel}), WithSyntaxColors(variant, SyntaxColors{String: str}))
	win := test.NewWindow(pv)
	defer win.Close()
	pv.Refresh()
	if pv.selColor != sel || pv.palette[model.RoleString] != str {
		t.Fatal("precondition: overrides should be applied before reset")
	}

	def := defaultTheme(variant)
	pv.ResetTheme(variant)
	if pv.selColor != def.Selection {
		t.Errorf("ResetTheme did not restore the default selection: got %v want %v", pv.selColor, def.Selection)
	}
	if pv.palette[model.RoleString] != def.String {
		t.Errorf("ResetTheme did not restore the default string color: got %v want %v", pv.palette[model.RoleString], def.String)
	}
	// And it composes with a fresh override afterward (the map entry was cleared, not corrupted).
	pv.SetSyntaxColors(variant, SyntaxColors{Number: testColor{5, 5, 5, 255}})
	if pv.palette[model.RoleNumber] != (testColor{5, 5, 5, 255}) {
		t.Error("override after ResetTheme did not apply")
	}
	if pv.palette[model.RoleString] != def.String {
		t.Error("a post-reset override revived a cleared one")
	}
}

// TestRecomputeMetricsMemoized verifies the metrics/palette are rebuilt only on a
// theme change, not on every data/fold/search refresh, and that a runtime override
// correctly forces a rebuild.
func TestRecomputeMetricsMemoized(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":1}`), FormatJSON)
	win := test.NewWindow(pv)
	defer win.Close()
	pv.Refresh()
	if !pv.metricsReady {
		t.Fatal("metrics should be ready after a render")
	}
	met0 := pv.met
	palette0 := pv.palette

	// A plain refresh (no theme/variant/text-size change) must reuse the measured
	// cell and the same palette backing array — no MeasureText, no realloc.
	pv.Refresh()
	if pv.met != met0 {
		t.Error("metrics changed on a non-theme refresh")
	}
	if &pv.palette[0] != &palette0[0] {
		t.Error("palette was reallocated on a non-theme refresh (memo missed)")
	}

	// A runtime syntax override must rebuild the palette.
	variant := fyne.CurrentApp().Settings().ThemeVariant()
	pv.SetSyntaxColors(variant, SyntaxColors{Number: testColor{1, 2, 3, 255}})
	if pv.palette[model.RoleNumber] != (testColor{1, 2, 3, 255}) {
		t.Error("SetSyntaxColors did not rebuild the palette")
	}
}

type testColor struct{ r, g, b, a uint8 }

func (c testColor) RGBA() (r, g, b, a uint32) {
	return uint32(c.r) << 8, uint32(c.g) << 8, uint32(c.b) << 8, uint32(c.a) << 8
}
