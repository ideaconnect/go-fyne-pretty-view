package prettyview

import (
	"bytes"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestIconResourcesRecolored verifies every embedded Font Awesome glyph loads and
// has its fill recolored from currentColor to a concrete theme-foreground hex, so
// the icon tracks the active theme regardless of how Fyne colorizes resources.
func TestIconResourcesRecolored(t *testing.T) {
	test.NewApp()
	for _, get := range []func() fyne.Resource{
		iconSearch, iconFolder, iconWrapText, iconExpand, iconCollapse, iconArrowUp, iconArrowDown,
	} {
		res := get()
		if res == nil || len(res.Content()) == 0 {
			t.Fatal("nil/empty icon resource")
		}
		if bytes.Contains(res.Content(), []byte("currentColor")) {
			t.Errorf("%s still contains currentColor (not recolored)", res.Name())
		}
		if !bytes.Contains(res.Content(), []byte(`fill="#`)) {
			t.Errorf("%s fill was not set to a hex color", res.Name())
		}
	}
}
