package model

// Format selects (or, with FormatAuto, detects) the input grammar. It lives in
// the model package because Document records the format it was built from; the
// public prettyview package re-exports it as an alias.
type Format uint8

const (
	FormatAuto  Format = iota // run AutoDetect heuristics
	FormatRaw                 // plain text, split into physical lines
	FormatJSON                // strict JSON
	FormatJSONC               // JSON with // and /* */ comments
	FormatXML                 // XML
	FormatHTML                // HTML (tolerant)
)

// String returns a short human-readable name for the format.
func (f Format) String() string {
	switch f {
	case FormatAuto:
		return "auto"
	case FormatRaw:
		return "raw"
	case FormatJSON:
		return "json"
	case FormatJSONC:
		return "jsonc"
	case FormatXML:
		return "xml"
	case FormatHTML:
		return "html"
	default:
		return "unknown"
	}
}
