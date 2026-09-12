package prettyview

import (
	"bytes"
	"image/color"
	"testing"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// drawnIcon returns the resource the button's renderer is actually drawing (the
// canvas.Image inside it), which is where Fyne applies its importance-based recolor.
func drawnIcon(t *testing.T, root fyne.CanvasObject) fyne.Resource {
	t.Helper()
	btn, ok := root.(fyne.Widget)
	if !ok {
		t.Fatalf("control is %T, not a widget", root)
	}
	for _, o := range test.TempWidgetRenderer(t, btn).Objects() {
		if img, ok := o.(*canvas.Image); ok {
			return img.Resource
		}
	}
	t.Fatal("no canvas.Image in the button renderer")
	return nil
}

// wrapToggleFromToolbar builds a toolbar holding only the wrap toggle (the documented
// way to style a single control) and returns that button.
func wrapToggleFromToolbar(t *testing.T, pv *PrettyView, cfg ToolbarConfig) *ttwidget.Button {
	t.Helper()
	cfg.ShowWrap = true
	root := NewToolbar(pv, cfg)
	box, ok := root.(*fyne.Container)
	if !ok || len(box.Objects) != 1 {
		t.Fatalf("NewToolbar with only ShowWrap returned %T with %d children", root, lenObjects(root))
	}
	btn, ok := box.Objects[0].(*ttwidget.Button)
	if !ok {
		t.Fatalf("wrap control is %T, want *ttwidget.Button", box.Objects[0])
	}
	return btn
}

func lenObjects(o fyne.CanvasObject) int {
	if c, ok := o.(*fyne.Container); ok {
		return len(c.Objects)
	}
	return -1
}

// TestWrapToggleDefaultUsesContrastColorOnPrimary pins the fix for the unreadable
// active wrap toggle: with no explicit colors the glyph is a ThemedResource, so while
// wrapping is on (HighImportance, primary fill) Fyne draws it in foregroundOnPrimary,
// the theme's contrast color for that fill, and once wrapping is off it is back on the
// plain foreground. A static, foreground-baked icon (the old behaviour) would stay
// foreground-colored on the primary fill.
func TestWrapToggleDefaultUsesContrastColorOnPrimary(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":1}`), FormatJSON)
	win := test.NewWindow(pv)
	defer win.Close()

	btn, ok := NewWrapToggle(pv).(*ttwidget.Button)
	if !ok {
		t.Fatal("NewWrapToggle is not a *ttwidget.Button")
	}
	colorName := func() fyne.ThemeColorName {
		themed, ok := drawnIcon(t, btn).(fyne.ThemedResource)
		if !ok {
			t.Fatal("drawn icon is not themed")
		}
		return themed.ThemeColorName()
	}
	if got := colorName(); got != theme.ColorNameForeground {
		t.Fatalf("idle glyph colors from %q, want foreground", got)
	}
	test.Tap(btn)
	if pv.Wrap() != WrapWord || btn.Importance != widget.HighImportance {
		t.Fatalf("tap did not enable wrap with a HighImportance button (wrap=%v importance=%v)", pv.Wrap(), btn.Importance)
	}
	if got := colorName(); got != theme.ColorNameForegroundOnPrimary {
		t.Errorf("active glyph colors from %q, want foregroundOnPrimary (unreadable on the primary fill otherwise)", got)
	}
	test.Tap(btn)
	if got := colorName(); got != theme.ColorNameForeground {
		t.Errorf("glyph after toggling off colors from %q, want foreground restored", got)
	}
}

// TestWrapToggleExplicitColors verifies ToolbarConfig.IconColor / ActiveIconColor: the
// glyph is baked with the idle color while wrapping is off and the active color while it
// is on, as a static resource Fyne does not recolor, and the swap is reversible.
func TestWrapToggleExplicitColors(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":1}`), FormatJSON)
	win := test.NewWindow(pv)
	defer win.Close()

	idle := color.NRGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff}
	active := color.NRGBA{R: 0x10, G: 0x18, B: 0x12, A: 0xff} // Helena's near-black on green
	btn := wrapToggleFromToolbar(t, pv, ToolbarConfig{IconColor: idle, ActiveIconColor: active})

	fill := func() []byte {
		res := drawnIcon(t, btn)
		if _, themed := res.(fyne.ThemedResource); themed {
			t.Fatal("explicit-color glyph is themed; Fyne would override the requested color")
		}
		return res.Content()
	}
	if !bytes.Contains(fill(), []byte(`fill="#aabbcc"`)) {
		t.Error("idle glyph does not carry IconColor")
	}
	// Fyne caches rasterized SVGs by resource NAME, so the two bakes must not share one
	// or the first color drawn would stick through every later toggle.
	if a, b := drawnIcon(t, btn).Name(), iconWrapText(active).Name(); a == b {
		t.Errorf("idle and active bakes share the resource name %q (raster cache collision)", a)
	}
	test.Tap(btn)
	if !bytes.Contains(fill(), []byte(`fill="#101812"`)) {
		t.Error("active glyph does not carry ActiveIconColor")
	}
	if bytes.Contains(fill(), []byte(`fill="#aabbcc"`)) {
		t.Error("active glyph still carries the idle color")
	}
	test.Tap(btn)
	if !bytes.Contains(fill(), []byte(`fill="#aabbcc"`)) {
		t.Error("glyph after toggling off does not restore IconColor")
	}
}

// TestToolbarIconColorAppliesToEveryGlyph: IconColor alone (no ActiveIconColor) must
// color every icon button the toolbar builds, and the wrap toggle's active state then
// falls back to the theme contrast color rather than an explicit one.
func TestToolbarIconColorAppliesToEveryGlyph(t *testing.T) {
	test.NewApp()
	pv := NewWithData([]byte(`{"a":1}`), FormatJSON)
	w := test.NewWindow(pv)
	defer w.Close()

	idle := color.NRGBA{R: 0x01, G: 0x02, B: 0x03, A: 0xff}
	cfg := DefaultToolbarConfig(w)
	cfg.IconColor = idle
	root := NewToolbar(pv, cfg)
	n := 0
	for _, b := range findToolTipButtons(root) {
		if b.Icon == nil {
			continue // the Aa / .* text toggles
		}
		n++
		if !bytes.Contains(b.Icon.Content(), []byte(`fill="#010203"`)) {
			t.Errorf("%s: icon does not carry IconColor", b.ToolTip())
		}
	}
	if n < 6 { // open, expand, collapse, wrap, magnifier, prev, next
		t.Errorf("only %d icon buttons found, want >= 6", n)
	}
	// With IconColor set but no ActiveIconColor the wrap toggle keeps the idle bake in
	// both states: an explicit idle color is a static resource, so there is no themed
	// contrast recolor to fall back to. That is by design and documented; assert it so a
	// future change here is deliberate.
	wrap := wrapToggleFromToolbar(t, pv, ToolbarConfig{IconColor: idle})
	test.Tap(wrap)
	if !bytes.Contains(drawnIcon(t, wrap).Content(), []byte(`fill="#010203"`)) {
		t.Error("active wrap glyph with IconColor only should keep the idle bake")
	}
}
