package parse

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

type ecSeg struct {
	text string
	role model.ColorRole
}

func editColorSegs(d *model.Document) []ecSeg {
	var out []ecSeg
	for li := 0; li < len(d.Lines); li++ {
		for _, s := range d.LineSegs(int32(li)) {
			out = append(out, ecSeg{string(d.SegBytes(s)), s.Role})
		}
	}
	return out
}

func editColorRoleOf(segs []ecSeg, text string) (model.ColorRole, bool) {
	for _, s := range segs {
		if s.text == text {
			return s.role, true
		}
	}
	return 0, false
}

// roleAtOffset returns the color role covering source byte off (adjacent same-role tokens
// merge into one seg, so role-by-offset is more robust than role-by-exact-text).
func roleAtOffset(d *model.Document, off int) (model.ColorRole, bool) {
	for li := 0; li < len(d.Lines); li++ {
		for _, s := range d.LineSegs(int32(li)) {
			if s.Buf == model.BufSrc && int(s.Start) <= off && off < int(s.End) {
				return s.Role, true
			}
		}
	}
	return 0, false
}

func TestEditColorJSONRoles(t *testing.T) {
	d := ParseEditableColored([]byte(`{"a":1,"b":true,"c":null,"d":"s"}`), FormatJSON, 0)
	segs := editColorSegs(d)
	want := map[string]model.ColorRole{
		`"a"`:  model.RoleKey,
		`1`:    model.RoleNumber,
		`"b"`:  model.RoleKey,
		`true`: model.RoleBool,
		`"c"`:  model.RoleKey,
		`null`: model.RoleNull,
		`"d"`:  model.RoleKey,
		`"s"`:  model.RoleString,
		`{`:    model.RolePunct,
		`:`:    model.RolePunct,
		`,`:    model.RolePunct,
	}
	for text, role := range want {
		got, ok := editColorRoleOf(segs, text)
		if !ok {
			t.Errorf("no seg %q in %v", text, segs)
			continue
		}
		if got != role {
			t.Errorf("seg %q role = %v, want %v", text, got, role)
		}
	}
}

// TestEditColorJSONArrayVsObjectStrings pins the frame-stack contract: a string inside an
// array (or an object VALUE position) is RoleString, only an object KEY is RoleKey.
func TestEditColorJSONArrayVsObjectStrings(t *testing.T) {
	d := ParseEditableColored([]byte(`{"key":["a","b"],"k2":"v"}`), FormatJSON, 0)
	segs := editColorSegs(d)
	want := map[string]model.ColorRole{
		`"key"`: model.RoleKey,    // object key
		`"k2"`:  model.RoleKey,    // object key
		`"a"`:   model.RoleString, // array element
		`"b"`:   model.RoleString, // array element
		`"v"`:   model.RoleString, // object value
	}
	for text, role := range want {
		if got, ok := editColorRoleOf(segs, text); !ok || got != role {
			t.Errorf("seg %q role = %v (ok=%v), want %v", text, got, ok, role)
		}
	}
}

func TestEditColorXMLRoles(t *testing.T) {
	d := ParseEditableColored([]byte(`<a x="1">t</a>`), FormatXML, 0)
	segs := editColorSegs(d)
	want := map[string]model.ColorRole{
		`a`:   model.RoleTag,
		`x`:   model.RoleAttr,
		`"1"`: model.RoleString,
		`t`:   model.RoleString,
		`<`:   model.RolePunct,
		`>`:   model.RolePunct,
	}
	for text, role := range want {
		got, ok := editColorRoleOf(segs, text)
		if !ok {
			t.Errorf("no xml seg %q in %v", text, segs)
			continue
		}
		if got != role {
			t.Errorf("xml seg %q role = %v, want %v", text, got, role)
		}
	}
}

func TestEditColorJSONCComment(t *testing.T) {
	d := ParseEditableColored([]byte("{\n  \"a\": 1 // note\n}"), FormatJSONC, 0)
	if r, ok := editColorRoleOf(editColorSegs(d), `// note`); !ok || r != model.RoleComment {
		t.Errorf("jsonc comment role = %v (ok=%v), want RoleComment", r, ok)
	}
}

// TestEditColorRuneCountMatchesBuffer is the caret-exactness invariant: every display
// line has exactly as many runes as its buffer line (each grid-hostile byte becomes one
// placeholder rune), across formats — so the caret column equals the buffer rune index.
func TestEditColorRuneCountMatchesBuffer(t *testing.T) {
	srcs := []string{`{"a":1}`, "{\"k\":\"a\tb\"}", "x\x01y", "héllo\nwörld", "", "a\n", "<x/>"}
	for _, src := range srcs {
		for _, f := range []Format{FormatJSON, FormatXML, FormatRaw} {
			d := ParseEditableColored([]byte(src), f, 0)
			lines := strings.Split(src, "\n")
			if d.TotalLines() != len(lines) {
				t.Errorf("src %q fmt %v: TotalLines %d, want %d", src, f, d.TotalLines(), len(lines))
				continue
			}
			for i, ln := range lines {
				if got, want := d.LineRuneLen(int32(i)), utf8.RuneCount([]byte(ln)); got != want {
					t.Errorf("src %q fmt %v line %d: display runes %d, want buffer runes %d", src, f, i, got, want)
				}
			}
		}
	}
}

