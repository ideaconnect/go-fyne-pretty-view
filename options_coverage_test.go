package prettyview

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
)

// TestMaxInputBytesTruncates: WithMaxInputBytes caps the source SetData/SetText load,
// truncating oversized input before parsing; under the cap (and uncapped) the input
// is retained in full.
func TestMaxInputBytesTruncates(t *testing.T) {
	big := strings.Repeat("x", 10000)

	capped := New(WithMaxInputBytes(100))
	capped.SetData([]byte(big), FormatRaw)
	if got := len(capped.doc.Src); got != 100 {
		t.Errorf("capped source = %d bytes, want 100", got)
	}
	capped.SetText("short")
	if got := len(capped.doc.Src); got != len("short") {
		t.Errorf("under-cap source = %d bytes, want %d", got, len("short"))
	}

	uncapped := New()
	uncapped.SetData([]byte(big), FormatRaw)
	if got := len(uncapped.doc.Src); got != len(big) {
		t.Errorf("uncapped source = %d bytes, want %d", got, len(big))
	}
}

// TestConstructionOptions covers each functional option and its clamp branches by
// reading back the resulting config (the option funcs are otherwise only exercised
// indirectly).
func TestConstructionOptions(t *testing.T) {
	test.NewApp()
	pv := New(
		WithFormat(FormatXML),
		WithWrap(WrapWord),
		WithTabWidth(8),
		WithIndentStep(20),
		WithDefaultCollapseDepth(3),
		WithSearchConfig(SearchConfig{MaxMatches: 42}),
		WithMaxInputBytes(4096),
	)
	if pv.cfg.maxInputBytes != 4096 {
		t.Errorf("WithMaxInputBytes: got %d", pv.cfg.maxInputBytes)
	}
	if pv.cfg.format != FormatXML {
		t.Errorf("WithFormat: got %v", pv.cfg.format)
	}
	if pv.cfg.wrap != WrapWord {
		t.Errorf("WithWrap: got %v", pv.cfg.wrap)
	}
	if pv.cfg.tabWidth != 8 {
		t.Errorf("WithTabWidth: got %d", pv.cfg.tabWidth)
	}
	if pv.cfg.indentStep != 20 {
		t.Errorf("WithIndentStep: got %v", pv.cfg.indentStep)
	}
	if pv.cfg.collapseDepth != 3 {
		t.Errorf("WithDefaultCollapseDepth: got %d", pv.cfg.collapseDepth)
	}
	if pv.cfg.search.MaxMatches != 42 {
		t.Errorf("WithSearchConfig: got %d", pv.cfg.search.MaxMatches)
	}

	// Clamp branches: non-positive values are ignored / floored.
	def := defaultConfig()
	clamped := New(WithTabWidth(0), WithIndentStep(-5), WithDefaultCollapseDepth(-1))
	if clamped.cfg.tabWidth != def.tabWidth {
		t.Errorf("WithTabWidth(0) should keep default %d, got %d", def.tabWidth, clamped.cfg.tabWidth)
	}
	if clamped.cfg.indentStep != def.indentStep {
		t.Errorf("WithIndentStep(-5) should keep default %v, got %v", def.indentStep, clamped.cfg.indentStep)
	}
	if clamped.cfg.collapseDepth != 0 {
		t.Errorf("WithDefaultCollapseDepth(-1) should floor to 0, got %d", clamped.cfg.collapseDepth)
	}
}
