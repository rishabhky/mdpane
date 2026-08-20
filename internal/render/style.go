package render

import (
	"regexp"
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
	// Page padding: GitHub gives .markdown-body generous side padding;
	// margin 2 reads cramped next to the reference.
	margin := uint(3)
	cfg.Document.Margin = &margin

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
	// GitHub headings carry more space above than below (margin-top
	// 24px): an extra blank line above each heading separates sections.
	cfg.Heading.BlockPrefix = "\n"
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
	// preprocessor). No leading newline: GitHub's rule hugs its heading
	// (padding-bottom .3em), it doesn't float a line below it. Width
	// minus the document margins on both sides.
	cfg.HorizontalRule.Color = &border
	if ruleWidth := width - 2*int(margin); ruleWidth >= 10 {
		cfg.HorizontalRule.Format = strings.Repeat("─", ruleWidth) + "\n"
	}

	return cfg
}

// githubRules preprocesses markdown for terminal legibility (outside
// fenced code blocks):
//
//   - a horizontal rule after every H1/H2 (GitHub's heading border-bottom)
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
		// "---" directly after an ATX heading is a thematic break (the
		// heading is already its own block), so no blank line is needed
		// and the rule renders tight beneath the heading text.
		if isH1orH2(trimmed) && !nextIsRule(lines, i) {
			out = append(out, "---")
		}
	}
	return strings.Join(out, "\n")
}

// renderedItemRe matches the start of a rendered list item: glamour emits
// "• " for bullets and "N. " for ordered items. Wrapped continuation
// lines never start with these, so item boundaries are unambiguous in the
// rendered output.
var renderedItemRe = regexp.MustCompile(`^\s*(•|\d{1,3}\.)\s`)

// spaceListItems adds a blank line before each rendered list item that
// directly follows another non-blank line. Terminals have no line-height;
// glamour renders all lists tight (source looseness is ignored), so
// without this, wrapped bullets fuse into an unreadable block. Applied to
// rendered output, where item boundaries are unambiguous.
func spaceListItems(rendered string) string {
	lines := strings.Split(rendered, "\n")
	out := make([]string, 0, len(lines)+16)
	for _, line := range lines {
		plain := stripANSI(line)
		if renderedItemRe.MatchString(plain) &&
			len(out) > 0 && strings.TrimSpace(stripANSI(out[len(out)-1])) != "" {
			out = append(out, "")
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;:]*m|\x1b\][^\x1b\x07]*(\x07|\x1b\\)`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

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
