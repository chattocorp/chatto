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

func typeFixture() memorytree.Snapshot {
	return memorytree.Snapshot{
		Types: []memorytree.Type{
			{Name: "place", Version: 1, Schema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Place name"}},"required":["name"]}`)},
			{Name: "place", Version: 2, Schema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Place name"},"country":{"type":"string"}},"required":["name","country"]}`)},
			{Name: "unused", Version: 1, Schema: json.RawMessage(`false`)},
		},
		Nodes: []memorytree.Node{
			{ID: "root", Title: "Agent"},
			{ID: "old", Parent: "root", Title: "Old record", Type: "place", Version: 1, Body: "Old details"},
			{ID: "new", Parent: "root", Title: "New record", Type: "place", Version: 2},
			{ID: "archived", Parent: "root", Title: "Archived record", Type: "place", Version: 1, Archived: true},
		},
	}
}

func TestTypesBrowserNavigationRecordsAndUpdates(t *testing.T) {
	m := newModel(nil)
	m.resize(120, 32)
	s := typeFixture()
	m.Update(Event{Kind: MemoryChanged, Memory: &s})
	m.Update(tea.KeyPressMsg{Code: 't'})
	if !m.typesMode || m.types.selected != "place v2" || !strings.Contains(ansi.Strip(m.View().Content), "country") {
		t.Fatal("schema screen missing selected version")
	}
	m.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	if m.types.selected != "place v1" {
		t.Fatal("mouse did not select older version")
	}
	a, b := typeCounts(s, m.types.rows[m.types.index()])
	if a != 1 || b != 1 {
		t.Fatal("record counts lost archived records")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.memoryMode || m.typesMode || len(m.memory.rows) != 2 || m.memory.filterVersion != 1 {
		t.Fatal("record shortcut lost version filter")
	}
	for _, r := range m.memory.rows {
		if r.node.Version != 1 {
			t.Fatal("wrong-version record in filtered memory")
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.memory.filterType != "" {
		t.Fatal("escape did not restore full tree")
	}
	m.Update(tea.KeyPressMsg{Code: 't'})
	s.Types = append(s.Types, memorytree.Type{Name: "place", Version: 3, Schema: json.RawMessage(`{"type":"object"}`)})
	m.Update(Event{Kind: MemoryChanged, Memory: &s})
	if m.types.selected != "place v1" || len(m.types.rows) != 4 {
		t.Fatal("live update lost selection")
	}
	m.Update(Event{Kind: Started, Session: 99, HumanID: "test-session"})
	if len(m.sessions) != 1 {
		t.Fatal("activity did not continue in types mode")
	}
	for _, size := range [][2]int{{120, 32}, {80, 24}, {50, 12}, {36, 12}, {20, 8}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatal("types overflow", size)
		}
	}
	m.requestQuit = func() {}
	m.Update(tea.KeyPressMsg{Code: 'q'})
	if !m.quitting || !strings.Contains(m.View().Content, "Shift-Q") {
		t.Fatal("types mode lost graceful quit")
	}
}

func TestSchemaDiffIncludesNestedConstraintsAndRequired(t *testing.T) {
	var before, after any
	json.Unmarshal([]byte(`{"properties":{"a/b":{"type":"string","minLength":1},"old":{"type":"boolean"}},"required":["old"]}`), &before)
	json.Unmarshal([]byte(`{"properties":{"a/b":{"type":"string","minLength":2},"new":{"type":"number"}},"required":["new"]}`), &after)
	diff := strings.Join(schemaChanges("", before, after), "\n")
	for _, want := range []string{"/properties/a~1b/minLength: 1 → 2", "+ /properties/new", "− /properties/old", "/required"} {
		if !strings.Contains(diff, want) {
			t.Fatal("diff missing", want, diff)
		}
	}
	if len(schemaChanges("", before, before)) != 0 {
		t.Fatal("equal schemas differ")
	}
}

func TestTypesEmptyAndUnusedVersion(t *testing.T) {
	m := newModel(nil)
	m.resize(80, 24)
	m.Update(tea.KeyPressMsg{Code: 't'})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.typesMode || !strings.Contains(m.View().Content, "No schemas") {
		t.Fatal("empty type browser incorrect")
	}
	s := typeFixture()
	m.Update(Event{Kind: MemoryChanged, Memory: &s})
	m.types.selected = "unused v1"
	m.types.refresh(s, 80, 24)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(m.View().Content, "No records use this version") {
		t.Fatal("unused schema showed unrelated node")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if len(m.memory.rows) == 0 {
		t.Fatal("empty filter trapped navigation")
	}
}
