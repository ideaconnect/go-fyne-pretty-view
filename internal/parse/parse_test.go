package parse

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
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

// TestRawParserFormatDetect covers the rawParser's trivial Format/Detect methods
// (raw never wins auto-detection; it is the explicit floor).
func TestRawParserFormatDetect(t *testing.T) {
	var p rawParser
	if p.Format() != FormatRaw {
		t.Errorf("rawParser.Format = %v, want raw", p.Format())
	}
	if got := p.Detect([]byte("anything at all")); got != 0 {
		t.Errorf("rawParser.Detect = %d, want 0", got)
	}
}

// TestJSONCCommentsRendered: in JSONC mode // line and /* */ block comments are
// emitted as nodes (visible, searchable, copyable) rather than silently stripped,
// while plain JSON still skips them. Covers a line comment, a block comment, and a
// block comment left unclosed at EOF.
func TestJSONCCommentsRendered(t *testing.T) {
	src := []byte("{ // a line comment\n  \"a\": 1, /* block */ \"b\": 2 /* unclosed")
	d := Parse(src, FormatJSONC, 0)
	if d.Format != FormatJSONC {
		t.Fatalf("format = %v, want jsonc", d.Format)
	}
	text := renderDoc(d)
	if !strings.Contains(text, `"a"`) || !strings.Contains(text, `"b"`) {
		t.Errorf("JSONC dropped keys:\n%s", text)
	}
	for _, c := range []string{"// a line comment", "/* block */"} {
		if !strings.Contains(text, c) {
			t.Errorf("JSONC comment %q not rendered as a node:\n%s", c, text)
		}
	}
	// Plain JSON of the same bytes still skips comments (no comment nodes).
	if pj := renderDoc(Parse(src, FormatJSON, 0)); strings.Contains(pj, "line comment") {
		t.Errorf("plain JSON should not render comments as nodes:\n%s", pj)
	}
	// A comment inside otherwise-empty braces makes a real container, not a {} leaf.
	withComment := renderDoc(Parse([]byte("{ /* c */ }"), FormatJSONC, 0))
	if !strings.Contains(withComment, "/* c */") {
		t.Errorf("comment in empty braces not rendered:\n%s", withComment)
	}
	// Truly empty braces stay a single {} leaf.
	if got := int(Parse([]byte("{}"), FormatJSONC, 0).TotalVisibleRows()); got != 1 {
		t.Errorf("empty {} = %d rows, want 1 (leaf)", got)
	}
	// A trailing comment after the root value renders too (symmetric with leading) and
	// must not trigger the trailing-content raw fallback.
	tr := Parse([]byte(`{"a":1} // trailing note`), FormatJSONC, 0)
	if tr.Format != FormatJSONC {
		t.Fatalf("trailing-comment format = %v, want jsonc (not raw fallback)", tr.Format)
	}
	if text := renderDoc(tr); !strings.Contains(text, "// trailing note") {
		t.Errorf("trailing JSONC comment not rendered:\n%s", text)
	}
}

// TestJSONCInlineCommentsRetained is the regression guard for issue #60/#61: the structured
// parser retains a JSONC comment in EVERY position — not just leading a key or element, but
// also between a key and its value and trailing a value inside a container (the positions the
// old skipSpace discarded). An inline comment before a scalar / after a value renders on its
// own line just below the member (relocated to keep node SrcStart non-decreasing) but is
// never dropped, so Reformat can prettify JSONC losslessly.
func TestJSONCInlineCommentsRetained(t *testing.T) {
	retained := map[string]string{
		"leading line, before key":    "{\n  // keep\n  \"a\": 1\n}",
		"leading block, before key":   "{\n  /* keep */\n  \"a\": 1\n}",
		"leading before array elem":   "[\n  1,\n  // keep\n  2\n]",
		"trailing after root value":   `{"a":1} // keep`,
		"between key colon and value": "{\n  \"a\": /* keep */ 1\n}",
		"trailing a member value":     "{\n  \"a\": 1,\n  \"b\": 2 // keep\n}",
		"between colon and container": "{\n  \"a\": /* keep */ {\"x\": 1}\n}",
	}
	for name, src := range retained {
		if text := renderDoc(Parse([]byte(src), FormatJSONC, 0)); !strings.Contains(text, "keep") {
			t.Errorf("%s: comment should be retained but was dropped:\n%s", name, text)
		}
	}
}

