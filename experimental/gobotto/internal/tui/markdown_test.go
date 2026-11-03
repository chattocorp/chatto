package tui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestBrainMarkdownRendersLocally(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer server.Close()
	input := "# Project\n\nA **bold** and *italic* paragraph.\n\n- First item\n- Second item\n\n| Key | Value |\n| --- | --- |\n| Name | Example |\n\n```go\nfmt.Println(42)\n```\n\n[Link](" + server.URL + ")\n\n![Image](" + server.URL + "/image.png)"
	text := ansi.Strip(strings.Join(renderMarkdown(input, 80), "\n"))
	for _, want := range []string{"Project", "bold", "italic", "First item", "Second item", "Example", "fmt.Println(42)", "Link"} {
		if !strings.Contains(text, want) {
			t.Fatal("missing Markdown content", want, text)
		}
	}
	if strings.Contains(text, "**bold**") || strings.Contains(text, "```go") || requests.Load() != 0 {
		t.Fatal("Markdown was not rendered offline", text)
	}
}

func TestBrainMarkdownKeepsPanelPaddingAndSanitizesControls(t *testing.T) {
	for _, width := range []int{13, 40, 80} {
		lines := renderDetails([]detailField{{"Selected", "Document"}, {"Body", "A paragraph with enough words to wrap.\n\n\x1b[2J\u202eText"}}, width)
		for _, line := range lines {
			if ansi.StringWidth(line) > width {
				t.Fatal("Markdown exceeds panel width")
			}
			if strings.Contains(line, "\x1b[2J") || strings.Contains(line, "\u202e") {
				t.Fatal("unsafe terminal control")
			}
		}
	}
	text := ansi.Strip(strings.Join(renderMarkdown("Ordinary paragraph.", 80), "\n"))
	if strings.HasPrefix(text, " ") {
		t.Fatal("renderer adds a second panel margin", text)
	}
}
