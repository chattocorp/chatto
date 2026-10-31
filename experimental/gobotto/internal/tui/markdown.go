package tui

import (
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

// markdownPreview caches the selected document at its current viewport width.
// Scrolling and focus changes must not parse and highlight the document again.
type markdownPreview struct {
	source, rendered string
	width            int
}

func (p *markdownPreview) render(source string, width int) string {
	width = max(1, width)
	if p.width == width && p.source == source {
		return p.rendered
	}
	p.source, p.width = source, width
	// Strip user-supplied terminal sequences before introducing renderer-owned
	// styling. Markdown links/images are rendered locally, never fetched.
	clean := displayText(strings.ReplaceAll(ansi.Strip(source), "\t", "    "))
	style := styles.DarkStyleConfig
	zero := uint(0)
	color := "#5EEAD4"
	style.Document.Margin = &zero
	style.Document.BlockPrefix, style.Document.BlockSuffix = "", ""
	style.Heading.Color = &color
	style.H1, style.H2, style.H3 = style.Heading, style.Heading, style.Heading
	style.H4, style.H5, style.H6 = style.Heading, style.Heading, style.Heading
	style.CodeBlock.Margin = &zero
	renderer, err := glamour.NewTermRenderer(glamour.WithStyles(style), glamour.WithWordWrap(width))
	p.rendered = clean
	if err == nil {
		if rendered, err := renderer.Render(clean); err == nil {
			p.rendered = strings.Trim(rendered, "\n")
		}
	}
	return p.rendered
}
