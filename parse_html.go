package prettyview

import (
	"bytes"
	"unicode"

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
	lower := bytes.ToLower(t)
	switch {
	case bytes.HasPrefix(lower, []byte("<!doctype html")):
		return 95
	case bytes.HasPrefix(lower, []byte("<html")):
		return 90
	}
	for _, tag := range [][]byte{[]byte("<body"), []byte("<head"), []byte("<div"), []byte("<span"), []byte("<p>"), []byte("<a ")} {
		if bytes.Contains(lower, tag) {
			return 60
		}
	}
	return 0
}

func (htmlParser) Parse(src []byte, b *Builder) error {
	z := html.NewTokenizer(bytes.NewReader(src))
	var names []string // open element names, mirroring the Builder's container stack

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

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break // EOF or read error; finish() closes danglers
		}
		tok := z.Token()
		switch tt {
		case html.StartTagToken:
			if isVoidElement(tok.Data) {
				b.Leaf(KindEmptyElement, 0, 0, htmlStartSegs(tok, false))
			} else {
				b.Open(KindElement, 0, htmlStartSegs(tok, false))
				names = append(names, tok.Data)
			}
		case html.SelfClosingTagToken:
			b.Leaf(KindEmptyElement, 0, 0, htmlStartSegs(tok, true))
		case html.EndTagToken:
			closeTo(tok.Data)
		case html.TextToken:
			if txt := collapseSpace(tok.Data); txt != "" {
				b.Leaf(KindText, 0, 0, []Seg{litSeg(RoleString, txt)})
			}
		case html.CommentToken:
			b.Leaf(KindComment, 0, 0, []Seg{litSeg(RoleComment, "<!-- "+collapseSpace(tok.Data)+" -->")})
		case html.DoctypeToken:
			b.Leaf(KindComment, 0, 0, []Seg{litSeg(RoleComment, "<!DOCTYPE "+collapseSpace(tok.Data)+">")})
		}
	}
	return nil
}

func htmlStartSegs(tok html.Token, selfClose bool) []Seg {
	segs := []Seg{litSeg(RolePunct, "<"), litSeg(RoleTag, tok.Data)}
	for _, a := range tok.Attr {
		segs = append(segs, litSeg(RolePlain, " "), litSeg(RoleAttr, a.Key))
		if a.Val != "" {
			segs = append(segs, litSeg(RolePunct, "="), litSeg(RoleString, `"`+a.Val+`"`))
		}
	}
	if selfClose {
		segs = append(segs, litSeg(RolePunct, "/>"))
	} else {
		segs = append(segs, litSeg(RolePunct, ">"))
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
