package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

type activityTickMsg struct{}

var activityFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func activityTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return activityTickMsg{} })
}

// activityPrefix reserves two cells so labels stay aligned when activity changes.
func (m *model) activityPrefix(busy bool) string {
	if busy && !m.workerStopped {
		return string(activityFrames[m.activityFrame]) + " "
	}
	return "  "
}
