// Package fonttheme bundles the typefaces go-fyne-pretty-view ships with —
// JetBrains Mono for monospace (the viewer body) and Inter for UI text — and
// exposes them as a fyne.Theme you can install on your app.
//
// The fonts are embedded in THIS package only, not in the core prettyview
// package, so importing the widget does not pull ~2 MB of font data into your
// binary. You opt in by importing fonttheme and installing its theme:
//
//	import (
//		"fyne.io/fyne/v2/app"
//		"fyne.io/fyne/v2/theme"
//		"github.com/ideaconnect/go-fyne-pretty-view/fonttheme"
//	)
//
//	a := app.New()
//	a.Settings().SetTheme(fonttheme.New(theme.DefaultTheme()))
//
// New wraps a base theme and overrides only its fonts, so all colors, sizes and
// icons of the base theme are preserved. Override any of the bundled faces with
// your own via WithFonts (a nil field keeps the bundled default):
//
//	a.Settings().SetTheme(fonttheme.New(theme.DefaultTheme(), fonttheme.WithFonts(fonttheme.Fonts{
//		Mono: myMonoResource, // keep Inter for UI, swap only the monospace face
//	})))
//
// Fonts are licensed under the SIL Open Font License 1.1 — see fonts/Inter/
// LICENSE.txt and fonts/JetBrainsMono/OFL.txt. If you ship a binary built with
// this package you redistribute those fonts; the OFL only asks that you carry the
// license text (which is embedded here) — see the README's licensing notes.
package fonttheme

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed fonts/JetBrainsMono/JetBrainsMono-Regular.ttf
var monoRegular []byte

//go:embed fonts/JetBrainsMono/JetBrainsMono-Bold.ttf
var monoBold []byte

//go:embed fonts/Inter/Inter-Regular.ttf
var sansRegular []byte

//go:embed fonts/Inter/Inter-Bold.ttf
var sansBold []byte

//go:embed fonts/Inter/Inter-Italic.ttf
var sansItalic []byte

//go:embed fonts/Inter/Inter-BoldItalic.ttf
var sansBoldItalic []byte

// The bundled faces, exposed so callers can reuse one without rebuilding it (e.g.
// pass MonoRegular to another widget, or to WithFonts to keep it explicitly).
var (
	MonoRegular    = fyne.NewStaticResource("JetBrainsMono-Regular.ttf", monoRegular)
	MonoBold       = fyne.NewStaticResource("JetBrainsMono-Bold.ttf", monoBold)
	SansRegular    = fyne.NewStaticResource("Inter-Regular.ttf", sansRegular)
	SansBold       = fyne.NewStaticResource("Inter-Bold.ttf", sansBold)
	SansItalic     = fyne.NewStaticResource("Inter-Italic.ttf", sansItalic)
	SansBoldItalic = fyne.NewStaticResource("Inter-BoldItalic.ttf", sansBoldItalic)
)

// Fonts overrides the faces New installs. A nil field keeps the bundled default
// (JetBrains Mono for monospace, Inter for UI text).
type Fonts struct {
	Mono       fyne.Resource // monospace, all weights (default JetBrains Mono Regular)
	MonoBold   fyne.Resource // bold monospace (default JetBrains Mono Bold)
	Regular    fyne.Resource // UI regular (default Inter Regular)
	Bold       fyne.Resource // UI bold (default Inter Bold)
	Italic     fyne.Resource // UI italic (default Inter Italic)
	BoldItalic fyne.Resource // UI bold italic (default Inter Bold Italic)
}

func defaultFonts() Fonts {
	return Fonts{
		Mono:       MonoRegular,
		MonoBold:   MonoBold,
		Regular:    SansRegular,
		Bold:       SansBold,
		Italic:     SansItalic,
		BoldItalic: SansBoldItalic,
	}
}

// mergeInto overlays f's non-nil fields onto dst.
func (f Fonts) mergeInto(dst *Fonts) {
	set := func(d *fyne.Resource, o fyne.Resource) {
		if o != nil {
			*d = o
		}
	}
	set(&dst.Mono, f.Mono)
	set(&dst.MonoBold, f.MonoBold)
	set(&dst.Regular, f.Regular)
	set(&dst.Bold, f.Bold)
	set(&dst.Italic, f.Italic)
	set(&dst.BoldItalic, f.BoldItalic)
}

// Option customizes the theme built by New.
type Option func(*Fonts)

// WithFonts overlays the given faces onto the bundled defaults (nil fields are
// left at their default). Options compose, applied left to right.
func WithFonts(f Fonts) Option { return func(dst *Fonts) { f.mergeInto(dst) } }

// fontTheme wraps a base fyne.Theme, overriding only Font; Color, Size and Icon
// are inherited from base via the embedded interface.
type fontTheme struct {
	fyne.Theme
	f Fonts
}

// New returns a fyne.Theme that draws base's colors, sizes and icons but renders
// text with JetBrains Mono (monospace) and Inter (UI), as overridden by opts.
// Install it with app.Settings().SetTheme(...). The symbol font is left to base.
func New(base fyne.Theme, opts ...Option) fyne.Theme {
	f := defaultFonts()
	for _, o := range opts {
		o(&f)
	}
	return &fontTheme{Theme: base, f: f}
}

func (t *fontTheme) Font(s fyne.TextStyle) fyne.Resource {
	if s.Symbol {
		return t.Theme.Font(s)
	}
	if s.Monospace {
		if s.Bold {
			return t.f.MonoBold
		}
		return t.f.Mono
	}
	switch {
	case s.Bold && s.Italic:
		return t.f.BoldItalic
	case s.Bold:
		return t.f.Bold
	case s.Italic:
		return t.f.Italic
	default:
		return t.f.Regular
	}
}
