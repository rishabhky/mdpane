// Mermaid support: fenced ```mermaid blocks are rendered to Unicode box
// art (via pgavlin/mermaid-ascii, ~20 diagram types with auto-detection)
// and re-fenced as plain code blocks, which glamour renders verbatim —
// crucially without word-wrapping, which would shred diagram alignment.
// Anything that fails to parse falls back to the original fence, so the
// worst case is exactly what other viewers show: the mermaid source.
package render

import (
	"strings"

	"github.com/pgavlin/mermaid-ascii/pkg/diagram"
	mermaid "github.com/pgavlin/mermaid-ascii/pkg/render"
)

// renderMermaidBlocks replaces ```mermaid fences with rendered box art.
func renderMermaidBlocks(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !isMermaidFence(trimmed) {
			out = append(out, lines[i])
			// Skip over non-mermaid fences untouched so their content
			// can never be mistaken for a mermaid opener.
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				marker := trimmed[:3]
				for i++; i < len(lines); i++ {
					out = append(out, lines[i])
					if strings.HasPrefix(strings.TrimSpace(lines[i]), marker) {
						break
					}
				}
			}
			continue
		}

		marker := trimmed[:3]
		var body []string
		closed := false
		j := i + 1
		for ; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), marker) {
				closed = true
				break
			}
			body = append(body, lines[j])
		}
		if !closed {
			out = append(out, lines[i]) // unterminated: leave as-is
			continue
		}

		art, ok := renderMermaid(strings.Join(body, "\n"))
		if ok {
			out = append(out, "```")
			out = append(out, strings.Split(strings.TrimRight(art, "\n"), "\n")...)
			out = append(out, "```")
		} else {
			out = append(out, lines[i:j+1]...)
		}
		i = j
	}
	return strings.Join(out, "\n")
}

func isMermaidFence(trimmed string) bool {
	for _, m := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, m) {
			info := strings.TrimSpace(strings.TrimLeft(trimmed, "`~"))
			return strings.EqualFold(info, "mermaid")
		}
	}
	return false
}

// renderMermaid converts one mermaid source block to box art. A parser or
// renderer failure (or panic — the library handles arbitrary agent
// output) means "not rendered", never a broken viewer.
func renderMermaid(src string) (art string, ok bool) {
	if strings.TrimSpace(src) == "" {
		return "", false
	}
	defer func() {
		if recover() != nil {
			art, ok = "", false
		}
	}()
	out, err := mermaid.Render(src, diagram.DefaultConfig())
	if err != nil || strings.TrimSpace(out) == "" {
		return "", false
	}
	// The library can emit ANSI for mermaid `style` directives; inside a
	// code fence those would render as literal escapes. Plain art only.
	return stripANSI(out), true
}
