package parse

import (
	"bytes"
	"errors"
	"unicode"
	"unicode/utf8"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"
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
	if p.jsonc {
		// JSONC is auto-selected ONLY for a document whose leading trivia (before the
		// first bracket) or first in-bracket token is a real // or /* */ comment — a
		// signal plain JSON cannot produce. A "//" inside a string value (e.g. a URL)
		// appears only after the first key/element, never this early, so there is no
		// ambiguity. A comment-free JSON document scores 0 here and is left to the
		// plain-JSON detector below. This fixes a ".jsonc" config with a leading
		// `// license` header (or `{ // note`) auto-detecting as raw (issue #82).
		return detectJSONCWithComment(src)
	}
	t := bytes.TrimLeftFunc(src, unicode.IsSpace)
	if len(t) == 0 || (t[0] != '{' && t[0] != '[') {
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
	//
	// A leading // comment makes t[0] a '/', so plain JSON declines here and the JSONC
	// detector claims the document instead (issue #82).
	return jsonBracketScore(t[0], t[1:])
}

// detectJSONCWithComment scores a document as JSONC when a // or /* */ comment appears in
// non-string trivia before the first value — either ahead of the opening bracket (a
// ".jsonc" license header) or as the first token inside it ("{ // note ..."). That is an
// unambiguous JSONC signal, so JSONC is safe to auto-select while a comment-free document
// is left to the plain-JSON detector (issue #82). It never enters strings, so a "//" in a
// string value is never mistaken for a comment.
func detectJSONCWithComment(src []byte) int {
	rest, sawComment := skipWSAndComments(bytes.TrimLeftFunc(src, unicode.IsSpace))
	if len(rest) == 0 || (rest[0] != '{' && rest[0] != '[') {
		return 0
	}
	inner, innerComment := skipWSAndComments(rest[1:])
	if !sawComment && !innerComment {
		return 0 // a comment-free JSON document — let the plain-JSON detector own it
	}
	return jsonBracketScore(rest[0], inner)
}

// jsonBracketScore scores how plausibly a '{'/'[' container is JSON, given the bytes that
// follow the opening byte (leading whitespace is trimmed here; the JSONC path also strips
// leading comments first). It is the structural sniff shared by both JSON detectors.
func jsonBracketScore(open byte, afterOpen []byte) int {
	head := afterOpen
	if len(head) > sniffLimit {
		head = head[:sniffLimit]
	}
	rest := bytes.TrimLeftFunc(head, unicode.IsSpace)
	if len(rest) == 0 {
		return 80 // a lone opening bracket (e.g. a document mid-edit): plausibly JSON
	}
	c := rest[0]
	if open == '{' {
		if c == '}' || c == '"' {
			return 80
		}
		return 0
	}
	// open == '[': the first element must start with a JSON value (or close the array).
	if c == ']' || c == '"' || c == '{' || c == '[' || c == '-' ||
		c == 't' || c == 'f' || c == 'n' || (c >= '0' && c <= '9') {
		return 80
	}
	return 0
}

// skipWSAndComments returns src advanced past leading ASCII/Unicode whitespace and any //
// line or /* */ block comments, plus whether at least one comment was skipped. It is the
// cheap detection-time trivia scan (issue #82); it never enters strings. Its whitespace set
// matches jsonScanner.scanTrivia so detection and parsing agree on what counts as trivia.
func skipWSAndComments(src []byte) (rest []byte, sawComment bool) {
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case isASCIISpace(c):
			i++
		case c >= utf8.RuneSelf:
			r, size := utf8.DecodeRune(src[i:])
			if !unicode.IsSpace(r) {
				return src[i:], sawComment
			}
			i += size
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			i = scanLineCommentExtent(src, i)
			sawComment = true
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i = scanBlockCommentExtent(src, i)
			sawComment = true
		default:
			return src[i:], sawComment
		}
	}
	return src[i:], sawComment
}

