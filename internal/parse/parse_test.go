package parse

import (
	"bytes"
	"strings"
	"testing"
)

func TestDumpXML(t *testing.T) {
	d := loadDoc(t, "catalog.xml", FormatAuto)
	if d.Format != FormatXML {
		t.Errorf("auto-detect = %v, want xml", d.Format)
	}
	t.Logf("format=%v nodes=%d lines=%d\n%s", d.Format, len(d.Nodes), len(d.Lines),
		firstLines(renderDoc(d), 24))
}

func TestDumpHTML(t *testing.T) {
	d := loadDoc(t, "page.html", FormatAuto)
	if d.Format != FormatHTML {
		t.Errorf("auto-detect = %v, want html", d.Format)
	}
	t.Logf("format=%v nodes=%d lines=%d\n%s", d.Format, len(d.Nodes), len(d.Lines),
		firstLines(renderDoc(d), 24))
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
		lines = append(lines, "…")
	}
	return strings.Join(lines, "\n")
}

func TestAutoDetect(t *testing.T) {
	cases := []struct {
		name string
		want Format
	}{
		{"small.json", FormatJSON},
		{"openapi.json", FormatJSON},
		{"catalog.xml", FormatXML},
		{"page.html", FormatHTML},
	}
	for _, c := range cases {
		d := loadDoc(t, c.name, FormatAuto)
		if d.Format != c.want {
			t.Errorf("%s: detected %v, want %v", c.name, d.Format, c.want)
		}
	}
}

func TestRawFallback(t *testing.T) {
	d := Parse([]byte("just some\nplain text\nlines"), FormatAuto, 0)
	if d.Format != FormatRaw {
		t.Errorf("format = %v, want raw", d.Format)
	}
	if got := int(d.TotalVisibleRows()); got != 3 {
		t.Errorf("raw lines = %d, want 3", got)
	}
}

func TestTruncatedJSONRecoversStructure(t *testing.T) {
	// A truncated value must NOT collapse the whole document to raw text: the
	// parser is tolerant (see the Parser contract) and keeps the recovered tree.
	d := Parse([]byte(`{"broken": `), FormatJSON, 0)
	if d.Format != FormatJSON {
		t.Fatalf("truncated JSON should recover partial structure, got %v", d.Format)
	}
	var text strings.Builder
	for li := 0; li < d.TotalLines(); li++ {
		text.WriteString(d.LineString(int32(li)))
		text.WriteByte('\n')
	}
	if !strings.Contains(text.String(), "broken") {
		t.Errorf("recovered structure should still show the key 'broken'; got:\n%s", text.String())
	}
}

func TestNonJSONFallsBackToRaw(t *testing.T) {
	// When nothing structural can be recovered (junk under a forced JSON format),
	// raw fallback still applies so content always displays.
	d := Parse([]byte("not json at all"), FormatJSON, 0)
	if d.Format != FormatRaw {
		t.Errorf("non-JSON forced as JSON should fall back to raw, got %v", d.Format)
	}
}

// TestDeeplyNestedJSONDoesNotCrash feeds far more nesting than the recursion cap.
// Without the cap this overflows the goroutine stack (a fatal crash); with it, the
// parser truncates to a partial document.
func TestDeeplyNestedJSONDoesNotCrash(t *testing.T) {
	const n = maxNestDepth * 5
	src := append(bytes.Repeat([]byte("["), n), bytes.Repeat([]byte("]"), n)...)
	d := Parse(src, FormatJSON, 0)
	if d == nil || d.TotalLines() == 0 {
		t.Fatal("deeply nested JSON should produce a (partial) document, not crash")
	}
}

func TestDeeplyNestedXMLDoesNotCrash(t *testing.T) {
	const n = maxNestDepth * 5
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("<a>")
	}
	for i := 0; i < n; i++ {
		sb.WriteString("</a>")
	}
	d := Parse([]byte(sb.String()), FormatXML, 0)
	if d == nil || d.TotalLines() == 0 {
		t.Fatal("deeply nested XML should produce a (partial) document, not crash")
	}
}

// TestProcInstInsideElementKeepsContent guards that a processing instruction nested
// inside an element renders its instruction content (not just the target).
func TestProcInstInsideElementKeepsContent(t *testing.T) {
	d := Parse([]byte(`<root><?php echo "hi"; ?></root>`), FormatXML, 0)
	var text strings.Builder
	for li := 0; li < d.TotalLines(); li++ {
		text.WriteString(d.LineString(int32(li)))
		text.WriteByte('\n')
	}
	if !strings.Contains(text.String(), `echo "hi";`) {
		t.Errorf("processing-instruction content dropped inside element; got:\n%s", text.String())
	}
}

// TestRawTabsExpandToStops verifies tabs in raw text are expanded to the configured
// tab stops so the uniform monospace grid holds (and that WithTabWidth, threaded
// through Parse, is no longer a no-op).
func TestRawTabsExpandToStops(t *testing.T) {
	// tabWidth 4: "a\tb" -> "a" then a tab to column 4 (3 spaces) then "b".
	d := Parse([]byte("a\tb\nabcd\tx\n\tlead"), FormatRaw, 0, 4)
	cases := []struct {
		row  int
		want string
	}{
		{0, "a   b"},     // 1 char, tab fills cols 1-3, then 'b' at col 4
		{1, "abcd    x"}, // 4 chars land exactly on a stop, so a full 4-space tab
		{2, "    lead"},  // leading tab -> 4 spaces
	}
	for _, c := range cases {
		li := d.LineAtRow(int32(c.row))
		if got := d.LineString(li); got != c.want {
			t.Errorf("row %d = %q, want %q", c.row, got, c.want)
		}
	}

	// A different tab width must be honored (proves the value is threaded, not fixed).
	d8 := Parse([]byte("a\tb"), FormatRaw, 0, 8)
	if got := d8.LineString(d8.LineAtRow(0)); got != "a       b" {
		t.Errorf("tabWidth 8: got %q, want %q", got, "a       b")
	}

	// The original bytes (with the tab) are preserved for copy-source / reparse.
	if !bytes.Contains(d.Src, []byte{'\t'}) {
		t.Error("Document.Src should retain the original tab bytes")
	}
}
