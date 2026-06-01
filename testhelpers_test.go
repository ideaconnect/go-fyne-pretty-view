package prettyview

import (
	"os"
	"testing"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
	"github.com/ideaconnect/go-fyne-pretty-view/internal/parse"
)

// Shared test helpers for the view package, built on the exported model API.

// loadDoc parses a fixture file under testdata/.
func loadDoc(t *testing.T, name string, f Format) *model.Document {
	t.Helper()
	src, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return parse.Parse(src, f, 0)
}

// findFoldHead returns the first foldable node whose head line begins with the
// given key text (e.g. `"info"`), or NoNode.
func findFoldHead(d *model.Document, keyText string) model.NodeID {
	for li := range d.Lines {
		l := &d.Lines[li]
		if l.Fold == model.NoNode {
			continue
		}
		segs := d.LineSegs(int32(li))
		if len(segs) > 0 && string(d.SegBytes(segs[0])) == keyText {
			return l.Fold
		}
	}
	return model.NoNode
}

// visSnapshot captures per-line visibility for comparison across projection ops.
func visSnapshot(d *model.Document) []bool {
	v := make([]bool, len(d.Lines))
	for i := range v {
		v[i] = d.Visible(int32(i))
	}
	return v
}

func firstFoldHeadAtDepth(d *model.Document, depth uint8) model.NodeID {
	for li := range d.Lines {
		l := &d.Lines[li]
		if l.Fold != model.NoNode && l.Depth == depth {
			return l.Fold
		}
	}
	return model.NoNode
}