// TestJSONNonASCIIWhitespace is the regression for the detector/scanner whitespace
// mismatch: auto-detection trims unicode.IsSpace, so the scanner must skip the same
// set or an input it labels JSON stalls mid-scan — a leading form-feed would fall
// back to raw, and a form-feed/NBSP between members would silently drop the rest of
// the container. Each case must stay JSON with every member rendered.
func TestJSONNonASCIIWhitespace(t *testing.T) {
	cases := []struct {
		name, src string
	}{
		{"leading form-feed", "\f{\"a\":1,\"b\":2}"},
		{"leading NBSP", " {\"a\":1,\"b\":2}"},
		{"form-feed between members", "{\"a\":1,\f\"b\":2}"},
		{"NBSP between members", "{\"a\":1, \"b\":2}"},
		{"vertical-tab between members", "{\"a\":1,\v\"b\":2}"},
	}
	for _, c := range cases {
		d := Parse([]byte(c.src), FormatAuto, 0)
		if d.Format != FormatJSON {
			t.Errorf("%s: format = %v, want json (whitespace defeated detection/scan)", c.name, d.Format)
			continue
		}
		text := renderDoc(d)
		if !strings.Contains(text, `"a"`) || !strings.Contains(text, `"b"`) {
			t.Errorf("%s: a member was dropped:\n%s", c.name, text)
		}
	}
}

// TestDetectXMLvsHTML guards the structural-tag sniff: an XML document that merely
// contains an HTML-ish tag name (substring, attribute, or longer element) must not
// be misclassified as HTML (which would fold its tag-name case), while a genuine
// HTML fragment that opens with a structural tag still detects as HTML.
func TestDetectXMLvsHTML(t *testing.T) {
	cases := []struct {
		name, src string
		want      Format
	}{
		{"longer element <divider>", "<divider>x</divider>", FormatXML},
		{"longer element <header>", "<header>x</header>", FormatXML},
		{"mixed-case root with embedded <p>", "<Root><Child><p>x</p></Child></Root>", FormatXML},
		{"html tag as attribute/child not at start", `<config><a href="x"></a></config>`, FormatXML},
		{"html-ish name in prose", "<root>mentions <div in text</root>", FormatXML},
		{"real html fragment <div>", `<div class="x">hi</div>`, FormatHTML},
		{"real html fragment <p>", "<p>hi</p>", FormatHTML},
	}
	for _, c := range cases {
		if d := Parse([]byte(c.src), FormatAuto, 0); d.Format != c.want {
			t.Errorf("%s: detected %v, want %v", c.name, d.Format, c.want)
		}
	}
}

// TestJSONTrailingJunkSurfaced is the regression for the delimiter check: a bare
// literal/number scan stops at the first foreign byte, so trailing junk after a
// value (no comma/close) was silently dropped. It must now surface as an error
// marker so the dropped bytes stay visible.
func TestJSONTrailingJunkSurfaced(t *testing.T) {
	cases := []struct{ name, src, junk string }{
		{"literal", "[trueX]", "X"},
		{"number", "[123abc]", "abc"},
		{"object value", `{"a":1bogus}`, "bogus"},
	}
	for _, c := range cases {
		d := Parse([]byte(c.src), FormatJSON, 0)
		if text := renderDoc(d); !strings.Contains(text, c.junk) {
			t.Errorf("%s: trailing junk %q was dropped, not surfaced:\n%s", c.name, c.junk, text)
		}
	}
}

// TestJSONDetectRejectsBracketLedNonJSON is the regression for over-eager
// auto-detection: a leading '{'/'[' alone is too weak a signal. Bracket-led log
// and markdown lines must NOT be classified as JSON (which made the tolerant
// scanner recover a sliver and silently drop the rest), while genuine JSON of
// every shape still detects.
func TestJSONDetectRejectsBracketLedNonJSON(t *testing.T) {
	cases := []struct {
		name, src string
		want      Format
	}{
		{"log level prefix", "[ERROR] disk full at /var\nretrying in 5s", FormatRaw},
		{"info tag line", "[INFO] starting up", FormatRaw},
		{"markdown link", "[label](https://example.com) see the docs", FormatRaw},
		{"prose in braces", "{this is prose, not json}", FormatRaw},
		{"real object", `{"a":1}`, FormatJSON},
		{"real array of numbers", `[1, 2, 3]`, FormatJSON},
		{"real array of strings", `["x","y"]`, FormatJSON},
		{"empty object", `{}`, FormatJSON},
		{"empty array", `[]`, FormatJSON},
		{"leading whitespace then object", "  \n  {\"a\":1}", FormatJSON},
	}
	for _, c := range cases {
		if d := Parse([]byte(c.src), FormatAuto, 0); d.Format != c.want {
			t.Errorf("%s (%q): detected %v, want %v", c.name, c.src, d.Format, c.want)
		}
	}
}

