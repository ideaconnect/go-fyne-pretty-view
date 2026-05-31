package prettyview

import (
	"bytes"
	"unicode"
)

// Format selects (or, with FormatAuto, detects) the input grammar.
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

// WrapMode controls long-line handling. The default, WrapNone, matches Bruno:
// long lines overflow and are reached by horizontal scrolling.
type WrapMode uint8

const (
	WrapNone WrapMode = iota // long lines overflow; horizontal scroll (default)
	WrapWord                 // soft-wrap to viewport width
)

// Parser turns a byte buffer into a Document by driving a Builder. A parser must
// be tolerant: on malformed input it emits whatever partial structure it has
// recovered rather than failing outright.
type Parser interface {
	// Format reports the concrete format this parser produces.
	Format() Format
	// Detect returns a 0..100 confidence that src is of this parser's format.
	Detect(src []byte) int
	// Parse consumes src and appends nodes/segments via b.
	Parse(src []byte, b *Builder) error
}

// parsers returns the registered parsers in detection-priority order. Raw is the
// universal fallback and is intentionally excluded from auto-detection here; it
// is selected only when nothing else has positive confidence.
func parsers() []Parser {
	return []Parser{
		jsonParser{jsonc: false},
		jsonParser{jsonc: true},
		xmlParser{},
		htmlParser{},
	}
}

// parserFor returns the parser implementing the requested concrete format.
// FormatAuto and FormatRaw return nil (handled by the caller).
func parserFor(f Format) Parser {
	switch f {
	case FormatJSON:
		return jsonParser{jsonc: false}
	case FormatJSONC:
		return jsonParser{jsonc: true}
	case FormatXML:
		return xmlParser{}
	case FormatHTML:
		return htmlParser{}
	default:
		return nil
	}
}

// parseDocument parses src under format (FormatAuto detects). On a structured
// parse failure it falls back to a raw document so content always displays.
func parseDocument(src []byte, format Format, collapseDepth int) *Document {
	if format == FormatAuto {
		format = AutoDetect(src)
	}
	p := parserFor(format)
	if p == nil { // FormatRaw (or anything without a parser)
		return parseRaw(src, collapseDepth)
	}
	b := newBuilder(src, format, collapseDepth)
	if err := p.Parse(src, b); err != nil {
		return parseRaw(src, collapseDepth)
	}
	return b.finish()
}

// AutoDetect picks the most likely format for src. It returns FormatRaw when no
// structured parser is confident.
func AutoDetect(src []byte) Format {
	trimmed := bytes.TrimLeftFunc(src, unicode.IsSpace)
	if len(trimmed) == 0 {
		return FormatRaw
	}
	best := FormatRaw
	bestScore := 0
	for _, p := range parsers() {
		if s := p.Detect(src); s > bestScore {
			bestScore, best = s, p.Format()
		}
	}
	if bestScore == 0 {
		return FormatRaw
	}
	return best
}
