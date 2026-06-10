// Package parse turns raw bytes into a model.Document. It hosts the format
// parsers (JSON/JSONC, XML, HTML, raw) and format auto-detection. It depends on
// internal/model and is depended on by the top-level prettyview package.
package parse

import (
	"bytes"
	"fmt"
	"math"
	"strings"
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

// A grid-hostile byte is any C0 control (< 0x20) or DEL (0x7f): \n/\r spill the
// segment onto another visual row, \t jumps to a tab stop, and the rest render as
// zero-width or replacement glyphs — all of which break the one-row-per-line and
// one-rune-per-cell invariants the renderer, hit-test, and selection math rely on.
// Data tokens (JSON strings/keys, XML/HTML attribute values) take these bytes
// verbatim — valid documents escape them — so a malformed input can carry them in.
// Multi-byte UTF-8 is safe: its lead/continuation bytes are all >= 0x80.
func isGridHostile(c byte) bool { return c < 0x20 || c == 0x7f }

// hasGridBreaker reports whether b contains a grid-hostile control byte.
func hasGridBreaker(b []byte) bool {
	for _, c := range b {
		if isGridHostile(c) {
			return true
		}
	}
	return false
}

// escapeGridBreakers rewrites grid-hostile control bytes in s to a visible escape
// so each stays on one row and one cell: \n, \r, and \t get their familiar
// two-character C escapes; every other control byte becomes \xNN. All other bytes
// pass through untouched (including multi-byte UTF-8). It is a cheap no-op when s is
// already clean. (A valid escape sequence in the source is already two display
// characters and never reaches here as a raw control byte.)
func escapeGridBreakers(s string) string {
	if !hasGridBreaker([]byte(s)) {
		return s
	}
	const hexDigits = "0123456789abcdef"
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case isGridHostile(c):
			b.WriteString(`\x`)
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0xf])
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// cleanSrcSeg builds a display segment for src[start:end]. It stays a zero-copy
// SrcSeg in the common case; only when those bytes contain a raw grid-breaker
// (\n/\r/\t — malformed input the scanners tolerate) does it fall back to a
// synthesized, C-escaped literal so the row and column grid stay intact.
func cleanSrcSeg(src []byte, role model.ColorRole, start, end int) model.Seg {
	if !hasGridBreaker(src[start:end]) {
		return model.SrcSeg(role, start, end)
	}
	return model.LitSeg(role, escapeGridBreakers(string(src[start:end])))
}

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
	// The model stores every byte offset as uint32 (Segment.Start/End, Node.SrcStart
	// /SrcEnd). A source past 4 GiB would wrap those casts into corrupt ranges that
	// mis-slice the buffer, so cap it before the Builder sees any offset. A single
	// multi-gigabyte document is explicitly out of scope (DESIGN §7.3); graceful
	// truncation beats silent corruption. (On a 32-bit platform len(src) can never
	// exceed this, so the branch is dead there — the runtime bound keeps it compiling.)
	if uint64(len(src)) > math.MaxUint32 {
		maxOffset := uint64(math.MaxUint32)
		src = src[:int(maxOffset)]
	}
	if format == FormatAuto {
		format = AutoDetect(src)
	}
	p := parserFor(format)
	if p == nil { // FormatRaw (or anything without a parser)
		return parseRaw(src, collapseDepth, tw)
	}
	b := model.NewBuilder(src, format, collapseDepth)
	if err := safeParse(p, src, b); err != nil {
		return parseRaw(src, collapseDepth, tw)
	}
	return b.Finish()
}

// safeParse runs a parser behind a panic boundary. The parsers are tolerance-first
// and should never panic, but they consume untrusted input; if an unforeseen case
// ever trips a panic, degrade to the raw fallback (content always displays) rather
// than crashing the host application. The partially-built model is discarded.
func safeParse(p Parser, src []byte, b *model.Builder) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("prettyview: recovered parser panic: %v", r)
		}
	}()
	return p.Parse(src, b)
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
