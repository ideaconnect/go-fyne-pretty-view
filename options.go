package prettyview

import (
	"time"

	"fyne.io/fyne/v2"
)

// SearchConfig tunes the incremental search behavior. The scan is synchronous on
// the Fyne goroutine (debounced, and bounded by MaxMatches); see runSearch.
type SearchConfig struct {
	MaxMatches  int           // cap on stored matches (default 10_000)
	DebounceFor time.Duration // keystroke debounce (default 150ms)
	MinQueryLen int           // shortest query that triggers a scan (default 1)
}

func defaultSearchConfig() SearchConfig {
	return SearchConfig{
		MaxMatches:  10_000,
		DebounceFor: 150 * time.Millisecond,
		MinQueryLen: 1,
	}
}

// config holds all construction-time settings. It is populated by Options.
type config struct {
	format        Format
	wrap          WrapMode
	search        SearchConfig
	collapseDepth int // auto-collapse containers deeper than this on load (0 = never)
	tabWidth      int
	indentStep    float32
	maxInputBytes int  // cap on SetData/SetText input; 0 = no cap (the 4 GiB model ceiling)
	lineNumbers   bool // render an opt-in line-number gutter
	editable      bool // construct as an editor (input) rather than a read-only viewer
	themeOverride map[fyne.ThemeVariant]Theme
}

// setThemeOverride merges t's non-nil fields into the per-variant override, so
// WithTheme / WithSyntaxColors / SetTheme compose rather than clobber.
func (c *config) setThemeOverride(variant fyne.ThemeVariant, t Theme) {
	if c.themeOverride == nil {
		c.themeOverride = map[fyne.ThemeVariant]Theme{}
	}
	cur := c.themeOverride[variant]
	t.mergeInto(&cur)
	c.themeOverride[variant] = cur
}

func defaultConfig() config {
	return config{
		format:        FormatAuto,
		wrap:          WrapNone,
		search:        defaultSearchConfig(),
		collapseDepth: 0,
		tabWidth:      4,
		indentStep:    16,
	}
}

// Option customizes a PrettyView at construction time.
type Option func(*config)

// WithFormat forces a specific input format, skipping auto-detection.
func WithFormat(f Format) Option { return func(c *config) { c.format = f } }

// WithWrap selects the long-line handling mode (default WrapNone): WrapNone lets
// long lines overflow and scroll horizontally (matching Bruno), WrapWord soft-wraps
// them to the viewport width. The mode can also be changed at runtime with SetWrap.
func WithWrap(m WrapMode) Option { return func(c *config) { c.wrap = m } }

// WithSearchConfig overrides the search tuning parameters, merging field-by-field:
// each NON-ZERO field of s replaces the default, and a zero field keeps its default
// (MaxMatches 10_000, DebounceFor 150ms, MinQueryLen 1). So setting only MaxMatches
// leaves debouncing at its default — there is no zero-value trap. To DISABLE keystroke
// debouncing, set a negative DebounceFor (e.g. -1), or drive input through Search,
// which always scans immediately (SearchDebounced is the coalescing path).
func WithSearchConfig(s SearchConfig) Option {
	return func(c *config) { s.mergeInto(&c.search) }
}

// mergeInto copies s's non-zero fields over dst (whose fields are the defaults), so a
// zero field keeps its default. A negative DebounceFor is non-zero, so it survives the
// merge and disables debouncing (searchDebounced scans immediately for DebounceFor<=0).
func (s SearchConfig) mergeInto(dst *SearchConfig) {
	if s.MaxMatches != 0 {
		dst.MaxMatches = s.MaxMatches
	}
	if s.DebounceFor != 0 {
		dst.DebounceFor = s.DebounceFor
	}
	if s.MinQueryLen != 0 {
		dst.MinQueryLen = s.MinQueryLen
	}
}

// WithDefaultCollapseDepth auto-collapses every container at nesting depth d or
// deeper on load. Top-level containers are at depth 0, so d=1 collapses everything
// below the root and d=0 (or any d <= 0) disables auto-collapse.
func WithDefaultCollapseDepth(d int) Option {
	return func(c *config) {
		if d < 0 {
			d = 0
		}
		c.collapseDepth = d
	}
}

// WithTabWidth sets the display width of a tab character (default 4).
func WithTabWidth(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.tabWidth = n
		}
	}
}

// WithMaxInputBytes caps the source size SetData/SetText will load. Input longer than
// n bytes is truncated to n before parsing (the parsers are tolerant, so a truncated
// document still renders). It bounds the synchronous parse work and the resulting
// model size (≈5–7× the source, built on the calling/Fyne goroutine) for untrusted or
// unbounded input. n <= 0 (the default) imposes no cap beyond the model's 4 GiB
// source ceiling.
func WithMaxInputBytes(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.maxInputBytes = n
		}
	}
}

// WithLineNumbers renders a line-number gutter to the left of the content. Numbers
// are the logical display-line indices (1-based) drawn from the struct-of-arrays
// model, so no extra widgets are created per line and the virtualization invariant
// holds; wrap-continuation rows leave the gutter blank. Off by default.
func WithLineNumbers() Option {
	return func(c *config) { c.lineNumbers = true }
}

// WithEditable constructs the widget as an editor (input) rather than the default
// read-only viewer (output): the host can let a user type or paste data and edit it
// in place. The input-vs-output purpose is fixed at construction and CANNOT be changed
// afterwards — there is deliberately no SetEditable and the widget renders no view/edit
// toggle in its chrome ("its purpose, input or output, should be defined, not user
// changeable"; see docs/DESIGN.md §12.3). Read Editable to query the constructed mode;
// a host-only runtime flip is a deferred future feature. Off by default, so a read-only
// widget behaves byte-for-byte like a v1 viewer.
func WithEditable() Option {
	return func(c *config) { c.editable = true }
}

// WithIndentStep sets the pixels of indentation per nesting level.
func WithIndentStep(px float32) Option {
	return func(c *config) {
		if px > 0 {
			c.indentStep = px
		}
	}
}

// WithTheme overrides any of the viewer's colors for a theme variant. Nil fields
// keep their (Fyne-theme-tracking) defaults; calls compose. Pass the variant your
// app uses (theme.VariantDark / theme.VariantLight), or set both.
func WithTheme(v fyne.ThemeVariant, t Theme) Option {
	return func(c *config) { c.setThemeOverride(v, t) }
}

// WithSyntaxColors overrides just the syntax token colors for a theme variant
// (shorthand for WithTheme with only the token fields set).
func WithSyntaxColors(v fyne.ThemeVariant, s SyntaxColors) Option {
	return func(c *config) { c.setThemeOverride(v, s.asTheme()) }
}
