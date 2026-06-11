package parse

import (
	"bytes"
	"encoding/xml"
	"strings"
	"unicode"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"
)

// xmlParser parses XML via encoding/xml's tokenizer. Because the tokenizer does
// not expose source byte offsets per token, XML segments use interned (Aux)
// literals rather than zero-copy Src ranges; copy-subtree therefore reconstructs
// from the displayed text. Namespace prefixes are rendered by local name.
type xmlParser struct{}

func (xmlParser) Format() Format { return FormatXML }

func (xmlParser) Detect(src []byte) int {
	t := bytes.TrimLeftFunc(src, unicode.IsSpace)
	if len(t) == 0 || t[0] != '<' {
		return 0
	}
	if hasPrefixFold(t, []byte("<!doctype html")) || hasPrefixFold(t, []byte("<html")) {
		return 0 // that's HTML
	}
	if bytes.HasPrefix(t, []byte("<?xml")) {
		return 90
	}
	// A bare element that also has a closing tag looks like XML. Bound the scan to a
	// head window (like the HTML detector) so a multi-MB non-XML or close-tag-far-in
	// file isn't scanned end-to-end on every auto-detect: a "</" within the first few
	// KB is a sufficient signal.
	head := t
	if len(head) > sniffLimit {
		head = head[:sniffLimit]
	}
	if len(t) > 1 && isNameStart(t[1]) && bytes.Contains(head, []byte("</")) {
		return 55
	}
	return 0
}

