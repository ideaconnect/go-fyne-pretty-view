package prettyview

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// Theme is the full set of colors the viewer draws. Any nil field falls back to
// the built-in default for the active theme variant — and the structural
// defaults (Foreground, Summary, IndentGuide, Selection) themselves follow the
// host Fyne app theme, so an un-themed PrettyView blends into its surroundings.
//
// Override per variant with WithTheme (construction) or SetTheme (at runtime).
// Overrides compose: setting only a few fields leaves the rest at their default,
// and repeated calls / WithSyntaxColors merge rather than replace.
type Theme struct {
	// Syntax token colors.
	Key     color.Color
	String  color.Color
	Number  color.Color
	Bool    color.Color
	Null    color.Color
	Punct   color.Color
	Tag     color.Color
	Attr    color.Color
	Comment color.Color

	// Structural / UI colors.
	Foreground  color.Color // default ("plain") text and punctuation fallback
	Summary     color.Color // fold-summary text, e.g. "{ 6 items }"
	IndentGuide color.Color // the vertical indent guide lines
	Selection   color.Color // free-text selection highlight fill
	Match       color.Color // inactive search-match highlight fill
	ActiveMatch color.Color // active search-match highlight fill
}

// SyntaxColors is the token-color subset of Theme, kept for the common case of
// recoloring only the syntax. A nil field keeps the default.
type SyntaxColors struct {
	Key     color.Color
	String  color.Color
	Number  color.Color
	Bool    color.Color
	Null    color.Color
	Punct   color.Color
	Tag     color.Color
	Attr    color.Color
	Comment color.Color
}

// asTheme lifts a SyntaxColors into a Theme with only the token fields set.
func (s SyntaxColors) asTheme() Theme {
	return Theme{
		Key: s.Key, String: s.String, Number: s.Number, Bool: s.Bool, Null: s.Null,
		Punct: s.Punct, Tag: s.Tag, Attr: s.Attr, Comment: s.Comment,
	}
}

// themeColor resolves a Fyne theme color for a specific variant via the active
// theme interface (the package-level theme.Color only knows the current variant).
// It falls back to the bundled default theme when no app/settings exist yet (e.g.
// SetData called before app.New()), so headless construction never panics.
func themeColor(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if app := fyne.CurrentApp(); app != nil && app.Settings() != nil && app.Settings().Theme() != nil {
		return app.Settings().Theme().Color(name, variant)
	}
	return theme.DefaultTheme().Color(name, variant)
}

// withAlpha returns c with its alpha replaced by a, preserving the source hue.
// It converts through the straight (non-premultiplied) NRGBA model first: reading
// c.RGBA() directly would give alpha-PREMULTIPLIED channels, so a non-opaque input
// (e.g. Fyne's selection color, alpha ~0x40) would come back darkened/desaturated.
func withAlpha(c color.Color, a uint8) color.NRGBA {
	nc := color.NRGBAModel.Convert(c).(color.NRGBA)
	nc.A = a
	return nc
}

// defaultSyntaxColors returns the built-in (Bruno-ish, VS Code Dark+ inspired)
// token palette for a theme variant.
func defaultSyntaxColors(variant fyne.ThemeVariant) SyntaxColors {
	if variant == theme.VariantLight {
		return SyntaxColors{
			Key:     color.NRGBA{0x00, 0x37, 0xA6, 0xff},
			String:  color.NRGBA{0xA3, 0x15, 0x15, 0xff},
			Number:  color.NRGBA{0x09, 0x69, 0x5d, 0xff},
			Bool:    color.NRGBA{0x00, 0x00, 0xff, 0xff},
			Null:    color.NRGBA{0x00, 0x00, 0xff, 0xff},
			Punct:   color.NRGBA{0x3b, 0x3b, 0x3b, 0xff},
			Tag:     color.NRGBA{0x22, 0x00, 0x9b, 0xff},
			Attr:    color.NRGBA{0x00, 0x37, 0xA6, 0xff},
			Comment: color.NRGBA{0x44, 0x80, 0x44, 0xff},
		}
	}
	return SyntaxColors{
		Key:     color.NRGBA{0x9C, 0xDC, 0xFE, 0xff},
		String:  color.NRGBA{0xCE, 0x91, 0x78, 0xff},
		Number:  color.NRGBA{0xB5, 0xCE, 0xA8, 0xff},
		Bool:    color.NRGBA{0x56, 0x9C, 0xD6, 0xff},
		Null:    color.NRGBA{0x56, 0x9C, 0xD6, 0xff},
		Punct:   color.NRGBA{0xD4, 0xD4, 0xD4, 0xff},
		Tag:     color.NRGBA{0x56, 0x9C, 0xD6, 0xff},
		Attr:    color.NRGBA{0x9C, 0xDC, 0xFE, 0xff},
		Comment: color.NRGBA{0x6A, 0x99, 0x55, 0xff},
	}
}

