package parse

import (
	"bytes"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// testINI is a tiny host grammar: "[section]" (brackets RolePunct, name RoleTag),
// "key = value" (RoleKey, RolePunct, RoleString) and "#"/";" comment lines (RoleComment).
// It appends into dst, as the Colorizer contract asks.
func testINI(dst []model.ColorSpan, src []byte) []model.ColorSpan {
	forEachLine(src, false, func(start, end int) {
		line := src[start:end]
		trim := bytes.TrimLeft(line, " \t")
		lead := start + len(line) - len(trim)
		switch {
		case len(trim) == 0:
		case trim[0] == '#' || trim[0] == ';':
			dst = append(dst, model.ColorSpan{Start: lead, End: end, Role: model.RoleComment})
		case trim[0] == '[':
			close := bytes.IndexByte(trim, ']')
			if close < 0 {
				close = len(trim)
			}
			dst = append(dst,
				model.ColorSpan{Start: lead, End: lead + 1, Role: model.RolePunct},
				model.ColorSpan{Start: lead + 1, End: lead + close, Role: model.RoleTag})
			if close < len(trim) {
				dst = append(dst, model.ColorSpan{Start: lead + close, End: lead + close + 1, Role: model.RolePunct})
			}
		default:
			eq := bytes.IndexByte(trim, '=')
			if eq < 0 {
				return
			}
			key := bytes.TrimRight(trim[:eq], " \t")
			dst = append(dst,
				model.ColorSpan{Start: lead, End: lead + len(key), Role: model.RoleKey},
				model.ColorSpan{Start: lead + eq, End: lead + eq + 1, Role: model.RolePunct},
				model.ColorSpan{Start: lead + eq + 1, End: end, Role: model.RoleString})
		}
	})
	return dst
}

// lineRuns renders line li's displayed segments as "role:text" runs joined by "|".
func lineRuns(d *model.Document, li int32) string {
	var parts []string
	for _, s := range d.DisplaySegs(li) {
		parts = append(parts, fmt.Sprintf("%d:%s", s.Role, d.SegBytes(s)))
	}
	return strings.Join(parts, "|")
}

func TestSanitizeSpans(t *testing.T) {
	cs := func(s, e int, r model.ColorRole) model.ColorSpan { return model.ColorSpan{Start: s, End: e, Role: r} }
	cases := []struct {
		name string
		in   []model.ColorSpan
		n    int
		want []colorSpan
	}{
		{"nil", nil, 10, nil},
		{"sorted passes through", []model.ColorSpan{cs(0, 2, 1), cs(4, 6, 2)}, 10, []colorSpan{{0, 2, 1}, {4, 6, 2}}},
		{"unsorted is ordered", []model.ColorSpan{cs(4, 6, 2), cs(0, 2, 1)}, 10, []colorSpan{{0, 2, 1}, {4, 6, 2}}},
		{"partial overlap: later trimmed", []model.ColorSpan{cs(0, 5, 1), cs(3, 8, 2)}, 10, []colorSpan{{0, 5, 1}, {5, 8, 2}}},
		{"nested span dropped", []model.ColorSpan{cs(0, 8, 1), cs(2, 4, 2)}, 10, []colorSpan{{0, 8, 1}}},
		{"same start: first wins (stable)", []model.ColorSpan{cs(2, 4, 3), cs(2, 6, 1)}, 10, []colorSpan{{2, 4, 3}, {4, 6, 1}}},
		{"negative start clamped", []model.ColorSpan{cs(-5, 3, 1)}, 10, []colorSpan{{0, 3, 1}}},
		{"end clamped to n", []model.ColorSpan{cs(7, 99, 1)}, 10, []colorSpan{{7, 10, 1}}},
		{"wholly out of range dropped", []model.ColorSpan{cs(10, 12, 1), cs(-4, 0, 1)}, 10, nil},
		{"empty and inverted dropped", []model.ColorSpan{cs(3, 3, 1), cs(6, 4, 1)}, 10, nil},
		{"unknown role becomes plain", []model.ColorSpan{cs(0, 1, model.NumColorRoles), cs(1, 2, 200)}, 10, []colorSpan{{0, 1, model.RolePlain}, {1, 2, model.RolePlain}}},
		{"last valid role kept", []model.ColorSpan{cs(0, 1, model.NumColorRoles-1)}, 10, []colorSpan{{0, 1, model.NumColorRoles - 1}}},
		{"adjacent spans both kept", []model.ColorSpan{cs(0, 2, 1), cs(2, 4, 2)}, 10, []colorSpan{{0, 2, 1}, {2, 4, 2}}},
	}
	for _, c := range cases {
		got := sanitizeSpans(nil, append([]model.ColorSpan(nil), c.in...), c.n)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: sanitizeSpans = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestParseWithColorizerReadOnlyRaw: raw text is split into lines exactly as Parse does
// (tab expanded to the tab stop, CR trimmed) and each run takes its span's role.
func TestParseWithColorizerReadOnlyRaw(t *testing.T) {
	src := "[Interface]\r\n# comment\nAddress = 10.0.0.2/32\n\tMTU = 1280\n\nplain"
	d := ParseWithColorizer([]byte(src), FormatRaw, 0, 4, testINI)
	want := []string{
		"6:[|7:Interface|6:]",
		"9:# comment",
		"1:Address|0: |6:=|2: 10.0.0.2/32",
		"0:    |1:MTU|0: |6:=|2: 1280", // leading tab sits outside the key span
		"0:",                           // an empty line is one empty plain segment, as in Parse
		"0:plain",
	}
	if len(d.Lines) != len(want) {
		t.Fatalf("%d lines, want %d", len(d.Lines), len(want))
	}
	for li, w := range want {
		if got := lineRuns(d, int32(li)); got != w {
			t.Errorf("line %d: %q, want %q", li, got, w)
		}
	}
	// The tab's role is the role of its own byte: here a key span starts at the tab.
	keyAtTab := func(dst []model.ColorSpan, src []byte) []model.ColorSpan {
		return append(dst, model.ColorSpan{Start: 0, End: 2, Role: model.RoleKey})
	}
	if got := lineRuns(ParseWithColorizer([]byte("\tab"), FormatRaw, 0, 4, keyAtTab), 0); got != "1:    |1:a|0:b" {
		t.Errorf("tab inside a span: %q", got)
	}
}

// TestParseWithColorizerDispatch: the host grammar only ever colors raw text.
func TestParseWithColorizerDispatch(t *testing.T) {
	calls := 0
	counting := func(dst []model.ColorSpan, src []byte) []model.ColorSpan {
		calls++
		return testINI(dst, src)
	}
	jsonSrc := []byte(`{"a": [1, 2]}`)
	if got, want := ParseWithColorizer(jsonSrc, FormatJSON, 0, 4, counting), Parse(jsonSrc, FormatJSON, 0, 4); !reflect.DeepEqual(got.Lines, want.Lines) || !reflect.DeepEqual(got.Segs, want.Segs) {
		t.Error("explicit JSON with a colorizer differs from Parse")
	}
	if got, want := ParseWithColorizer(jsonSrc, FormatAuto, 0, 4, counting), Parse(jsonSrc, FormatAuto, 0, 4); got.Format != FormatJSON || !reflect.DeepEqual(got.Segs, want.Segs) {
		t.Errorf("auto-detected JSON with a colorizer: format %v, differs from Parse", got.Format)
	}
	bad := []byte(`{"a": `) // malformed JSON falls back to plain raw, not the host grammar
	if got, want := ParseWithColorizer(bad, FormatJSON, 0, 4, counting), Parse(bad, FormatJSON, 0, 4); !reflect.DeepEqual(got.Segs, want.Segs) {
		t.Error("failed structured parse with a colorizer differs from Parse's raw fallback")
	}
	if calls != 0 {
		t.Errorf("colorizer called %d times for structured input, want 0", calls)
	}
	ini := []byte("[Peer]\nEndpoint = 1.2.3.4:51820\n")
	d := ParseWithColorizer(ini, FormatAuto, 0, 4, counting)
	if calls != 1 || d.Format != FormatRaw || lineRuns(d, 0) != "6:[|7:Peer|6:]" {
		t.Errorf("auto-detected raw: calls=%d format=%v line0=%q", calls, d.Format, lineRuns(d, 0))
	}
	bom := ParseWithColorizer(append([]byte("\xEF\xBB\xBF"), ini...), FormatRaw, 0, 4, counting)
	if got := lineRuns(bom, 0); got != "6:[|7:Peer|6:]" {
		t.Errorf("BOM not stripped before coloring: %q", got)
	}
	if got, want := ParseWithColorizer(ini, FormatRaw, 0, 4, nil), Parse(ini, FormatRaw, 0, 4); !reflect.DeepEqual(got.Segs, want.Segs) {
		t.Error("nil colorizer differs from Parse")
	}
	if got, want := ParseWithColorizer([]byte("a\tb"), FormatRaw, 0, 0, testINI), Parse([]byte("a\tb"), FormatRaw, 0, 4); !reflect.DeepEqual(got.Segs, want.Segs) {
		t.Error("tab width 0 is not defaulted to 4 like Parse")
	}
}

// TestHostColorizerPanicAndLongLine: a panicking colorizer degrades to monochrome raw, and
// a line past maxColorLineBytes stays monochrome while its neighbours are colored.
func TestHostColorizerPanicAndLongLine(t *testing.T) {
	boom := func([]model.ColorSpan, []byte) []model.ColorSpan { panic("host bug") }
	src := []byte("k = v\n\tx")
	if got, want := ParseWithColorizer(src, FormatRaw, 0, 4, boom), Parse(src, FormatRaw, 0, 4); !reflect.DeepEqual(got.Segs, want.Segs) {
		t.Error("panicking colorizer: read-only doc differs from plain raw")
	}
	if got, want := parseEditableColoredWith(src, FormatRaw, 0, boom), ParseEditableColored(src, FormatRaw, 0); !reflect.DeepEqual(got.Segs, want.Segs) {
		t.Error("panicking colorizer: editable doc differs from monochrome edit-raw")
	}

	long := "k = " + strings.Repeat("v", maxColorLineBytes)
	edge := "k = " + strings.Repeat("v", maxColorLineBytes-4) // exactly maxColorLineBytes
	d := ParseWithColorizer([]byte(long+"\n"+edge+"\na = b"), FormatRaw, 0, 4, testINI)
	if got := lineRuns(d, 0); got != "0:"+long {
		t.Errorf("over-long line colored: %.40q", got)
	}
	if got := lineRuns(d, 1); !strings.HasPrefix(got, "1:k|0: |6:=|2: v") {
		t.Errorf("line of exactly maxColorLineBytes not colored: %.40q", got)
	}
	if got := lineRuns(d, 2); got != "1:a|0: |6:=|2: b" {
		t.Errorf("line after an over-long one: %q", got)
	}
}

// TestHostColorizerNeverSplitsRunes: random spans, many cutting through multi-byte runes,
// never change a line's text or rune count, in either projection, and every segment holds
// whole runes. Each rune takes the role active at its first byte.
func TestHostColorizerNeverSplitsRunes(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	src := []byte("ключ = значение\n\t日本 = 語テキスト; x\nemoji = 🙂🙂 end\n")
	for iter := 0; iter < 300; iter++ {
		var spans []model.ColorSpan
		for k := rng.Intn(12); k > 0; k-- {
			a, b := rng.Intn(len(src)+6)-3, rng.Intn(len(src)+6)-3
			spans = append(spans, model.ColorSpan{Start: a, End: b, Role: model.ColorRole(rng.Intn(int(model.NumColorRoles) + 2))})
		}
		host := func(dst []model.ColorSpan, _ []byte) []model.ColorSpan { return append(dst, spans...) }
		docs := map[string][2]*model.Document{
			"read-only": {ParseWithColorizer(src, FormatRaw, 0, 4, host), Parse(src, FormatRaw, 0, 4)},
			"editable":  {parseEditableColoredWith(src, FormatRaw, 0, host), ParseEditableColored(src, FormatRaw, 0)},
		}
		for name, pair := range docs {
			got, plain := pair[0], pair[1]
			if len(got.Lines) != len(plain.Lines) {
				t.Fatalf("iter %d %s: %d lines, want %d", iter, name, len(got.Lines), len(plain.Lines))
			}
			for li := range got.Lines {
				var gt, pt []byte
				for _, s := range got.DisplaySegs(int32(li)) {
					b := got.SegBytes(s)
					if !utf8.Valid(b) {
						t.Fatalf("iter %d %s line %d: segment %q splits a rune (spans %v)", iter, name, li, b, spans)
					}
					if s.Role >= model.NumColorRoles {
						t.Fatalf("iter %d %s: role %d escaped the palette", iter, name, s.Role)
					}
					gt = append(gt, b...)
				}
				for _, s := range plain.DisplaySegs(int32(li)) {
					pt = append(pt, plain.SegBytes(s)...)
				}
				if !bytes.Equal(gt, pt) {
					t.Fatalf("iter %d %s line %d: text %q, want %q", iter, name, li, gt, pt)
				}
			}
		}
	}
}

// TestHostColorizerSnapsToRuneStart pins the snap direction: a span starting inside a
// multi-byte rune begins at the next rune; one ending inside a rune keeps that whole rune.
func TestHostColorizerSnapsToRuneStart(t *testing.T) {
	src := []byte("aжbжc") // a(0) ж(1,2) b(3) ж(4,5) c(6)
	host := func(dst []model.ColorSpan, _ []byte) []model.ColorSpan {
		return append(dst, model.ColorSpan{Start: 2, End: 5, Role: model.RoleKey})
	}
	want := "0:aж|1:bж|0:c"
	if got := lineRuns(ParseWithColorizer(src, FormatRaw, 0, 4, host), 0); got != want {
		t.Errorf("read-only: %q, want %q", got, want)
	}
	if got := lineRuns(parseEditableColoredWith(src, FormatRaw, 0, host), 0); got != want {
		t.Errorf("editable: %q, want %q", got, want)
	}
}

// TestEditableHostColorizer: the editable projection colors raw buffers with the host
// grammar (grid-hostile bytes keep their one-rune placeholder and take the span's role),
// leaves structured formats to the built-in lexers, and skips the host above the budget.
func TestEditableHostColorizer(t *testing.T) {
	calls := 0
	counting := func(dst []model.ColorSpan, src []byte) []model.ColorSpan {
		calls++
		return testINI(dst, src)
	}
	d := parseEditableColoredWith([]byte("[Peer]\nkey = a\tb\n"), FormatRaw, 0, counting)
	if got := lineRuns(d, 0); got != "6:[|7:Peer|6:]" {
		t.Errorf("section line: %q", got)
	}
	if got := lineRuns(d, 1); got != "1:key|0: |6:=|2: a|2:"+ctlPlaceholder+"|2:b" {
		t.Errorf("key line with a tab: %q", got)
	}
	if len(d.Lines) != 3 {
		t.Errorf("editable projection lost the trailing caret line: %d lines", len(d.Lines))
	}

	jsonSrc := []byte(`{"a": 1}`)
	if got, want := parseEditableColoredWith(jsonSrc, FormatJSON, 0, counting), ParseEditableColored(jsonSrc, FormatJSON, 0); !reflect.DeepEqual(got.Segs, want.Segs) {
		t.Error("JSON buffer with a colorizer differs from the built-in lexer")
	}
	if calls != 1 {
		t.Errorf("colorizer calls = %d, want 1 (raw only)", calls)
	}

	big := bytes.Repeat([]byte("k = v\n"), LiveColorBudgetBytes/6+1)
	if got, want := parseEditableColoredWith(big, FormatRaw, 0, counting), ParseEditableColored(big, FormatRaw, 0); !reflect.DeepEqual(got.Segs, want.Segs) {
		t.Error("over-budget raw buffer is not monochrome")
	}
	if calls != 1 {
		t.Errorf("colorizer ran over the live-color budget (calls = %d)", calls)
	}
}

// TestEditPoolColorizerEqualsFresh: a pooled reproject with SetColorizer equals the free
// parse across a run of edits, and SetColorizer(nil) returns the pool to monochrome raw.
func TestEditPoolColorizerEqualsFresh(t *testing.T) {
	pool := NewEditPool()
	pool.SetColorizer(testINI)
	rng := rand.New(rand.NewSource(3))
	buf := []byte("[Interface]\nPrivateKey = abc\n# c\n[Peer]\nAllowedIPs = 0.0.0.0/0\n")
	alphabet := []byte("[]=#; \tab\n")
	for step := 0; step < 200; step++ {
		at := rng.Intn(len(buf) + 1)
		if rng.Intn(3) == 0 && len(buf) > 0 && at < len(buf) {
			buf = append(buf[:at], buf[at+1:]...)
		} else {
			buf = append(buf[:at], append([]byte{alphabet[rng.Intn(len(alphabet))]}, buf[at:]...)...)
		}
		snap := append(pool.Snapshot()[:0], buf...)
		got := pool.Reproject(snap, FormatRaw, 0)
		want := parseEditableColoredWith(cloneBytes(buf), FormatRaw, 0, testINI)
		if !assertDocsIdentical(t, FormatRaw, step, buf, got, want) {
			return
		}
	}
	pool.SetColorizer(nil)
	if got, want := pool.Reproject(append(pool.Snapshot()[:0], buf...), FormatRaw, 0), ParseEditableColored(cloneBytes(buf), FormatRaw, 0); !reflect.DeepEqual(got.Segs, want.Segs) {
		t.Error("SetColorizer(nil) did not restore monochrome raw")
	}
}

// TestEditPoolColorizerSteadyStateAllocs: the pool hands the host its previous span slice
// as dst, so an appending colorizer adds no per-keystroke allocation to the pooled reproject.
func TestEditPoolColorizerSteadyStateAllocs(t *testing.T) {
	src := bytes.Repeat([]byte("[Peer]\nEndpoint = host:51820\n# note\n"), 2000)
	pool := NewEditPool()
	pool.SetColorizer(testINI)
	pool.Reproject(src, FormatRaw, 0)
	pool.Reproject(src, FormatRaw, 0)
	if n := testing.AllocsPerRun(20, func() { _ = pool.Reproject(src, FormatRaw, 0) }); n > 8 {
		t.Errorf("steady-state colorized raw reproject allocated %.0f times, want <= 8", n)
	}
}