// TestJSONTrailingContentFallsBackToRaw is the regression for the silent
// drop-everything-after-the-root bug: a cleanly-parsed root value followed by more
// bytes (NDJSON, concatenated values, or trailing prose) is not a single JSON
// document. It must fall back to raw with every byte still visible, rather than
// rendering only the first value and discarding the rest. (An inner-truncated value
// still recovers a tolerant partial tree — see TestJSONTrailingJunkSurfaced.)
func TestJSONTrailingContentFallsBackToRaw(t *testing.T) {
	cases := []struct{ name, src, tail string }{
		{"ndjson", "{\"a\":1}\n{\"b\":2}\n{\"c\":3}", `"c"`},
		{"concatenated arrays", "[1,2][3,4]", "[3,4]"},
		{"value then prose", `{"ok":true} and then some notes`, "notes"},
	}
	for _, c := range cases {
		d := Parse([]byte(c.src), FormatAuto, 0)
		if d.Format != FormatRaw {
			t.Errorf("%s: format = %v, want raw (trailing content was silently dropped)", c.name, d.Format)
		}
		if text := renderDoc(d); !strings.Contains(text, c.tail) {
			t.Errorf("%s: trailing content %q missing from raw render:\n%s", c.name, c.tail, text)
		}
	}

	// The same fallback must hold for an explicitly forced FormatJSON bare-scalar
	// root (which auto-detect never picks, so it exercises the Parse-side trailing
	// check directly rather than Detect).
	if d := Parse([]byte("42 43"), FormatJSON, 0); d.Format != FormatRaw {
		t.Errorf("forced-JSON bare scalar with trailing content: format = %v, want raw", d.Format)
	}
}

// TestNoControlCharsInDisplayLines pins the one-row-per-line / uniform-grid
// invariant for the structured parsers: a display line must never contain a raw C0
// control byte or DEL, which would spill onto another row or desync the monospace
// column math (only the raw parser deliberately expands \t to stops). Data tokens
// (string values, keys, attribute values) and recovered junk spans are byte ranges
// the scanners take verbatim, so a malformed input could otherwise leak a control
// byte straight onto the row. The `want` substrings also assert the bytes are
// *escaped*, not silently dropped (a one-sided absence check would pass on a drop),
// and that multi-byte runes adjacent to a control byte survive intact.
func TestNoControlCharsInDisplayLines(t *testing.T) {
	hasCtrl := func(s string) bool {
		for i := 0; i < len(s); i++ {
			if s[i] < 0x20 || s[i] == 0x7f {
				return true
			}
		}
		return false
	}
	cases := []struct {
		name   string
		format Format
		src    string
		want   []string // visible escapes / preserved runes that must appear (no silent drop)
	}{
		{"json string with raw newline", FormatJSON, "{\"a\":\"line1\nline2\"}", []string{`line1\nline2`}},
		{"json string with raw tab", FormatJSON, "{\"a\":\"col1\tcol2\"}", []string{`col1\tcol2`}},
		{"json string with raw escape byte", FormatJSON, "{\"a\":\"x\x1by\"}", []string{`x\x1by`}},
		{"json multibyte beside control", FormatJSON, "{\"a\":\"é\tü\"}", []string{"é", "ü", `\t`}},
		{"json key with raw newline", FormatJSON, "{\"k\ney\":1}", nil},
		{"json in-container junk with newline", FormatJSON, "[true bogus\nmore]", nil},
		{"jsonc value with raw newline", FormatJSONC, "{\"k\":\"x\ny\"}", nil},
		{"xml attribute with newline", FormatXML, "<a t=\"x\ny\">z</a>", nil},
		{"xml attribute with tab", FormatXML, "<a t=\"x\ty\">z</a>", nil},
		{"html attribute with tab", FormatHTML, "<p data-x=\"a\tb\">hi</p>", nil},
		{"html attribute with newline", FormatHTML, "<p data-x=\"a\nb\">hi</p>", nil},
	}
	for _, c := range cases {
		d := Parse([]byte(c.src), c.format, 0)
		var all strings.Builder
		for li := 0; li < d.TotalLines(); li++ {
			s := d.LineString(int32(li))
			if hasCtrl(s) {
				t.Errorf("%s: line %d contains a raw control char: %q", c.name, li, s)
			}
			all.WriteString(s)
		}
		text := all.String()
		for _, w := range c.want {
			if !strings.Contains(text, w) {
				t.Errorf("%s: %q missing from render (byte silently dropped?):\n%s", c.name, w, text)
			}
		}
	}
}