func TestEditColorNoRawControl(t *testing.T) {
	d := ParseEditableColored([]byte("a\tb\x01c\x7f"), FormatJSON, 0)
	for li := 0; li < d.TotalLines(); li++ {
		s := d.LineString(int32(li))
		for i := 0; i < len(s); i++ {
			if s[i] < 0x20 || s[i] == 0x7f {
				t.Errorf("display line %d has a raw control byte: %q", li, s)
			}
		}
	}
}

func TestEditColorTolerant(t *testing.T) {
	// Partial / invalid input must never panic and still colors the recognizable parts.
	for _, src := range []string{`{"a":`, `{"a"`, `[1,2`, `{`, `"unterminated`, `<a x=`, `</`, ""} {
		_ = ParseEditableColored([]byte(src), FormatJSON, 0)
		_ = ParseEditableColored([]byte(src), FormatXML, 0)
		_ = ParseEditableColored([]byte(src), FormatJSONC, 0)
	}
	// A partial object still marks its key.
	if r, ok := editColorRoleOf(editColorSegs(ParseEditableColored([]byte(`{"a":`), FormatJSON, 0)), `"a"`); !ok || r != model.RoleKey {
		t.Errorf("partial object key role = %v (ok=%v), want RoleKey", r, ok)
	}
}

func TestEditColorRawIsPlain(t *testing.T) {
	d := ParseEditableColored([]byte("just plain text, not structured"), FormatRaw, 0)
	for _, s := range editColorSegs(d) {
		if s.role != model.RolePlain {
			t.Errorf("raw seg %q has role %v, want RolePlain", s.text, s.role)
		}
	}
}

func TestEditColorXMLConstructs(t *testing.T) {
	src := `<?xml version="1.0"?><!DOCTYPE html><!-- c --><a b='x' c>t</a><br/>`
	d := ParseEditableColored([]byte(src), FormatXML, 0)
	comments := []string{"xml version", "DOCTYPE", "<!-- c"}
	for _, sub := range comments {
		if r, ok := roleAtOffset(d, strings.Index(src, sub)); !ok || r != model.RoleComment {
			t.Errorf("construct %q role = %v (ok=%v), want RoleComment", sub, r, ok)
		}
	}
	if r, ok := roleAtOffset(d, strings.Index(src, `'x'`)); !ok || r != model.RoleString {
		t.Errorf("single-quoted attr value role = %v (ok=%v), want RoleString", r, ok)
	}
	if r, ok := roleAtOffset(d, strings.Index(src, " b=")+1); !ok || r != model.RoleAttr {
		t.Errorf("attr name role = %v (ok=%v), want RoleAttr", r, ok)
	}
	// A bare attribute (no value), then a self-closing slash, must not panic and stay 1:1.
	if d.LineRuneLen(0) != utf8.RuneCount([]byte(src)) {
		t.Errorf("xml constructs line rune mismatch: %d vs %d", d.LineRuneLen(0), utf8.RuneCount([]byte(src)))
	}
}

func TestEditColorJSONCBlockComment(t *testing.T) {
	src := "[1, /* block */ 2, -3.5e2]"
	d := ParseEditableColored([]byte(src), FormatJSONC, 0)
	if r, ok := roleAtOffset(d, strings.Index(src, "block")); !ok || r != model.RoleComment {
		t.Errorf("block comment role = %v (ok=%v), want RoleComment", r, ok)
	}
	if r, ok := roleAtOffset(d, strings.Index(src, "-3.5e2")); !ok || r != model.RoleNumber {
		t.Errorf("negative float role = %v (ok=%v), want RoleNumber", r, ok)
	}
	// An unterminated block comment runs to EOF without panicking.
	_ = ParseEditableColored([]byte("[1] /* unterminated"), FormatJSONC, 0)
}

func TestEditColorJSONStringEscapes(t *testing.T) {
	for _, src := range []string{`{"a":"x\"y"}`, "{\"a\":\"unterm\nnext\":1}", `["sea\\"]`} {
		d := ParseEditableColored([]byte(src), FormatJSON, 0)
		for i, ln := range strings.Split(src, "\n") {
			if d.LineRuneLen(int32(i)) != utf8.RuneCount([]byte(ln)) {
				t.Errorf("src %q line %d rune mismatch", src, i)
			}
		}
	}
	// An escaped quote keeps the value one string token.
	d := ParseEditableColored([]byte(`{"a":"x\"y"}`), FormatJSON, 0)
	if r, ok := roleAtOffset(d, strings.Index(`{"a":"x\"y"}`, "x")); !ok || r != model.RoleString {
		t.Errorf("escaped-quote string role = %v (ok=%v), want RoleString", r, ok)
	}
}

// TestEditColorLongLineMonochrome: a pathologically long single line falls back to the
// monochrome segmentation (bounded segment count), still preserving the rune invariant.
func TestEditColorLongLineMonochrome(t *testing.T) {
	long := `["` + strings.Repeat("x", maxColorLineBytes+100) + `"]`
	d := ParseEditableColored([]byte(long), FormatJSON, 0)
	if d.TotalLines() != 1 {
		t.Fatalf("expected one line, got %d", d.TotalLines())
	}
	if got, want := d.LineRuneLen(0), utf8.RuneCount([]byte(long)); got != want {
		t.Errorf("long line display runes %d, want buffer runes %d", got, want)
	}
}
