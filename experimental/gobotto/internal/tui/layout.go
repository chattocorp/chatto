package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// fitLine fills exactly the panel interior width in terminal cells. Content may
// contain our own styles; sanitize external text before applying those styles.
func fitLine(content string, width int) string {
	if width <= 0 {
		return ""
	}
	content = ansi.Truncate(content, width, "…")
	return content + strings.Repeat(" ", max(0, width-ansi.StringWidth(content)))
}

func contentWidth(width int) int { return max(1, width-2*len(contentIndent)) }

// splitWidths and dividerRatio share clamping for both full-screen views.
func splitWidths(width int, ratio float64) (int, int) {
	available := max(2, width-1)
	minimum := min(16, available/2)
	left := max(minimum, min(available-minimum, int(float64(available)*ratio)))
	return left, available - left
}

func dividerRatio(width, x int) float64 {
	available := max(2, width-1)
	minimum := min(16, available/2)
	x = max(minimum, min(available-minimum, x))
	return (float64(x) + 0.5) / float64(available)
}

// insetLine applies the same two-cell inset to both sides of body content.
func insetLine(content string, width int) string {
	return fitLine(contentIndent+ansi.Truncate(content, contentWidth(width), "…"), width)
}

// panelTitle is part of the border, never a background band.
func panelTitle(title string, width int, active bool, border func(...string) string) string {
	label := ansi.Truncate(" "+displayText(title)+" ", max(0, width-1), "…")
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#8593A4"))
	if active {
		style = style.Foreground(lipgloss.Color("#5EEAD4")).Bold(true)
	}
	return border("─") + style.Render(label) + border(strings.Repeat("─", max(0, width-1-ansi.StringWidth(label))))
}
