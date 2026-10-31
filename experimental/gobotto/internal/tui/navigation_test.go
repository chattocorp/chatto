package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestModeTabsHitAreasAndWidth(t *testing.T) {
	m := newModel(nil)
	for _, width := range []int{36, 50, 80, 120} {
		m.resize(width, 24)
		for _, memory := range []bool{false, true} {
			m.memoryMode = memory
			if lipgloss.Width(m.navigation()) > width || lipgloss.Height(m.navigation()) != 1 {
				t.Fatal("navigation overflow", width)
			}
		}
		left := 1
		for i, label := range m.modeTabs() {
			for x := left; x < left+lipgloss.Width(label); x++ {
				m.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
				if m.memoryMode != (i == 1) || m.typesMode != (i == 2) {
					t.Fatal("tab hit area differs from label", x)
				}
			}
			left += lipgloss.Width(label)
			if _, hit := m.modeAt(left); hit {
				t.Fatal("gap switches modes")
			}
			left++
		}
	}
}
