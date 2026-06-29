package prettyview

import (
	"strings"
	"testing"
)

// TestReformatXMLRoundTrip and TestReformatHTMLRoundTrip are the editor round-trip PARITY
// tests for #70: an editable XML/HTML buffer reformats to the pretty form without losing
// element / attribute / text content, and a single Undo restores the original bytes exactly
// (Reformat is one undo unit — CODE_BIBLE rule 7). They mirror TestReformatJSONCPreservesComments
// so the lossless-edit guarantee is no longer JSON/JSONC-only.

func TestReformatXMLRoundTrip(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	const src = `<root><item id="1">alpha</item><item id="2">béta</item><empty/></root>`
	pv.SetData([]byte(src), FormatXML)
	pv.Reformat()

	got := string(pv.Source())
	if got == src || !strings.Contains(got, "\n  ") {
		t.Fatalf("XML Reformat should rewrite to an indented form:\n%s", got)
	}
	for _, frag := range []string{`id="1"`, "alpha", `id="2"`, "béta", "<empty/>", "</root>"} {
		if !strings.Contains(got, frag) {
			t.Errorf("XML Reformat dropped %q:\n%s", frag, got)
		}
	}
	if pv.Format() != FormatXML {
		t.Errorf("Format() = %v, want FormatXML", pv.Format())
	}
	pv.Undo()
	if u := string(pv.Source()); u != src {
		t.Errorf("Undo after XML Reformat must restore the bytes:\n got  %q\n want %q", u, src)
	}
}

func TestReformatHTMLRoundTrip(t *testing.T) {
	pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
	defer win.Close()

	const src = `<div class="x"><p>hi &amp; bye</p><br><span>z</span></div>`
	pv.SetData([]byte(src), FormatHTML)
	pv.Reformat()

	got := string(pv.Source())
	if got == src || !strings.Contains(got, "\n  ") {
		t.Fatalf("HTML Reformat should rewrite to an indented form:\n%s", got)
	}
	// Structure + text survive AND the entity round-trips: serialization re-encodes the
	// decoded "&" back to "&amp;", so a reformat-then-save stays valid HTML (issue #81). A
	// bare "& bye" would be the old lossy behavior.
	for _, frag := range []string{`class="x"`, "<p>", "hi &amp; bye", "<br>", "<span>", "z"} {
		if !strings.Contains(got, frag) {
			t.Errorf("HTML Reformat dropped %q:\n%s", frag, got)
		}
	}
	if strings.Contains(got, "hi & bye") {
		t.Errorf("HTML Reformat decoded the entity (bare '&'), should stay &amp;:\n%s", got)
	}
	if pv.Format() != FormatHTML {
		t.Errorf("Format() = %v, want FormatHTML", pv.Format())
	}
	pv.Undo()
	if u := string(pv.Source()); u != src {
		t.Errorf("Undo after HTML Reformat must restore the original bytes (incl. &amp;):\n got  %q\n want %q", u, src)
	}
}

// TestReformatPreservesRawText is the issue #85 regression: HTML <script>/<style> bodies and XML
// CDATA are raw text whose bytes are NOT entity-decoded, so a Reformat must emit them verbatim —
// never whitespace-collapsed or entity-escaped (escaping a script's '<' to "&lt;" is invalid JS).
// Ordinary text content is still escaped (the #81 behavior), so the two paths must not bleed.
func TestReformatPreservesRawText(t *testing.T) {
	cases := []struct {
		name, src string
		format    Format
		want      []string // must appear verbatim after Reformat
		absent    []string // the corrupted/escaped forms that must NOT appear
	}{
		{
			name:   "script body verbatim",
			src:    `<div><script>if (a < b && c > d) { f(); }</script></div>`,
			format: FormatHTML,
			want:   []string{"if (a < b && c > d) { f(); }"},
			absent: []string{"&lt;", "&gt;", "&amp;amp;"},
		},
		{
			name:   "multiline script keeps newlines and indentation",
			src:    "<script>\n  // first\n  run();\n</script>",
			format: FormatHTML,
			// A collapse would join the // line comment with run() and comment it out; the
			// newline between them must survive.
			want:   []string{"// first\n", "run();"},
			absent: []string{"// first run();", "&lt;"},
		},
		{
			name:   "style body verbatim",
			src:    `<style>a > b { color: #fff; }</style>`,
			format: FormatHTML,
			want:   []string{"a > b { color: #fff; }"},
			absent: []string{"&gt;"},
		},
		{
			name:   "xml cdata verbatim",
			src:    `<r><v><![CDATA[a < b & c]]></v></r>`,
			format: FormatXML,
			want:   []string{"<![CDATA[a < b & c]]>"},
			absent: []string{"&lt;", "&amp;"},
		},
		{
			name:   "ordinary html text is still escaped",
			src:    `<p>tom &amp; jerry</p>`,
			format: FormatHTML,
			want:   []string{"tom &amp; jerry"},
			absent: []string{"tom & jerry"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
			defer win.Close()
			pv.SetData([]byte(c.src), c.format)
			pv.Reformat()
			got := string(pv.Source())
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("Reformat dropped/altered raw text %q:\n%s", w, got)
				}
			}
			for _, a := range c.absent {
				if strings.Contains(got, a) {
					t.Errorf("Reformat corrupted raw text with %q:\n%s", a, got)
				}
			}
		})
	}
}

