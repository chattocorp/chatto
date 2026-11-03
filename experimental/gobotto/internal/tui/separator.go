package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const contentIndent = "  "

// separator renders one full-width background band for group and section headers.
// Width includes label padding; active marks the selected group.
func separator(label string, width int, active bool) string {
	return separatorBar(contentIndent+displayText(label), width, active)
}

// separatorBar owns the style of the whole row, including whitespace. Strip
// nested styles so their resets cannot punch gaps in the background.
func separatorBar(content string, width int, active bool) string {
	if width <= 0 {
		return ""
	}
	background, foreground := "#293542", "#C4B5FD"
	if active {
		background, foreground = "#244F52", "#5EEAD4"
	}
	text := fitLine(ansi.Strip(content), width)
	return lipgloss.NewStyle().Background(lipgloss.Color(background)).
		Foreground(lipgloss.Color(foreground)).Bold(true).Render(text)
}
