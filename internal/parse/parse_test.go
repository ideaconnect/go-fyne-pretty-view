package parse

import (
	"strings"
	"testing"
)

func TestDumpXML(t *testing.T) {
	d := loadDoc(t, "catalog.xml", FormatAuto)
	if d.Format != FormatXML {
		t.Errorf("auto-detect = %v, want xml", d.Format)
	}
	t.Logf("format=%v nodes=%d lines=%d\n%s", d.Format, len(d.Nodes), len(d.Lines),
		firstLines(renderDoc(d), 24))
}

func TestDumpHTML(t *testing.T) {
	d := loadDoc(t, "page.html", FormatAuto)
	if d.Format != FormatHTML {
		t.Errorf("auto-detect = %v, want html", d.Format)
	}
	t.Logf("format=%v nodes=%d lines=%d\n%s", d.Format, len(d.Nodes), len(d.Lines),
		firstLines(renderDoc(d), 24))
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
		lines = append(lines, "…")
	}
	return strings.Join(lines, "\n")
}

func TestAutoDetect(t *testing.T) {
	cases := []struct {
		name string
		want Format
	}{
		{"small.json", FormatJSON},
		{"openapi.json", FormatJSON},
		{"catalog.xml", FormatXML},
		{"page.html", FormatHTML},
	}
	for _, c := range cases {
		d := loadDoc(t, c.name, FormatAuto)
		if d.Format != c.want {
			t.Errorf("%s: detected %v, want %v", c.name, d.Format, c.want)
		}
	}
}

func TestRawFallback(t *testing.T) {
	d := Parse([]byte("just some\nplain text\nlines"), FormatAuto, 0)
	if d.Format != FormatRaw {
		t.Errorf("format = %v, want raw", d.Format)
	}
	if got := int(d.TotalVisibleRows()); got != 3 {
		t.Errorf("raw lines = %d, want 3", got)
	}
}

func TestMalformedJSONFallsBackToRaw(t *testing.T) {
	d := Parse([]byte(`{"broken": `), FormatJSON, 0)
	if d.Format != FormatRaw {
		t.Errorf("malformed JSON should fall back to raw, got %v", d.Format)
	}
}
