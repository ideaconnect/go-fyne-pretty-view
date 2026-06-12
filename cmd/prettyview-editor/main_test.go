package main

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestIsSample(t *testing.T) {
	if !isSample("messy JSON") {
		t.Error("isSample should recognize a bundled sample")
	}
	if isSample("not a sample") {
		t.Error("isSample should reject an unknown name")
	}
}

func TestEditorBuildUI(t *testing.T) {
	test.NewApp()
	// Each case renders the UI into a window and closes it, so the editable widget's
	// renderer is created and then Destroyed — which cancels the auto-format debounce
	// timer SetData armed (otherwise it would fire after the test, off the Fyne thread).
	// A readable file exercises the file-load success branch.
	tmp := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(tmp, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []string{"", "messy XML", tmp, "/no/such/file.json"}
	for _, start := range cases {
		w := test.NewWindow(nil)
		ui := buildUI(w, start)
		if ui == nil {
			t.Fatalf("buildUI(%q) returned nil", start)
		}
		w.SetContent(ui) // create the editable widget's renderer
		w.Close()        // Destroy -> stopEditTimer (no leaked timer / race)
	}
}

func TestStartArgDefault(t *testing.T) {
	// With no extra CLI args under `go test`, startArg may be empty or a test flag;
	// either way it must not panic and returns a string.
	_ = startArg()
}
