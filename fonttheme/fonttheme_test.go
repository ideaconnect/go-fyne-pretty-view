package fonttheme

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

// TestBundledFacesLoad checks every embedded face is non-empty and parses as a
// font (Fyne loads it lazily, so we assert the bytes look like a TTF/OTF).
func TestBundledFacesLoad(t *testing.T) {
	for _, r := range []fyne.Resource{MonoRegular, MonoBold, SansRegular, SansBold, SansItalic, SansBoldItalic} {
		c := r.Content()
		if len(c) < 4 {
			t.Fatalf("%s: empty font resource", r.Name())
		}
		// TrueType ("\x00\x01\x00\x00") or OpenType ("OTTO").
		magic := string(c[:4])
		if magic != "\x00\x01\x00\x00" && magic != "OTTO" && magic != "true" && magic != "ttcf" {
			t.Errorf("%s: unexpected font magic %q", r.Name(), magic)
		}
	}
}

// TestFontSelection verifies the wrapper routes each text style to the expected
// face and falls back to the base theme for the symbol font.
func TestFontSelection(t *testing.T) {
	base := theme.DefaultTheme()
	th := New(base)

	cases := []struct {
		style fyne.TextStyle
		want  fyne.Resource
	}{
		{fyne.TextStyle{Monospace: true}, MonoRegular},
		{fyne.TextStyle{Monospace: true, Bold: true}, MonoBold},
		{fyne.TextStyle{}, SansRegular},
		{fyne.TextStyle{Bold: true}, SansBold},
		{fyne.TextStyle{Italic: true}, SansItalic},
		{fyne.TextStyle{Bold: true, Italic: true}, SansBoldItalic},
	}
	for _, c := range cases {
		if got := th.Font(c.style); got != c.want {
			t.Errorf("Font(%+v) = %s, want %s", c.style, got.Name(), c.want.Name())
		}
	}
	// Symbol delegates to base, not to one of our faces.
	if got := th.Font(fyne.TextStyle{Symbol: true}); got != base.Font(fyne.TextStyle{Symbol: true}) {
		t.Errorf("symbol font should delegate to base, got %s", got.Name())
	}
}

// TestWithFontsOverride verifies an override replaces only the named face and
// leaves the rest at their bundled defaults.
func TestWithFontsOverride(t *testing.T) {
	custom := fyne.NewStaticResource("custom-mono.ttf", []byte("\x00\x01\x00\x00custom"))
	th := New(theme.DefaultTheme(), WithFonts(Fonts{Mono: custom}))

	if got := th.Font(fyne.TextStyle{Monospace: true}); got != custom {
		t.Errorf("monospace override not applied: got %s", got.Name())
	}
	if got := th.Font(fyne.TextStyle{}); got != SansRegular {
		t.Errorf("UI font should stay Inter default: got %s", got.Name())
	}
}

// TestWithFontsComposes verifies that multiple options apply left to right (a
// later override wins per field) and that fields an option leaves nil survive
// from earlier options rather than reverting to the bundled default.
func TestWithFontsComposes(t *testing.T) {
	a := fyne.NewStaticResource("mono-a.ttf", []byte("\x00\x01\x00\x00a"))
	b := fyne.NewStaticResource("mono-b.ttf", []byte("\x00\x01\x00\x00b"))
	c := fyne.NewStaticResource("italic-c.ttf", []byte("\x00\x01\x00\x00c"))

	th := New(theme.DefaultTheme(), WithFonts(Fonts{Mono: a}), WithFonts(Fonts{Mono: b, Italic: c}))

	if got := th.Font(fyne.TextStyle{Monospace: true}); got != b {
		t.Errorf("later Mono override should win: got %s, want mono-b", got.Name())
	}
	if got := th.Font(fyne.TextStyle{Italic: true}); got != c {
		t.Errorf("Italic from the second option not applied: got %s", got.Name())
	}
	if got := th.Font(fyne.TextStyle{Bold: true}); got != SansBold {
		t.Errorf("untouched Bold should stay Inter default: got %s", got.Name())
	}
}

// TestDelegatesNonFontToBase pins the "overrides only the fonts" contract: every
// other theme method (Color, Size, Icon) must pass through to the base theme. The
// pass-through is via struct embedding today, so this guards against a future
// stray override or a switch away from embedding.
func TestDelegatesNonFontToBase(t *testing.T) {
	test.NewApp() // builtinTheme.Color/Size read the current app
	base := theme.DefaultTheme()
	th := New(base)

	for _, v := range []fyne.ThemeVariant{theme.VariantDark, theme.VariantLight} {
		for _, name := range []fyne.ThemeColorName{theme.ColorNameForeground, theme.ColorNameBackground, theme.ColorNamePrimary} {
			if th.Color(name, v) != base.Color(name, v) {
				t.Errorf("Color(%s, %d) not delegated to base", name, v)
			}
		}
	}
	for _, name := range []fyne.ThemeSizeName{theme.SizeNameText, theme.SizeNamePadding} {
		if th.Size(name) != base.Size(name) {
			t.Errorf("Size(%s) not delegated to base", name)
		}
	}
	if th.Icon(theme.IconNameSearch) != base.Icon(theme.IconNameSearch) {
		t.Error("Icon() not delegated to base")
	}
}

// TestNewNilBaseDoesNotPanic verifies a nil base is defaulted (rather than left to
// panic on the first inherited Color/Size/Icon or symbol-font call).
func TestNewNilBaseDoesNotPanic(t *testing.T) {
	test.NewApp() // so the only thing that could panic is a nil base, not a nil app
	th := New(nil)
	// These all delegate to the embedded base; with no guard they would panic.
	_ = th.Color(theme.ColorNameForeground, theme.VariantDark)
	_ = th.Size(theme.SizeNameText)
	_ = th.Font(fyne.TextStyle{Symbol: true})
	if got := th.Font(fyne.TextStyle{Monospace: true}); got != MonoRegular {
		t.Errorf("nil base should still install bundled fonts: got %s", got.Name())
	}
}
