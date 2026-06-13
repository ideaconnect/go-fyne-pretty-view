package parse

import (
	"strings"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// TestParseRecoversFromFinishPanic is the issue #86 regression: the Builder finish step and the
// raw fallback run outside safeParse, so an unforeseen panic there used to escape Parse and crash
// the host. Parse now recovers and degrades to the raw fallback (content still shown), proving the
// boundary covers the whole function, not just the structured parsers.
func TestParseRecoversFromFinishPanic(t *testing.T) {
	orig := buildFinish
	buildFinish = func(*model.Builder) *model.Document { panic("injected finish panic") }
	defer func() { buildFinish = orig }()

	d := Parse([]byte(`{"a":1,"b":2}`), FormatJSON, 0)
	if d == nil {
		t.Fatal("Parse returned nil after a finish panic instead of recovering")
	}
	// Degraded to the raw fallback: every byte still visible, not a crash.
	if txt := renderDoc(d); !strings.Contains(txt, `"a"`) || !strings.Contains(txt, `"b"`) {
		t.Errorf("recovered document dropped content:\n%s", txt)
	}
}

// TestEmptyDocumentIsRenderable covers the last-resort fallback: a valid, non-nil, zero-line
// Document (the same one a fresh widget starts with) whose accessors don't panic. recoverDoc
// returns it when even the raw splitter panics.
func TestEmptyDocumentIsRenderable(t *testing.T) {
	d := model.EmptyDocument()
	if d == nil {
		t.Fatal("EmptyDocument returned nil")
	}
	if d.TotalLines() != 0 {
		t.Errorf("EmptyDocument TotalLines = %d, want 0", d.TotalLines())
	}
	if d.TotalVisibleRows() != 0 {
		t.Errorf("EmptyDocument TotalVisibleRows = %d, want 0", d.TotalVisibleRows())
	}
}
