package parse

import (
	"bytes"
	"unicode"

	"github.com/ideaconnect/go-fyne-pretty-view/internal/model"

	"golang.org/x/net/html"
)

// htmlParser parses HTML with x/net/html's tolerant tokenizer: it handles
// unclosed tags (auto-closing on a later end tag or at EOF), void elements, and
// raw structure without imposing a normalized DOM. Like XML, segments are
// interned literals (no zero-copy offsets).
type htmlParser struct{}

func (htmlParser) Format() Format { return FormatHTML }

func (htmlParser) Detect(src []byte) int {
	t := bytes.TrimLeftFunc(src, unicode.IsSpace)
	if len(t) == 0 || t[0] != '<' {
		return 0
	}
	if hasPrefixFold(t, []byte("<!doctype html")) {
		return 95
	}
	if hasPrefixFold(t, []byte("<html")) {
		return 90
	}
	// Only the head needs sniffing for structural tags; bound the lower-case + scan
	// so a large file is not fully copied just to detect the format.
	head := t
	if len(head) > sniffLimit {
		head = head[:sniffLimit]
	}
	lower := bytes.ToLower(head)
	for _, tag := range [][]byte{[]byte("<body"), []byte("<head"), []byte("<div"), []byte("<span"), []byte("<p>"), []byte("<a ")} {
		if bytes.Contains(lower, tag) {
			return 60
		}
	}
	return 0
}

func (htmlParser) Parse(src []byte, b *model.Builder) error {
	z := html.NewTokenizer(bytes.NewReader(src))
	var names []string // open element names, mirroring the model.Builder's container stack

	closeTo := func(name string) {
		idx := -1
		for i := len(names) - 1; i >= 0; i-- {
			if names[i] == name {
				idx = i
				break
			}
		}
		if idx < 0 {
			return // stray end tag; ignore
		}
		for len(names) > idx {
			nm := names[len(names)-1]
			names = names[:len(names)-1]
			b.Close(0, endSegs(nm))
		}
	}

	havePending := false
	var pendTT html.TokenType
	var pendTok html.Token
	nextToken := func() (html.TokenType, html.Token) {
		if havePending {
			havePending = false
			return pendTT, pendTok
		}
		tt := z.Next()
		return tt, z.Token()
	}

	for {
		tt, tok := nextToken()
		if tt == html.ErrorToken {
			break // EOF or read error; Finish() closes danglers
		}
		switch tt {
		case html.StartTagToken:
			switch {
			case isVoidElement(tok.Data):
				b.Leaf(model.KindEmptyElement, 0, 0, htmlStartSegs(tok, false))
			default:
				// Peek one token: a start tag immediately followed by its matching end
				// tag is an empty element, emitted inline (non-foldable) like the XML
				// path, rather than a foldable node that collapses to "0 children".
				ntt, ntok := nextToken()
				if ntt == html.EndTagToken && ntok.Data == tok.Data {
					b.Leaf(model.KindEmptyElement, 0, 0, htmlStartSegs(tok, true))
				} else {
					b.Open(model.KindElement, 0, htmlStartSegs(tok, false))
					names = append(names, tok.Data)
					havePending, pendTT, pendTok = true, ntt, ntok // re-process the peeked token
				}
			}
		case html.SelfClosingTagToken:
			b.Leaf(model.KindEmptyElement, 0, 0, htmlStartSegs(tok, true))
		case html.EndTagToken:
			closeTo(tok.Data)
		case html.TextToken:
			if txt := collapseSpace(tok.Data); txt != "" {
				b.Leaf(model.KindText, 0, 0, []model.Seg{model.LitSeg(model.RoleString, txt)})
			}
		case html.CommentToken:
			b.Leaf(model.KindComment, 0, 0, []model.Seg{model.LitSeg(model.RoleComment, "<!-- "+collapseSpace(tok.Data)+" -->")})
		case html.DoctypeToken:
			b.Leaf(model.KindComment, 0, 0, []model.Seg{model.LitSeg(model.RoleComment, "<!DOCTYPE "+collapseSpace(tok.Data)+">")})
		}
	}
	return nil
}

func htmlStartSegs(tok html.Token, selfClose bool) []model.Seg {
	segs := []model.Seg{model.LitSeg(model.RolePunct, "<"), model.LitSeg(model.RoleTag, tok.Data)}
	for _, a := range tok.Attr {
		segs = append(segs, model.LitSeg(model.RolePlain, " "), model.LitSeg(model.RoleAttr, a.Key))
		if a.Val != "" {
			segs = append(segs, model.LitSeg(model.RolePunct, "="), model.LitSeg(model.RoleString, `"`+a.Val+`"`))
		}
	}
	if selfClose {
		segs = append(segs, model.LitSeg(model.RolePunct, "/>"))
	} else {
		segs = append(segs, model.LitSeg(model.RolePunct, ">"))
	}
	return segs
}

// isVoidElement reports whether name is an HTML void element (no closing tag).
func isVoidElement(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}
