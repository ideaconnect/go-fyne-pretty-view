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

// sniffLimit bounds how many bytes format auto-detection inspects, so a large file
// is not fully lower-cased / scanned just to identify its format.
const sniffLimit = 4096

// hasPrefixFold reports whether s begins with prefix, case-insensitively, without
// allocating a lower-cased copy (unlike bytes.ToLower + bytes.HasPrefix).
func hasPrefixFold(s, prefix []byte) bool {
	return len(s) >= len(prefix) && bytes.EqualFold(s[:len(prefix)], prefix)
}

// maxNestDepth bounds how deeply the recursive parsers (JSON containers, XML
// elements) will descend. Real data never approaches this; the cap exists so a
// pathologically deep input (e.g. hundreds of thousands of nested brackets) is
// truncated to a partial document instead of overflowing the goroutine stack,
// which is a fatal, unrecoverable crash. The recovered structure above the cap is
// kept (parsers are tolerant), so the cap degrades gracefully rather than failing.
const maxNestDepth = 10000

// utf8BOM is the UTF-8 byte-order mark (U+FEFF). Some editors and tooling
// (notably on Windows/.NET) prefix it to otherwise-valid UTF-8. Left in place it
// defeats both auto-detection and the structured parsers — the leading 0xEF is not
// whitespace (unicode.IsSpace/skipSpace skip it) and matches no value, so the
// document degrades to raw text. We strip it once, before the model Builder is
// constructed, so the zero-copy Src and every SrcSeg byte range are computed
// against the trimmed buffer (trimming later would desync all segment offsets).
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// stripBOM removes a single leading UTF-8 BOM if present. It is idempotent and
// returns a subslice (no copy).
func stripBOM(src []byte) []byte { return bytes.TrimPrefix(src, utf8BOM) }

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
// failure it falls back to a raw document so content always displays. The optional
// tabWidth (default 4) sets how raw-mode tabs are expanded to the monospace grid;
// it is variadic so the many internal callers that don't care can omit it.
func Parse(src []byte, format Format, collapseDepth int, tabWidth ...int) *model.Document {
	tw := 4
	if len(tabWidth) > 0 && tabWidth[0] > 0 {
		tw = tabWidth[0]
	}
	src = stripBOM(src) // before AutoDetect and NewBuilder, so SrcSeg offsets stay aligned
	if format == FormatAuto {
		format = AutoDetect(src)
	}
	p := parserFor(format)
	if p == nil { // FormatRaw (or anything without a parser)
		return parseRaw(src, collapseDepth, tw)
	}
	b := model.NewBuilder(src, format, collapseDepth)
	if err := p.Parse(src, b); err != nil {
		return parseRaw(src, collapseDepth, tw)
	}
	return b.Finish()
}

// AutoDetect picks the most likely format for src. It returns FormatRaw when no
// structured parser is confident.
func AutoDetect(src []byte) Format {
	src = stripBOM(src) // a leading BOM is not whitespace and would mask the first real byte
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
