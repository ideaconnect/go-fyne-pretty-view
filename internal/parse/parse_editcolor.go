package parse

import (
	"bytes"
	"math"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
)

// This file is the v2 LIVE edit projection: a tolerant, layout-preserving syntax
// colorizer (issue: real-time highlighting while typing). It produces the same display
// shape as the monochrome edit-raw segmentation (editRawLineSegs) — one display line per
// physical line, a trailing empty line for the caret, and a single placeholder rune per
// grid-hostile byte so display-runes equal buffer-runes (the caret stays an exact buffer
// position) — but it ALSO assigns a syntax color role to each run by lexing in place.
//
// Unlike a structured parse it NEVER reflows and NEVER fails: mid-edit / invalid input is
// colored best-effort, so highlighting stays live on every keystroke without the document
// ever flickering back to a monochrome raw view. The structured parse (parse.Parse) is
// still used separately, on an explicit Reformat, to pretty-print and to compute validity.

// maxColorLineBytes bounds the bytes on a single line that the colorizer will tokenize.
// A pathological very long single line (e.g. a multi-megabyte minified document before it
// is reformatted into many short lines) is rendered monochrome instead, so the per-line
// segment count can never approach the Line.SegCount (uint16) saturation point and drop a
// line's tail. Reformatting such a line splits it into short, fully-colored lines.
const maxColorLineBytes = 16384

// editColorParser is the editable-mode projection parser. format selects the colorizer;
// FormatRaw (or any format without a lexer) colors nothing (every byte stays RolePlain),
// which makes the output byte-identical to the monochrome edit-raw segmentation.
type editColorParser struct{ format Format }

func (p editColorParser) Parse(src []byte, b *model.Builder) error {
	spans := lexColorSpans(src, p.format)
	si := 0
	start := 0
	for {
		nl := bytes.IndexByte(src[start:], '\n')
		if nl < 0 {
			appendColorLine(b, src, start, len(src), spans, &si)
			return nil
		}
		end := start + nl
		appendColorLine(b, src, start, end, spans, &si)
		start = end + 1
	}
}

// appendColorLine emits the segments for src[start:end] as a KindRawLine leaf: runs of
// clean bytes that share a color role become one zero-copy SrcSeg, and each grid-hostile
// byte becomes a one-rune placeholder LitSeg carrying the run's role. si is the caller's
// running cursor into spans (advanced in place, since lines are walked in order).
func appendColorLine(b *model.Builder, src []byte, start, end int, spans []colorSpan, si *int) {
	if len(spans) == 0 || end-start > maxColorLineBytes {
		// No syntax to color (raw), or a pathologically long line: fall back to the
		// monochrome edit-raw segmentation (still placeholder-safe and 1:1).
		b.Leaf(model.KindRawLine, start, end, editRawLineSegs(src, start, end))
		return
	}
	var segs []model.Seg
	runStart := start
	var runRole model.ColorRole
	haveRun := false
	flush := func(to int) {
		if haveRun && to > runStart {
			segs = append(segs, model.SrcSeg(runRole, runStart, to))
		}
		haveRun = false
	}
	for i := start; i < end; i++ {
		for *si < len(spans) && spans[*si].end <= i {
			*si++
		}
		role := model.RolePlain
		if *si < len(spans) && spans[*si].start <= i {
			role = spans[*si].role
		}
		if isGridHostile(src[i]) {
			flush(i)
			segs = append(segs, model.LitSeg(role, ctlPlaceholder))
			runStart = i + 1
			continue
		}
		switch {
		case !haveRun:
			runStart, runRole, haveRun = i, role, true
		case role != runRole:
			flush(i)
			runStart, runRole, haveRun = i, role, true
		}
	}
	flush(end)
	if len(segs) == 0 {
		segs = append(segs, model.SrcSeg(model.RolePlain, start, end)) // empty line
	}
	b.Leaf(model.KindRawLine, start, end, segs)
}

// colorSpan assigns a color role to a [start,end) byte range. Spans are non-overlapping
// and ascending; bytes covered by no span default to RolePlain.
type colorSpan struct {
	start, end int
	role       model.ColorRole
}