func (p jsonParser) Parse(src []byte, b *model.Builder) error {
	s := &jsonScanner{src: src, jsonc: p.jsonc, b: b}
	s.emitComments(s.scanTrivia()) // leading JSONC comments become top-level nodes
	if s.pos >= len(s.src) {
		return errors.New("prettyview: empty JSON input")
	}
	// Tolerant by contract (see Parser): on a truncated/malformed value we keep
	// whatever structure was recovered rather than failing outright. parseValue
	// emits nodes as it goes and returns the root node even when it stops early,
	// and Builder.Finish force-closes any dangling container.
	id, ok := s.parseValue(nil, false, s.pos, nil)
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
	// silently dropping everything past the first value. Trailing JSONC comments are
	// emitted as nodes (symmetric with leading comments); only real trailing data
	// triggers the raw fallback.
	s.emitComments(s.scanTrivia())
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

// commentSpan is the byte range of a // or /* */ comment, returned by scanTrivia in
// JSONC mode so the container/top-level parser can emit it as a KindComment node.
type commentSpan struct{ start, end int }

// scanTrivia consumes whitespace and // line / /* */ block comments, and in JSONC
// mode returns each comment's byte span (nil in plain JSON, where comments are still
// tolerated and skipped). The caller emits the spans as nodes at a structurally sound
// point (see emitComments).
//
// Its whitespace set must match what auto-detection trims (unicode.IsSpace, in
// jsonParser.Detect / AutoDetect): if the scanner stopped on a byte the detector
// treats as whitespace, an input confidently labelled JSON would stall mid-scan —
// a leading form-feed would fall back to raw, and a form-feed/NBSP *between*
// members would silently drop the rest of the container. So the ASCII fast path
// includes \f and \v, and any non-ASCII byte is decoded and tested with
// unicode.IsSpace (covering NBSP, the line/paragraph separators, etc.).
func (s *jsonScanner) scanTrivia() []commentSpan {
	var spans []commentSpan
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch {
		case isASCIISpace(c):
			s.pos++
		case c >= utf8.RuneSelf:
			// Possible multi-byte Unicode space. On invalid UTF-8, DecodeRune
			// returns RuneError (not a space), so we stop and let parseValue report
			// the byte rather than skipping past it.
			r, size := utf8.DecodeRune(s.src[s.pos:])
			if !unicode.IsSpace(r) {
				return spans
			}
			s.pos += size
		case c == '/' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '/':
			start := s.pos
			s.pos = scanLineCommentExtent(s.src, s.pos)
			if s.jsonc {
				spans = append(spans, commentSpan{start, s.pos})
			}
		case c == '/' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '*':
			start := s.pos
			s.pos = scanBlockCommentExtent(s.src, s.pos)
			if s.jsonc {
				spans = append(spans, commentSpan{start, s.pos})
			}
		default:
			return spans
		}
	}
	return spans
}

