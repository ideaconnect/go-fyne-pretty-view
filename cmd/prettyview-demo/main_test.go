package main

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestFixtureHelpers(t *testing.T) {
	if !isFixture("testdata/small.json") {
		t.Error("isFixture should recognize a bundled fixture")
	}
	if isFixture("not/a/fixture.json") {
		t.Error("isFixture should reject an unknown path")
	}
	if _, err := readFixture("testdata/openapi.json"); err != nil {
		t.Errorf("readFixture(bundled) = %v", err)
	}
	if _, err := readFixture("/definitely/missing/file.json"); err == nil {
		t.Error("readFixture of a missing file should error")
	}
	if repoRoot() == "" {
		t.Error("repoRoot should not be empty")
	}
}

func TestBuildUI(t *testing.T) {
	test.NewApp()
	w := test.NewWindow(nil)
	defer w.Close()

	// A bundled fixture goes through the dropdown-selected path.
	if ui := buildUI(w, "testdata/small.json"); ui == nil {
		t.Fatal("buildUI returned nil for a fixture")
	}
	// A non-fixture path goes through loadFixture directly; a missing file must
	// surface as an error message rather than panic.
	if ui := buildUI(w, "/no/such/file.json"); ui == nil {
		t.Fatal("buildUI returned nil for a non-fixture path")
	}
}

func TestStartPathDefault(t *testing.T) {
	if startPath() == "" {
		t.Error("startPath should fall back to a default")
	}
}
