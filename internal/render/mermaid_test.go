package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const flowSrc = "```mermaid\ngraph LR\n    A[Client]-->B[Gateway]\n    B-->C[API]\n```"

func TestMermaidFenceRendersToArt(t *testing.T) {
	r, err := New("mdpane", 96)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Render("# Doc\n\n" + flowSrc + "\n\nafter text\n")
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "─►") && !strings.Contains(plain, "►") {
		t.Fatalf("no arrows in output — diagram not rendered:\n%s", plain)
	}
	if !strings.Contains(plain, "Client") || !strings.Contains(plain, "Gateway") {
		t.Fatalf("node labels missing:\n%s", plain)
	}
	if strings.Contains(plain, "graph LR") {
		t.Fatalf("mermaid source leaked into output:\n%s", plain)
	}
	if strings.Contains(plain, "\x1b") {
		t.Fatal("ANSI escapes inside rendered art")
	}
}

func TestSequenceDiagramRenders(t *testing.T) {
	r, _ := New("mdpane", 96)
	src := "```mermaid\nsequenceDiagram\n    A->>B: hello\n    B-->>A: world\n```"
	out, err := r.Render(src)
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "hello") || !strings.Contains(plain, "│") {
		t.Fatalf("sequence diagram not rendered:\n%s", plain)
	}
}

func TestInvalidMermaidFallsBackToSource(t *testing.T) {
	r, _ := New("mdpane", 96)
	src := "```mermaid\nthis is not a diagram at all {{{\n```"
	out, err := r.Render(src)
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "not a diagram") {
		t.Fatalf("fallback source missing:\n%s", plain)
	}
}

func TestNonMermaidFencesUntouched(t *testing.T) {
	r, _ := New("mdpane", 96)
	src := "```go\n// graph LR looks mermaidy but is code\n```\n\n```mermaid\ngraph LR\n    X-->Y\n```"
	out, err := r.Render(src)
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "looks mermaidy but is code") {
		t.Fatalf("go fence content lost:\n%s", plain)
	}
	if !strings.Contains(plain, "►") {
		t.Fatalf("mermaid after code fence not rendered:\n%s", plain)
	}
}

func TestUnterminatedMermaidFenceIsSafe(t *testing.T) {
	r, _ := New("mdpane", 96)
	if _, err := r.Render("```mermaid\ngraph LR\n  A-->B\n"); err != nil {
		t.Fatalf("unterminated fence errored: %v", err)
	}
}
