package prettyview

import (
	"bytes"
	"encoding/xml"
	"strings"
	"unicode"
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
	lower := bytes.ToLower(t)
	if bytes.HasPrefix(lower, []byte("<!doctype html")) || bytes.HasPrefix(lower, []byte("<html")) {
		return 0 // that's HTML
	}
	if bytes.HasPrefix(t, []byte("<?xml")) {
		return 90
	}
	// A bare element that also has a closing tag looks like XML.
	if len(t) > 1 && (isNameStart(t[1])) && bytes.Contains(t, []byte("</")) {
		return 55
	}
	return 0
}

func isNameStart(b byte) bool {
	return b == '_' || b == ':' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func (xmlParser) Parse(src []byte, b *Builder) error {
	dec := xml.NewDecoder(bytes.NewReader(src))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	s := &xmlScanner{dec: dec, b: b}
	for {
		t, err := s.next()
		if err != nil {
			break // EOF or unrecoverable; Builder.finish closes danglers
		}
		switch tok := t.(type) {
		case xml.StartElement:
			s.parseElement(tok)
		case xml.Comment:
			b.Leaf(KindComment, 0, 0, []Seg{litSeg(RoleComment, "<!-- "+collapseSpace(string(tok))+" -->")})
		case xml.ProcInst:
			b.Leaf(KindComment, 0, 0, []Seg{litSeg(RoleComment, "<?"+tok.Target+" "+collapseSpace(string(tok.Inst))+"?>")})
		case xml.Directive:
			b.Leaf(KindComment, 0, 0, []Seg{litSeg(RoleComment, "<!"+collapseSpace(string(tok))+">")})
		case xml.CharData:
			if txt := collapseSpace(string(tok)); txt != "" {
				b.Leaf(KindText, 0, 0, []Seg{litSeg(RoleString, txt)})
			}
		}
	}
	return nil
}

type xmlScanner struct {
	dec     *xml.Decoder
	b       *Builder
	peeked  xml.Token
	hasPeek bool
}

func (s *xmlScanner) next() (xml.Token, error) {
	if s.hasPeek {
		s.hasPeek = false
		return s.peeked, nil
	}
	t, err := s.dec.Token()
	if err != nil {
		return nil, err
	}
	return xml.CopyToken(t), nil
}

func (s *xmlScanner) peek() (xml.Token, bool) {
	if !s.hasPeek {
		t, err := s.dec.Token()
		if err != nil {
			return nil, false
		}
		s.peeked = xml.CopyToken(t)
		s.hasPeek = true
	}
	return s.peeked, true
}

func (s *xmlScanner) parseElement(start xml.StartElement) {
	// Empty element: <tag/> or <tag></tag> — render inline as a single leaf.
	if t, ok := s.peek(); ok {
		if end, isEnd := t.(xml.EndElement); isEnd && end.Name == start.Name {
			s.hasPeek = false // consume the end token
			s.b.Leaf(KindEmptyElement, 0, 0, startSegs(start, true))
			return
		}
	}

	s.b.Open(KindElement, 0, startSegs(start, false))
	for {
		t, err := s.next()
		if err != nil {
			return // finish() closes the dangling element
		}
		switch tok := t.(type) {
		case xml.StartElement:
			s.parseElement(tok)
		case xml.EndElement:
			s.b.Close(0, endSegs(start.Name.Local))
			return
		case xml.CharData:
			if txt := collapseSpace(string(tok)); txt != "" {
				s.b.Leaf(KindText, 0, 0, []Seg{litSeg(RoleString, txt)})
			}
		case xml.Comment:
			s.b.Leaf(KindComment, 0, 0, []Seg{litSeg(RoleComment, "<!-- "+collapseSpace(string(tok))+" -->")})
		case xml.ProcInst:
			s.b.Leaf(KindComment, 0, 0, []Seg{litSeg(RoleComment, "<?"+tok.Target+"?>")})
		}
	}
}

// startSegs builds the segments for an opening (or self-closing) element line.
func startSegs(start xml.StartElement, selfClose bool) []Seg {
	segs := []Seg{litSeg(RolePunct, "<"), litSeg(RoleTag, start.Name.Local)}
	for _, a := range start.Attr {
		name := a.Name.Local
		if a.Name.Space != "" && (a.Name.Space == "xmlns" || a.Name.Local == "xmlns") {
			name = a.Name.Local
		}
		segs = append(segs,
			litSeg(RolePlain, " "),
			litSeg(RoleAttr, name),
			litSeg(RolePunct, "="),
			litSeg(RoleString, `"`+a.Value+`"`),
		)
	}
	if selfClose {
		segs = append(segs, litSeg(RolePunct, "/>"))
	} else {
		segs = append(segs, litSeg(RolePunct, ">"))
	}
	return segs
}

func endSegs(name string) []Seg {
	return []Seg{litSeg(RolePunct, "</"), litSeg(RoleTag, name), litSeg(RolePunct, ">")}
}

// collapseSpace trims and collapses internal whitespace runs to single spaces.
func collapseSpace(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}
