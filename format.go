package prettyview

import "github.com/ideaconnect/go-fyne-pretty-view/internal/model"

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

// WrapMode controls long-line handling. The default, WrapNone, matches Bruno:
// long lines overflow and are reached by horizontal scrolling.
type WrapMode uint8

const (
	WrapNone WrapMode = iota // long lines overflow; horizontal scroll (default)
	WrapWord                 // soft-wrap to viewport width
)
