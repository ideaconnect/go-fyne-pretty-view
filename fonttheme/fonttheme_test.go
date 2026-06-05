package fonttheme

import (
	"testing"

	"fyne.io/fyne/v2"
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
