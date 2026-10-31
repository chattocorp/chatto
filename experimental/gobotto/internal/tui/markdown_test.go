package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMemoryMarkdownRenderingAndResize(t *testing.T) {
	p := markdownPreview{}
	source := "# Wedel\n\n**Location:** A town beside the river, with history and landmarks.\n\n- Harbour\n- Old town\n\n> A riverside town.\n\n```go\nanswer := 42\n```\n\n[Source](https://example.com)"
	wide := p.render(source, 70)
	plain := ansi.Strip(wide)
	for _, raw := range []string{"# Wedel", "**Location:**", "```go", "[Source]("} {
		if strings.Contains(plain, raw) {
			t.Fatalf("unrendered Markdown: %q", raw)
		}
	}
	for _, text := range []string{"Wedel", "Location:", "Harbour", "answer := 42", "https://example.com"} {
		if !strings.Contains(plain, text) {
			t.Fatalf("rendering lost %q", text)
		}
	}
	if wide == plain {
		t.Fatal("Markdown has no terminal styling")
	}
	if p.render(source, 70) != wide {
		t.Fatal("cached rendering changed")
	}
	narrow := p.render(source, 30)
	if narrow == wide {
		t.Fatal("resize did not reflow Markdown")
	}
	for _, line := range strings.Split(narrow, "\n") {
		if ansi.StringWidth(line) > 30 {
			t.Fatal("Markdown exceeds viewport width")
		}
	}
	if strings.Contains(p.render("new **content**", 30), "Wedel") {
		t.Fatal("document change retained stale rendering")
	}
}

func TestMemoryMarkdownStripsTerminalCommands(t *testing.T) {
	p := markdownPreview{}
	out := p.render("Safe\x1b]52;c;Y2xpcGJvYXJk\a text\x1b[2J", 60)
	if strings.Contains(out, "52;c;") || strings.Contains(out, "\x1b[2J") {
		t.Fatal("untrusted terminal command retained")
	}
	if !strings.Contains(ansi.Strip(out), "Safe text") {
		t.Fatal("sanitization lost visible text")
	}
}