// defaultTheme returns the fully-resolved built-in theme for a variant. The
// structural colors are derived from the host Fyne theme at call time, so the
// defaults track the app's light/dark theme; the token and match colors are the
// built-in palette.
func defaultTheme(variant fyne.ThemeVariant) Theme {
	sc := defaultSyntaxColors(variant)
	fg := themeColor(theme.ColorNameForeground, variant)
	return Theme{
		Key: sc.Key, String: sc.String, Number: sc.Number, Bool: sc.Bool, Null: sc.Null,
		Punct: sc.Punct, Tag: sc.Tag, Attr: sc.Attr, Comment: sc.Comment,
		Foreground:  fg,
		Summary:     themeColor(theme.ColorNameDisabled, variant),
		IndentGuide: withAlpha(fg, 0x22),
		Selection:   withAlpha(themeColor(theme.ColorNameSelection, variant), 0x66),
		Match:       color.NRGBA{0xff, 0xd5, 0x4f, 0x55}, // soft yellow
		ActiveMatch: color.NRGBA{0xff, 0x8c, 0x1a, 0xaa}, // strong orange
	}
}

// mergeInto copies this theme's non-nil fields over dst (used to overlay an
// override onto the resolved default, or to compose successive overrides).
func (t Theme) mergeInto(dst *Theme) {
	mergeColor(&dst.Key, t.Key)
	mergeColor(&dst.String, t.String)
	mergeColor(&dst.Number, t.Number)
	mergeColor(&dst.Bool, t.Bool)
	mergeColor(&dst.Null, t.Null)
	mergeColor(&dst.Punct, t.Punct)
	mergeColor(&dst.Tag, t.Tag)
	mergeColor(&dst.Attr, t.Attr)
	mergeColor(&dst.Comment, t.Comment)
	mergeColor(&dst.Foreground, t.Foreground)
	mergeColor(&dst.Summary, t.Summary)
	mergeColor(&dst.IndentGuide, t.IndentGuide)
	mergeColor(&dst.Selection, t.Selection)
	mergeColor(&dst.Match, t.Match)
	mergeColor(&dst.ActiveMatch, t.ActiveMatch)
}

// palette resolves the ColorRole -> color.Color table the renderer indexes.
func (t Theme) palette() []color.Color {
	p := make([]color.Color, model.NumColorRoles)
	p[model.RolePlain] = t.Foreground
	p[model.RoleKey] = t.Key
	p[model.RoleString] = t.String
	p[model.RoleNumber] = t.Number
	p[model.RoleBool] = t.Bool
	p[model.RoleNull] = t.Null
	p[model.RolePunct] = t.Punct
	p[model.RoleTag] = t.Tag
	p[model.RoleAttr] = t.Attr
	p[model.RoleComment] = t.Comment
	p[model.RoleMuted] = t.Summary
	return p
}

// resolveTheme returns the effective theme for a variant: the built-in default
// with any per-variant override overlaid.
func (pv *PrettyView) resolveTheme(variant fyne.ThemeVariant) Theme {
	eff := defaultTheme(variant)
	if pv.cfg.themeOverride != nil {
		if ov, ok := pv.cfg.themeOverride[variant]; ok {
			ov.mergeInto(&eff)
		}
	}
	return eff
}

func mergeColor(dst *color.Color, override color.Color) {
	if override != nil {
		*dst = override
	}
}
