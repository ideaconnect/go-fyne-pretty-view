package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// TestContextMenuItems checks the right-click menu's items, their enabled state,
// keyboard accelerators, and that their actions drive the public selection and
// clipboard API.
func TestContextMenuItems(t *testing.T) {
	src := []byte(`{"a":{"b":"value"},"c":[1,2,3]}`)
	pv, win := renderInWindow(t, src, FormatJSON, 500, 400)
	defer win.Close()

	// With nothing selected, Copy is greyed out; Select all is available.
	m := pv.contextMenu(model.NoNode)
	if len(m.Items) != 2 {
		t.Fatalf("contextMenu has %d items, want 2", len(m.Items))
	}
	copyItem, selectAll := m.Items[0], m.Items[1]
	if copyItem.Label != "Copy" || selectAll.Label != "Select all" {
		t.Fatalf("menu labels = %q, %q", copyItem.Label, selectAll.Label)
	}
	if !copyItem.Disabled {
		t.Error("Copy should be disabled with no selection")
	}
	if selectAll.Disabled {
		t.Error("Select all should be enabled on a non-empty document")
	}
	// Accelerators make the menu read like a native one.
	if _, ok := copyItem.Shortcut.(*fyne.ShortcutCopy); !ok {
		t.Errorf("Copy shortcut = %T, want *fyne.ShortcutCopy", copyItem.Shortcut)
	}
	if _, ok := selectAll.Shortcut.(*fyne.ShortcutSelectAll); !ok {
		t.Errorf("Select all shortcut = %T, want *fyne.ShortcutSelectAll", selectAll.Shortcut)
	}

	// The Select all action activates a selection; a freshly built menu then offers
	// an enabled Copy whose action populates the clipboard.
	selectAll.Action()
	if !pv.sel.active {
		t.Fatal("Select all action did not activate a selection")
	}
	enabledCopy := pv.contextMenu(model.NoNode).Items[0]
	if enabledCopy.Disabled {
		t.Fatal("Copy should be enabled once there is a selection")
	}
	enabledCopy.Action()
	if got := fyne.CurrentApp().Clipboard().Content(); !strings.Contains(got, `"value"`) {
		t.Errorf("Copy action clipboard = %q, want it to contain the document text", got)
	}
}

// TestContextMenuEmptyDocument: both items are disabled when there is nothing to
// act on (a viewer constructed without data must not panic building the menu).
func TestContextMenuEmptyDocument(t *testing.T) {
	pv := New()
	m := pv.contextMenu(model.NoNode)
	if !m.Items[0].Disabled {
		t.Error("Copy should be disabled on an empty viewer")
	}
	if !m.Items[1].Disabled {
		t.Error("Select all should be disabled on an empty viewer")
	}
}

// TestContextMenuCopySubtree: with a node resolved from the clicked line, the menu
// offers "Copy subtree" whose action copies that node's whole subtree (here, the "a"
// object), regardless of fold state and for any format (it keys off the line Owner).
func TestContextMenuCopySubtree(t *testing.T) {
	src := []byte(`{"a":{"b":"value"},"c":42}`)
	pv, win := renderInWindow(t, src, FormatJSON, 500, 400)
	defer win.Close()

	// The fold head of the inner "a" object owns its head line.
	node := findFoldHead(pv.doc, `"a"`)
	if node == model.NoNode {
		t.Fatal(`could not find the "a" fold head`)
	}
	m := pv.contextMenu(node)
	var sub *fyne.MenuItem
	for _, it := range m.Items {
		if it.Label == "Copy subtree" {
			sub = it
		}
	}
	if sub == nil {
		t.Fatal(`"Copy subtree" item not offered for a resolved node`)
	}
	sub.Action()
	got := fyne.CurrentApp().Clipboard().Content()
	if !strings.Contains(got, `"b"`) || !strings.Contains(got, `"value"`) {
		t.Errorf("Copy subtree clipboard = %q, want the \"a\" subtree", got)
	}
	if strings.Contains(got, `"c"`) {
		t.Errorf("Copy subtree leaked a sibling: %q", got)
	}
	// With no resolved node the item is absent (base menu of 2).
	if len(pv.contextMenu(model.NoNode).Items) != 2 {
		t.Error("base menu (NoNode) should not include Copy subtree")
	}
}

// TestTappedSecondaryShowsMenu: a right-click pops the context menu as a canvas
// overlay and leaves any existing selection untouched.
func TestTappedSecondaryShowsMenu(t *testing.T) {
	src := []byte(`{"a":{"b":"value"},"c":[1,2,3]}`)
	pv, win := renderInWindow(t, src, FormatJSON, 500, 400)
	defer win.Close()

	pv.SelectAll()
	before := pv.sel

	test.TapSecondaryAt(pv, fyne.NewPos(20, 10))

	// The context menu is shown as a single canvas overlay (Fyne wraps the pop-up
	// in its own overlay container, so we assert presence, not a concrete type —
	// the menu's contents are covered by TestContextMenuItems).
	if got := len(win.Canvas().Overlays().List()); got != 1 {
		t.Fatalf("overlays after right-click = %d, want 1 (the context menu)", got)
	}
	if pv.sel != before {
		t.Error("right-click must not change the selection")
	}
}
