package parse

import (
	"strings"
	"testing"
)

// FuzzParse drives every format through the tolerance-first parsers with arbitrary
// bytes. The parsers consume untrusted input, so the invariants under fuzz are:
//   - Parse never panics (the safeParse boundary recovers any it would) and returns a
//     non-nil, internally consistent document;
//   - every display line assembles and measures without panicking or going
//     out of range, and the visible-row <-> line projection round-trips;
//   - for a STRUCTURED result (not the raw fallback) no display line contains a raw
//     control byte (the one-row / uniform-grid invariant from v0.3.0).
//
// Run locally with: go test -run '^$' -fuzz=FuzzParse ./internal/parse
func FuzzParse(f *testing.F) {
	seeds := []string{
		`{"a":[1,2,{"b":null}],"c":"x\ny"}`,
		`{ // jsonc` + "\n" + `  "a": 1, /* b */ "b": 2 }`,
		`<root id="1"><child>text</child><e/></root>`,
		`<div class="x"><p>hi</p><br></div>`,
		"plain text\nwith\ttabs\nand lines",
		"[1,2,3] trailing junk",
		"[ERROR] not json",
		"", "{", "[", "<", "<!--", `{"a":`, "\x00\x1b\x7f", "\ufeff{\"a\":1}",
		// Control bytes the fuzzer found leaking into display segments (now escaped):
		// non-whitespace controls in element text, a tag name, and an attribute name.
		"<x>\x00\x1b\x7f</x>", "<a\x00>t</a>", "<a b\x00=\"c\">t</a>",
	}
	formats := []Format{FormatAuto, FormatJSON, FormatJSONC, FormatXML, FormatHTML, FormatRaw}
	for _, s := range seeds {
		for _, fm := range formats {
			f.Add(s, int(fm))
		}
	}

	f.Fuzz(func(t *testing.T, src string, fmtSel int) {
		format := formats[((fmtSel%len(formats))+len(formats))%len(formats)]
		d := Parse([]byte(src), format, 0)
		if d == nil {
			t.Fatal("Parse returned nil")
		}

		structured := d.Format != FormatRaw
		var buf []byte
		for li := int32(0); li < int32(d.TotalLines()); li++ {
			buf = d.AssembleLine(li, buf[:0]) // must not panic / out-of-range
			_ = d.DisplayString(li)
			if d.LineRuneLen(li) < 0 {
				t.Fatalf("line %d: negative rune length", li)
			}
			if structured {
				for i, b := range buf {
					if b < 0x20 || b == 0x7f {
						t.Fatalf("structured %v line %d byte %d is a raw control char %#x: %q",
							d.Format, li, i, b, strings.TrimSpace(string(buf)))
					}
				}
			}
		}

		// Visible-row projection must round-trip and stay in range.
		rows := d.TotalVisibleRows()
		for r := int32(0); r < rows; r++ {
			li := d.LineAtRow(r)
			if int(li) < 0 || int(li) >= d.TotalLines() {
				t.Fatalf("LineAtRow(%d) = %d out of range [0,%d)", r, li, d.TotalLines())
			}
			if got := d.RowOfLine(li); got != r {
				t.Fatalf("projection round-trip: row %d -> line %d -> row %d", r, li, got)
			}
		}
	})
}
