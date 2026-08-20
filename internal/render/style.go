package render

import (
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
)

// mdpaneStyle is the default look: glamour's dark palette, but headings
// render as styled document headings instead of keeping their literal
// "##" markdown markers. A plan should read like a document, not source.
func mdpaneStyle() ansi.StyleConfig {
	cfg := styles.DarkStyleConfig

	noPrefix := ""
	boolTrue := true
	boolFalse := false

	// H1 keeps the highlighted title pill from the dark theme.
	// H2: bold, blue, underlined — a section heading.
	cfg.H2.Prefix = noPrefix
	cfg.H2.Underline = &boolTrue
	cfg.H2.Bold = &boolTrue
	// H3: bold cyan, no underline.
	cfg.H3.Prefix = noPrefix
	color3 := "44"
	cfg.H3.Color = &color3
	cfg.H3.Bold = &boolTrue
	// H4: bold default text color.
	cfg.H4.Prefix = noPrefix
	color4 := "252"
	cfg.H4.Color = &color4
	cfg.H4.Bold = &boolTrue
	// H5/H6: dim, still distinct from prose.
	cfg.H5.Prefix = noPrefix
	color56 := "245"
	cfg.H5.Color = &color56
	cfg.H6.Prefix = noPrefix
	cfg.H6.Color = &color56
	cfg.H6.Bold = &boolFalse

	return cfg
}