func isNameStart(b byte) bool {
	return b == '_' || b == ':' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func (xmlParser) Parse(src []byte, b *model.Builder) error {
	dec := xml.NewDecoder(bytes.NewReader(src))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	s := &xmlScanner{dec: dec, b: b}
	for {
		t, err := s.next()
		if err != nil {
			break // EOF or unrecoverable; model.Builder.finish closes danglers
		}
		switch tok := t.(type) {
		case xml.StartElement:
			s.parseElement(tok, s.tokStart)
		case xml.Comment:
			b.Leaf(model.KindComment, s.tokStart, s.tokEnd, []model.Seg{model.LitSeg(model.RoleComment, "<!-- "+collapseSpace(string(tok))+" -->")})
		case xml.ProcInst:
			b.Leaf(model.KindComment, s.tokStart, s.tokEnd, []model.Seg{procInstSeg(tok)})
		case xml.Directive:
			b.Leaf(model.KindComment, s.tokStart, s.tokEnd, []model.Seg{model.LitSeg(model.RoleComment, "<!"+collapseSpace(string(tok))+">")})
		case xml.CharData:
			if txt := collapseSpace(string(tok)); txt != "" {
				b.Leaf(model.KindText, s.tokStart, s.tokEnd, []model.Seg{model.LitSeg(model.RoleString, txt)})
			}
		}
	}
	return nil
}

type xmlScanner struct {
	dec     *xml.Decoder
	b       *model.Builder
	peeked  xml.Token
	hasPeek bool

	// Byte span of the most recent token from next() and the peeked token, via
	// dec.InputOffset(), so element nodes carry real Src ranges for copy-subtree.
	tokStart, tokEnd   int
	peekStart, peekEnd int
}

func (s *xmlScanner) next() (xml.Token, error) {
	if s.hasPeek {
		s.hasPeek = false
		s.tokStart, s.tokEnd = s.peekStart, s.peekEnd
		return s.peeked, nil
	}
	start := int(s.dec.InputOffset())
	t, err := s.dec.Token()
	if err != nil {
		return nil, err
	}
	s.tokStart, s.tokEnd = start, int(s.dec.InputOffset())
	return xml.CopyToken(t), nil
}

func (s *xmlScanner) peek() (xml.Token, bool) {
	if !s.hasPeek {
		start := int(s.dec.InputOffset())
		t, err := s.dec.Token()
		if err != nil {
			return nil, false
		}
		s.peeked = xml.CopyToken(t)
		s.peekStart, s.peekEnd = start, int(s.dec.InputOffset())
		s.hasPeek = true
	}
	return s.peeked, true
}

// parseElement builds one element. srcStart is the byte offset of its '<', so its
// node carries a real [SrcStart,SrcEnd) span into the source (for copy-subtree).
func (s *xmlScanner) parseElement(start xml.StartElement, srcStart int) {
	// Empty element: <tag/> or <tag></tag> — render inline as a single leaf.
	if t, ok := s.peek(); ok {
		if end, isEnd := t.(xml.EndElement); isEnd && end.Name == start.Name {
			s.hasPeek = false // consume the end token
			s.b.Leaf(model.KindEmptyElement, srcStart, s.peekEnd, startSegs(start, true))
			return
		}
	}

	// Bound recursion: past the nesting cap, render this element as a leaf marker
	// and drain its subtree (keeping the decoder balanced) instead of recursing,
	// so adversarial deeply-nested XML can't overflow the stack.
	if s.b.Depth() >= maxNestDepth {
		s.b.Leaf(model.KindError, srcStart, srcStart, startSegs(start, false))
		s.skipElement()
		return
	}

	s.b.Open(model.KindElement, srcStart, startSegs(start, false))
	for {
		t, err := s.next()
		if err != nil {
			return // Finish() closes the dangling element
		}
		switch tok := t.(type) {
		case xml.StartElement:
			s.parseElement(tok, s.tokStart)
		case xml.EndElement:
			s.b.Close(s.tokEnd, endSegs(start.Name.Local))
			return
		case xml.CharData:
			if txt := collapseSpace(string(tok)); txt != "" {
				s.b.Leaf(model.KindText, s.tokStart, s.tokEnd, []model.Seg{model.LitSeg(model.RoleString, txt)})
			}
		case xml.Comment:
			s.b.Leaf(model.KindComment, s.tokStart, s.tokEnd, []model.Seg{model.LitSeg(model.RoleComment, "<!-- "+collapseSpace(string(tok))+" -->")})
		case xml.ProcInst:
			s.b.Leaf(model.KindComment, s.tokStart, s.tokEnd, []model.Seg{procInstSeg(tok)})
		}
	}
}

// procInstSeg renders a processing instruction (<?target inst?>), keeping its
// instruction content. Shared by the top-level and in-element handlers so the two
// stay consistent.
func procInstSeg(tok xml.ProcInst) model.Seg {
	s := "<?" + tok.Target
	if inst := collapseSpace(string(tok.Inst)); inst != "" {
		s += " " + inst
	}
	return model.LitSeg(model.RoleComment, s+"?>")
}

// skipElement drains tokens until the end of the current element (whose start was
// already consumed), tracking nesting so the matching EndElement is found. Used to
// skip a subtree past the recursion cap without recursing or unbalancing the
// decoder.
func (s *xmlScanner) skipElement() {
	depth := 1
	for depth > 0 {
		t, err := s.next()
		if err != nil {
			return
		}
		switch t.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
}

// startSegs builds the segments for an opening (or self-closing) element line.
func startSegs(start xml.StartElement, selfClose bool) []model.Seg {
	segs := []model.Seg{model.LitSeg(model.RolePunct, "<"), model.LitSeg(model.RoleTag, escapeGridBreakers(start.Name.Local))}
	for _, a := range start.Attr {
		name := escapeGridBreakers(a.Name.Local)
		segs = append(segs,
			model.LitSeg(model.RolePlain, " "),
			model.LitSeg(model.RoleAttr, name),
			model.LitSeg(model.RolePunct, "="),
			model.LitSeg(model.RoleString, `"`+escapeGridBreakers(a.Value)+`"`),
		)
	}
	if selfClose {
		segs = append(segs, model.LitSeg(model.RolePunct, "/>"))
	} else {
		segs = append(segs, model.LitSeg(model.RolePunct, ">"))
	}
	return segs
}

func endSegs(name string) []model.Seg {
	return []model.Seg{model.LitSeg(model.RolePunct, "</"), model.LitSeg(model.RoleTag, escapeGridBreakers(name)), model.LitSeg(model.RolePunct, ">")}
}

// collapseSpace trims and collapses internal whitespace runs to single spaces, then
// C-escapes any remaining control bytes. Every caller turns the result into display
// text (XML/HTML char data, comments, processing instructions), so it must hold the
// one-row / uniform-grid invariant: FieldsFunc(unicode.IsSpace) removes \n/\t/\r, and
// escapeGridBreakers handles the non-whitespace C0/DEL bytes those leave behind
// (e.g. NUL, ESC) which would otherwise render as a stray glyph and desync the grid.
func collapseSpace(s string) string {
	return escapeGridBreakers(strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " "))
}
