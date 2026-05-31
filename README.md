# go-fyne-pretty-view

A memory-efficient, virtualized [Fyne](https://fyne.io) widget for viewing
structured data — **JSON, JSONC, XML, HTML, and raw text** — in the style of
[Bruno](https://www.usebruno.com)'s response viewer.

![JSON](docs/shot-json.png)

## Features

- **Syntax highlighting** for JSON / XML / HTML, with a dark/light palette you can override.
- **Expand / fold** every container, with a collapse summary on folded nodes (`{ 38 items }`, `[ 3 items ]`, `<tag> 5 children`).
- **Copy a whole section** (subtree) to the clipboard.
- **True character-level free-text selection** across rows, with exact-substring copy (`Ctrl/Cmd+C`) and select-all (`Ctrl/Cmd+A`).
- **Search** with plain or regular-expression matching, match navigation, and **auto-reveal into folded nodes**.
- **Auto-detection** of the input format, with a raw-text fallback for anything else (or malformed input).

## Why it stays small

The widget is built around a hard memory bound: **only the rows currently
visible in the viewport ever exist as live canvas objects.** Everything else
lives in a compact, pointer-free, struct-of-arrays model, and selection, search
and copy all operate on that model rather than on widgets.

Measured on the included fixtures:

| Input | Visible rows | Live row widgets | Heap after scrolling the whole file |
|---|---|---|---|
| `big.json` (7.5 MB) | 440,005 | **31** | **~80 MB** |

The parsed model is about **5× the source size** (e.g. a 467 KB JSON → ~2.3 MB),
and a single multi-megabyte line is horizontally culled so no individual text
texture is ever wider than the viewport (without that, Fyne would try to
rasterize a ~1 GB bitmap for the line).

## Install

```sh
go get github.com/ideaconnect/go-fyne-pretty-view
```

Requires Go 1.26+ and the usual Fyne build dependencies (a C compiler and the
OpenGL/X11 headers on Linux).

## Usage

```go
import (
    "fyne.io/fyne/v2/app"
    prettyview "github.com/ideaconnect/go-fyne-pretty-view"
)

func main() {
    a := app.New()
    w := a.NewWindow("viewer")

    pv := prettyview.New()
    pv.SetData(jsonBytes, prettyview.FormatAuto) // or FormatJSON/FormatXML/FormatHTML/FormatRaw

    w.SetContent(pv)
    w.ShowAndRun()
}
```

### Construction options

```go
pv := prettyview.New(
    prettyview.WithFormat(prettyview.FormatJSON),       // skip auto-detect
    prettyview.WithWrap(prettyview.WrapNone),           // long lines scroll horizontally (default)
    prettyview.WithDefaultCollapseDepth(3),             // auto-collapse below depth 3 on load
    prettyview.WithIndentStep(16),                      // pixels per nesting level
    prettyview.WithTabWidth(4),
    prettyview.WithSearchConfig(prettyview.SearchConfig{MaxMatches: 5000}),
)
```

### Key methods

| Method | Purpose |
|---|---|
| `SetData(src, format)` / `SetText(s)` | load content |
| `ExpandAll()` / `CollapseAll()` | fold control |
| `ExpandTo(byteOffset)` | reveal & scroll to a node |
| `SelectAll()` / `ClearSelection()` / `SelectedText()` | selection |
| `CopySelection()` / `CopySubtree(byteOffset)` | clipboard |
| `Search(SearchQuery{...})` / `SearchNext()` / `SearchPrev()` / `SearchStatus()` | search |
| `SetSyntaxColors(variant, SyntaxColors{...})` | theming |
| `SetOnSearchRequested(fn)` | host hook for `Ctrl/Cmd+F` |

## Demo

```sh
go run ./cmd/prettyview-demo               # loads testdata/openapi.json
go run ./cmd/prettyview-demo path/to/file  # or any file
```

The demo provides a file picker, a format selector, expand/collapse buttons, and
a search box with a match counter.

## Design

The full, source-grounded architecture (the virtualization invariant, the
struct-of-arrays model, the Fenwick fold index, the char-level selection math,
and the adversarial risk analysis) lives in [docs/DESIGN.md](docs/DESIGN.md).

## Documentation

| File | For whom / what |
|---|---|
| [README.md](README.md) | This overview: features, install, usage, API. |
| [STRUCTURE.md](STRUCTURE.md) | The codebase map — every file, the layering, the mental model. |
| [WORKFLOWS.md](WORKFLOWS.md) | How to build, run, test, benchmark, and extend (parsers, colors). |
| [docs/DESIGN.md](docs/DESIGN.md) | The authoritative architecture + adversarial risk analysis. |
| [HUMANS.md](HUMANS.md) | Onboarding and contribution guide for people. |
| [AGENTS.md](AGENTS.md) | Brief for AI coding agents: invariants to preserve, conventions. |
| [CLAUDE.md](CLAUDE.md) | Claude Code entry point (points at AGENTS.md). |

## License

A `LICENSE` file has not been added yet. Until one is, all rights are reserved by
the maintainers (`ideaconnect`); open an issue if you need a specific license.