// TestLineIsByteGrid pins the per-line byte==column flag the renderer's fast path
// relies on: true for a line with no multi-byte rune (so a column's byte offset can
// be found by arithmetic), false when a multi-byte rune is present, and — crucially
// — true for a lone invalid byte, which advances one column and one byte in both
// RuneCount and the renderer's decode walk.
func TestLineIsByteGrid(t *testing.T) {
	lineWith := func(d *model.Document, sub string) int32 {
		for li := 0; li < d.TotalLines(); li++ {
			if strings.Contains(d.LineString(int32(li)), sub) {
				return int32(li)
			}
		}
		t.Fatalf("no line contains %q", sub)
		return -1
	}
	ascii := Parse([]byte(`["abcdefghij"]`), FormatJSON, 0)
	if !ascii.LineIsByteGrid(lineWith(ascii, "abcdefghij")) {
		t.Error("ASCII value line should be a byte grid")
	}
	multi := Parse([]byte(`["héllo wörld"]`), FormatJSON, 0)
	if multi.LineIsByteGrid(lineWith(multi, "wörld")) {
		t.Error("multi-byte value line should NOT be a byte grid")
	}
	inv := Parse([]byte("[\"a\x80b\"]"), FormatJSON, 0)
	if !inv.LineIsByteGrid(lineWith(inv, "a")) {
		t.Error("a lone invalid byte still advances 1 col / 1 byte — should be a byte grid")
	}
	// Out-of-range is the safe slow-path default.
	if ascii.LineIsByteGrid(int32(ascii.TotalLines()) + 5) {
		t.Error("out-of-range line should report false (slow path)")
	}
}

// panicParser is a stub that panics during Parse, to exercise the safeParse boundary.
type panicParser struct{}

func (panicParser) Format() Format                     { return FormatRaw }
func (panicParser) Detect([]byte) int                  { return 0 }
func (panicParser) Parse([]byte, *model.Builder) error { panic("synthetic parser panic") }

// TestSafeParseRecoversPanic is the regression for the panic boundary (#10): a parser
// consuming untrusted input that trips an unforeseen panic must be recovered into an
// error so Parse degrades to the raw fallback, never crashing the host.
func TestSafeParseRecoversPanic(t *testing.T) {
	b := model.NewBuilder([]byte("x"), FormatHTML, 0)
	err := safeParse(panicParser{}, []byte("x"), b)
	if err == nil {
		t.Fatal("safeParse must return an error when the parser panics, not propagate the panic")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("recovered error = %q, want it to mention the panic", err)
	}
}

// TestHTMLDeeplyNestedDoesNotCrash mirrors the JSON/XML deep-nesting guards: HTML far
// beyond the nesting cap must parse to a valid bounded document (the cap emits past-cap
// elements as leaves so the builder stack stays bounded) rather than crash or hang.
func TestHTMLDeeplyNestedDoesNotCrash(t *testing.T) {
	var sb strings.Builder
	const depth = maxNestDepth + 50
	for i := 0; i < depth; i++ {
		sb.WriteString("<div>")
	}
	sb.WriteString("x")
	for i := 0; i < depth; i++ {
		sb.WriteString("</div>")
	}
	d := Parse([]byte(sb.String()), FormatHTML, 0)
	if d.Format != FormatHTML {
		t.Fatalf("deeply-nested HTML format = %v, want html", d.Format)
	}
	if d.TotalLines() == 0 {
		t.Error("deeply-nested HTML produced no display lines")
	}
}

