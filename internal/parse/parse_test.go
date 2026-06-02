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

// TestUTF8BOMStripped is the regression for BOM-prefixed input (common from
// Windows/.NET tooling). A leading UTF-8 BOM must not defeat detection or force a
// raw fallback, and stripping it must not desync the zero-copy segment offsets.
func TestUTF8BOMStripped(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	withBOM := func(s string) []byte { return append(append([]byte{}, bom...), s...) }

	jsonSrc := `{"a":1,"b":[2,3]}`

	// Auto-detect must see through the BOM.
	if d := Parse(withBOM(jsonSrc), FormatAuto, 0); d.Format != FormatJSON {
		t.Fatalf("BOM+JSON auto-detect = %v, want JSON (BOM defeated detection)", d.Format)
	}
	// A forced format must strip the BOM rather than fail into raw.
	auto := Parse(withBOM(jsonSrc), FormatJSON, 0)
	if auto.Format != FormatJSON {
		t.Fatalf("BOM+JSON forced = %v, want JSON (BOM caused raw fallback)", auto.Format)
	}
	// Structure AND zero-copy segment offsets must match the BOM-free parse exactly,
	// proving the BOM was stripped before the builder computed SrcSeg ranges.
	plain := Parse([]byte(jsonSrc), FormatJSON, 0)
	if got, want := renderDoc(auto), renderDoc(plain); got != want {
		t.Errorf("BOM parse render mismatch:\n got: %q\nwant: %q", got, want)
	}

	// XML detection is defeated the same way; verify it too.
	if d := Parse(withBOM("<root><a/></root>"), FormatAuto, 0); d.Format != FormatXML {
		t.Errorf("BOM+XML auto-detect = %v, want XML", d.Format)
	}
}

// TestModelAccessorsGuardOutOfRange checks the projection accessors no longer panic
// or return a fake one-past-last index for an out-of-range line/row, so the public
// API is defensively safe for external callers (P9).
func TestModelAccessorsGuardOutOfRange(t *testing.T) {
	d := Parse([]byte(`{"a":1,"b":2}`), FormatJSON, 0)
	nLines := int32(d.TotalLines())
	nRows := d.TotalVisibleRows()
	// None of these may panic on an out-of-range argument.
	_ = d.VisibleLine(nLines + 3)
	_ = d.RowOfLine(nLines + 3)
	_ = d.RevealLine(nLines + 3)
	if got := d.LineAtRow(nRows + 3); int(got) >= d.TotalLines() {
		t.Errorf("LineAtRow past the end returned a fake index %d (>= %d lines)", got, d.TotalLines())
	}
}

// TestHTMLEmptyElementInline checks an explicit empty element <div></div> renders as
// a single inline (non-foldable) line, like the XML path, instead of a foldable node
// that collapses to a nonsensical "0 children" summary (P12).
func TestHTMLEmptyElementInline(t *testing.T) {
	if got := int(Parse([]byte(`<div></div>`), FormatHTML, 0).TotalVisibleRows()); got != 1 {
		t.Errorf("<div></div> = %d visible rows, want 1 (inline empty element)", got)
	}
	// A non-empty parent stays foldable; only the empty child is inlined.
	if got := int(Parse([]byte(`<a><b></b></a>`), FormatHTML, 0).TotalVisibleRows()); got != 3 {
		t.Errorf("<a><b></b></a> = %d visible rows, want 3 (<a> / <b></b> / </a>)", got)
	}
}

// TestDefaultCollapseProjection guards the single-Fenwick-build construction path
// (P11): a parse with a default-collapse depth must still yield a correct, consistent
// projection — some lines hidden, lineAtRow/rowOfLine round-trips, and ExpandAll
// restores the full row count.
func TestDefaultCollapseProjection(t *testing.T) {
	src := `{"a":{"b":{"c":1}},"d":[1,2,3]}`
	full := Parse([]byte(src), FormatJSON, 0)
	collapsed := Parse([]byte(src), FormatJSON, 1) // collapse below depth 1

	if collapsed.TotalVisibleRows() >= full.TotalVisibleRows() {
		t.Fatalf("collapseDepth=1 hid no rows: collapsed=%d full=%d",
			collapsed.TotalVisibleRows(), full.TotalVisibleRows())
	}
	for row := int32(0); row < collapsed.TotalVisibleRows(); row++ {
		li := collapsed.LineAtRow(row)
		if got := collapsed.RowOfLine(li); got != row {
			t.Errorf("projection round-trip: row %d -> line %d -> row %d", row, li, got)
		}
	}
	collapsed.ExpandAll()
	if collapsed.TotalVisibleRows() != full.TotalVisibleRows() {
		t.Errorf("ExpandAll after default-collapse = %d rows, want %d",
			collapsed.TotalVisibleRows(), full.TotalVisibleRows())
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
