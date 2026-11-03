package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestYAMLHighlightingPreservesContentWithoutGutter(t *testing.T) {
	value := "schema:\n  type: object\ndata:\n  count: 42\n  enabled: true\n  title: 'a: b # literal'\n  note: |-\n    key: this stays a string\n    # still a string\n"
	lines := highlightYAML(value, 100)
	if ansi.Strip(strings.Join(lines, "\n")) != value {
		t.Fatal("highlighting changed YAML")
	}
	for _, want := range []struct{ text, color string }{{"schema", "#5EEAD4"}, {"42", "#FCD34D"}, {"true", "#C4B5FD"}, {"'a: b # literal'", "#A7D8A5"}} {
		styled := lipgloss.NewStyle().Foreground(lipgloss.Color(want.color)).Render(want.text)
		if !strings.Contains(strings.Join(lines, "\n"), styled) {
			t.Fatal("missing token color", want.text)
		}
	}
	for _, width := range []int{13, 40, 100} {
		view := renderDetails([]detailField{{"Selected", "Document"}, {"Metadata", value}}, width)
		for _, line := range view {
			if ansi.StringWidth(line) > width || strings.Contains(ansi.Strip(line), "│") {
				t.Fatal("metadata has gutter or exceeds width", line)
			}
		}
	}
}

func TestYAMLHighlightingSanitizesInvalidInput(t *testing.T) {
	text := strings.Join(highlightYAML("invalid: [\x1b[2J\u202e", 20), "\n")
	if strings.Contains(text, "\x1b[2J") || strings.Contains(text, "\u202e") {
		t.Fatal("unsafe terminal control")
	}
}

func TestYAMLWrapAlignsValuesAndKeepsColor(t *testing.T) {
	lines := highlightYAML("  summary: A long sentence with several words that should wrap neatly.", 38)
	if len(lines) < 2 {
		t.Fatal("test did not wrap")
	}
	for i, line := range lines {
		if ansi.StringWidth(line) > 38 {
			t.Fatal("line exceeds width")
		}
		if i > 0 {
			plain := ansi.Strip(line)
			if !strings.HasPrefix(plain, strings.Repeat(" ", 11)) {
				t.Fatal("continuation is not aligned under value", plain)
			}
			word := strings.TrimSpace(plain)
			green := lipgloss.NewStyle().Foreground(lipgloss.Color("#A7D8A5")).Render(word)
			if !strings.Contains(line, green) {
				t.Fatal("continuation lost string color", line)
			}
		}
	}
}
