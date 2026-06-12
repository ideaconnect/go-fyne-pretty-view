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
	// Structure + text survive. NOTE: the HTML parser decodes entities, so the reformatted
	// text is "hi & bye", not "hi &amp; bye" — an HTML normalization the editor inherits. The
	// raw entity is restored by Undo below; the lossy-reformat-for-entities behavior is
	// flagged as a follow-up, locked here.
	for _, frag := range []string{`class="x"`, "<p>", "hi & bye", "<br>", "<span>", "z"} {
		if !strings.Contains(got, frag) {
			t.Errorf("HTML Reformat dropped %q:\n%s", frag, got)
		}
	}
	if pv.Format() != FormatHTML {
		t.Errorf("Format() = %v, want FormatHTML", pv.Format())
	}
	pv.Undo()
	if u := string(pv.Source()); u != src {
		t.Errorf("Undo after HTML Reformat must restore the original bytes (incl. &amp;):\n got  %q\n want %q", u, src)
	}
}