// lexColorSpans returns the colored token spans for src under format. A format without a
// colorizer (raw) returns nil — every byte then renders as RolePlain.
func lexColorSpans(src []byte, format Format) []colorSpan {
	switch format {
	case FormatJSON:
		return jsonColorSpans(src, false)
	case FormatJSONC:
		return jsonColorSpans(src, true)
	case FormatXML, FormatHTML:
		return markupColorSpans(src)
	default:
		return nil
	}
}

// ParseEditableColored builds the editable-mode projection of src, syntax-colored under
// format (FormatRaw colors nothing). It does not rewrite bytes, so display line/column
// offsets map 1:1 onto the edit buffer (the caret math depends on that alignment).
func ParseEditableColored(src []byte, format Format, collapseDepth, tabWidth int) *model.Document {
	_ = tabWidth // each grid-hostile byte (incl. tab) is one placeholder rune, never an expansion
	if uint64(len(src)) > math.MaxUint32 {
		src = src[:int(uint64(math.MaxUint32))] // dead in practice; keeps offsets sane
	}
	b := model.NewBuilder(src, format, collapseDepth)
	_ = editColorParser{format: format}.Parse(src, b)
	return b.Finish()
}

// --- JSON / JSONC colorizer -------------------------------------------------------------

// jsonColorSpans is a tolerant, single-pass JSON/JSONC lexer that assigns color roles
// matching the structured parser (keys RoleKey, strings RoleString, numbers RoleNumber,
// true/false RoleBool, null RoleNull, structural punctuation RolePunct, comments
// RoleComment). It never errors: an unterminated string stops at the newline, an unknown
// byte is left RolePlain, and a partial document mid-type colors as far as it parses.
func jsonColorSpans(src []byte, jsonc bool) []colorSpan {
	spans := make([]colorSpan, 0, len(src)/8+8)
	add := func(start, end int, role model.ColorRole) {
		if end > start {
			spans = append(spans, colorSpan{start, end, role})
		}
	}
	// A frame per open container tracks object-vs-array and, for objects, whether a key
	// (true) or a value (false) is expected next, so a string can be colored as a key.
	type frame struct{ obj, expectKey bool }
	var stack []frame
	inObjKey := func() bool {
		return len(stack) > 0 && stack[len(stack)-1].obj && stack[len(stack)-1].expectKey
	}
	setExpectKey := func(v bool) {
		if len(stack) > 0 && stack[len(stack)-1].obj {
			stack[len(stack)-1].expectKey = v
		}
	}
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			i++
		case c == '{':
			add(i, i+1, model.RolePunct)
			stack = append(stack, frame{obj: true, expectKey: true})
			i++
		case c == '[':
			add(i, i+1, model.RolePunct)
			stack = append(stack, frame{})
			i++
		case c == '}' || c == ']':
			add(i, i+1, model.RolePunct)
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			i++
		case c == ':':
			add(i, i+1, model.RolePunct)
			setExpectKey(false) // a value is expected after the colon
			i++
		case c == ',':
			add(i, i+1, model.RolePunct)
			setExpectKey(true) // another key is expected after a comma (in an object)
			i++
		case c == '"':
			start := i
			i = scanJSONString(src, i)
			role := model.RoleString
			if inObjKey() {
				role = model.RoleKey
			}
			add(start, i, role)
		case c == '-' || (c >= '0' && c <= '9'):
			start := i
			i = scanJSONNumber(src, i)
			add(start, i, model.RoleNumber)
		case matchWord(src, i, "true"):
			add(i, i+4, model.RoleBool)
			i += 4
		case matchWord(src, i, "false"):
			add(i, i+5, model.RoleBool)
			i += 5
		case matchWord(src, i, "null"):
			add(i, i+4, model.RoleNull)
			i += 4
		case jsonc && c == '/' && i+1 < len(src) && src[i+1] == '/':
			start := i
			i += 2
			for i < len(src) && src[i] != '\n' {
				i++
			}
			add(start, i, model.RoleComment)
		case jsonc && c == '/' && i+1 < len(src) && src[i+1] == '*':
			start := i
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			if i+1 < len(src) {
				i += 2
			} else {
				i = len(src)
			}
			add(start, i, model.RoleComment)
		default:
			i++ // stray byte mid-edit: leave RolePlain
		}
	}
	return spans
}

