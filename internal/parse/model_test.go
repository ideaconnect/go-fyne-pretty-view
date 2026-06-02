package parse

import (
	"os"
	"strings"
	"testing"
	"unsafe"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// renderDoc reproduces the document as the viewer would show it fully expanded:
// indentation by depth, fold triangles, and the line text. Used by tests.
func renderDoc(d *model.Document) string {
	var sb strings.Builder
	total := d.TotalVisibleRows()
	for r := int32(0); r < total; r++ {
		li := d.LineAtRow(r)
		l := &d.Lines[li]
		sb.WriteString(strings.Repeat("  ", int(l.Depth)))
		if l.Fold != model.NoNode {
			if d.Collapsed(l.Fold) {
				sb.WriteString("▶ ")
				sb.WriteString(segsString(d, d.CollapsedSegs(li)))
				sb.WriteByte('\n')
				continue
			}
			sb.WriteString("▼ ")
		}
		sb.WriteString(d.LineString(li))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func segsString(d *model.Document, segs []model.Segment) string {
	var sb strings.Builder
	for _, s := range segs {
		sb.Write(d.SegBytes(s))
	}
	return sb.String()
}

func TestDumpSmall(t *testing.T) {
	src, err := os.ReadFile("../../testdata/small.json")
	if err != nil {
		t.Fatal(err)
	}
	d := Parse(src, FormatAuto, 0)
	t.Logf("format=%v nodes=%d lines=%d segs=%d visible=%d\n%s",
		d.Format, len(d.Nodes), len(d.Lines), len(d.Segs), d.TotalVisibleRows(), renderDoc(d))
}

func TestZeroCopy(t *testing.T) {
	src := []byte(`{"key":"value","n":123,"b":true}`)
	d := Parse(src, FormatJSON, 0)

	// Src must be retained, not copied.
	if &d.Src[0] != &src[0] {
		t.Error("Src was copied; expected zero-copy retention of the input buffer")
	}

	var sawString, sawNumber, sawKey bool
	for _, seg := range d.Segs {
		if seg.Buf != model.BufSrc {
			continue
		}
		b := d.SegBytes(seg)
		// A model.BufSrc segment must alias Src exactly.
		if len(b) > 0 && &b[0] != &d.Src[seg.Start] {
			t.Fatal("model.BufSrc segment does not alias Src")
		}
		switch seg.Role {
		case model.RoleString:
			if string(b) == `"value"` {
				sawString = true
			}
		case model.RoleNumber:
			if string(b) == "123" {
				sawNumber = true
			}
		case model.RoleKey:
			if string(b) == `"key"` {
				sawKey = true
			}
		}
	}
	if !sawKey || !sawString || !sawNumber {
		t.Errorf("missing zero-copy tokens: key=%v string=%v number=%v", sawKey, sawString, sawNumber)
	}
}

func TestSummaries(t *testing.T) {
	d := Parse([]byte(`{"info":{"title":"Tiny","version":"1.0"},"list":[1,2,3],"one":{"x":1}}`), FormatJSON, 0)

	cases := map[string]string{
		`"info"`: `"info": { 2 items },`,
		`"list"`: `"list": [ 3 items ],`,
		`"one"`:  `"one": { 1 item }`, // last member, no trailing comma
	}
	for key, want := range cases {
		id := findFoldHead(d, key)
		if id == model.NoNode {
			t.Errorf("no fold head for %s", key)
			continue
		}
		got := segsString(d, d.CollapsedSegs(d.Nodes[id].HeadLine))
		if got != want {
			t.Errorf("collapsed %s = %q, want %q", key, got, want)
		}
	}
}

func TestEmptyContainersAreLeaves(t *testing.T) {
	d := Parse([]byte(`{"o":{},"a":[],"x":1}`), FormatJSON, 0)
	// "o" and "a" must render inline and not be foldable.
	if id := findFoldHead(d, `"o"`); id != model.NoNode {
		t.Error(`empty object "o" should not be foldable`)
	}
	out := renderDoc(d)
	if !strings.Contains(out, `"o": {}`) || !strings.Contains(out, `"a": []`) {
		t.Errorf("empty containers not rendered inline:\n%s", out)
	}
}

func TestNodeLineSize(t *testing.T) {
	t.Logf("sizeof(model.Node)=%d sizeof(model.Line)=%d sizeof(model.Segment)=%d",
		unsafe.Sizeof(model.Node{}), unsafe.Sizeof(model.Line{}), unsafe.Sizeof(model.Segment{}))
	if unsafe.Sizeof(model.Node{}) > 32 {
		t.Errorf("model.Node grew to %d bytes (want <= 32)", unsafe.Sizeof(model.Node{}))
	}
	if unsafe.Sizeof(model.Line{}) > 24 {
		t.Errorf("model.Line grew to %d bytes (want <= 24)", unsafe.Sizeof(model.Line{}))
	}
}
