package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// eventColumns shares widths between the fixed header and every tree row.
// Narrow terminals drop runtime, then usage columns to preserve readable text.
func eventColumns(width int) [6]int {
	columns := [6]int{24, 0, 10, 15, 12, 11}
	if width < 120 {
		columns[2] = 0
	}
	if width < 100 {
		columns[4], columns[5] = 0, 0
	}
	if width < 65 {
		columns[0], columns[3] = 14, 0
	}
	used, count := 0, 1 // description always participates
	for _, n := range columns {
		used += n
		if n > 0 {
			count++
		}
	}
	columns[1] = max(1, width-used-(count-1)*2)
	return columns
}

func renderEventCells(width int, values [6]string, colors [6]string, style lipgloss.Style) string {
	var cells []string
	for i, size := range eventColumns(width) {
		if size == 0 {
			continue
		}
		if len(cells) > 0 {
			cells = append(cells, style.Width(2).Render("  "))
		}
		align := lipgloss.Left
		if i == 2 || i >= 4 {
			align = lipgloss.Right
		}
		text := ansi.Truncate(values[i], size, "…")
		cells = append(cells, style.Foreground(lipgloss.Color(colors[i])).Width(size).MaxWidth(size).MaxHeight(1).Align(align).Render(text))
	}
	return strings.Join(cells, "")
}

func renderEventHeader(width int) string {
	return renderEventCells(width, [6]string{"Type", "Description / metadata", "Runtime", "  Status", "Tokens", "Cost"}, [6]string{"#8593A4", "#8593A4", "#8593A4", "#8593A4", "#8593A4", "#8593A4"}, lipgloss.NewStyle().Bold(true))
}