// scanJSONString returns the position just past a JSON string that starts at the opening
// quote i. It is tolerant: an unterminated string stops at the next newline (so the rest
// of the document still lexes) or at EOF.
func scanJSONString(src []byte, i int) int {
	i++ // opening quote
	for i < len(src) {
		switch src[i] {
		case '\n':
			return i
		case '\\':
			if i+1 < len(src) && src[i+1] != '\n' {
				i += 2
			} else {
				i++
			}
		case '"':
			return i + 1
		default:
			i++
		}
	}
	return i
}

func scanJSONNumber(src []byte, i int) int {
	for i < len(src) {
		c := src[i]
		if (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' {
			i++
			continue
		}
		break
	}
	return i
}

// matchWord reports whether the bytes at i are exactly word (no bounds-checking of the
// following byte, matching the structured scanner's tolerant literal match).
func matchWord(src []byte, i int, word string) bool {
	return i+len(word) <= len(src) && string(src[i:i+len(word)]) == word
}

// --- XML / HTML colorizer ---------------------------------------------------------------

// markupColorSpans is a tolerant XML/HTML lexer assigning roles matching the structured
// parsers: tag names RoleTag, attribute names RoleAttr, attribute values and text content
// RoleString, the < > / = </ punctuation RolePunct, and comments / processing instructions
// / doctypes RoleComment. It never errors: an unterminated tag or comment runs to EOF.
func markupColorSpans(src []byte) []colorSpan {
	spans := make([]colorSpan, 0, len(src)/8+8)
	add := func(start, end int, role model.ColorRole) {
		if end > start {
			spans = append(spans, colorSpan{start, end, role})
		}
	}
	i := 0
	for i < len(src) {
		if src[i] != '<' {
			start := i
			for i < len(src) && src[i] != '<' {
				i++
			}
			add(start, i, model.RoleString) // text content
			continue
		}
		if hasPrefixAt(src, i, "<!--") {
			start := i
			i += 4
			for i+2 < len(src) && !(src[i] == '-' && src[i+1] == '-' && src[i+2] == '>') {
				i++
			}
			if i+2 < len(src) {
				i += 3
			} else {
				i = len(src)
			}
			add(start, i, model.RoleComment)
			continue
		}
		if hasPrefixAt(src, i, "<!") || hasPrefixAt(src, i, "<?") {
			start := i
			for i < len(src) && src[i] != '>' {
				i++
			}
			if i < len(src) {
				i++
			}
			add(start, i, model.RoleComment) // doctype / directive / processing instruction
			continue
		}
		// A start or end tag.
		tagOpen := i
		i++ // '<'
		if i < len(src) && src[i] == '/' {
			i++ // '</'
		}
		add(tagOpen, i, model.RolePunct)
		nameStart := i
		for i < len(src) && isNameByte(src[i]) {
			i++
		}
		add(nameStart, i, model.RoleTag)
		for i < len(src) && src[i] != '>' {
			switch c := src[i]; {
			case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
				i++
			case c == '/':
				add(i, i+1, model.RolePunct)
				i++
			case c == '=':
				add(i, i+1, model.RolePunct)
				i++
			case c == '"' || c == '\'':
				q := c
				start := i
				i++
				for i < len(src) && src[i] != q && src[i] != '\n' {
					i++
				}
				if i < len(src) && src[i] == q {
					i++
				}
				add(start, i, model.RoleString)
			default:
				start := i
				for i < len(src) && isAttrNameByte(src[i]) {
					i++
				}
				if i == start {
					i++ // stray byte: advance so we never loop
				}
				add(start, i, model.RoleAttr)
			}
		}
		if i < len(src) && src[i] == '>' {
			add(i, i+1, model.RolePunct)
			i++
		}
	}
	return spans
}

func hasPrefixAt(src []byte, i int, p string) bool {
	return i+len(p) <= len(src) && string(src[i:i+len(p)]) == p
}

// isNameByte reports whether c can appear in an XML/HTML tag name (names also admit any
// non-ASCII byte, for Unicode element names).
func isNameByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == ':' || c == '-' || c == '_' || c == '.':
		return true
	case c >= 0x80:
		return true
	}
	return false
}

// isAttrNameByte reports whether c can be part of an attribute name (anything up to the
// next whitespace, '=', '/', '>', or quote).
func isAttrNameByte(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f', '\v', '=', '/', '>', '"', '\'':
		return false
	}
	return true
}
