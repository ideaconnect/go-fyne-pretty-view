# Codebase structure

A map of `go-fyne-pretty-view` for anyone (human or agent) navigating the code.
The project is split into packages by layer. Three small leaf/logic packages live
under `internal/` (`model`, `parse`, `geometry`); the **view** (the importable
widget + public API) is the repo-root `prettyview` package. A demo binary lives
under `cmd/`.

## Layering

```
  prettyview/  (repo root, package prettyview) ── the view + public API
      │ imports
      ├──► internal/parse     (package parse)    ── bytes → *model.Document
      ├──► internal/geometry  (package geometry) ── Metrics, HitTest, CellOrigin
      └──► internal/model     (package model)    ── the document model
                  ▲ ▲
   parse ─────────┘ │   geometry ────────────────┘   (both depend on model only)
```

Dependency direction is strictly one-way: **`model` is the base**; `parse` and
`geometry` each depend only on `model`; the view depends on all three. `model`
knows nothing about parsing, geometry, or Fyne; `parse` drives `model.Builder`;
`geometry` is pure coordinate math; the view reads the model and renders it. Input
(mouse/keyboard) mutates only fold state and selection/search state, never the
parsed arenas. See [docs/DESIGN.md](docs/DESIGN.md) for the full rationale.

The view is intentionally a **single** package: its renderer, rows, selection,
search, highlight, and input handlers are mutually recursive around the
`PrettyView` struct, so splitting them into sub-packages would create import
cycles and force exporting internals. Only genuine leaves (`model`, `parse`,
`geometry`) are separate packages.

The model is exposed to the view through a small public surface
(`internal/model/api.go`): the fold index itself stays unexported, and the view
drives folding/projection through delegating `Document` methods
(`TotalVisibleRows`, `LineAtRow`, `Fold`, `RevealLine`, …). The flat arenas
(`Src`, `Aux`, `Nodes`, `Lines`, `Segs`) are exported and read directly by the view
for rendering/selection/search; they are **read-only after parse** — only the fold
index and the selection/search state mutate, always on the Fyne goroutine.

## `internal/model` — the document (the source of truth)
| File | Responsibility |
|---|---|
| `model.go` | Struct-of-arrays `Document`: `Node` (32 B), `Line` (24 B), `Segment` (12 B), `Kind`, `ColorRole`; display-text helpers. |
| `foldindex.go` | The visible-line projection: `bitset`, `fenwick`, `foldIndex` (fold/unfold, reveal, expand/collapse-all, row↔line lookup). Unexported. |
| `api.go` | The public `Document` surface the view uses: projection/fold delegators (`TotalVisibleRows`, `Fold`, `RevealLine`, …), `VisibleLine`, `AssembleLine`. |
| `builder.go` | `Builder` — the arena-construction API parsers call (`Open`/`Leaf`/`Close`/`AppendComma`); `Finish` (collapsed renderings, extent, fold index). |
| `format.go` | `Format` type + constants (lives here because `Document` records its format; re-exported by the view). |
| `summary.go` | Collapsed-state summary text (`{ N items }`, `<tag> N children`). |

## `internal/parse` — the parsers (one `Parser` per format)
| File | Responsibility |
|---|---|
| `parse.go` | `Parser` interface, registry, `AutoDetect`, and the entry point `Parse(src, format, depth) *model.Document` (with raw fallback). |
| `parse_json.go` | Hand-written, zero-copy JSON/JSONC scanner. |
| `parse_xml.go` | XML via `encoding/xml`. |
| `parse_html.go` | HTML via the tolerant `golang.org/x/net/html` tokenizer. |
| `parse_raw.go` | Line-splitting fallback for plain text / malformed input. |