// emitComments emits each collected comment as a KindComment leaf child of the
// current container. The bytes are zero-copy unless they hold a control byte (a block
// comment spanning lines), in which case cleanSrcSeg C-escapes them so the comment
// stays on one display row.
func (s *jsonScanner) emitComments(spans []commentSpan) {
	for _, c := range spans {
		s.b.Leaf(model.KindComment, c.start, c.end,
			[]model.Seg{cleanSrcSeg(s.src, model.RoleComment, c.start, c.end)})
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
			if s.pos > len(s.src) {
				s.pos = len(s.src) // a trailing backslash must not push pos past EOF (#76)
			}
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
	s.pos = scanNumberExtent(s.src, s.pos)
	return start
}

// matchLiteral checks for a bare word (true/false/null) at s.pos.
func (s *jsonScanner) matchLiteral(word string) bool {
	return matchLiteralAt(s.src, s.pos, word)
}

// parseValue parses one value and emits a node for it. prefix segments (e.g.
// `"key": `) are rendered before the value on its head/leaf line. isMember marks
// object members (vs array elements / the root). srcStart is the node's first
// source byte. Returns the node id.
// parseValue's lead carries JSONC comments collected before the value (between a member's
// key and its colon, and between the colon and the value); it scans any further leading
// trivia itself. A comment before a CONTAINER value is threaded in as the container's first
// leading comment (rendered just inside it); a comment before a SCALAR is emitted as a node
// right AFTER the scalar — both keep node SrcStart non-decreasing (nodeAtByteOffset relies
// on it) while losing no comment. In plain JSON lead is always empty (scanTrivia collects no
// spans), so this is a JSONC-only path.
func (s *jsonScanner) parseValue(prefix []model.Seg, isMember bool, srcStart int, lead []commentSpan) (model.NodeID, bool) {
	lead = append(lead, s.scanTrivia()...)
	if s.pos >= len(s.src) {
		s.emitComments(lead)
		return model.NoNode, s.fail("unexpected end of input")
	}
	c := s.src[s.pos]
	switch {
	case c == '{':
		return s.parseContainer(prefix, isMember, srcStart, '{', '}', model.KindObject, lead)
	case c == '[':
		return s.parseContainer(prefix, isMember, srcStart, '[', ']', model.KindArray, lead)
	}

	// Scalar value: emit it, then its leading comments just below (monotonic + lossless).
	var node model.NodeID
	switch {
	case c == '"':
		start, ok := s.scanString()
		if !ok {
			s.emitComments(lead)
			return model.NoNode, false
		}
		node = s.emitScalar(prefix, isMember, srcStart, s.pos, cleanSrcSeg(s.src, model.RoleString, start, s.pos))
	case c == '-' || (c >= '0' && c <= '9'):
		start := s.scanNumber()
		node = s.emitScalar(prefix, isMember, srcStart, s.pos, model.SrcSeg(model.RoleNumber, start, s.pos))
	case s.matchLiteral("true"):
		start := s.pos
		s.pos += 4
		node = s.emitScalar(prefix, isMember, srcStart, s.pos, model.SrcSeg(model.RoleBool, start, s.pos))
	case s.matchLiteral("false"):
		start := s.pos
		s.pos += 5
		node = s.emitScalar(prefix, isMember, srcStart, s.pos, model.SrcSeg(model.RoleBool, start, s.pos))
	case s.matchLiteral("null"):
		start := s.pos
		s.pos += 4
		node = s.emitScalar(prefix, isMember, srcStart, s.pos, model.SrcSeg(model.RoleNull, start, s.pos))
	default:
		s.emitComments(lead)
		return model.NoNode, s.fail("unexpected character in value")
	}
	s.emitComments(lead)
	return node, true
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

func (s *jsonScanner) parseContainer(prefix []model.Seg, isMember bool, srcStart int, open, close byte, kind model.Kind, lead []commentSpan) (model.NodeID, bool) {
	// Bound recursion: refuse to descend past the nesting cap so adversarial input
	// can't overflow the stack. The error routes to the tolerant partial render (or
	// raw fallback if nothing was recovered at all).
	if s.b.Depth() >= maxNestDepth {
		return model.NoNode, s.fail("maximum nesting depth exceeded")
	}
	s.pos++ // consume opening brace/bracket

	// Empty container: render as a single leaf `{}` / `[]`. JSONC comments inside the
	// braces (or threaded in via lead, i.e. between a key's colon and this `{`) are
	// buffered first, so `{}` stays a leaf but `{ /* c */ }` / `"a": /* c */ {}` is a
	// real (foldable) container with the comment as its first child.
	comments := append(lead, s.scanTrivia()...)
	if s.peek() == close && len(comments) == 0 {
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
	s.emitComments(comments) // leading comments inside the container, now that it is open

	for {
		s.emitComments(s.scanTrivia()) // comments between members / before the close
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
		var keyComments []commentSpan
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
			keyComments = s.scanTrivia() // JSONC comments between the key and its colon
			if s.peek() != ':' {
				s.fail("expected ':' after key")
				break
			}
			s.pos++
			childPrefix = []model.Seg{cleanSrcSeg(s.src, model.RoleKey, keyStart, keyEnd), model.LitSeg(model.RolePunct, ": ")}
		}

		childID, ok := s.parseValue(childPrefix, kind == model.KindObject, childStart, keyComments)
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

		s.emitComments(s.scanTrivia()) // JSONC comments trailing the value (before its comma / the close)
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
