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
// fill="currentColor" on every path.
//
// By default each glyph is wrapped in a theme.ThemedResource, so Fyne colorizes it
// from the theme's foreground at draw time: it tracks a runtime light/dark switch,
// and, because widget.Button recolors a ThemedResource icon to the contrast color of
// its fill (foregroundOnPrimary on a HighImportance button), the wrap toggle's glyph
// stays readable while it sits on the primary fill. A host that wants a specific
// color instead (ToolbarConfig.IconColor / ActiveIconColor) gets the older bake: the
// hex is substituted for currentColor when the resource is built, and Fyne leaves a
// plain static resource alone.

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

// colorToHex renders c as a 6-digit SVG hex, reading straight (non-premultiplied)
// NRGBA channels like withAlpha in theme.go: c.RGBA() yields alpha-PREMULTIPLIED
// values, so a non-opaque color would otherwise bake a darkened/desaturated hex. (A
// 6-digit hex can't carry alpha, so a translucent color still renders opaque — but at
// its true, undistorted color.)
func colorToHex(c color.Color) string {
	nc := color.NRGBAModel.Convert(c).(color.NRGBA)
	return fmt.Sprintf("#%02x%02x%02x", nc.R, nc.G, nc.B)
}

// iconResource returns a Font Awesome SVG as a Fyne resource. With c == nil it is a
// theme.ThemedResource colorized from the theme foreground at draw time (and recolored
// by widget.Button to the contrast color of a highlighted fill); with a color it is a
// static resource with that exact color baked in for currentColor, which Fyne never
// recolors. name is used for the resource id; the explicit bake carries its hex in the
// id because Fyne's rasterized-SVG cache is keyed by resource name, so two bakes of the
// same glyph in different colors would otherwise share one raster and the first drawn
// color would win (the wrap toggle's on/off swap, or a toolbar rebuilt in a new theme).
func iconResource(name string, svg []byte, c color.Color) fyne.Resource {
	if c == nil {
		return theme.NewThemedResource(fyne.NewStaticResource(name+".svg", svg))
	}
	hex := colorToHex(c)
	colored := bytes.ReplaceAll(svg, []byte("currentColor"), []byte(hex))
	return fyne.NewStaticResource(name+"-"+hex[1:]+".svg", colored)
}

func iconSearch(c color.Color) fyne.Resource   { return iconResource("fa-search", svgSearch, c) }
func iconFolder(c color.Color) fyne.Resource   { return iconResource("fa-folder", svgFolder, c) }
func iconWrapText(c color.Color) fyne.Resource { return iconResource("fa-wrap-text", svgWrapText, c) }
func iconExpand(c color.Color) fyne.Resource   { return iconResource("fa-expand", svgExpand, c) }
func iconCollapse(c color.Color) fyne.Resource { return iconResource("fa-collapse", svgCollapse, c) }
func iconArrowUp(c color.Color) fyne.Resource  { return iconResource("fa-arrow-up", svgArrowUp, c) }
func iconArrowDown(c color.Color) fyne.Resource {
	return iconResource("fa-arrow-down", svgArrowDown, c)
}
