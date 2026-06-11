package prettyview

import "github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"

// Format selects (or, with FormatAuto, detects) the input grammar. It is an alias
// of the model package's type so the document model and the public API share one
// type while keeping the model internal.
type Format = model.Format

const (
	FormatAuto  = model.FormatAuto  // run AutoDetect heuristics
	FormatRaw   = model.FormatRaw   // plain text, split into physical lines
	FormatJSON  = model.FormatJSON  // strict JSON
	FormatJSONC = model.FormatJSONC // JSON with // and /* */ comments
	FormatXML   = model.FormatXML   // XML
	FormatHTML  = model.FormatHTML  // HTML (tolerant)
)

// WrapMode controls long-line handling. WrapNone lets long lines overflow and be
// reached by horizontal scrolling (matching Bruno); WrapWord soft-wraps them to the
// viewport width (breaking at word boundaries, with a char-break fallback for an
// unbreakable run). Wrapping is presentational only — selection, search, and copy
// still operate on whole logical lines.
type WrapMode uint8

const (
	WrapNone WrapMode = iota // long lines overflow; horizontal scroll (default)
	WrapWord                 // soft-wrap to the viewport width
)
