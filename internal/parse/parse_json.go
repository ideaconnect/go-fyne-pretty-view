package parse

import (
	"bytes"
	"errors"
	"unicode"
	"unicode/utf8"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// jsonParser is a hand-written, zero-copy JSON / JSONC scanner. It walks the
// source byte-by-byte and drives a model.Builder, keeping the actual data tokens
// (keys, string/number literals) as byte ranges into the source. Structural
// punctuation is synthesized so that even minified input renders pretty-printed.
type jsonParser struct{ jsonc bool }

func (p jsonParser) Format() Format {
	if p.jsonc {
		return FormatJSONC
	}
	return FormatJSON
}

func (p jsonParser) Detect(src []byte) int {
	t := bytes.TrimLeftFunc(src, unicode.IsSpace)
	if len(t) == 0 || (t[0] != '{' && t[0] != '[') {
		return 0
	}
	if p.jsonc {
		// JSONC is never chosen by auto-detection: a "//" inside a string (e.g. a
		// URL) is not a comment, so it can't be told apart cheaply. The scanner is
		// comment-tolerant regardless, and FormatJSONC remains explicitly selectable.
		return 0
	}
	// A leading '{'/'[' alone is too weak a signal: log lines and markdown
	// ("[ERROR] disk full", "[label](url)") begin with a bracket but are not JSON,
	// and labelling them JSON makes the tolerant scanner recover a sliver and drop
	// the rest (the README's raw fallback should win instead). Require the first
	// significant byte *inside* the bracket to be plausible JSON: an object must be
	// followed by '}' or a '"' key; an array by ']' or a value-start byte. This is a
	// cheap structural sniff, not a full validation — the parser stays tolerant, and
	// a clean parse that leaves trailing junk still falls back to raw (see Parse).
	head := t[1:]
	if len(head) > sniffLimit {
		head = head[:sniffLimit]
	}
	rest := bytes.TrimLeftFunc(head, unicode.IsSpace)
	if len(rest) == 0 {
		return 80 // a lone opening bracket (e.g. a document mid-edit): plausibly JSON
	}
	c := rest[0]
	if t[0] == '{' {
		if c == '}' || c == '"' {
			return 80
		}
		return 0
	}
	// t[0] == '[': the first element must start with a JSON value (or close the array).
	if c == ']' || c == '"' || c == '{' || c == '[' || c == '-' ||
		c == 't' || c == 'f' || c == 'n' || (c >= '0' && c <= '9') {
		return 80
	}
	return 0
}

func (p jsonParser) Parse(src []byte, b *model.Builder) error {
	s := &jsonScanner{src: src, jsonc: p.jsonc, b: b}
	s.skipSpace()
	if s.pos >= len(s.src) {
		return errors.New("prettyview: empty JSON input")
	}
	// Tolerant by contract (see Parser): on a truncated/malformed value we keep
	// whatever structure was recovered rather than failing outright. parseValue
	// emits nodes as it goes and returns the root node even when it stops early,
	// and Builder.Finish force-closes any dangling container.
	id, ok := s.parseValue(nil, false, s.pos)
	if !ok {
		// Nothing structural was recovered at all (e.g. a bare unterminated string
		// or non-JSON junk under a forced JSON format): fall back to raw. A partial
		// structure (id != NoNode, an inner value truncated) is kept and displayed
		// tolerantly, with its recovered error markers — that path matches the
		// in-container junk recovery and is covered by the `[trueX]` tests.
		if id == model.NoNode {
			return s.err
		}
		return nil
	}
	// The root value parsed cleanly. A single JSON document has nothing after its
	// root but whitespace (and, in our comment-tolerant scanner, comments). Real
	// trailing content means this is not one JSON document — NDJSON, concatenated
	// values, or a log/markdown line that merely begins with a bracket — so fall
	// back to raw, where every byte stays visible on its own line, instead of
	// silently dropping everything past the first value.
	s.skipSpace()
	if s.pos < len(s.src) {
		return errors.New("prettyview: trailing content after top-level JSON value")
	}
	return nil
}

type jsonScanner struct {
	src   []byte
	pos   int
	jsonc bool
	b     *model.Builder
	err   error
}

func (s *jsonScanner) fail(msg string) bool {
	if s.err == nil {
		s.err = errors.New("prettyview: " + msg)
	}
	return false
}

func (s *jsonScanner) peek() byte {
	if s.pos < len(s.src) {
		return s.src[s.pos]
	}
	return 0
}

// skipSpace consumes whitespace and, in JSONC mode, // line and /* */ block
// comments (treated as whitespace in M1; rendered as nodes is a later refinement).
//
// Its whitespace set must match what auto-detection trims (unicode.IsSpace, in
// jsonParser.Detect / AutoDetect): if the scanner stopped on a byte the detector
// treats as whitespace, an input confidently labelled JSON would stall mid-scan —
// a leading form-feed would fall back to raw, and a form-feed/NBSP *between*
// members would silently drop the rest of the container. So the ASCII fast path
// includes \f and \v, and any non-ASCII byte is decoded and tested with
// unicode.IsSpace (covering NBSP, the line/paragraph separators, etc.).
func (s *jsonScanner) skipSpace() {
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			s.pos++
		case c >= utf8.RuneSelf:
			// Possible multi-byte Unicode space. On invalid UTF-8, DecodeRune
			// returns RuneError (not a space), so we stop and let parseValue report
			// the byte rather than skipping past it.
			r, size := utf8.DecodeRune(s.src[s.pos:])
			if !unicode.IsSpace(r) {
				return
			}
			s.pos += size
		case c == '/' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '/':
			s.pos += 2
			for s.pos < len(s.src) && s.src[s.pos] != '\n' {
				s.pos++
			}
		case c == '/' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '*':
			s.pos += 2
			for s.pos+1 < len(s.src) && !(s.src[s.pos] == '*' && s.src[s.pos+1] == '/') {
				s.pos++
			}
			if s.pos+1 < len(s.src) {
				s.pos += 2
			} else {
				s.pos = len(s.src)
			}
		default:
			return
		}
	}
}

