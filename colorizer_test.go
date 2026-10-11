package prettyview

import (
	"bytes"
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// uriColorizer is a host grammar for one-share-URI-per-line text:
// scheme "://" (RoleTag + RolePunct), userinfo before "@" (RoleKey), host:port (RoleString),
// "?k=v&k=v" (RoleAttr keys, RolePunct "?=&", RoleString values) and "#fragment" (RoleComment).
func uriColorizer(dst []ColorSpan, src []byte) []ColorSpan {
	start := 0
	for start <= len(src) {
		end := bytes.IndexByte(src[start:], '\n')
		if end < 0 {
			end = len(src)
		} else {
			end += start
		}
		dst = colorURILine(dst, src, start, end)
		start = end + 1
	}
	return dst
}

func colorURILine(dst []ColorSpan, src []byte, start, end int) []ColorSpan {
	line := src[start:end]
	sep := bytes.Index(line, []byte("://"))
	if sep < 0 {
		return dst
	}
	dst = append(dst, ColorSpan{Start: start, End: start + sep, Role: RoleTag},
		ColorSpan{Start: start + sep, End: start + sep + 3, Role: RolePunct})
	i := start + sep + 3
	frag := end
	if h := bytes.IndexByte(src[i:end], '#'); h >= 0 {
		frag = i + h
		dst = append(dst, ColorSpan{Start: frag, End: end, Role: RoleComment})
	}
	query := frag
	if q := bytes.IndexByte(src[i:frag], '?'); q >= 0 {
		query = i + q
	}
	if at := bytes.IndexByte(src[i:query], '@'); at >= 0 {
		dst = append(dst, ColorSpan{Start: i, End: i + at, Role: RoleKey},
			ColorSpan{Start: i + at, End: i + at + 1, Role: RolePunct})
		i += at + 1
	}
	dst = append(dst, ColorSpan{Start: i, End: query, Role: RoleString})
	for k := query; k < frag; {
		dst = append(dst, ColorSpan{Start: k, End: k + 1, Role: RolePunct}) // '?' or '&'
		k++
		next := bytes.IndexByte(src[k:frag], '&')
		if next < 0 {
			next = frag - k
		}
		if eq := bytes.IndexByte(src[k:k+next], '='); eq >= 0 {
			dst = append(dst, ColorSpan{Start: k, End: k + eq, Role: RoleAttr},
				ColorSpan{Start: k + eq, End: k + eq + 1, Role: RolePunct},
				ColorSpan{Start: k + eq + 1, End: k + next, Role: RoleString})
		}
		k += next
	}
	return dst
}

// liveRowAt returns the live row widget showing sub-row sub of display line li, or nil.
func liveRowAt(pv *PrettyView, li, sub int32) *rowWidget {
	for _, rw := range pv.r.live {
		if rw.line == li && rw.sub == sub {
			return rw
		}
	}
	return nil
}

// paintedColors returns the color of every rune line li paints, across all its visual
// rows in order (so it is the same sequence wrapped or not).
func paintedColors(t *testing.T, pv *PrettyView, li int32) []color.Color {
	t.Helper()
	var out []color.Color
	for sub := int32(0); sub < pv.doc.RowsOfLine(li); sub++ {
		rw := liveRowAt(pv, li, sub)
		if rw == nil {
			t.Fatalf("line %d sub-row %d is not live", li, sub)
		}
		for _, tx := range rw.rr.texts {
			if !tx.Visible() {
				continue
			}
			for range []rune(tx.Text) {
				out = append(out, tx.Color)
			}
		}
	}
	return out
}

// expectedColors maps each rune of line text (which starts at byte off of src) to the
// palette color of the span covering its first byte.
func expectedColors(pv *PrettyView, src []byte, off int, text string, spans []ColorSpan) []color.Color {
	var out []color.Color
	b := off
	for _, r := range text {
		role := RolePlain
		for _, s := range spans {
			if s.Start <= b && b < s.End {
				role = s.Role
				break
			}
		}
		out = append(out, pv.palette[role])
		b += len(string(r))
	}
	return out
}

func sameColors(a, b []color.Color) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestWithColorizerPaintsBothModes: a raw document is painted with the host grammar's
// roles in the read-only viewer and in the editor, soft-wrapped or not, and the editor
// recolors on every keystroke. A long share URI wrapped over several rows keeps every
// rune's color across the row breaks.
func TestWithColorizerPaintsBothModes(t *testing.T) {
	uri := "vless://0b1c2d3e-aaaa-bbbb-cccc-0123456789ab@edge.example.com:443?security=reality&sni=www.example.com&fp=chrome&pbk=" +
		strings.Repeat("Q", 60) + "&type=tcp#My%20node"
	src := []byte("# subscription\n" + uri + "\n")
	spans := uriColorizer(nil, src)
	off := len("# subscription\n")

	for _, mode := range []struct {
		name string
		opts []Option
	}{
		{"read-only", nil},
		{"read-only wrapped", []Option{WithWrap(WrapWord)}},
		{"editable", []Option{WithEditable(), WithInputConfig(InputConfig{AutoFormat: AutoFormatOff})}},
		{"editable wrapped", []Option{WithEditable(), WithInputConfig(InputConfig{AutoFormat: AutoFormatOff}), WithWrap(WrapWord)}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			test.NewApp()
			pv := New(append([]Option{WithColorizer(uriColorizer)}, mode.opts...)...)
			pv.SetData(src, FormatRaw)
			win := test.NewWindow(pv)
			defer win.Close()
			win.Resize(fyne.NewSize(2400, 400))
			if strings.Contains(mode.name, "wrapped") {
				win.Resize(fyne.NewSize(420, 900))
			}
			pv.Refresh()
			if strings.Contains(mode.name, "wrapped") && pv.doc.RowsOfLine(1) < 3 {
				t.Fatalf("URI line should wrap to several rows, got %d", pv.doc.RowsOfLine(1))
			}
			got := paintedColors(t, pv, 1)
			want := expectedColors(pv, src, off, uri, spans)
			if !sameColors(got, want) {
				t.Fatalf("URI line painted %d runes with the wrong colors (want %d)", len(got), len(want))
			}
			if c := paintedColors(t, pv, 0); len(c) == 0 || c[0] != pv.palette[RolePlain] {
				t.Errorf("a line the grammar does not cover is not plain")
			}
			if pv.palette[RoleTag] == pv.palette[RolePlain] {
				t.Fatal("test theme gives RoleTag the plain color: the assertions above have no teeth")
			}
			if !pv.Editable() {
				return
			}
			// Live: turning the comment line into a URI recolors it on the keystroke.
			pv.SetCaret(0, 0)
			for range 2 {
				pv.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDelete})
			}
			pv.TypedRune('x')
			pv.TypedRune(':')
			pv.TypedRune('/')
			pv.TypedRune('/')
			if c := paintedColors(t, pv, 0); len(c) == 0 || c[0] != pv.palette[RoleTag] {
				t.Errorf("edited line was not recolored by the host grammar")
			}
		})
	}
}

// TestWithColorizerLeavesStructuredFormats: JSON keeps its built-in highlighting and the
// host colorizer is never consulted; a nil colorizer is ignored.
func TestWithColorizerLeavesStructuredFormats(t *testing.T) {
	test.NewApp()
	calls := 0
	spy := func(dst []ColorSpan, src []byte) []ColorSpan { calls++; return dst }
	for _, opts := range [][]Option{{WithColorizer(spy)}, {WithColorizer(spy), WithEditable()}} {
		pv := New(opts...)
		pv.SetData([]byte(`{"a": 1}`), FormatJSON)
		pv.SetData([]byte(`{"a": 1}`), FormatAuto)
	}
	if calls != 0 {
		t.Errorf("colorizer consulted %d times for JSON", calls)
	}
	pv := New(WithColorizer(nil))
	if pv.cfg.colorizer != nil {
		t.Error("WithColorizer(nil) installed a colorizer")
	}
}
