// Package parse turns raw bytes into a model.Document. It hosts the format
// parsers (JSON/JSONC, XML, HTML, raw) and format auto-detection. It depends on
// internal/model and is depended on by the top-level prettyview package.
package parse

import (
	"bytes"
	"unicode"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// Format and its constants live in the model package (Document records its
// format); aliased here for terse use within the parsers.
type Format = model.Format

const (
	FormatAuto  = model.FormatAuto
	FormatRaw   = model.FormatRaw
	FormatJSON  = model.FormatJSON
	FormatJSONC = model.FormatJSONC
	FormatXML   = model.FormatXML
	FormatHTML  = model.FormatHTML
)

// Parser turns a byte buffer into a Document by driving a model.Builder. A parser
// must be tolerant: on malformed input it emits whatever partial structure it has
// recovered rather than failing outright.
type Parser interface {
	// Format reports the concrete format this parser produces.
	Format() Format
	// Detect returns a 0..100 confidence that src is of this parser's format.
	Detect(src []byte) int
	// Parse consumes src and appends nodes/segments via b.
	Parse(src []byte, b *model.Builder) error
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

// Parse parses src under format (FormatAuto detects). On a structured parse
// failure it falls back to a raw document so content always displays.
func Parse(src []byte, format Format, collapseDepth int) *model.Document {
	if format == FormatAuto {
		format = AutoDetect(src)
	}
	p := parserFor(format)
	if p == nil { // FormatRaw (or anything without a parser)
		return parseRaw(src, collapseDepth)
	}
	b := model.NewBuilder(src, format, collapseDepth)
	if err := p.Parse(src, b); err != nil {
		return parseRaw(src, collapseDepth)
	}
	return b.Finish()
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
