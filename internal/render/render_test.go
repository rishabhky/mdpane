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
