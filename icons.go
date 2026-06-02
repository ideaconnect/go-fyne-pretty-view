package prettyview

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Toolbar glyphs are from Iconoir (https://iconoir.com), MIT-licensed — see
// icons/iconoir/LICENSE and the README's attribution. They are stroke-drawn line
// icons (fill="none", stroke="currentColor"). Fyne's themed-resource colorizer only
// rewrites fill (it skips fill="none" and never touches stroke), so a stroke icon
// would render in its literal color regardless of the theme. We therefore bake the
// active theme's foreground color into the stroke when the resource is built.

//go:embed icons/iconoir/search.svg
var svgSearch []byte

//go:embed icons/iconoir/folder.svg
var svgFolder []byte

//go:embed icons/iconoir/wrap-text.svg
var svgWrapText []byte

//go:embed icons/iconoir/expand.svg
var svgExpand []byte

//go:embed icons/iconoir/collapse.svg
var svgCollapse []byte

//go:embed icons/iconoir/arrow-up.svg
var svgArrowUp []byte

//go:embed icons/iconoir/arrow-down.svg
var svgArrowDown []byte

// foregroundHex returns the active theme's foreground color as an SVG hex string.
func foregroundHex() string {
	variant := fyne.ThemeVariant(theme.VariantDark)
	if a := fyne.CurrentApp(); a != nil && a.Settings() != nil {
		variant = a.Settings().ThemeVariant()
	}
	return colorToHex(theme.DefaultTheme().Color(theme.ColorNameForeground, variant))
}

func colorToHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

// iconResource returns an Iconoir SVG as a Fyne resource, recolored to the current
// theme foreground (see the package note above). name is used for the resource id.
func iconResource(name string, svg []byte) fyne.Resource {
	colored := bytes.ReplaceAll(svg, []byte("currentColor"), []byte(foregroundHex()))
	return fyne.NewStaticResource(name+".svg", colored)
}

// The toolbar icon set (resolved against the active theme at call time).
func iconSearch() fyne.Resource    { return iconResource("iconoir-search", svgSearch) }
func iconFolder() fyne.Resource    { return iconResource("iconoir-folder", svgFolder) }
func iconWrapText() fyne.Resource  { return iconResource("iconoir-wrap-text", svgWrapText) }
func iconExpand() fyne.Resource    { return iconResource("iconoir-expand", svgExpand) }
func iconCollapse() fyne.Resource  { return iconResource("iconoir-collapse", svgCollapse) }
func iconArrowUp() fyne.Resource   { return iconResource("iconoir-arrow-up", svgArrowUp) }
func iconArrowDown() fyne.Resource { return iconResource("iconoir-arrow-down", svgArrowDown) }
