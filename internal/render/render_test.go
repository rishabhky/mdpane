package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderBasics(t *testing.T) {
	r, err := New("dark", 80)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Render("# Title\n\nSome *emphasis* and `code`.\n\n- a\n- b\n")
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"Title", "emphasis", "code"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered output missing %q:\n%s", want, plain)
		}
	}
}

func TestWidthClampAndRebuild(t *testing.T) {
	r, err := New("dark", 500)
	if err != nil {
		t.Fatal(err)
	}
	if r.Width() != MaxWidth {
		t.Fatalf("width %d not clamped to %d", r.Width(), MaxWidth)
	}
	if err := r.SetWidth(5); err != nil {
		t.Fatal(err)
	}
	if r.Width() != MinWidth {
		t.Fatalf("width %d not clamped to %d", r.Width(), MinWidth)
	}
	if _, err := r.Render("hello world, a reasonably long line to wrap"); err != nil {
		t.Fatal(err)
	}
}

func TestDeterministic(t *testing.T) {
	r, _ := New("dark", 80)
	a, _ := r.Render("# Same\n\ncontent")
	b, _ := r.Render("# Same\n\ncontent")
	if a != b {
		t.Fatal("render is not deterministic")
	}
}

func TestMdpaneStyleHidesHeadingMarkers(t *testing.T) {
	r, err := New("mdpane", 80)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Render("# Title\n\n## Section\n\n### Sub\n\nbody text")
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(out)
	if strings.Contains(plain, "##") {
		t.Fatalf("mdpane style leaked heading markers:\n%s", plain)
	}
	for _, want := range []string{"Title", "Section", "Sub"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing heading text %q", want)
		}
	}
}

func TestGithubStyleRulesUnderHeadings(t *testing.T) {
	r, err := New("mdpane", 60)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Render("# Title\n\nintro\n\n## Section\n\nbody\n\n### Sub\n\nmore\n\n```\n# not a heading\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(out)
	rules := 0
	for _, line := range strings.Split(plain, "\n") {
		if strings.Count(strings.TrimSpace(line), "─") >= 10 {
			rules++
		}
	}
	// Exactly two rule LINES: under H1 and under H2 — none for H3, none
	// for the fenced pseudo-heading, and no wrapping onto extra lines.
	if rules != 2 {
		t.Fatalf("want exactly 2 rule lines, found %d:\n%s", rules, plain)
	}
	if strings.Contains(plain, "##") {
		t.Fatalf("heading markers leaked:\n%s", plain)
	}
}

func TestGithubRulesPreprocessor(t *testing.T) {
	in := "# A\n\n## B\n\n---\n\n### C\n\n```\n# fenced\n```"
	out := githubRules(in)
	if strings.Count(out, "---") != 2 { // one injected after A, existing one after B kept, none for C/fenced
		t.Fatalf("unexpected rule count in:\n%s", out)
	}
}

func TestListItemsAreSpaced(t *testing.T) {
	r, err := New("mdpane", 60)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Render("- alpha item\n- beta item\n- gamma item\n")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(ansi.Strip(out), "\n")
	var itemIdx []int
	for i, l := range lines {
		if strings.Contains(l, "•") {
			itemIdx = append(itemIdx, i)
		}
	}
	if len(itemIdx) != 3 {
		t.Fatalf("want 3 items, got %d:\n%s", len(itemIdx), ansi.Strip(out))
	}
	for k := 1; k < len(itemIdx); k++ {
		gap := itemIdx[k] - itemIdx[k-1]
		if gap < 2 {
			t.Fatalf("items %d and %d are adjacent (no blank between):\n%s", k-1, k, ansi.Strip(out))
		}
	}
}
