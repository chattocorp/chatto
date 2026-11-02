package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

func TestOverviewAndExpandedSessionNavigation(t *testing.T) {
	m := treeFixture()
	m.moveTree(0) // manual selection must survive later session arrivals
	if len(m.rows) != 1 || !strings.Contains(m.View().Content, "Gobotto · Overview") {
		t.Fatal("overview must contain stats and sessions only")
	}
	m.apply(Event{Kind: Started, Session: 2, HumanID: "second"})
	if m.selected().id != 1 {
		t.Fatal("new session stole overview selection")
	}
	key(m, tea.KeyEnter)
	if m.activeSession != 1 || len(m.rows) != 15 || m.rows[0].t.Number != 3 || m.rows[1].label != "← Input" || m.treeWidth() != m.width {
		t.Fatal("session did not open with all turns expanded")
	}
	key(m, tea.KeyRight)
	selected := m.currentRow().key
	m.apply(Event{Kind: Started, Session: 3})
	if m.currentRow().key != selected || len(m.rows) != 15 {
		t.Fatal("other session changed turn tree")
	}
	key(m, tea.KeyLeft)
	key(m, tea.KeyLeft)
	if m.folds[m.currentRow().key] {
		t.Fatal("turn did not collapse")
	}
	r := m.selected().records[2].Clone()
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	if m.folds[m.currentRow().key] {
		t.Fatal("update reset manual fold")
	}
	key(m, tea.KeyEscape)
	if m.activeSession != 0 || m.selected().id != 1 || len(m.rows) != 3 {
		t.Fatal("back did not restore overview selection")
	}
	key(m, tea.KeyEnter)
	if len(m.rows) != 15 {
		t.Fatal("session visit did not expand all turns")
	}
}

func TestStartupSelectionStaysOnNewestSession(t *testing.T) {
	m := newModel(nil)
	for id := uint64(1); id <= 40; id++ {
		m.apply(Event{Kind: Started, Session: id})
		m.apply(Event{Kind: Completed, Session: id})
		if m.cursor != 0 || m.offset != 0 || m.selected().id != id {
			t.Fatal("startup restore moved selection away from newest session")
		}
	}
	key(m, tea.KeyDown)
	selected := m.selected().id
	m.apply(Event{Kind: Started, Session: 41})
	if m.selected().id != selected {
		t.Fatal("startup pin overrode manual navigation")
	}
	key(m, tea.KeyEnter)
	m.apply(Event{Kind: Started, Session: 42})
	key(m, tea.KeyEscape)
	if m.selected().id != selected {
		t.Fatal("return to overview lost manual selection")
	}
}

func TestSessionFollowIgnoresOtherSessions(t *testing.T) {
	m := treeFixture()
	key(m, tea.KeyEnter)
	key(m, 'f')
	if m.currentRow().key != (nodeKey{session: 1, turn: 3, step: 2}) {
		t.Fatal("follow did not select latest step")
	}
	m.Update(Event{Kind: Started, Session: 2})
	if m.activeSession != 1 || m.currentRow().key.session != 1 {
		t.Fatal("follow escaped session")
	}
	r := inspection.Turn{Number: 4, State: "running", Steps: []inspection.Step{{Name: "Read", State: "running"}}}
	m.Update(Event{Kind: Inspected, Session: 1, Inspection: &r})
	if m.currentRow().key != (nodeKey{session: 1, turn: 4, step: 1}) {
		t.Fatal("follow did not reveal new turn")
	}
	key(m, tea.KeyUp)
	if m.followActive {
		t.Fatal("manual navigation did not stop following")
	}
	key(m, tea.KeyEscape)
	key(m, 'f')
	m.Update(Event{Kind: Started, Session: 3})
	if m.activeSession != 0 || m.selected().id != 3 {
		t.Fatal("overview follow should select sessions without entering")
	}
}

func TestOverviewTotalsDistinguishRestoredAndUnknownUsage(t *testing.T) {
	m := newModel(nil)
	m.resize(140, 30)
	m.apply(Event{Kind: Started, Session: 1, Turns: 5})
	cost := 0.125
	m.apply(Event{Kind: Tokens, Session: 1, Input: 1000, CachedInput: 800, Output: 100, Cost: &cost})
	m.apply(Event{Kind: Completed, Session: 1, Background: true})
	m.apply(Event{Kind: Started, Session: 2})
	m.apply(Event{Kind: Queued, Session: 2, Pending: 3})
	header := m.overviewHeader()
	for _, want := range []string{"Retained history", "Sessions", "Turns", "Active", "Queued", "3 pending", "1.1k+", "$0.1250+", "Partial"} {
		if !strings.Contains(header, want) {
			t.Fatal("header missing", want)
		}
	}
	empty := newModel(nil).overviewHeader()
	if strings.Count(empty, "—") < 2 {
		t.Fatal("unknown totals presented as zero")
	}
}

func TestOverviewMetricsUseAvailableWidth(t *testing.T) {
	m := treeFixture()
	for _, width := range []int{36, 80, 100, 160, 240} {
		m.resize(width, 24)
		header := m.overviewHeader()
		if lipgloss.Width(header) != width || lipgloss.Height(header) != m.headerHeight() {
			t.Fatalf("header geometry incorrect at %d columns", width)
		}
		for _, label := range []string{"Sessions", "Turns", "Active", "Queued", "Tokens", "Cost"} {
			if !strings.Contains(header, label) {
				t.Fatalf("missing %s at width %d", label, width)
			}
		}
		if width >= 100 && strings.Count(header, "│") != 25 {
			t.Fatal("wide metrics must share three rows with five separators each")
		}
	}
}

