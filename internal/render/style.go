package render

import (
	"strings"

	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
)

// GitHub dark palette, taken verbatim from github-markdown-css 5.3.0
// (the stylesheet GitHub itself ships; also what web markdown viewers use).
const (
	ghText       = "#e6edf3" // body + headings
	ghMuted      = "#7d8590" // h6, blockquote text
	ghLink       = "#2f81f7" // links, no underline
	ghBorder     = "#30363d" // blockquote bar; used for rules too (see below)
	ghCodeChipBg = "#343941" // inline code: rgba(110,118,129,.4) over #0d1117
)

// mdpaneStyle is the default look: GitHub dark markdown rendering,
// translated to the terminal. Headings are weight-only (no "#" markers, no
// pills); H1/H2 carry a full-width hairline rule (GitHub's border-bottom).
//
// One deliberate deviation: GitHub's rule color #21262d is designed for a
// #0d1117 page background and disappears on many terminal backgrounds, so
// rules use the one-step-up border color #30363d.
//
// Styles are static strings, so the config is rebuilt with the renderer
// whenever the width changes (the rule must span the content width).
func mdpaneStyle(width int) ansi.StyleConfig {
	cfg := styles.DarkStyleConfig

	empty := ""
	boolTrue := true
	text := ghText
	muted := ghMuted
	link := ghLink
	border := ghBorder
	chipBg := ghCodeChipBg

	cfg.Document.Color = &text

	// Headings: font-weight 600, default foreground. Hierarchy in a
	// browser is font size; in a terminal it's the H1/H2 rules plus
	// H6's muted color, exactly as far as GitHub's own palette goes.
	for _, h := range []*ansi.StyleBlock{
		&cfg.Heading, &cfg.H1, &cfg.H2, &cfg.H3, &cfg.H4, &cfg.H5,
	} {
		h.Prefix = empty
		h.Suffix = empty
		h.Color = &text
		h.BackgroundColor = nil
		h.Bold = &boolTrue
		h.Underline = nil
	}
	cfg.H6.Prefix = empty
	cfg.H6.Color = &muted
	cfg.H6.Bold = &boolTrue

	// Links: GitHub blue, no underline (GitHub underlines on hover only).
	boolFalse := false
	cfg.Link.Color = &link
	cfg.Link.Underline = &boolFalse
	cfg.LinkText.Color = &link
	cfg.LinkText.Underline = &boolFalse

	// Inline code: GitHub renders a neutral chip, not colored text.
	cfg.Code.Color = &text
	cfg.Code.BackgroundColor = &chipBg

	// Code blocks: chroma's github-dark theme (same colors highlight.js
	// produces on github.com).
	cfg.CodeBlock.Theme = "github-dark"

	// Blockquote: muted text behind a border bar.
	cfg.BlockQuote.Color = &muted

	// Horizontal rule doubles as the H1/H2 border-bottom (injected by the
	// preprocessor). Width minus the document's 2-column margins.
	cfg.HorizontalRule.Color = &border
	if ruleWidth := width - 4; ruleWidth >= 10 {
		cfg.HorizontalRule.Format = "\n" + strings.Repeat("─", ruleWidth) + "\n"
	}

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
