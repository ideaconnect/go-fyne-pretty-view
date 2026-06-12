package parse

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// FuzzEditableCaretCells is the property fuzz for the caret-exactness invariant (#72): for
// arbitrary bytes, the editable projection has one display line per physical line, each
// display line has exactly as many runes as the buffer line (so the caret column is exact),
// and no display line carries a raw grid-hostile byte (each becomes a placeholder rune).
// Inputs are bounded below the SegCount-saturation / maxColorLineBytes regime where per-line
// rune-equality is the documented guarantee.
func FuzzEditableCaretCells(f *testing.F) {
	f.Add("hello\nworld")
	f.Add("a\tb\x01c中")
	f.Add("{\"k\":\"v\",\n\"n\":[1,2]}")
	f.Add("<a x=\"y\">t&amp;x</a>")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 8000 {
			return // keep lines short of SegCount saturation, where the tail may drop
		}
		lines := strings.Split(s, "\n")
		for _, format := range []Format{FormatJSON, FormatJSONC, FormatXML, FormatHTML, FormatRaw} {
			d := ParseEditableColored([]byte(s), format, 0, 4)
			if d.TotalLines() != len(lines) {
				t.Fatalf("fmt=%v TotalLines=%d want %d", format, d.TotalLines(), len(lines))
			}
			for i, ln := range lines {
				if got, want := d.LineRuneLen(int32(i)), utf8.RuneCount([]byte(ln)); got != want {
					t.Fatalf("fmt=%v line %d: display runes %d != buffer runes %d (%q)", format, i, got, want, ln)
				}
				disp := d.LineString(int32(i))
				for k := 0; k < len(disp); k++ {
					if disp[k] < 0x20 || disp[k] == 0x7f {
						t.Fatalf("fmt=%v line %d carries raw control 0x%02x", format, i, disp[k])
					}
				}
			}
			_ = model.RolePlain // keep the model import meaningful across edits
		}
	})
}
