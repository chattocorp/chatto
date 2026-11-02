package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// enterSession keeps overview navigation separate from the turn tree. Each
// visit opens all retained turns; later updates preserve manual fold choices.
func (m *model) enterSession() tea.Cmd {
	s := m.selected()
	if s == nil || m.activeSession != 0 {
		return nil
	}
	m.overviewSelection, m.overviewOffset = s.id, m.offset
	m.overviewNavigated = true
	m.activeSession = s.id
	m.operationalLog = false
	m.textOffset = 0
	m.followActive = false
	for _, t := range s.records {
		m.folds[nodeKey{session: s.id, turn: t.Number}] = true
	}
	m.rows = nil
	m.cursor, m.offset = 0, 0
	m.rebuildTree()
	m.resize(m.width, m.height)
	m.viewCounter++
	s.lastViewed = m.viewCounter
	m.trimDetails()
	if !s.detailsLoaded {
		return m.loadPage(s, 0)
	}
	return nil
}

func (m *model) leaveSession() {
	if m.activeSession == 0 {
		return
	}
	m.activeSession = 0
	m.operationalLog = false
	m.textOffset = 0
	m.followActive = false
	m.rows = nil
	m.cursor, m.offset = 0, m.overviewOffset
	m.rebuildTree()
	for i, row := range m.rows {
		if row.s.id == m.overviewSelection {
			m.cursor = i
			break
		}
	}
	m.offset = m.overviewOffset
	m.resize(m.width, m.height)
}

// overviewHeader aggregates retained session counters, including restored
// history. It does not describe these as charges incurred by this process.
func (m *model) overviewHeader() string {
	active, queued, turns, pending := 0, 0, 0, 0
	var tokens int64
	var cost float64
	known, missing, costKnown, costMissing := false, false, false, false
	for _, s := range m.sessions {
		if s.state == "running" || s.state == "investigating" {
			active++
		}
		if s.state == "queued" {
			queued++
		}
		turns += s.turns
		pending += s.pending
		tokens += s.input + s.output
		cost += s.cost
		known = known || s.usageKnown
		missing = missing || !s.usageKnown
		costKnown = costKnown || s.costKnown
		costMissing = costMissing || !s.costKnown || s.costMissing
	}
	tokenText, costText := "—", "—"
	if known {
		tokenText = compactTokens(tokens)
		if missing {
			tokenText += "+"
		}
	}
	if costKnown {
		costText = fmt.Sprintf("$%.4f", cost)
		if costMissing {
			costText += "+"
		}
	}
	tokenNote, costNote := "Reported", "USD · reported"
	if missing {
		tokenNote = "Partial"
	}
	if costMissing {
		costNote = "USD · partial"
	}
	if !known {
		tokenNote = "Not reported"
	}
	if !costKnown {
		costNote = "Not reported"
	}
	activityNote, activityColor := "All quiet", "#8593A4"
	if active > 0 {
		activityNote, activityColor = "Working now", "#5EEAD4"
	}
	queueColor := "#8593A4"
	if queued > 0 || pending > 0 {
		queueColor = "#F2C879"
	}
	metrics := []overviewMetric{
		{"Sessions", fmt.Sprint(len(m.sessions)), "Retained", "#89B4FA"},
		{"Turns", groupedTokens(int64(turns)), "Total turns", "#C4B5FD"},
		{"Active", fmt.Sprint(active), activityNote, activityColor},
		{"Queued", fmt.Sprint(queued), fmt.Sprintf("%d pending", pending), queueColor},
		{"Tokens", tokenText, tokenNote, "#E8FFFF"},
		{"Cost", costText, costNote, "#F2C879"},
	}
	return renderMetrics(metrics, m.width, "Gobotto · Overview", "Retained history", "Uptime "+time.Since(m.runtimeStarted).Truncate(time.Second).String())
}

type overviewMetric struct{ label, value, note, color string }

// renderMetrics spends horizontal space on aligned metrics, rather
// than stretching a text paragraph. Compact terminals use two rows of values.
func renderMetrics(metrics []overviewMetric, width int, title, subtitle, right string) string {
	inner := max(1, width-4)
	base := lipgloss.NewStyle()
	cell := func(text, color string, size int, bold bool) string {
		return base.Foreground(lipgloss.Color(color)).Bold(bold).Width(size).MaxWidth(size).MaxHeight(1).Render(text)
	}
	columns := 6
	if width < 100 {
		columns = 3
	}
	var rows []string
	if columns == 6 {
		leftWidth := max(1, inner-lipgloss.Width(right))
		rows = append(rows, cell(displayText(subtitle), "#8593A4", leftWidth, false)+cell(right, "#AEB8C5", inner-leftWidth, false), cell("", "#8593A4", inner, false))
	}
	for start := 0; start < len(metrics); start += columns {
		if start > 0 {
			rows = append(rows, cell("", "#8593A4", inner, false))
		}
		lineCount := 3
		if columns == 3 {
			lineCount = 2
		}
		for line := 0; line < lineCount; line++ {
			var cells []string
			usable := inner - (columns-1)*3
			for i := 0; i < columns; i++ {
				metric := metrics[start+i]
				w := usable / columns
				if i < usable%columns {
					w++
				}
				text, color, bold := metric.label, "#8593A4", false
				if line == 1 {
					text, color, bold = metric.value, metric.color, true
				}
				if line == 2 {
					text = metric.note
				}
				if i > 0 {
					cells = append(cells, cell(" │ ", "#3C4654", 3, false))
				}
				cells = append(cells, cell(text, color, w, bold))
			}
			rows = append(rows, strings.Join(cells, ""))
		}
	}
	return renderPane(title, strings.Join(rows, "\n"), width, false)
}

func (m *model) sessionHeader() string {
	s := m.selected()
	if s == nil {
		return m.overviewHeader()
	}
	input, output, cost := "—", "—", "—"
	if s.usageKnown {
		input = compactTokens(s.input)
		output = compactTokens(s.output)
	}
	if s.costKnown {
		cost = fmt.Sprintf("$%.4f", s.cost)
		if s.costMissing {
			cost += "+"
		}
	}
	color := "#8593A4"
	if s.state == "running" || s.state == "investigating" {
		color = "#5EEAD4"
	}
	if s.state == "failed" {
		color = "#F38BA8"
	}
	metrics := []overviewMetric{
		{"State", s.state, "Host status", color},
		{"Turns", fmt.Sprint(s.turns), "Retained history", "#C4B5FD"},
		{"Pending", fmt.Sprint(s.pending), "Messages", "#F2C879"},
		{"Input", input, compactTokens(s.cached) + " cached", "#89B4FA"},
		{"Output", output, reasoningLabel(s.reasoning, s.reasoningKnown, s.reasoningMissing), "#E8FFFF"},
		{"Cost", cost, "USD · reported", "#F2C879"},
	}
	if !s.usageKnown {
		metrics[3].note = "Not reported"
	}
	if s.costMissing {
		metrics[5].note = "USD · partial"
	}
	if !s.costKnown {
		metrics[5].note = "Not reported"
	}
	return renderMetrics(metrics, m.width, "Session · "+s.humanID, displayText(s.title), "Started "+s.started.Format("15:04:05"))
}
