package prettyview

import (
	"os"
	"strings"
	"testing"
)

// TestModulePathIsV2 guards the v2 module path. Go semantic import versioning requires
// a v2+ module to declare a path ending in /v2; an accidental revert to the bare path
// would still compile and test green here, yet silently break `go get .../v2` for every
// downstream consumer. Stdlib-only parse, matching TestExportedSurfaceGolden's
// no-x/tools philosophy.
func TestModulePathIsV2(t *testing.T) {
	b, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	var modPath string
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			modPath = strings.TrimSpace(rest)
			break
		}
	}
	if modPath == "" {
		t.Fatal("no module directive found in go.mod")
	}
	const want = "github.com/ideaconnect/go-fyne-pretty-view/v2"
	if modPath != want {
		t.Errorf("module path = %q, want %q — v2+ needs the /v2 suffix for `go get` to resolve", modPath, want)
	}
}
