# Codebase structure

A map of `go-fyne-pretty-view` for anyone (human or agent) navigating the code.
The whole widget lives in one package, `prettyview` (the repo root), with a demo
binary under `cmd/`. There are no internal sub-packages.

## Layering

```
                    public API  (prettyview.go, options.go, theme.go, parse.go)
                          │
        ┌─────────────────┼──────────────────┐
        │                 │                  │
   parsers           document model       view (Fyne)
 parse_*.go ──drives──►  model.go  ◄──reads──  renderer.go / row.go
   builder.go            foldindex.go          geometry.go
   summary.go            (Document, fold)       widget_input.go
                                                selection*.go / search.go
                                                highlight.go
```

Data flows one way: **parsers build the model; the view reads it.** Input
(mouse/keyboard) mutates only fold state and selection/search state, never the
parsed arenas. See [docs/DESIGN.md](docs/DESIGN.md) for the full rationale.

## Files

### Public surface
| File | Responsibility |
|---|---|
| `prettyview.go` | The `PrettyView` widget: constructors, `SetData`/`SetText`/`Reparse`, fold/selection/search public methods, host-hook setters, struct fields. |
| `options.go` | `config`, functional `Option`s (`WithFormat`, `WithWrap`, …), `SearchConfig`. |
| `theme.go` | `SyntaxColors`, the default dark/light palettes, `buildPalette`, theme-color helpers. |
| `controls.go` | **Optional** ready-made controls bound to a `PrettyView`: `NewToolbar` (+ `ToolbarConfig`), `NewSearchBar`, `NewFormatSelect`, `NewFoldButtons`, `ShowOpenDialog`. Nothing here is required to use the viewer. |

### Document model (the source of truth)
| File | Responsibility |
|---|---|
| `model.go` | Struct-of-arrays `Document`: `Node` (32 B), `Line` (24 B), `Segment` (12 B), `Kind`, `ColorRole`; display-text helpers. |
| `foldindex.go` | The visible-line projection: `bitset`, `fenwick`, `foldIndex` (fold/unfold, reveal, expand/collapse-all, row↔line lookup). |
| `builder.go` | `Builder` — the arena-construction API parsers call (`Open`/`Leaf`/`Close`/`AppendComma`); `finish` (collapsed renderings, extent, fold index). |
| `summary.go` | Collapsed-state summary text (`{ N items }`, `<tag> N children`). |

### Parsers (one `Parser` per format)
| File | Responsibility |
|---|---|
| `parse.go` | `Parser` interface, registry, `AutoDetect`, `parseDocument` (with raw fallback). |
| `parse_json.go` | Hand-written, zero-copy JSON/JSONC scanner. |
| `parse_xml.go` | XML via `encoding/xml`. |
| `parse_html.go` | HTML via the tolerant `golang.org/x/net/html` tokenizer. |
| `parse_raw.go` | Line-splitting fallback for plain text / malformed input. |

### View (Fyne)
| File | Responsibility |
|---|---|
| `renderer.go` | `prettyViewRenderer`: manual `container.Scroll` virtualization, `reflow`, `contentLayout`, `contentSize`, metric/palette recompute. |
| `row.go` | `rowWidget` + `rowRenderer`: per-row colored text (horizontally culled), indent guides, fold triangle. |
| `geometry.go` | `metrics` and the **one coordinate convention**; `hitTest`/`cellOrigin`; `modelPos`. |
| `widget_input.go` | Input-interface assertions, `Tapped` (fold toggle), `Cursor`, coordinate conversion. |
| `selection.go` | Char-level selection state + mouse/drag/focus/shortcut handlers, copy, select-all. |
| `selection_words.go` | Word / line bounds for double- and triple-click. |
| `search.go` | `SearchQuery`/`Match`, the scan, reveal-into-folds, navigation, status. |
| `highlight.go` | Pooled selection and search-match rectangles (visible-window only). |

### Demo & assets
| Path | Responsibility |
|---|---|
| `cmd/prettyview-demo/main.go` | Standalone viewer: file/format pickers, expand/collapse, search box. |
| `testdata/` | Fixtures: `small.json`, `openapi.json` (~478 KB), `big.json` (~7.5 MB stress), `catalog.xml`, `page.html`. |
| `docs/DESIGN.md` | The authoritative architecture + the adversarial risk analysis. |

## Tests (co-located, `*_test.go`)

`model_test`, `foldindex_test`, `parse_test`, `geometry_test` (golden hit-test
round-trip), `selection_test`, `search_test`, `renderer_test` (virtualization
bound), `memory_test` (heap ceiling + long-line culling), `fold_tap_test`,
`theme_test`, `bench_test` (parse/fold/lookup/search), and `screenshot_test`
(software-rendered PNGs, gated by `PV_SHOTS=1`).

## Mental model in one paragraph

The document is a flat array of **lines**; a foldable container owns a head line
and a (non-adjacent) close line. A Fenwick tree projects lines to **visible
rows** in O(log n) and supports folding without O(n) rebuilds. The renderer maps
the scroll offset to a visible row range and recycles ~viewport-many `rowWidget`s
to draw only those rows. Selection, search, and copy are computed from `(line,
col)` positions against the model — never by reading widgets — which is why a
multi-megabyte document costs a near-constant number of canvas objects.
