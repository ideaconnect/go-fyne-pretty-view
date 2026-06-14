package parse

import (
	"strings"
	"testing"
)

// TestParseJSONCFormatTag covers jsonParser.Format()'s JSONC branch: parsing under
// FormatJSONC must tag the document JSONC (not plain JSON).
func TestParseJSONCFormatTag(t *testing.T) {
	d := Parse([]byte(`{"a":1}`), FormatJSONC, 0)
	if d.Format != FormatJSONC {
		t.Errorf("Parse(_, FormatJSONC).Format = %v, want FormatJSONC", d.Format)
	}
}

// TestParseMalformedObjectsTolerant drives parseContainer's object error paths — a
// non-string key, a missing colon, and truncated/unterminated objects. The tolerant
// parser must produce a document for each without panicking.
func TestParseMalformedObjectsTolerant(t *testing.T) {
	for _, src := range []string{
		`{1:2}`,          // key is not a string -> "expected object key"
		`{"a" 1}`,        // missing ':' after key
		`{"a":1,`,        // trailing comma, truncated
		`{"unterminated`, // unterminated key string
		`{`,              // bare open brace
		`{"a":1 "b":2}`,  // missing comma between members
	} {
		d := Parse([]byte(src), FormatJSON, 0)
		if d == nil || d.TotalLines() == 0 {
			t.Errorf("Parse(%q) produced no renderable document", src)
		}
	}
}

// TestForcedJSONNeverDropsContainerContent is the #94 regression: under a FORCED JSON
// (or JSONC) format, an array element or object member whose first byte cannot begin a
// JSON value/key must NOT silently collapse the container to an empty []/{}. Every byte
// has to stay visible (the project's core contract). Auto-detect routes these JSON5-ish
// inputs to raw, so this only ever bit the forced-format path — exactly what a host
// passing an explicit format for a .json file or known-JSON API response hits.
func TestForcedJSONNeverDropsContainerContent(t *testing.T) {
	cases := []struct {
		src  string
		want []string // every fragment must survive in the rendered document
	}{
		{`[NaN,1]`, []string{"NaN", "1"}},
		{`[Infinity]`, []string{"Infinity"}},
		{`[undefined]`, []string{"undefined"}},
		{`['a','b']`, []string{"'a'", "'b'"}},
		{`[@x,1]`, []string{"@x", "1"}},
		{`[+5,99]`, []string{"+5", "99"}},
		{`[[1,2],[@,9],[3,4]]`, []string{"1", "2", "@", "9", "3", "4"}},
		{`{foo:1,"ok":2}`, []string{"foo", "ok"}},
		{`{'a':1,"b":2}`, []string{"'a'", "b"}},
		{`{"a":NaN,"b":2}`, []string{"a", "NaN", "b"}},
	}
	for _, f := range []Format{FormatJSON, FormatJSONC} {
		for _, c := range cases {
			d := Parse([]byte(c.src), f, 0)
			// A recovered partial document stays structured (never raw) on the forced path.
			if d.Format != f {
				t.Errorf("Parse(%q, %v): Format = %v, want %v (fell back to raw?)", c.src, f, d.Format, f)
			}
			text := renderDoc(d)
			for _, frag := range c.want {
				if !strings.Contains(text, frag) {
					t.Errorf("Parse(%q, %v) dropped %q:\n%s", c.src, f, frag, text)
				}
			}
		}
	}
}

// TestForcedJSONMalformedKeysSurfaced is the object-side half of #94: a non-string key
// (unquoted/single-quoted) and a key with no colon must surface as visible content, not
// vanish. The old TotalLines()!=0 check in TestParseMalformedObjectsTolerant could not
// catch the silent drop (the closing } kept the count at 2).
func TestForcedJSONMalformedKeysSurfaced(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`{1:2}`, "1:2"},
		{`{"a" 1}`, "a"},
		{`{bad:1,"good":2}`, "good"}, // the member after a bad key still renders
	}
	for _, c := range cases {
		text := renderDoc(Parse([]byte(c.src), FormatJSON, 0))
		if !strings.Contains(text, c.want) {
			t.Errorf("Parse(%q) dropped %q:\n%s", c.src, c.want, text)
		}
	}
}
