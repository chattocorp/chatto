package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

// turnStats keeps unknown and partial accounting distinct from reported zeroes.
type turnStats struct {
	input, output, cache, cacheValue, cost, costDetail, reasoning string
	calls                                                         int
}

func groupedTokens(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// renderTurnStats uses four metric columns on wide terminals and wraps them
// into two or one on narrow terminals. The stats viewport owns overflow.
func renderTurnStats(t inspection.Turn, duration time.Duration, s turnStats, width int) string {
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#8593A4"))
	strong := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E8FFFF"))
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5EEAD4"))
	warning := lipgloss.NewStyle().Foreground(lipgloss.Color("#EABF76"))
	state := displayText(t.State)
	status := accent.Render("● " + state)
	if state == "failed" || state == "uncertain" || state == "interrupted" || state == "cancelled" {
		status = warning.Render("! " + state)
	}
	rejected, failed := 0, 0
	for _, step := range t.Steps {
		switch step.State {
		case "rejected":
			rejected++
		case "failed", "uncertain", "interrupted", "cancelled":
			failed++
		}
	}
	alerts := ""
	if rejected > 0 {
		alerts += warning.Render(fmt.Sprintf("  ·  %d rejected", rejected))
	}
	if failed > 0 {
		alerts += warning.Render(fmt.Sprintf("  ·  %d unsuccessful", failed))
	}
	header := strong.Render(fmt.Sprintf("TURN %02d", t.Number)) + "  " + status + alerts
	meta := fmt.Sprintf("%s  ·  %s elapsed  ·  %d %s", t.Started.Format("15:04:05"), duration.Round(time.Millisecond), s.calls, pluralCalls(s.calls))
	model := displayText(t.Model)
	if model == "" {
		model = "Model not recorded"
	}
	if s.cacheValue == "" {
		s.cacheValue = "—"
	}
	metrics := [][3]string{
		{"INPUT TOKENS", s.input, "Total prompt tokens"},
		{"OUTPUT TOKENS", s.output, s.reasoning},
		{"CACHE HIT RATE", s.cacheValue, s.cache},
		{"TURN COST", s.cost, s.costDetail},
	}
	columns := 4
	if width < 88 {
		columns = 2
	}
	if width < 50 {
		columns = 1
	}
	cellWidth := max(1, (width-(columns-1)*2)/columns)
	var rows []string
	for start := 0; start < len(metrics); start += columns {
		var cells []string
		for _, metric := range metrics[start:min(start+columns, len(metrics))] {
			valueStyle := strong
			if metric[0] == "CACHE HIT RATE" || metric[0] == "TURN COST" {
				valueStyle = accent
			}
			cell := muted.Render(metric[0]) + "\n" + valueStyle.Render(metric[1]) + "\n" + muted.Render(metric[2])
			cells = append(cells, lipgloss.NewStyle().Width(cellWidth).Render(cell))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, intersperseCells(cells)...))
	}
	footer := muted.Render(fmt.Sprintf("%s  ·  batch wait %s  ·  output includes reasoning", model, t.Wait.Round(time.Millisecond)))
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(strings.Join([]string{header + "  " + muted.Render(meta), "", strings.Join(rows, "\n\n"), footer}, "\n"))
}

func intersperseCells(cells []string) []string {
	var result []string
	for i, cell := range cells {
		if i > 0 {
			result = append(result, "  ")
		}
		result = append(result, cell)
	}
	return result
}