// scanString consumes a JSON string starting at s.pos (which must be '"') and
// returns the position just past the closing quote.
func (s *jsonScanner) scanString() (int, bool) {
	start := s.pos
	s.pos++ // opening quote
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if c == '\\' {
			s.pos += 2
			continue
		}
		if c == '"' {
			s.pos++
			return start, true
		}
		s.pos++
	}
	return start, s.fail("unterminated string")
}

func (s *jsonScanner) scanNumber() int {
	start := s.pos
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' {
			s.pos++
			continue
		}
		break
	}
	return start
}

// matchLiteral checks for a bare word (true/false/null) at s.pos.
func (s *jsonScanner) matchLiteral(word string) bool {
	if s.pos+len(word) <= len(s.src) && string(s.src[s.pos:s.pos+len(word)]) == word {
		return true
	}
	return false
}

// parseValue parses one value and emits a node for it. prefix segments (e.g.
// `"key": `) are rendered before the value on its head/leaf line. isMember marks
// object members (vs array elements / the root). srcStart is the node's first
// source byte. Returns the node id.
func (s *jsonScanner) parseValue(prefix []model.Seg, isMember bool, srcStart int) (model.NodeID, bool) {
	s.skipSpace()
	if s.pos >= len(s.src) {
		return model.NoNode, s.fail("unexpected end of input")
	}
	c := s.src[s.pos]
	switch {
	case c == '{':
		return s.parseContainer(prefix, isMember, srcStart, '{', '}', model.KindObject)
	case c == '[':
		return s.parseContainer(prefix, isMember, srcStart, '[', ']', model.KindArray)
	case c == '"':
		start, ok := s.scanString()
		if !ok {
			return model.NoNode, false
		}
		return s.emitScalar(prefix, isMember, srcStart, s.pos, cleanSrcSeg(s.src, model.RoleString, start, s.pos)), true
	case c == '-' || (c >= '0' && c <= '9'):
		start := s.scanNumber()
		return s.emitScalar(prefix, isMember, srcStart, s.pos, model.SrcSeg(model.RoleNumber, start, s.pos)), true
	case s.matchLiteral("true"):
		start := s.pos
		s.pos += 4
		return s.emitScalar(prefix, isMember, srcStart, s.pos, model.SrcSeg(model.RoleBool, start, s.pos)), true
	case s.matchLiteral("false"):
		start := s.pos
		s.pos += 5
		return s.emitScalar(prefix, isMember, srcStart, s.pos, model.SrcSeg(model.RoleBool, start, s.pos)), true
	case s.matchLiteral("null"):
		start := s.pos
		s.pos += 4
		return s.emitScalar(prefix, isMember, srcStart, s.pos, model.SrcSeg(model.RoleNull, start, s.pos)), true
	default:
		return model.NoNode, s.fail("unexpected character in value")
	}
}