// TestJSONStringEscapes covers scanString's escape branch (no fixture has a
// backslash): an escaped quote/backslash must not terminate the string early, and a
// trailing backslash at EOF (unterminated) must recover tolerantly, not crash.
func TestJSONStringEscapes(t *testing.T) {
	d := Parse([]byte(`{"k":"a \"quote\" and a \\ slash"}`), FormatJSON, 0)
	if d.Format != FormatJSON {
		t.Fatalf("escaped-string format = %v, want json", d.Format)
	}
	if text := renderDoc(d); !strings.Contains(text, "quote") || !strings.Contains(text, "slash") {
		t.Errorf("escaped string was truncated:\n%s", text)
	}
	// Unterminated (trailing backslash before EOF): tolerant recovery, key kept visible.
	d2 := Parse([]byte(`{"k":"x\`), FormatJSON, 0)
	if d2 == nil || d2.TotalLines() == 0 {
		t.Fatal("unterminated string produced no document")
	}
	if !strings.Contains(renderDoc(d2), `"k"`) {
		t.Error("unterminated-string recovery dropped the key")
	}
}

// TestXMLHTMLSourceOffsets verifies element nodes now carry a real [SrcStart,SrcEnd)
// span into the source (previously 0,0), so CopySubtree/ExpandTo work by byte offset.
func TestXMLHTMLSourceOffsets(t *testing.T) {
	for _, c := range []struct {
		name, src string
		format    Format
	}{
		{"xml", `<root><a>x</a></root>`, FormatXML},
		{"html", `<div><p>hi</p></div>`, FormatHTML},
	} {
		d := Parse([]byte(c.src), c.format, 0)
		found := false
		for id := 1; id < len(d.Nodes); id++ {
			n := d.Nodes[id]
			if n.Kind != model.KindElement || n.SrcEnd <= n.SrcStart {
				continue
			}
			found = true
			if int(n.SrcEnd) > len(c.src) {
				t.Fatalf("%s: node %d SrcEnd %d past source length %d", c.name, id, n.SrcEnd, len(c.src))
			}
			if span := c.src[n.SrcStart:n.SrcEnd]; !strings.HasPrefix(span, "<") || !strings.HasSuffix(span, ">") {
				t.Errorf("%s: node %d span %q is not a <...> region", c.name, id, span)
			}
		}
		if !found {
			t.Errorf("%s: no element node carries a source span (SrcEnd>SrcStart)", c.name)
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

// TestJSONUnterminatedBackslashNoOverrun is the #76 guard: an unterminated string ending in a
// backslash must not push the scanner past EOF — a node span End > len(Src) would later panic
// when SegBytes/CopySubtree slices Src. The parse is tolerant (must not panic) and every node
// span must stay within Src.
func TestJSONUnterminatedBackslashNoOverrun(t *testing.T) {
	for _, src := range []string{`"\`, `["\`, `{"k":"\`, `{"a":1,"k":"\`, `["a","\`} {
		d := Parse([]byte(src), FormatJSON, 0) // tolerant: must not panic
		for _, n := range d.Nodes {
			if int(n.SrcStart) > len(d.Src) || int(n.SrcEnd) > len(d.Src) {
				t.Errorf("src %q: node span [%d,%d) exceeds Src len %d", src, n.SrcStart, n.SrcEnd, len(d.Src))
			}
		}
		_ = renderDoc(d) // rendering must not panic on the recovered structure either
	}
}

// TestStructuredInputsNeverFallBackToRaw is the deterministic complement to FuzzParse's
// structured-mode control-byte check (#78): that fuzz assertion runs only when the input
// parses structured (d.Format != FormatRaw), so a regression that wrongly classified valid
// structured input as raw would SILENTLY disable the strongest invariant for exactly the
// inputs most likely to expose it. This table pins clearly-structured inputs that must parse
// to their format (and never to raw), failing loudly on a misclassification.
func TestStructuredInputsNeverFallBackToRaw(t *testing.T) {
	cases := []struct {
		src  string
		want Format
		auto bool // also assert FormatAuto keeps it structured (JSONC with a leading comment
		// does not auto-detect — see issue #82 — so it is checked under its forced format only)
	}{
		{`{"a":1,"b":[2,3],"c":{"d":"x"}}`, FormatJSON, true},
		{`[1,2,3,{"k":"v"}]`, FormatJSON, true},
		{"{ // c\n\"a\": 1 /* b */ }", FormatJSONC, false},
		{`<root><a id="1">x</a><b/></root>`, FormatXML, true},
		{`<div class="x"><p>hi</p><br></div>`, FormatHTML, true},
	}
	for _, c := range cases {
		if got := Parse([]byte(c.src), c.want, 0); got.Format != c.want {
			t.Errorf("forced %v on %q: Format = %v (regressed to raw fallback?)", c.want, c.src, got.Format)
		}
		if c.auto {
			if got := Parse([]byte(c.src), FormatAuto, 0); got.Format == FormatRaw {
				t.Errorf("auto-detect of %q fell back to raw", c.src)
			}
		}
	}
}
