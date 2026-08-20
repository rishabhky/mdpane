package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSpacingInspect(t *testing.T) {
	r, _ := New("mdpane", 70)
	out, _ := r.Render("# Title\n\nIntro paragraph text.\n\n## Section\n\nBody text here.\n\n- item one\n- item two\n\n## Another\n\nMore.\n")
	for i, line := range strings.Split(ansi.Strip(out), "\n") {
		mark := "text"
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			mark = "BLANK"
		} else if strings.Count(trimmed, "─") > 5 {
			mark = "RULE"
		}
		fmt.Printf("%2d %-5s |%s\n", i, mark, strings.TrimRight(line, " "))
	}
}
