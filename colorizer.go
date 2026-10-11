package prettyview

import "github.com/ideaconnect/go-fyne-pretty-view/v2/internal/model"

// ColorRole names one of the syntax color slots of the active Theme. It is an alias of the
// model package's type, like Format, so host colorizers and the document model share it.
type ColorRole = model.ColorRole

// The color roles a Colorizer can assign. Each resolves to the Theme field named in its
// comment, so a host grammar picks up WithTheme / WithSyntaxColors overrides for free.
const (
	RolePlain   = model.RolePlain   // Theme.Foreground: default text
	RoleKey     = model.RoleKey     // Theme.Key: keys, names
	RoleString  = model.RoleString  // Theme.String: string literals, values
	RoleNumber  = model.RoleNumber  // Theme.Number: numbers
	RoleBool    = model.RoleBool    // Theme.Bool: booleans, flags
	RoleNull    = model.RoleNull    // Theme.Null: null / empty markers
	RolePunct   = model.RolePunct   // Theme.Punct: punctuation, delimiters
	RoleTag     = model.RoleTag     // Theme.Tag: tags, section headers, schemes
	RoleAttr    = model.RoleAttr    // Theme.Attr: attributes, parameters
	RoleComment = model.RoleComment // Theme.Comment: comments
	RoleMuted   = model.RoleMuted   // Theme.Summary: de-emphasized text
)

// ColorSpan colors the bytes [Start, End) of the text a Colorizer is given with Role.
// Offsets are byte offsets, not rune or column indices.
type ColorSpan = model.ColorSpan

// Colorizer is a host-supplied syntax colorizer for raw text (see WithColorizer). It reports
// the color spans of src by APPENDING them to dst and returning the extended slice, like
// strconv.AppendInt; the widget hands the previous call's slice back as dst, so a colorizer
// that appends allocates nothing in steady state. Bytes no span covers are RolePlain.
//
// Spans need not be sorted and may overlap or run out of range: the widget sorts them by
// Start, resolves overlaps first-wins, clamps them to src, maps an unknown role to RolePlain,
// and only changes color at the start of a rune. A colorizer that panics leaves the text
// uncolored instead of crashing the widget.
//
// src is the whole document (or edit buffer) and is only valid during the call: do not
// retain or modify it. In an editable widget the colorizer runs on every keystroke, on the
// UI goroutine, for buffers up to the live-color budget (2 MiB); above it the buffer is shown
// uncolored, like the built-in formats.
type Colorizer func(dst []ColorSpan, src []byte) []ColorSpan

// WithColorizer colors FormatRaw text with a host grammar. It applies whenever the effective
// format is FormatRaw: an explicit FormatRaw, or FormatAuto when detection finds no JSON /
// XML / HTML. Structured formats keep their built-in highlighting, folding and reformatting,
// and a structured document that fails to parse still falls back to plain raw lines. It
// works in both modes: the read-only viewer colors each raw line (tabs still expand to
// WithTabWidth), and an editable widget colors its live projection on every keystroke while
// keeping display runes 1:1 with buffer runes. Lines longer than 16 KiB stay uncolored, as
// with the built-in editable colorizer. A nil c is ignored.
func WithColorizer(c Colorizer) Option {
	return func(cfg *config) {
		if c != nil {
			cfg.colorizer = c
		}
	}
}
