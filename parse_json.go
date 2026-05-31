package prettyview

import (
	"bytes"
	"errors"
	"unicode"
)

// jsonParser is a hand-written, zero-copy JSON / JSONC scanner. It walks the
// source byte-by-byte and drives a Builder, keeping the actual data tokens
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
	return 80
}

func (p jsonParser) Parse(src []byte, b *Builder) error {
	s := &jsonScanner{src: src, jsonc: p.jsonc, b: b}
	s.skipSpace()
	if s.pos >= len(s.src) {
		return errors.New("prettyview: empty JSON input")
	}
	if _, ok := s.parseValue(nil, false, s.pos); !ok {
		return s.err
	}
	return nil
}

type jsonScanner struct {
	src   []byte
	pos   int
	jsonc bool
	b     *Builder
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
func (s *jsonScanner) skipSpace() {
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			s.pos++
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
func (s *jsonScanner) parseValue(prefix []Seg, isMember bool, srcStart int) (NodeID, bool) {
	s.skipSpace()
	if s.pos >= len(s.src) {
		return NoNode, s.fail("unexpected end of input")
	}
	c := s.src[s.pos]
	switch {
	case c == '{':
		return s.parseContainer(prefix, isMember, srcStart, '{', '}', KindObject)
	case c == '[':
		return s.parseContainer(prefix, isMember, srcStart, '[', ']', KindArray)
	case c == '"':
		start, ok := s.scanString()
		if !ok {
			return NoNode, false
		}
		return s.emitScalar(prefix, isMember, srcStart, s.pos, srcSeg(RoleString, start, s.pos)), true
	case c == '-' || (c >= '0' && c <= '9'):
		start := s.scanNumber()
		return s.emitScalar(prefix, isMember, srcStart, s.pos, srcSeg(RoleNumber, start, s.pos)), true
	case s.matchLiteral("true"):
		start := s.pos
		s.pos += 4
		return s.emitScalar(prefix, isMember, srcStart, s.pos, srcSeg(RoleBool, start, s.pos)), true
	case s.matchLiteral("false"):
		start := s.pos
		s.pos += 5
		return s.emitScalar(prefix, isMember, srcStart, s.pos, srcSeg(RoleBool, start, s.pos)), true
	case s.matchLiteral("null"):
		start := s.pos
		s.pos += 4
		return s.emitScalar(prefix, isMember, srcStart, s.pos, srcSeg(RoleNull, start, s.pos)), true
	default:
		return NoNode, s.fail("unexpected character in value")
	}
}

func (s *jsonScanner) emitScalar(prefix []Seg, isMember bool, srcStart, srcEnd int, value Seg) NodeID {
	kind := KindScalar
	if isMember {
		kind = KindKeyValue
	}
	segs := make([]Seg, 0, len(prefix)+1)
	segs = append(segs, prefix...)
	segs = append(segs, value)
	return s.b.Leaf(kind, srcStart, srcEnd, segs)
}

func (s *jsonScanner) parseContainer(prefix []Seg, isMember bool, srcStart int, open, close byte, kind Kind) (NodeID, bool) {
	s.pos++ // consume opening brace/bracket

	// Empty container: render as a single leaf `{}` / `[]`.
	s.skipSpace()
	if s.peek() == close {
		s.pos++
		segs := make([]Seg, 0, len(prefix)+1)
		segs = append(segs, prefix...)
		segs = append(segs, litSeg(RolePunct, string(open)+string(close)))
		kv := KindScalar
		if isMember {
			kv = KindKeyValue
		}
		return s.b.Leaf(kv, srcStart, s.pos, segs), true
	}

	head := make([]Seg, 0, len(prefix)+1)
	head = append(head, prefix...)
	head = append(head, litSeg(RolePunct, string(open)))
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
			s.b.Close(closeOff, []Seg{litSeg(RolePunct, string(close))})
			break
		}

		var childPrefix []Seg
		childStart := s.pos
		if kind == KindObject {
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
			childPrefix = []Seg{srcSeg(RoleKey, keyStart, keyEnd), litSeg(RolePunct, ": ")}
		}

		childID, ok := s.parseValue(childPrefix, kind == KindObject, childStart)
		if !ok {
			break
		}

		s.skipSpace()
		if s.peek() == ',' {
			s.pos++
			s.b.AppendComma(s.b.LastLine(childID))
			continue
		}
		// Otherwise expect the closing brace on the next iteration.
	}
	return id, s.err == nil
}
