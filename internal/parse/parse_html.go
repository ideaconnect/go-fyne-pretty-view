package parse

import (
	"bytes"
	"unicode"

	"github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"

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
	// An HTML fragment without a doctype/<html> wrapper: treat it as HTML only when it
	// OPENS with a structural tag (the tag name followed by a boundary). Matching the
	// document start — rather than a substring anywhere — keeps an XML document that
	// merely *contains* an HTML-ish tag name (in text, an attribute, or a longer
	// element like <divider>) from being misclassified as HTML and having its tag-name
	// case folded. A real fragment leads with its structural tag; a full page is caught
	// by the doctype/<html> checks above.
	for _, name := range []string{"body", "head", "div", "span", "p", "a"} {
		if htmlTagAtStart(t, name) {
			return 60
		}
	}
	return 0
}

// htmlTagAtStart reports whether t opens with the element "<name" (case-insensitive)
// immediately followed by a tag-name boundary — whitespace, '>', '/', or end of input
// — so "<div" matches "<div>" / "<div " but not "<divider>".
func htmlTagAtStart(t []byte, name string) bool {
	if len(t) < 1+len(name) || t[0] != '<' || !bytes.EqualFold(t[1:1+len(name)], []byte(name)) {
		return false
	}
	if len(t) == 1+len(name) {
		return true
	}
	switch t[1+len(name)] {
	case ' ', '\t', '\n', '\r', '>', '/':
		return true
	}
	return false
}

func (htmlParser) Parse(src []byte, b *model.Builder) error {
	z := html.NewTokenizer(bytes.NewReader(src))
	var names []string // open element names, mirroring the model.Builder's container stack

	closeTo := func(name string, srcEnd int) {
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
			b.Close(srcEnd, endSegs(nm))
		}
	}

	havePending := false
	var pendTT html.TokenType
	var pendTok html.Token
	// Byte spans via summing z.Raw() lengths, so element nodes carry real Src ranges
	// into the source (for copy-subtree). tokStart/tokEnd is the span of the token
	// returned by nextToken; pendStart/pendEnd that of a one-token lookahead.
	off, tokStart, tokEnd := 0, 0, 0
	var pendStart, pendEnd int
	nextToken := func() (html.TokenType, html.Token) {
		if havePending {
			havePending = false
			tokStart, tokEnd = pendStart, pendEnd
			return pendTT, pendTok
		}
		tt := z.Next()
		tokStart = off
		off += len(z.Raw())
		tokEnd = off
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
				b.Leaf(model.KindEmptyElement, tokStart, tokEnd, htmlStartSegs(tok, false))
			case b.Depth() >= maxNestDepth:
				// Past the nesting cap: emit as a non-foldable leaf so adversarial
				// deeply-nested HTML stays bounded (its end tag finds no open match in
				// names and is ignored). HTML's tokenizer is iterative, so unlike the
				// recursive JSON/XML parsers this guards model shape, not a stack overflow.
				b.Leaf(model.KindEmptyElement, tokStart, tokEnd, htmlStartSegs(tok, false))
			default:
				// Peek one token: a start tag immediately followed by its matching end
				// tag is an empty element, emitted inline (non-foldable) like the XML
				// path, rather than a foldable node that collapses to "0 children".
				// Whitespace-only text in between renders as nothing, so <tag>  </tag>
				// is still empty and inlines too (#102) — skip blank text first.
				startOff := tokStart // the start tag's offset, captured before peeking
				ntt, ntok := nextToken()
				for ntt == html.TextToken && collapseSpace(ntok.Data) == "" {
					ntt, ntok = nextToken()
				}
				if ntt == html.EndTagToken && ntok.Data == tok.Data {
					b.Leaf(model.KindEmptyElement, startOff, tokEnd, htmlStartSegs(tok, true))
				} else {
					b.Open(model.KindElement, startOff, htmlStartSegs(tok, false))
					names = append(names, tok.Data)
					pendStart, pendEnd = tokStart, tokEnd          // restore the peeked token's span when re-read
					havePending, pendTT, pendTok = true, ntt, ntok // re-process the peeked token
				}
			}
		case html.SelfClosingTagToken:
			b.Leaf(model.KindEmptyElement, tokStart, tokEnd, htmlStartSegs(tok, true))
		case html.EndTagToken:
			closeTo(tok.Data, tokEnd)
		case html.TextToken:
			if txt := collapseSpace(tok.Data); txt != "" {
				id := b.Leaf(model.KindText, tokStart, tokEnd, []model.Seg{model.LitSeg(model.RoleString, txt)})
				// <script>/<style> bodies are raw text: their bytes are not entity-decoded, so
				// re-escaping '<' to "&lt;" on Reformat would corrupt the JS/CSS. Mark them so
				// serialization emits the source verbatim (#85).
				if len(names) > 0 && isRawTextElement(names[len(names)-1]) {
					b.MarkRawText(id)
				}
			}
		case html.CommentToken:
			b.Leaf(model.KindComment, tokStart, tokEnd, []model.Seg{model.LitSeg(model.RoleComment, "<!-- "+collapseSpace(tok.Data)+" -->")})
		case html.DoctypeToken:
			b.Leaf(model.KindComment, tokStart, tokEnd, []model.Seg{model.LitSeg(model.RoleComment, "<!DOCTYPE "+collapseSpace(tok.Data)+">")})
		}
	}
	return nil
}

func htmlStartSegs(tok html.Token, selfClose bool) []model.Seg {
	segs := []model.Seg{model.LitSeg(model.RolePunct, "<"), model.LitSeg(model.RoleTag, escapeGridBreakers(tok.Data))}
	for _, a := range tok.Attr {
		segs = append(segs, model.LitSeg(model.RolePlain, " "), model.LitSeg(model.RoleAttr, escapeGridBreakers(a.Key)))
		if a.Val != "" {
			segs = append(segs, model.LitSeg(model.RolePunct, "="), model.LitSeg(model.RoleString, `"`+escapeGridBreakers(a.Val)+`"`))
		}
	}
	if selfClose {
		segs = append(segs, model.LitSeg(model.RolePunct, "/>"))
	} else {
		segs = append(segs, model.LitSeg(model.RolePunct, ">"))
	}
	return segs
}

// isRawTextElement reports whether an element's text content is raw (CDATA-like): its bytes are
// not entity-decoded by the HTML parser, so they must round-trip verbatim, never re-escaped, on
// Reformat (#85). script and style are HTML's raw-text elements; textarea/title are "escapable
// raw text" whose entities ARE decoded, so they are treated as ordinary (escaped) text.
func isRawTextElement(name string) bool { return name == "script" || name == "style" }

// isVoidElement reports whether name is an HTML void element (no closing tag).
func isVoidElement(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}
