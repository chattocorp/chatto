package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestEventTableAlignmentAndPanning(t *testing.T) {
	m := treeFixture()
	m.enterSession()
	for _, width := range []int{32, 64, 65, 99, 100, 119, 120, 180} {
		if lipgloss.Width(renderEventHeader(width)) != width {
			t.Fatal("header width mismatch", width)
		}
		for _, row := range m.rows {
			if lipgloss.Width(m.renderTreeRow(row, width, false)) != width {
				t.Fatal("row width mismatch", width)
			}
		}
	}
	input := m.rows[1]
	input.detail = "first portion of message, followed by additional content"
	before := ansi.Strip(m.renderTreeRow(input, 180, false))
	m.textOffset = 16
	after := ansi.Strip(m.renderTreeRow(input, 180, false))
	if before[:26] != after[:26] || before == after || !strings.Contains(after, "← Input") {
		t.Fatal("panning moved the event type or failed to move description")
	}
	m.textOffset = 0
	row := m.rows[2]
	line := ansi.Strip(m.renderTreeRow(row, 180, false))
	header := ansi.Strip(renderEventHeader(180))
	if lipgloss.Width(line[:strings.Index(line, "complete")]) != lipgloss.Width(header[:strings.Index(header, "Status")]) {
		t.Fatal("status column shifted")
	}
	if lipgloss.Width(line[:strings.Index(line, "110 tok")+len("110 tok")]) != lipgloss.Width(header[:strings.Index(header, "Tokens")+len("Tokens")]) {
		t.Fatal("token column is not right aligned with its header")
	}
}

func TestEventTableHeaderMouseAndScroll(t *testing.T) {
	m := treeFixture()
	m.enterSession()
	m.moveTree(2)
	selected := m.currentRow().key
	m.Update(tea.MouseClickMsg{X: 8, Y: m.headerHeight() + 1, Button: tea.MouseLeft})
	if m.currentRow().key != selected {
		t.Fatal("column header selected a row")
	}
	m.Update(tea.MouseClickMsg{X: 8, Y: m.headerHeight() + 2, Button: tea.MouseLeft})
	if m.cursor != 0 {
		t.Fatal("first table row hit area is shifted")
	}
	m.moveTree(len(m.rows))
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(strings.Split(view, "\n")[m.headerHeight()+1], "Description / metadata") {
		t.Fatal("column header scrolled away")
	}
	if lipgloss.Height(view) != m.height {
		t.Fatal("table changed terminal geometry")
	}
}
