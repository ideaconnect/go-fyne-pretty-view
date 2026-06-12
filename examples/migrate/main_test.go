package main

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

// TestDemoBuilds runs the migration snippet headlessly (demo omits ShowAndRun) and asserts
// the v1/v2 distinction it documents: the first widget is a read-only viewer, the second is
// the new opt-in editor.
func TestDemoBuilds(t *testing.T) {
	viewer, editor := demo(test.NewApp())
	if viewer == nil || editor == nil {
		t.Fatal("demo returned nil widgets")
	}
	if viewer.Editable() {
		t.Error("the v1-style viewer must be read-only")
	}
	if !editor.Editable() {
		t.Error("the v2 editor must report Editable() == true")
	}
}
