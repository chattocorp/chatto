package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/chatto-chat/chatto/experimental/gobotto/memorytree"
)

func TestMemoryBrowserNavigationAndActivity(t *testing.T) {
	m := newModel(nil)
	m.resize(120, 36)
	s := memorytree.Snapshot{Nodes: []memorytree.Node{
		{ID: "root", Title: "Agent", Body: "Personality", Type: "agent", Version: 1, Revision: 1, Data: json.RawMessage(`{}`)},
		{ID: "projects", Parent: "root", Title: "Projects", Body: "Project notes"},
		{ID: "fact", Parent: "projects", Title: "A fact", Body: "Nested knowledge"},
		{ID: "old", Parent: "root", Title: "Old", Archived: true},
	}}
	m.Update(Event{Kind: MemoryChanged, Memory: &s})
	m.Update(tea.KeyPressMsg{Code: 'm'})
	if !m.memoryMode || !strings.Contains(m.View().Content, "Personality") || len(m.memory.rows) != 2 {
		t.Fatal("memory root view incorrect")
	}
	m.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	if m.memory.selected != "projects" || len(m.memory.rows) != 3 {
		t.Fatal("mouse select/expand failed")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if !strings.Contains(ansi.Strip(m.View().Content), "Nested knowledge") {
		t.Fatal("node details did not follow selection")
	}
	m.Update(tea.KeyPressMsg{Code: 'a'})
	if len(m.memory.rows) != 4 {
		t.Fatal("archives not discoverable")
	}
	m.Update(Event{Kind: Started, Session: 42, HumanID: "new-session"})
	m.Update(tea.KeyPressMsg{Code: 'm'})
	if m.memoryMode || m.selected().id != 42 {
		t.Fatal("activity stopped while browsing")
	}
	m.Update(tea.KeyPressMsg{Code: 'm'})
	if m.memory.selected != "fact" {
		t.Fatal("switch lost memory selection")
	}
	for _, size := range [][2]int{{120, 36}, {80, 24}, {50, 12}, {30, 8}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("memory view exceeds %v", size)
		}
	}
	m.requestQuit = func() {}
	m.Update(tea.KeyPressMsg{Code: 'q'})
	if !m.quitting || !strings.Contains(m.View().Content, "Shift-Q") {
		t.Fatal("memory view lost graceful shutdown")
	}
}
