package prettyview

import (
	"os"
	"testing"
)

func benchSrc(b *testing.B, name string) []byte {
	b.Helper()
	src, err := os.ReadFile("testdata/" + name)
	if err != nil {
		b.Fatal(err)
	}
	return src
}

func benchParse(b *testing.B, file string, f Format) {
	src := benchSrc(b, file)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = parseDocument(src, f, 0)
	}
}

func BenchmarkParseJSON(b *testing.B)    { benchParse(b, "openapi.json", FormatJSON) }
func BenchmarkParseBigJSON(b *testing.B) { benchParse(b, "big.json", FormatJSON) }
func BenchmarkParseXML(b *testing.B)     { benchParse(b, "catalog.xml", FormatXML) }
func BenchmarkParseHTML(b *testing.B)    { benchParse(b, "page.html", FormatHTML) }

// BenchmarkFoldToggle measures an incremental fold/unfold of a top-level node.
func BenchmarkFoldToggle(b *testing.B) {
	d := parseDocument(benchSrc(b, "openapi.json"), FormatJSON, 0)
	node := firstFoldHeadAtDepth(d, 1)
	if node == NoNode {
		b.Skip("no foldable node")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.fold.fold(d, node)
		d.fold.unfold(d, node)
	}
}

// BenchmarkProjectionLookup measures the O(log n) visible-row -> line lookup on a
// 440k-row document.
func BenchmarkProjectionLookup(b *testing.B) {
	d := parseDocument(benchSrc(b, "big.json"), FormatJSON, 0)
	total := d.fold.TotalVisibleRows()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.fold.lineAtRow(int32(i) % total)
	}
}

// BenchmarkExpandCollapseAll measures the full-document projection rebuild.
func BenchmarkExpandCollapseAll(b *testing.B) {
	d := parseDocument(benchSrc(b, "openapi.json"), FormatJSON, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.fold.collapseAll(d)
		d.fold.expandAll(d)
	}
}

// BenchmarkSearch measures a full-document scan for a common token.
func BenchmarkSearch(b *testing.B) {
	pv := New()
	pv.doc = parseDocument(benchSrc(b, "openapi.json"), FormatJSON, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pv.runSearch(SearchQuery{Text: "type"})
	}
}
