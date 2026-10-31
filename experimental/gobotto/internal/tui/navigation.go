package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// modeTabs is shared by rendering and hit testing so counts can grow without
// making the clickable areas drift from their visible labels.
func (m *model) modeTabs() [3]string {
	names := map[string]bool{}
	for _, typ := range m.memory.snapshot.Types {
		names[typ.Name] = true
	}
	labels := [3]string{
		fmt.Sprintf(" Activity  %d ", len(m.sessions)),
		fmt.Sprintf(" Memory  %d ", len(m.memory.snapshot.Nodes)),
		fmt.Sprintf(" Types  %d ", len(names)),
	}
	if m.width < 50 {
		return [3]string{" Activity ", " Memory ", " Types "}
	}
	return labels
}

func (m *model) modeAt(x int) (mode int, hit bool) {
	left := 1
	for i, label := range m.modeTabs() {
		right := left + lipgloss.Width(label)
		if x >= left && x < right && x < m.width {
			return i, true
		}
		left = right + 1
	}
	return 0, false
}

// navigation uses one row, preserving the panes' existing mouse coordinates.
func (m *model) navigation() string {
	bar := lipgloss.NewStyle().Background(lipgloss.Color("#202630"))
	active := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#102724")).Background(lipgloss.Color("#5EEAD4"))
	inactive := lipgloss.NewStyle().Foreground(lipgloss.Color("#B9C5D5")).Background(lipgloss.Color("#303B4B"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#8593A4")).Background(lipgloss.Color("#202630"))
	var tabs []string
	selected := 0
	if m.memoryMode {
		selected = 1
	}
	if m.typesMode {
		selected = 2
	}
	for i, label := range m.modeTabs() {
		style := inactive
		if i == selected {
			style = active
		}
		tabs = append(tabs, style.Render(label))
	}
	left := bar.Render(" ") + strings.Join(tabs, bar.Render(" "))
	hint := inactive.Render(" m / t ") + muted.Render(" switch ")
	status := m.status
	if m.quitting {
		status = "Finishing active turn…"
	}
	room := m.width - lipgloss.Width(left) - lipgloss.Width(hint)
	if room < 1 {
		return bar.MaxWidth(max(1, m.width)).MaxHeight(1).Render(left)
	}
	middle := ""
	if room >= lipgloss.Width(status)+4 && status != "" {
		middle = muted.Render("  " + displayText(status))
	}
	middle += bar.Render(strings.Repeat(" ", max(0, room-lipgloss.Width(middle))))
	return left + middle + hint
}
