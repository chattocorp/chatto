package tui

import (
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

// renderMarkdown formats local document bodies without loading remote resources
// or terminal-dependent theme configuration. The panel owns outer indentation.
func renderMarkdown(value string, width int) []string {
	input := strings.Split(value, "\n")
	for i := range input {
		input[i] = displayText(ansi.Strip(input[i]))
	}
	value = strings.Join(input, "\n")
	style := styles.DarkStyleConfig
	zero := uint(0)
	style.Document.Margin = &zero
	style.Document.BlockPrefix, style.Document.BlockSuffix = "", ""
	renderer, err := glamour.NewTermRenderer(glamour.WithStyles(style), glamour.WithWordWrap(max(1, width)), glamour.WithTableWrap(true))
	if err == nil {
		if rendered, err := renderer.Render(value); err == nil {
			return strings.Split(strings.Trim(rendered, "\n"), "\n")
		}
	}
	return strings.Split(ansi.Wrap(value, max(1, width), ""), "\n")
}
