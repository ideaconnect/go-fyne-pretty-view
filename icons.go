package prettyview

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Toolbar glyphs are from Font Awesome Free (https://fontawesome.com), used under
// the CC BY 4.0 license — see icons/fontawesome/LICENSE.txt and the README's
// attribution (the original licence/attribution comment is preserved inside each
// SVG). They are solid (fill-drawn) icons; the vendored copies carry
// fill="currentColor" on every path. Fyne's themed-resource colorizer would also
// work for fill icons, but we keep the same explicit bake the project has always
// used: substitute the active theme's foreground color for currentColor when the
// resource is built, so the icon tracks the theme without a ThemedResource wrapper.

//go:embed icons/fontawesome/search.svg
var svgSearch []byte

//go:embed icons/fontawesome/folder.svg
var svgFolder []byte

//go:embed icons/fontawesome/wrap-text.svg
var svgWrapText []byte

//go:embed icons/fontawesome/expand.svg
var svgExpand []byte

//go:embed icons/fontawesome/collapse.svg
var svgCollapse []byte

//go:embed icons/fontawesome/arrow-up.svg
var svgArrowUp []byte

//go:embed icons/fontawesome/arrow-down.svg
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

// iconResource returns a Font Awesome SVG as a Fyne resource, recolored to the
// current theme foreground (see the package note above). name is used for the
// resource id.
func iconResource(name string, svg []byte) fyne.Resource {
	colored := bytes.ReplaceAll(svg, []byte("currentColor"), []byte(foregroundHex()))
	return fyne.NewStaticResource(name+".svg", colored)
}

// The toolbar icon set (resolved against the active theme at call time).
func iconSearch() fyne.Resource    { return iconResource("fa-search", svgSearch) }
func iconFolder() fyne.Resource    { return iconResource("fa-folder", svgFolder) }
func iconWrapText() fyne.Resource  { return iconResource("fa-wrap-text", svgWrapText) }
func iconExpand() fyne.Resource    { return iconResource("fa-expand", svgExpand) }
func iconCollapse() fyne.Resource  { return iconResource("fa-collapse", svgCollapse) }
func iconArrowUp() fyne.Resource   { return iconResource("fa-arrow-up", svgArrowUp) }
func iconArrowDown() fyne.Resource { return iconResource("fa-arrow-down", svgArrowDown) }
