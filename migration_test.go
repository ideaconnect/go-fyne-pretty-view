package prettyview

import (
	"os"
	"strings"
	"testing"
)

// TestMigrationDocLinked: the README points readers at MIGRATION.md.
func TestMigrationDocLinked(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	if !strings.Contains(string(readme), "MIGRATION.md") {
		t.Error("README.md should link to MIGRATION.md")
	}
	if _, err := os.Stat("MIGRATION.md"); err != nil {
		t.Errorf("MIGRATION.md should exist: %v", err)
	}
}

// TestMigrationSnippetCompiles guards the checked-in migration example: it must import
// the /v2 module path and exercise the editing surface. The example builds as part of
// `go build ./...` (and `make check`/CI), so this presence+import check, together with
// the build, is the "the snippet compiles against /v2" guarantee.
func TestMigrationSnippetCompiles(t *testing.T) {
	const path = "examples/migrate/main.go"
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	src := string(b)
	if !strings.Contains(src, "go-fyne-pretty-view/v2") {
		t.Errorf("%s must import the /v2 module path", path)
	}
	if !strings.Contains(src, "WithEditable()") {
		t.Errorf("%s should demonstrate the new editing surface", path)
	}
	// MIGRATION.md must show the /v2 import too.
	mig, err := os.ReadFile("MIGRATION.md")
	if err != nil {
		t.Fatalf("read MIGRATION.md: %v", err)
	}
	if !strings.Contains(string(mig), "go-fyne-pretty-view/v2") {
		t.Error("MIGRATION.md should document the /v2 import path")
	}
}

// TestChangelogHasV2BreakingEntry: the CHANGELOG records the v2 module-path bump under
// a Changed section.
func TestChangelogHasV2BreakingEntry(t *testing.T) {
	b, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}
	cl := string(b)
	if !strings.Contains(cl, "## [v2.0.0") {
		t.Error("CHANGELOG.md should have a v2.0.0 section")
	}
	// The v2 section must call out the breaking module-path change.
	v2 := cl[strings.Index(cl, "## [v2.0.0"):]
	if i := strings.Index(v2, "## [v1"); i >= 0 {
		v2 = v2[:i] // bound to just the v2 section
	}
	if !strings.Contains(v2, "Changed") || !strings.Contains(v2, "/v2") {
		t.Error("the v2.0.0 CHANGELOG section should note the breaking module-path bump under Changed")
	}
}
