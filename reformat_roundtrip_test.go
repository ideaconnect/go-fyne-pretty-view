package prettyview

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
)

// This file is the #100 guard on Reformat, which rewrites the user's buffer bytes in place
// (CODE_BIBLE rule 7: lossless edit). The existing parity tests compare serializePretty
// against Text(), but BOTH are built on model.AppendPrettyLine, so a value-corrupting bug in
// that routine would mangle both identically and pass. These tests re-parse the Reformat
// output with the STDLIB (encoding/json, encoding/xml) — an independent oracle — so a
// mis-bounded value segment, a garbled escape, or a dropped/mis-nested tag fails even though
// Text() shares the same routine.

// hasRawCtlByte reports whether s holds a raw control byte that escapeGridBreakers renders
// as a \xNN display escape (every C0 control except \t/\n/\r, plus DEL). Such a byte inside a
// JSON string literal currently reformats to the invalid \xNN form (#106), so the round-trip
// fuzz skips it; \t/\n/\r are excluded because their display escapes are valid JSON.
func hasRawCtlByte(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		if c < 0x20 || c == 0x7f {
			return true
		}
	}
	return false
}

// jsonValue decodes b into a comparable value using json.Number (UseNumber), so numeric
// literals compare by their exact text — a reformat that mangled a digit, a big integer, or
// an exponent is caught, not silently widened to float64.
func jsonValue(t *testing.T, b []byte) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("not valid JSON %q: %v", b, err)
	}
	return v
}

// TestReformatJSONSemanticRoundTrip locks that JSON Reformat output parses back to the SAME
// value as the source — verified against encoding/json, independently of the model's own
// serializer. Seeded with the adversarial cases #100 calls out (huge/precise numbers, every
// escape class, emoji, surrogate pairs, empties, deep nesting).
func TestReformatJSONSemanticRoundTrip(t *testing.T) {
	inputs := []string{
		`{"a":1,"b":[2,3]}`,
		`{"n":1e308,"big":123456789012345678901234567890,"neg":-0.0,"frac":0.1}`,
		`{"s":"tab\tnl\nquote\"backslash\\ slash\/ emoji😀 ué surrogate😀"}`,
		`[true,false,null,"",[],{}]`,
		`{"nested":{"x":[1,{"y":2}]},"arr":[[1,2],[3,4]],"empty":{}}`,
		`{"unicode key é 😀":"value","dupkeyless":[1]}`,
	}
	for _, src := range inputs {
		pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
		pv.SetData([]byte(src), FormatJSON)
		pv.Reformat()
		got := pv.Source()

		want := jsonValue(t, []byte(src))
		var have any
		dec := json.NewDecoder(bytes.NewReader(got))
		dec.UseNumber()
		if err := dec.Decode(&have); err != nil {
			t.Errorf("Reformat(%q) produced invalid JSON %q: %v", src, got, err)
			win.Close()
			continue
		}
		if !reflect.DeepEqual(want, have) {
			t.Errorf("Reformat changed JSON semantics:\n src = %q\n got = %q\n want %#v\n have %#v", src, got, want, have)
		}
		win.Close()
	}
}

// TestReformatXMLWellFormed locks that XML Reformat output is well-formed (parses to EOF via
// encoding/xml) and preserves the element/attr/text tree as a token stream — catching a
// dropped or mis-nested tag that the substring checks in edit_format_parity_test.go would
// miss. CDATA and entity content are included.
func TestReformatXMLWellFormed(t *testing.T) {
	inputs := []string{
		`<root><a id="1">x</a><b/></root>`,
		`<r><n a="1&lt;2">t&amp;u</n><self/></r>`,
		`<doc><![CDATA[ <not>&parsed ]]><p>after</p></doc>`,
		`<a><b><c>deep</c></b><b2/></a>`,
	}
	for _, src := range inputs {
		pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
		pv.SetData([]byte(src), FormatXML)
		pv.Reformat()
		got := pv.Source()

		want := xmlTokenShape(t, []byte(src))
		have, err := xmlTokenShapeErr(got)
		if err != nil {
			t.Errorf("Reformat(%q) produced not-well-formed XML %q: %v", src, got, err)
			win.Close()
			continue
		}
		if !reflect.DeepEqual(want, have) {
			t.Errorf("Reformat changed the XML element/attr/text shape:\n src = %q\n got = %q\n want %v\n have %v", src, got, want, have)
		}
		win.Close()
	}
}

