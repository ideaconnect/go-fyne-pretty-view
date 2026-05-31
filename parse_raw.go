package prettyview

import "bytes"

// rawParser is the universal fallback: it splits the source into physical lines,
// each rendered as a single plain, non-foldable row. It is also used whenever a
// structured parse fails, so malformed input still displays.
type rawParser struct{}

func (rawParser) Format() Format { return FormatRaw }

// Detect never claims raw during auto-detection; raw is the floor chosen by
// AutoDetect only when no structured parser is confident.
func (rawParser) Detect([]byte) int { return 0 }

func (rawParser) Parse(src []byte, b *Builder) error {
	start := 0
	for start <= len(src) {
		nl := bytes.IndexByte(src[start:], '\n')
		if nl < 0 {
			if start < len(src) {
				b.Leaf(KindRawLine, start, len(src), rawLineSegs(src, start, len(src)))
			}
			break
		}
		end := start + nl
		b.Leaf(KindRawLine, start, end, rawLineSegs(src, start, end))
		start = end + 1
	}
	return nil
}

// rawLineSegs builds the (possibly empty) segment for one raw line, trimming a
// trailing carriage return.
func rawLineSegs(src []byte, start, end int) []Seg {
	segEnd := end
	if segEnd > start && src[segEnd-1] == '\r' {
		segEnd--
	}
	return []Seg{srcSeg(RolePlain, start, segEnd)}
}

// parseRaw builds a raw document directly (used as a parse-failure fallback).
func parseRaw(src []byte, collapseDepth int) *Document {
	b := newBuilder(src, FormatRaw, collapseDepth)
	_ = rawParser{}.Parse(src, b)
	return b.finish()
}