func (s *jsonScanner) emitScalar(prefix []model.Seg, isMember bool, srcStart, srcEnd int, value model.Seg) model.NodeID {
	kind := model.KindScalar
	if isMember {
		kind = model.KindKeyValue
	}
	segs := make([]model.Seg, 0, len(prefix)+1)
	segs = append(segs, prefix...)
	segs = append(segs, value)
	return s.b.Leaf(kind, srcStart, srcEnd, segs)
}

func (s *jsonScanner) parseContainer(prefix []model.Seg, isMember bool, srcStart int, open, close byte, kind model.Kind) (model.NodeID, bool) {
	// Bound recursion: refuse to descend past the nesting cap so adversarial input
	// can't overflow the stack. The error routes to the tolerant partial render (or
	// raw fallback if nothing was recovered at all).
	if s.b.Depth() >= maxNestDepth {
		return model.NoNode, s.fail("maximum nesting depth exceeded")
	}
	s.pos++ // consume opening brace/bracket

	// Empty container: render as a single leaf `{}` / `[]`.
	s.skipSpace()
	if s.peek() == close {
		s.pos++
		segs := make([]model.Seg, 0, len(prefix)+1)
		segs = append(segs, prefix...)
		segs = append(segs, model.LitSeg(model.RolePunct, string(open)+string(close)))
		kv := model.KindScalar
		if isMember {
			kv = model.KindKeyValue
		}
		return s.b.Leaf(kv, srcStart, s.pos, segs), true
	}

	head := make([]model.Seg, 0, len(prefix)+1)
	head = append(head, prefix...)
	head = append(head, model.LitSeg(model.RolePunct, string(open)))
	id := s.b.Open(kind, srcStart, head)

	for {
		s.skipSpace()
		if s.pos >= len(s.src) {
			s.fail("unterminated container")
			break
		}
		if s.peek() == close {
			closeOff := s.pos + 1
			s.pos++
			s.b.Close(closeOff, []model.Seg{model.LitSeg(model.RolePunct, string(close))})
			break
		}

		var childPrefix []model.Seg
		childStart := s.pos
		if kind == model.KindObject {
			if s.peek() != '"' {
				s.fail("expected object key")
				break
			}
			keyStart, ok := s.scanString()
			if !ok {
				break
			}
			keyEnd := s.pos
			s.skipSpace()
			if s.peek() != ':' {
				s.fail("expected ':' after key")
				break
			}
			s.pos++
			childPrefix = []model.Seg{cleanSrcSeg(s.src, model.RoleKey, keyStart, keyEnd), model.LitSeg(model.RolePunct, ": ")}
		}

		childID, ok := s.parseValue(childPrefix, kind == model.KindObject, childStart)
		if !ok {
			// Tolerant recovery: if a key was consumed but its value was truncated
			// (no value node emitted), keep the key visible as an error marker so a
			// cut-off member isn't silently dropped. If parseValue already emitted a
			// partial nested node, that node carries the key — don't duplicate it.
			if childID == model.NoNode && len(childPrefix) > 0 {
				s.b.Leaf(model.KindError, childStart, s.pos, childPrefix)
			}
			break
		}

		s.skipSpace()
		if s.peek() == ',' {
			s.pos++
			s.b.AppendComma(s.b.LastLine(childID))
			continue
		}
		// A complete value followed by neither ',' nor the close byte (nor EOF) is
		// trailing junk, e.g. the "X" in "[trueX]" or "abc" in "[123abc]" — a bare
		// literal/number scan stops at the first foreign byte without a delimiter
		// check. Surface it as an error marker rather than silently dropping the rest
		// of the container, mirroring the truncated-key recovery above.
		if s.pos < len(s.src) && s.peek() != close {
			junkStart := s.pos
			for s.pos < len(s.src) && s.src[s.pos] != ',' && s.src[s.pos] != close {
				s.pos++
			}
			s.b.Leaf(model.KindError, junkStart, s.pos, []model.Seg{cleanSrcSeg(s.src, model.RolePlain, junkStart, s.pos)})
			s.fail("unexpected content after value")
			break
		}
		// Otherwise expect the closing brace on the next iteration.
	}
	return id, s.err == nil
}