## `internal/geometry` — the coordinate math (a leaf)
| File | Responsibility |
|---|---|
| `geometry.go` | `Metrics` (the **one coordinate convention**: `ColX`/`RowY`/`TextOriginX`/…), `NewMetrics`, and `HitTest`/`CellOrigin` over `*model.Document`. Pure math; no Fyne, no view state — a single testable place for pixel↔model mapping. |

## Root `prettyview` — the view + public API
| File | Responsibility |
|---|---|
| `prettyview.go` | The `PrettyView` widget: constructors, `SetData`/`SetText`/`Reparse`, fold/selection/search public methods, host-hook setters. |
| `format.go` | Re-exports `Format` (alias of `model.Format`) + constants, and `WrapMode`, so the public API stays `prettyview.FormatJSON`. |
| `options.go` | `config`, functional `Option`s (`WithFormat`, `WithWrap`, …), `SearchConfig`. |
| `theme.go` | `SyntaxColors`, the default dark/light palettes, `buildPalette`, theme-color helpers. |
| `controls.go` | **Optional** ready-made controls: `NewToolbar` (+ `ToolbarConfig`), `NewSearchBar`, `NewFormatSelect`, `NewFoldButtons`, `ShowOpenDialog`. |
| `renderer.go` | `prettyViewRenderer`: manual `container.Scroll` virtualization, `reflow`, `contentLayout`, `contentSize`, metric/palette recompute. |
| `row.go` | `rowWidget` + `rowRenderer`: per-row colored text (horizontally culled), indent guides, fold triangle. |
| `widget_input.go` | Input-interface assertions, `Tapped` (fold toggle), `Cursor`, coordinate conversion. |
| `selection.go` | `modelPos` + the `hitTest` wrapper over `geometry`; char-level selection state + mouse/drag/focus/shortcut handlers, copy, select-all. |
| `selection_words.go` | Word / line bounds for double- and triple-click. |
| `search.go` | `SearchQuery`/`Match`, the (byte-scan, debounced) scan, reveal-into-folds, navigation, status. |
| `highlight.go` | Pooled selection and search-match rectangles (visible-window only). |

## Demo & assets
| Path | Responsibility |
|---|---|
| `cmd/prettyview-demo/main.go` | Standalone viewer: file/format pickers, expand/collapse, search box. |
| `testdata/` | Fixtures: `small.json`, `openapi.json` (~478 KB), `big.json` (~7.5 MB stress), `catalog.xml`, `page.html`. |
| `docs/DESIGN.md`, `docs/PERFORMANCE.md` | Authoritative architecture + adversarial risk analysis; the performance review. |

## Tests (co-located with the package they test)

- `internal/model/`: `sizes_test` (arena struct sizes; empty document).
- `internal/parse/`: `parse_test` (format detection, raw fallback), `model_test`
  (parser → node/segment/summary output, zero-copy), `foldindex_test` (projection
  round-trip, fold/unfold, expand/collapse-all) — all via the exported model API.
- `internal/geometry/`: `geometry_test` (metric round-trips + the **golden
  hit-test** round-trip — the single most important correctness guard).
- root `prettyview`: `selection_test`, `search_test`, `renderer_test`
  (virtualization bound), `memory_test` (heap ceiling + long-line culling),
  `perf_test` (single-build-per-reflow, incremental reveal), `fold_tap_test`,
  `controls_test`, `theme_test`, `bench_test`, `screenshot_test`
  (software-rendered PNGs, gated by `PV_SHOTS=1`), and `testhelpers_test`
  (shared helpers built on the exported model API).

## Mental model in one paragraph

The document is a flat array of **lines**; a foldable container owns a head line
and a (non-adjacent) close line. A Fenwick tree projects lines to **visible
rows** in O(log n) and supports folding without O(n) rebuilds. The renderer maps
the scroll offset to a visible row range and recycles ~viewport-many `rowWidget`s
to draw only those rows. Selection, search, and copy are computed from `(line,
col)` positions against the model — never by reading widgets — which is why a
multi-megabyte document costs a near-constant number of canvas objects.