func TestOverviewMouseOffsetsAndSessionResize(t *testing.T) {
	m := treeFixture()
	for id := uint64(2); id <= 40; id++ {
		m.apply(Event{Kind: Started, Session: id})
	}
	key(m, tea.KeyHome)
	selected := m.currentRow().key
	m.Update(tea.MouseClickMsg{X: 8, Y: 2, Button: tea.MouseLeft})
	if m.currentRow().key != selected {
		t.Fatal("header click selected a row")
	}
	key(m, tea.KeyEnd)
	want := m.rows[m.offset].key
	m.Update(tea.MouseClickMsg{X: 8, Y: m.headerHeight() + 1, Button: tea.MouseLeft})
	if m.currentRow().key != want {
		t.Fatal("mouse ignored overview header or scroll offset")
	}
	m.Update(tea.MouseClickMsg{X: 2, Y: m.headerHeight() + 1, Button: tea.MouseLeft})
	key(m, tea.KeyRight)
	if m.activeSession != 0 {
		t.Fatal("click or right arrow entered a session")
	}
	if strings.Contains(m.renderTreeRow(*m.currentRow(), 90, false), "›") {
		t.Fatal("overview row has a caret")
	}
	key(m, tea.KeyEnter)
	if m.activeSession != want.session {
		t.Fatal("Enter did not open session")
	}
	for _, size := range [][2]int{{36, 12}, {80, 24}, {100, 24}, {140, 30}, {20, 5}} {
		m.resize(size[0], size[1])
		{
			view := m.View()
			if lipgloss.Width(view.Content) > size[0] || lipgloss.Height(view.Content) > size[1] {
				t.Fatal("session exceeds terminal", size)
			}
		}
	}
	m.leaveSession()
	for _, size := range [][2]int{{36, 12}, {80, 24}, {140, 30}} {
		m.resize(size[0], size[1])
		view := m.View()
		if lipgloss.Height(view.Content) != size[1] || lipgloss.Width(view.Content) > size[0] {
			t.Fatal("overview geometry incorrect", size)
		}
	}
}

func TestSessionLeavesAndRetention(t *testing.T) {
	m := treeFixture()
	key(m, tea.KeyEnter)
	key(m, tea.KeyRight)
	leaf := m.currentRow().key
	key(m, tea.KeyRight)
	if m.currentRow().key != leaf || strings.ContainsAny(m.renderTreeRow(*m.currentRow(), 90, false), "›⌄") {
		t.Fatal("leaf has expansion behavior")
	}
	key(m, tea.KeyEnd)
	for n := 4; n <= 60; n++ {
		r := inspection.Turn{Number: n, State: "delivered"}
		m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	}
	if m.currentRow() == nil || m.currentRow().t.Number < 11 || len(m.folds) != 50 {
		t.Fatal("retention left stale tree state")
	}
	m.apply(Event{Kind: Evicted, Session: 1})
	if m.activeSession != 0 || len(m.rows) != 0 || len(m.folds) != 0 {
		t.Fatal("evicted session did not return to overview")
	}
	m.apply(Event{Kind: Started, Session: 2})
	key(m, tea.KeyEnter)
	if len(m.rows) != 0 || m.selected().id != 2 || !strings.Contains(m.View().Content, "No recorded turns") {
		t.Fatal("empty session unavailable")
	}
	r := inspection.Turn{Number: 1, State: "running", Steps: []inspection.Step{{Name: "Completion", State: "running"}}}
	m.apply(Event{Kind: Inspected, Session: 2, Inspection: &r})
	if len(m.rows) != 2 {
		t.Fatal("first turn did not appear expanded")
	}
}

func TestSessionBusyDeliveryAndCopy(t *testing.T) {
	m := treeFixture()
	frame := m.busy.View()
	if !strings.Contains(m.renderTreeRow(m.rows[0], 90, false), frame) {
		t.Fatal("busy session spinner missing")
	}
	key(m, tea.KeyEnter)
	before := m.renderTreeRow(m.rows[0], 90, true)
	_, cmd := m.Update(m.busy.Tick())
	if cmd == nil || before == m.renderTreeRow(m.rows[0], 90, true) || m.currentRow().key.turn != 3 {
		t.Fatal("animation did not advance independently")
	}
	copied := ""
	m.copyText = func(text string) error { copied = text; return nil }
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'y'})
	m.Update(cmd())
	if copied != "private input" || strings.Contains(m.clipboardNotice, "private") {
		t.Fatal("private copy failed")
	}
	key(m, 'l')
	if strings.Contains(m.View().Content, "private") {
		t.Fatal("operational logs contain transcript")
	}
	key(m, 'l')
	key(m, tea.KeyRight)
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c'})
	m.Update(cmd())
	if copied != "private input" {
		t.Fatal("input event copy failed")
	}
	key(m, ']')
	pan := m.textOffset
	m.Update(inspectorTick{})
	if pan == 0 || m.textOffset != pan {
		t.Fatal("tick reset horizontal scroll")
	}
	key(m, tea.KeyEscape)
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c'})
	m.Update(cmd())
	if copied != "calm-test-123" {
		t.Fatal("session ID copy failed")
	}
	key(m, tea.KeyEnter)
	r := inspection.Turn{Number: 4, State: "uncertain", Started: time.Now(), Steps: []inspection.Step{{Name: "Post reply", State: "uncertain"}}}
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	r.Steps[0].Name = "mutated"
	if !strings.Contains(m.renderTreeRow(m.rows[0], 90, false), "uncertain") || m.rows[0].t.Steps[0].Name == "mutated" {
		t.Fatal("delivery state or detached snapshot lost")
	}
}
