package prettyview

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// themeColor resolves a theme color for a specific variant via the active theme
// interface (the package-level theme.Color only knows the current variant).
func themeColor(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	return fyne.CurrentApp().Settings().Theme().Color(name, variant)
}

// SyntaxColors overrides the per-role syntax palette for a theme variant. A nil
// color falls back to the built-in default for that role. The full theming layer
// (wrapping theme, palette rebuild on Refresh) lands at M11; this type is the
// stable public surface used by WithSyntaxColors / SetSyntaxColors.
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

// defaultSyntaxColors returns the built-in (Bruno-ish, VS Code Dark+ inspired)
// palette for a theme variant.
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

// buildPalette resolves a model.ColorRole -> color.Color table for a variant, applying
// any user override.
func buildPalette(variant fyne.ThemeVariant, override *SyntaxColors) []color.Color {
	c := defaultSyntaxColors(variant)
	if override != nil {
		mergeColor(&c.Key, override.Key)
		mergeColor(&c.String, override.String)
		mergeColor(&c.Number, override.Number)
		mergeColor(&c.Bool, override.Bool)
		mergeColor(&c.Null, override.Null)
		mergeColor(&c.Punct, override.Punct)
		mergeColor(&c.Tag, override.Tag)
		mergeColor(&c.Attr, override.Attr)
		mergeColor(&c.Comment, override.Comment)
	}
	fg := themeColor(theme.ColorNameForeground, variant)
	muted := themeColor(theme.ColorNameDisabled, variant)

	pal := make([]color.Color, model.NumColorRoles)
	pal[model.RolePlain] = fg
	pal[model.RoleKey] = c.Key
	pal[model.RoleString] = c.String
	pal[model.RoleNumber] = c.Number
	pal[model.RoleBool] = c.Bool
	pal[model.RoleNull] = c.Null
	pal[model.RolePunct] = c.Punct
	pal[model.RoleTag] = c.Tag
	pal[model.RoleAttr] = c.Attr
	pal[model.RoleComment] = c.Comment
	pal[model.RoleMuted] = muted
	return pal
}

func mergeColor(dst *color.Color, override color.Color) {
	if override != nil {
		*dst = override
	}
}
