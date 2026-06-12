package parse

import "testing"

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
