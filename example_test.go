package prettyview_test

import (
	"bytes"
	"fmt"

	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"
)

// ExampleNew builds a viewer, loads JSON with auto-detection, adds the optional
// toolbar, and shows it.
func ExampleNew() {
	a := app.New()
	w := a.NewWindow("viewer")

	pv := prettyview.New(prettyview.WithWrap(prettyview.WrapWord))
	pv.SetData([]byte(`{"hello":"world","items":[1,2,3]}`), prettyview.FormatAuto)

	bar := prettyview.NewToolbar(pv, prettyview.ToolbarConfig{
		ShowSearch:         true,
		ShowExpandCollapse: true,
		ShowWrap:           true,
	})
	w.SetContent(container.NewBorder(bar, nil, nil, nil, pv))
	w.ShowAndRun()
}

// ExamplePrettyView_Search runs a regular-expression search and reads the live match
// counter. Search is synchronous, so the count is available immediately.
func ExamplePrettyView_Search() {
	test.NewApp() // self-contained: provides the fyne.CurrentApp the widget reads
	pv := prettyview.NewWithData([]byte(`{"a":"x1","b":"x2","c":"yy"}`), prettyview.FormatJSON)

	pv.Search(prettyview.SearchQuery{Text: `x\d`, Mode: prettyview.SearchRegex})
	_, total, _ := pv.SearchStatus() // (active, total, capped)
	pv.SearchNext()                  // advance to the next match (wraps at the end)
	fmt.Println(total)
	// Output: 2
}

// ExamplePrettyView_CopySubtree copies a node's whole subtree to the clipboard by its
// source byte offset. Source offsets are populated for every structured format — JSON,
// JSONC, XML and HTML — so it resolves a node on all of them (only the raw view, which
// has no nodes, returns false). The right-click "Copy subtree" menu item does the same.
func ExamplePrettyView_CopySubtree() {
	test.NewApp() // self-contained: CopySubtree writes to the clipboard via the current app
	pv := prettyview.NewWithData([]byte(`{"user":{"name":"ada","id":1}}`), prettyview.FormatJSON)

	// Byte offset of the '{' opening the "user" object in the source.
	fmt.Println(pv.CopySubtree(8)) // true; the "user" subtree is now on the clipboard
	// Output: true
}

// iniColorizer is a minimal host grammar for INI-style files such as a WireGuard .conf:
// "[Section]" headers, "Key = Value" pairs and "#" / ";" comment lines. It appends its
// spans to dst, so the widget can reuse the slice from one call to the next.
func iniColorizer(dst []prettyview.ColorSpan, src []byte) []prettyview.ColorSpan {
	for start := 0; start < len(src); {
		end := bytes.IndexByte(src[start:], '\n')
		if end < 0 {
			end = len(src)
		} else {
			end += start
		}
		line := src[start:end]
		body := bytes.TrimLeft(line, " \t")
		at := start + len(line) - len(body) // byte offset of the first non-blank
		switch {
		case len(body) == 0:
		case body[0] == '#' || body[0] == ';':
			dst = append(dst, prettyview.ColorSpan{Start: at, End: end, Role: prettyview.RoleComment})
		case body[0] == '[':
			dst = append(dst, prettyview.ColorSpan{Start: at, End: end, Role: prettyview.RoleTag})
		default:
			if eq := bytes.IndexByte(body, '='); eq >= 0 {
				dst = append(dst,
					prettyview.ColorSpan{Start: at, End: at + eq, Role: prettyview.RoleKey},
					prettyview.ColorSpan{Start: at + eq, End: at + eq + 1, Role: prettyview.RolePunct},
					prettyview.ColorSpan{Start: at + eq + 1, End: end, Role: prettyview.RoleString})
			}
		}
		start = end + 1
	}
	return dst
}

// ExampleWithColorizer colors a format the library has no parser for, here an INI-style
// WireGuard config, with a host-supplied grammar. It applies to FormatRaw text in the
// read-only viewer and in the editor alike; the roles resolve through the active Theme.
func ExampleWithColorizer() {
	test.NewApp()
	conf := []byte("[Interface]\n# client\nAddress = 10.0.0.2/32\n")
	pv := prettyview.NewWithData(conf, prettyview.FormatRaw, prettyview.WithColorizer(iniColorizer))
	_ = pv // add it to a window like any other widget

	for _, s := range iniColorizer(nil, conf) {
		fmt.Printf("%d %q\n", s.Role, conf[s.Start:s.End])
	}
	// Output:
	// 7 "[Interface]"
	// 9 "# client"
	// 1 "Address "
	// 6 "="
	// 2 " 10.0.0.2/32"
}
