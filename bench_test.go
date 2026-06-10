package prettyview

import (
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/parse"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
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
		_ = parse.Parse(src, f, 0)
	}
}

func BenchmarkParseJSON(b *testing.B)    { benchParse(b, "openapi.json", FormatJSON) }
func BenchmarkParseBigJSON(b *testing.B) { benchParse(b, "big.json", FormatJSON) }
func BenchmarkParseXML(b *testing.B)     { benchParse(b, "catalog.xml", FormatXML) }
func BenchmarkParseHTML(b *testing.B)    { benchParse(b, "page.html", FormatHTML) }

// BenchmarkFoldToggle measures an incremental fold/unfold of a top-level node.
func BenchmarkFoldToggle(b *testing.B) {
	d := parse.Parse(benchSrc(b, "openapi.json"), FormatJSON, 0)
	node := firstFoldHeadAtDepth(d, 1)
	if node == model.NoNode {
		b.Skip("no foldable node")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Fold(node)
		d.Unfold(node)
	}
}

// BenchmarkProjectionLookup measures the O(log n) visible-row -> line lookup on a
// 440k-row document.
func BenchmarkProjectionLookup(b *testing.B) {
	d := parse.Parse(benchSrc(b, "big.json"), FormatJSON, 0)
	total := d.TotalVisibleRows()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.LineAtRow(int32(i) % total)
	}
}

// BenchmarkExpandCollapseAll measures the full-document projection rebuild.
func BenchmarkExpandCollapseAll(b *testing.B) {
	d := parse.Parse(benchSrc(b, "openapi.json"), FormatJSON, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.CollapseAll()
		d.ExpandAll()
	}
}

// BenchmarkSearch measures a full-document scan for a common token.
func BenchmarkSearch(b *testing.B) {
	pv := New()
	pv.doc = parse.Parse(benchSrc(b, "openapi.json"), FormatJSON, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pv.runSearch(SearchQuery{Text: "type"})
	}
}

// BenchmarkSearchRegex measures a full-document regex scan — the path the plain
// benchmarks don't exercise (SearchQuery without a Mode defaults to SearchPlain).
func BenchmarkSearchRegex(b *testing.B) {
	pv := New()
	pv.doc = parse.Parse(benchSrc(b, "openapi.json"), FormatJSON, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pv.runSearch(SearchQuery{Text: "type|name", Mode: SearchRegex})
	}
}

// BenchmarkSearchRegexLiteralPrefix measures a regex whose pattern has a selective
// literal head ("needle", present on ~0.1% of lines): the literal-prefix prefilter
// skips RE2 on every line lacking it via a cheap bytes.Index, so the scan is bounded
// by the number of candidate lines, not the document size.
func BenchmarkSearchRegexLiteralPrefix(b *testing.B) {
	var sb strings.Builder
	sb.WriteByte('[')
	for i := 0; i < 20000; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		if i%1000 == 0 {
			sb.WriteString(`"needle42"`)
		} else {
			sb.WriteString(`"filler-text-goes-right-here"`)
		}
	}
	sb.WriteByte(']')
	pv := New()
	pv.doc = parse.Parse([]byte(sb.String()), FormatJSON, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pv.runSearch(SearchQuery{Text: `needle\d+`, Mode: SearchRegex, CaseSensitive: true})
	}
}

// BenchmarkHorizontalScrollHugeLine measures one reflow (rebuild of the visible
// rows) of a document whose single value is a ~2 MB line, scrolled horizontally to
// column 1000. This is the case build()'s cull must bound: the old code paid a full
// utf8.RuneCount over the whole segment every reflow; the rewrite decodes only up to
// the visible window.
func BenchmarkHorizontalScrollHugeLine(b *testing.B) {
	test.NewApp()
	long := strings.Repeat("abcdefghij", 200_000) // 2 MB single line
	pv := NewWithData([]byte(`["`+long+`"]`), FormatJSON)
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))
	pv.Refresh()
	li := pv.doc.LineAtRow(1)
	depth := pv.doc.Lines[li].Depth
	row := int(pv.doc.RowOfLine(li))
	pv.r.scrollToOffset(fyne.NewPos(pv.met.ColX(depth, 1000), pv.met.RowY(row)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pv.r.reflow()
	}
}

// BenchmarkReflowTopOfWrappedLine / BenchmarkReflowDeepIntoWrappedLine measure one
// reflow of a soft-wrapped 2 MB single line at sub-row 0 vs. scrolled deep into the
// line. With the byte==column-grid fast path the per-row build is O(visible window),
// so the DEEP/TOP ratio stays near 1x; before it, deep paid O(start column).
func benchReflowWrappedAt(b *testing.B, deep bool) {
	test.NewApp()
	long := strings.Repeat("abcdefghij", 200_000) // 2 MB single ASCII line
	pv := NewWithData([]byte(`["`+long+`"]`), FormatJSON, WithWrap(WrapWord))
	win := test.NewWindow(pv)
	defer win.Close()
	win.Resize(fyne.NewSize(400, 300))
	pv.Refresh()
	li := pv.doc.LineAtRow(1)
	row := int(pv.doc.RowOfLine(li))
	target := row
	if deep {
		target = row + int(pv.doc.TotalVisibleRows())/2 // halfway down the wrapped line
	}
	pv.r.scrollToOffset(fyne.NewPos(0, pv.met.RowY(target)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pv.r.reflow()
	}
}

func BenchmarkReflowTopOfWrappedLine(b *testing.B)    { benchReflowWrappedAt(b, false) }
func BenchmarkReflowDeepIntoWrappedLine(b *testing.B) { benchReflowWrappedAt(b, true) }

// BenchmarkSearchManyMatchesOneLine measures a search where thousands of matches
// land on a single (raw) line — the O(K*L) case the addMatchB rune cursor fixes.
func BenchmarkSearchManyMatchesOneLine(b *testing.B) {
	line := strings.Repeat("needle ", 5000) // one raw line, 5000 matches
	pv := New()
	pv.doc = parse.Parse([]byte(line), FormatRaw, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pv.runSearch(SearchQuery{Text: "needle"})
	}
}

// BenchmarkSelectAllCopyBig measures the SelectAll+Copy gather (selectedText) on a
// multi-megabyte document — the largest transient allocation a normal user gesture
// triggers. The optimized path appends whole interior lines byte-for-byte into a
// reused buffer with no per-line string/[]rune.
func BenchmarkSelectAllCopyBig(b *testing.B) {
	pv := New()
	pv.doc = parse.Parse(benchSrc(b, "big.json"), FormatJSON, 0)
	pv.SelectAll()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pv.selectedText()
	}
}
