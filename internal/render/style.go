package render

import (
	"strings"

	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
)

// mdpaneStyle is the default look: GitHub-flavored document rendering
// adapted for the terminal. Headings are bold bright text (no literal "#"
// markers, no background pills), and H1/H2 get a dim full-width rule
// beneath them — the terminal translation of GitHub's border-bottom.
//
// The rule width must match the render width, and glamour styles are
// static strings, so the style is rebuilt alongside the renderer whenever
// the width changes.
func mdpaneStyle(width int) ansi.StyleConfig {
	cfg := styles.DarkStyleConfig

	empty := ""
	boolTrue := true
	boolFalse := false
	bright := "255"
	body := "252"
	dim := "246"
	ruleColor := "240"

	// Headings: GitHub renders them as bold foreground text, largest on
	// top, with no decoration besides the H1/H2 rule (added via the
	// preprocessor + hr style below).
	cfg.Heading.Color = &bright
	cfg.Heading.Bold = &boolTrue

	cfg.H1.Prefix = empty
	cfg.H1.Suffix = empty
	cfg.H1.Color = &bright
	cfg.H1.BackgroundColor = nil // no pill
	cfg.H1.Bold = &boolTrue

	cfg.H2.Prefix = empty
	cfg.H2.Color = &bright
	cfg.H2.Bold = &boolTrue
	cfg.H2.Underline = nil

	cfg.H3.Prefix = empty
	cfg.H3.Color = &bright
	cfg.H3.Bold = &boolTrue

	cfg.H4.Prefix = empty
	cfg.H4.Color = &body
	cfg.H4.Bold = &boolTrue

	cfg.H5.Prefix = empty
	cfg.H5.Color = &dim
	cfg.H5.Bold = &boolTrue

	cfg.H6.Prefix = empty
	cfg.H6.Color = &dim
	cfg.H6.Bold = &boolFalse

	// Full-width dim rule; also used under H1/H2 via the preprocessor.
	// The document style carries a 2-column margin on each side, so the
	// rule must be narrower than the wrap width or it wraps onto a
	// second line.
	if ruleWidth := width - 4; ruleWidth >= 10 {
		cfg.HorizontalRule.Format = "\n" + strings.Repeat("─", ruleWidth) + "\n"
	}
	cfg.HorizontalRule.Color = &ruleColor

	return cfg
}

// githubRules inserts a horizontal rule after every H1/H2 heading (outside
// fenced code blocks), reproducing GitHub's heading border-bottom.
func githubRules(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines)+8)
	inFence := false
	for i, line := range lines {
		out = append(out, line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if isH1orH2(trimmed) && !nextIsRule(lines, i) {
			out = append(out, "", "---")
		}
	}
	return strings.Join(out, "\n")
}

func isH1orH2(line string) bool {
	if strings.HasPrefix(line, "# ") || line == "#" {
		return true
	}
	return strings.HasPrefix(line, "## ") || line == "##"
}

func nextIsRule(lines []string, i int) bool {
	for j := i + 1; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		if t == "" {
			continue
		}
		return t == "---" || t == "***" || t == "___"
	}
	return false
}
