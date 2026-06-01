package prettyview

import (
	"time"

	"fyne.io/fyne/v2"
)

// SearchConfig tunes the incremental search behavior.
type SearchConfig struct {
	MaxMatches  int           // cap on stored matches (default 10_000)
	DebounceFor time.Duration // keystroke debounce (default 150ms)
	ChunkBytes  int           // bytes scanned per cooperative slice (default 256 KiB)
	MinQueryLen int           // shortest query that triggers a scan (default 1)
}

func defaultSearchConfig() SearchConfig {
	return SearchConfig{
		MaxMatches:  10_000,
		DebounceFor: 150 * time.Millisecond,
		ChunkBytes:  256 << 10,
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

// WithWrap selects the long-line handling mode (default WrapNone).
func WithWrap(m WrapMode) Option { return func(c *config) { c.wrap = m } }

// WithSearchConfig overrides the search tuning parameters.
func WithSearchConfig(s SearchConfig) Option { return func(c *config) { c.search = s } }

// WithDefaultCollapseDepth auto-collapses every container deeper than d on load.
func WithDefaultCollapseDepth(d int) Option { return func(c *config) { c.collapseDepth = d } }

// WithTabWidth sets the display width of a tab character (default 4).
func WithTabWidth(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.tabWidth = n
		}
	}
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