// TestReformatIdempotentRawText is the issue #110 regression: repeated Reformat of HTML with a
// non-empty <script>/<style> body must converge — the second pass byte-identical to the first —
// rather than appending a blank line to the raw-text body on every pass (the old behavior grew
// the buffer ~4 bytes per Reformat, reachable via AutoFormatOnPause/OnBlur). The stable controls
// (CDATA, JSON) guard against the trim over-reaching into formats that were already fixed points.
func TestReformatIdempotentRawText(t *testing.T) {
	cases := []struct {
		name, src string
		format    Format
	}{
		{"html script", `<div><script>x();</script></div>`, FormatHTML},
		{"html style", `<div><style>a{color:red}</style></div>`, FormatHTML},
		{"html multiline script", "<section><script>\n  // c\n  run();\n</script></section>", FormatHTML},
		{"html plain control", `<div><p>hello</p></div>`, FormatHTML},
		{"xml cdata control", `<root><data><![CDATA[a < b & c]]></data></root>`, FormatXML},
		{"json control", `{"a":1,"b":[1,2,3]}`, FormatJSON},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
			defer win.Close()
			pv.SetData([]byte(c.src), c.format)

			pv.Reformat()
			first := string(pv.Source())
			for i := 0; i < 4; i++ {
				pv.Reformat()
				got := string(pv.Source())
				if got != first {
					t.Fatalf("Reformat is not idempotent (pass %d diverged from the first):\n first  (%d bytes) %q\n pass %d (%d bytes) %q",
						i+2, len(first), first, i+2, len(got), got)
				}
			}
			// The fix only normalizes the raw-text body's surrounding blank lines; the meaningful
			// content of every case must survive the first reformat.
			for _, frag := range map[string][]string{
				"html script":           {"x();"},
				"html style":            {"a{color:red}"},
				"html multiline script": {"// c", "run();"},
				"html plain control":    {"hello"},
				"xml cdata control":     {"<![CDATA[a < b & c]]>"},
				"json control":          {`"a"`, `"b"`},
			}[c.name] {
				if !strings.Contains(first, frag) {
					t.Errorf("Reformat dropped %q:\n%s", frag, first)
				}
			}
		})
	}
}

// TestReformatMarkupReencodesEntities is the issue #81 regression: an XML/HTML Reformat must
// re-encode the reserved characters its parser decoded (& and <, plus a " inside an attribute
// value) so the rewritten buffer is valid markup that round-trips, rather than emitting a bare
// "&" / "<" that is invalid when re-served. A second Reformat must be idempotent on the entity
// (no double-escape to &amp;amp;).
func TestReformatMarkupReencodesEntities(t *testing.T) {
	cases := []struct {
		name, src string
		format    Format
		want      []string // fragments that must appear after Reformat
		absent    []string // fragments that must NOT appear (the lossy decoded form)
	}{
		{
			name:   "html text ampersand and lt",
			src:    `<p>a &amp; b &lt; c</p>`,
			format: FormatHTML,
			want:   []string{"a &amp; b &lt; c"},
			absent: []string{"a & b", "b < c"},
		},
		{
			name:   "html attribute entities",
			src:    `<a href="x?a=1&amp;b=2" title="3 &lt; 4">go</a>`,
			format: FormatHTML,
			want:   []string{`href="x?a=1&amp;b=2"`, `title="3 &lt; 4"`},
			absent: []string{`a=1&b=2`, `3 < 4`},
		},
		{
			name:   "xml text ampersand",
			src:    `<r><v>tom &amp; jerry</v></r>`,
			format: FormatXML,
			want:   []string{"tom &amp; jerry"},
			absent: []string{"tom & jerry"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pv, win := newEditPV(t, InputConfig{AutoFormat: AutoFormatOff})
			defer win.Close()
			pv.SetData([]byte(c.src), c.format)
			pv.Reformat()
			got := string(pv.Source())
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("Reformat dropped %q:\n%s", w, got)
				}
			}
			for _, a := range c.absent {
				if strings.Contains(got, a) {
					t.Errorf("Reformat emitted the lossy/invalid form %q:\n%s", a, got)
				}
			}
			// Idempotent: reformatting the already-escaped buffer must not double-escape.
			pv.Reformat()
			if again := string(pv.Source()); strings.Contains(again, "&amp;amp;") || again != got {
				t.Errorf("second Reformat was not idempotent:\n first  %q\n second %q", got, again)
			}
		})
	}
}
