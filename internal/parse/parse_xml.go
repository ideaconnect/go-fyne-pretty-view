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
	// A bare element that also has a closing tag looks like XML.
	if len(t) > 1 && (isNameStart(t[1])) && bytes.Contains(t, []byte("</")) {
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
			s.parseElement(tok)
		case xml.Comment:
			b.Leaf(model.KindComment, 0, 0, []model.Seg{model.LitSeg(model.RoleComment, "<!-- "+collapseSpace(string(tok))+" -->")})
		case xml.ProcInst:
			b.Leaf(model.KindComment, 0, 0, []model.Seg{procInstSeg(tok)})
		case xml.Directive:
			b.Leaf(model.KindComment, 0, 0, []model.Seg{model.LitSeg(model.RoleComment, "<!"+collapseSpace(string(tok))+">")})
		case xml.CharData:
			if txt := collapseSpace(string(tok)); txt != "" {
				b.Leaf(model.KindText, 0, 0, []model.Seg{model.LitSeg(model.RoleString, txt)})
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
			s.b.Leaf(model.KindEmptyElement, 0, 0, startSegs(start, true))
			return
		}
	}

	// Bound recursion: past the nesting cap, render this element as a leaf marker
	// and drain its subtree (keeping the decoder balanced) instead of recursing,
	// so adversarial deeply-nested XML can't overflow the stack.
	if s.b.Depth() >= maxNestDepth {
		s.b.Leaf(model.KindError, 0, 0, startSegs(start, false))
		s.skipElement()
		return
	}

	s.b.Open(model.KindElement, 0, startSegs(start, false))
	for {
		t, err := s.next()
		if err != nil {
			return // Finish() closes the dangling element
		}
		switch tok := t.(type) {
		case xml.StartElement:
			s.parseElement(tok)
		case xml.EndElement:
			s.b.Close(0, endSegs(start.Name.Local))
			return
		case xml.CharData:
			if txt := collapseSpace(string(tok)); txt != "" {
				s.b.Leaf(model.KindText, 0, 0, []model.Seg{model.LitSeg(model.RoleString, txt)})
			}
		case xml.Comment:
			s.b.Leaf(model.KindComment, 0, 0, []model.Seg{model.LitSeg(model.RoleComment, "<!-- "+collapseSpace(string(tok))+" -->")})
		case xml.ProcInst:
			s.b.Leaf(model.KindComment, 0, 0, []model.Seg{procInstSeg(tok)})
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
	segs := []model.Seg{model.LitSeg(model.RolePunct, "<"), model.LitSeg(model.RoleTag, start.Name.Local)}
	for _, a := range start.Attr {
		name := a.Name.Local
		segs = append(segs,
			model.LitSeg(model.RolePlain, " "),
			model.LitSeg(model.RoleAttr, name),
			model.LitSeg(model.RolePunct, "="),
			model.LitSeg(model.RoleString, `"`+a.Value+`"`),
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
	return []model.Seg{model.LitSeg(model.RolePunct, "</"), model.LitSeg(model.RoleTag, name), model.LitSeg(model.RolePunct, ">")}
}

// collapseSpace trims and collapses internal whitespace runs to single spaces.
func collapseSpace(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}