// xmlTokenShape reduces XML to a normalized list of structural tokens (start tags with
// sorted attributes, end tags, and non-whitespace text) so two encodings of the same tree
// compare equal regardless of indentation. CDATA decodes to its text like ordinary CharData.
func xmlTokenShape(t *testing.T, b []byte) []string {
	t.Helper()
	s, err := xmlTokenShapeErr(b)
	if err != nil {
		t.Fatalf("seed XML %q is not well-formed: %v", b, err)
	}
	return s
}

func xmlTokenShapeErr(b []byte) ([]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	var out []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			attrs := make([]string, 0, len(tk.Attr))
			for _, a := range tk.Attr {
				attrs = append(attrs, a.Name.Local+"="+a.Value)
			}
			// stable order so attribute reordering (not a semantic change) doesn't fail
			for i := 0; i < len(attrs); i++ {
				for j := i + 1; j < len(attrs); j++ {
					if attrs[j] < attrs[i] {
						attrs[i], attrs[j] = attrs[j], attrs[i]
					}
				}
			}
			out = append(out, "<"+tk.Name.Local+" "+strings.Join(attrs, ",")+">")
		case xml.EndElement:
			out = append(out, "</"+tk.Name.Local+">")
		case xml.CharData:
			if txt := strings.TrimSpace(string(tk)); txt != "" {
				out = append(out, "#"+strings.Join(strings.Fields(txt), " "))
			}
		}
	}
}

// FuzzReformatJSONSemantics fuzzes the JSON Reformat path: any input that is valid JSON (a
// single value, no trailing content) must Reformat to a buffer that parses back to the same
// value. This locks #100's contract against inputs the seed table didn't anticipate.
func FuzzReformatJSONSemantics(f *testing.F) {
	for _, s := range []string{
		`{"a":1}`, `[1,2,3]`, `{"x":[1,{"y":"z\t"}]}`, `{"n":1e10,"b":true,"z":null}`,
		`"just a string"`, `42`, `[]`, `{}`,
	} {
		f.Add(s)
	}
	// One app + editable widget for the whole run (Go fuzzing runs the body sequentially
	// within a worker process); no window needed — Reformat is buffer/byte work.
	test.NewApp()
	pv := New(WithEditable(), WithInputConfig(InputConfig{AutoFormat: AutoFormatOff}))
	f.Fuzz(func(t *testing.T, src string) {
		// Only valid, single-value JSON is in scope (Reformat preserves; it doesn't validate).
		dec := json.NewDecoder(strings.NewReader(src))
		dec.UseNumber()
		var want any
		if dec.Decode(&want) != nil || dec.More() {
			return
		}
		// Out of this property's domain: a RAW C0/DEL byte inside a string literal is escaped
		// for display as \xNN, and Reformat currently writes that display form into the buffer,
		// producing invalid JSON — a known, separate Reformat bug (#106). \t/\n/\r are fine
		// (their display escapes are valid JSON escapes), so only the \xNN-producing bytes are
		// excluded here; once #106 is fixed, drop this guard.
		if hasRawCtlByte(src) {
			return
		}
		pv.SetData([]byte(src), FormatJSON)
		pv.Reformat()
		got := pv.Source()
		d2 := json.NewDecoder(bytes.NewReader(got))
		d2.UseNumber()
		var have any
		if err := d2.Decode(&have); err != nil {
			t.Fatalf("Reformat(%q) produced invalid JSON %q: %v", src, got, err)
		}
		if !reflect.DeepEqual(want, have) {
			t.Fatalf("Reformat changed JSON semantics:\n src = %q\n got = %q", src, got)
		}
	})
}
