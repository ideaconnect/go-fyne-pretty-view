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
// used: substitute the installed theme's foreground color (for the active
// light/dark variant) for currentColor when the resource is built, so the icon
// tracks the theme without a ThemedResource wrapper.

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

// foregroundHex returns the installed theme's foreground color, for the active
// light/dark variant, as an SVG hex string. It resolves through themeColor (the
// same helper the viewer's structural colors use), so the toolbar icons track a
// custom app theme's foreground rather than only the bundled default's.
func foregroundHex() string {
	variant := fyne.ThemeVariant(theme.VariantDark)
	if a := fyne.CurrentApp(); a != nil && a.Settings() != nil {
		variant = a.Settings().ThemeVariant()
	}
	return colorToHex(themeColor(theme.ColorNameForeground, variant))
}

func colorToHex(c color.Color) string {
	// Convert through the straight (non-premultiplied) NRGBA model first, like
	// withAlpha in theme.go: reading c.RGBA() directly yields alpha-PREMULTIPLIED
	// channels, so a non-opaque themed foreground would bake a darkened/desaturated
	// hex. (A 6-digit SVG hex can't carry alpha, so a translucent foreground still
	// renders opaque — but at its true, undistorted color.)
	nc := color.NRGBAModel.Convert(c).(color.NRGBA)
	return fmt.Sprintf("#%02x%02x%02x", nc.R, nc.G, nc.B)
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
