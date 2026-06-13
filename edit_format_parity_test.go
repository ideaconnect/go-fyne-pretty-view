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
