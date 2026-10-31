package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

func TestInspectorIsSeparateSelectableAndCopyable(t *testing.T) {
	m := newModel(nil)
	m.resize(100, 40)
	m.apply(Event{Kind: Started, Session: 1, HumanID: "calm-otter-012345abcdef", Text: "Session restored"})
	private := "Private input " + strings.Repeat("details ", 40) + "end-marker"
	for n := 1; n <= 2; n++ {
		r := inspection.Turn{Number: n, Started: time.Now(), Finished: time.Now(), State: "delivered", Inputs: []inspection.Input{{Text: private}}, Response: "Private reply", Steps: []inspection.Step{{Kind: "model", Name: "Completion", State: "complete", Usage: inspection.Usage{Input: 100, Cached: 80, Output: 10, Known: true, Cost: 0.001, CostKnown: true}}}}
		m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	}
	if strings.Contains(m.logs.GetContent(), "Private") {
		t.Fatal("inspector content appeared in log")
	}
	m.Update(tea.KeyPressMsg{Code: 'i'})
	if !m.inspectMode || !strings.Contains(m.logs.GetContent(), "Private reply") || !strings.Contains(m.logs.GetContent(), "80 cached (80%)") || strings.Contains(m.logs.GetContent(), "end-marker") {
		t.Fatal("inspector preview or usage missing")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(m.logs.GetContent(), "end-marker") {
		t.Fatal("expand did not show full input")
	}
	copied := ""
	m.copyText = func(text string) error { copied = text; return nil }
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})
	m.Update(cmd())
	if copied != private || strings.Contains(m.clipboardNotice, private) {
		t.Fatal("copy input failed or content entered notice")
	}
	m.Update(tea.KeyPressMsg{Code: '['})
	if m.inspectedTurn().Number != 1 {
		t.Fatal("previous turn not selected")
	}
	// Click the log tab; switching back restores the selected turn.
	m.Update(tea.MouseClickMsg{X: 3, Y: m.list.Height() + 3, Button: tea.MouseLeft})
	if m.inspectMode || strings.Contains(m.logs.GetContent(), "Private") {
		t.Fatal("log tab did not restore operational log")
	}
	m.Update(tea.MouseClickMsg{X: 10, Y: m.list.Height() + 3, Button: tea.MouseLeft})
	if !m.inspectMode || m.inspectedTurn().Number != 1 {
		t.Fatal("inspector lost turn selection")
	}
}

func TestInspectorStripsTerminalControls(t *testing.T) {
	if got := displayText("safe\x1b\u202e\x00\ntext"); got != "safe\ntext" {
		t.Fatal("unsafe display characters remain")
	}
}

func TestInspectorLeadsWithInputAndKeepsFailureVisible(t *testing.T) {
	m := newModel(nil)
	m.resize(120, 40)
	m.apply(Event{Kind: Started, Session: 1})
	r := inspection.Turn{Number: 1, State: "delivered", Emoji: "custom_reaction", Inputs: []inspection.Input{{Text: "Good night"}}, Steps: []inspection.Step{{Kind: "delivery", Name: "Add reaction", State: "complete"}}}
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	m.switchMode(true)
	content := m.logs.GetContent()
	if strings.Index(content, ":custom_reaction:") < strings.Index(content, "Good night") || !strings.Contains(content, ":custom_reaction:") || !strings.Contains(content, "Good night") {
		t.Fatal("input must precede reaction, including custom emoji")
	}
	if m.turnList.SelectedItem().(turnItem).state != "Reacted" {
		t.Fatal("turn lacks outcome label")
	}
	r.State = "uncertain"
	r.Failure = "Delivery was not confirmed"
	r.Steps[0].State = "uncertain"
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	if !strings.Contains(m.logs.GetContent(), "Delivery was not confirmed") || !strings.Contains(m.logs.GetContent(), "Add reaction (uncertain)") {
		t.Fatal("uncertain delivery hidden")
	}
}

func TestInspectorDistinguishesMissingCacheUsage(t *testing.T) {
	m := newModel(nil)
	m.resize(120, 40)
	m.apply(Event{Kind: Started, Session: 1})
	m.switchMode(true)
	for _, known := range []bool{false, true} {
		r := inspection.Turn{Number: 1, State: "delivered", Steps: []inspection.Step{{Kind: "model", Provider: "Test provider", RequestID: "gen-test", Usage: inspection.Usage{Input: 100, Known: true, CacheKnown: known}}}}
		m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
		content := m.logs.GetContent()
		if strings.Contains(content, "0 cached (0%)") != known || strings.Contains(content, "cache n/a") == known {
			t.Fatalf("incorrect cache display for known=%t: %s", known, content)
		}
		if !strings.Contains(content, "Test provider") {
			t.Fatal("provider missing")
		}
		m.selected().expanded = true
		m.refreshLogs(false)
		if !strings.Contains(m.logs.GetContent(), "gen-test") {
			t.Fatal("request ID missing")
		}
	}
}

func TestTurnListMouseSelectionAndIndependentScroll(t *testing.T) {
	m := newModel(nil)
	m.resize(120, 32)
	m.apply(Event{Kind: Started, Session: 1})
	for n := 1; n <= 30; n++ {
		r := inspection.Turn{Number: n, Started: time.Now(), State: "delivered", Response: strings.Repeat("long response\n", 60)}
		m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	}
	m.switchMode(true)
	// Select a row on a later page; mouse coordinates stay relative to that page.
	m.turnList.Select(22)
	first := m.turnList.Paginator.Page * m.turnList.Paginator.PerPage
	m.Update(tea.MouseClickMsg{X: 4, Y: m.list.Height() + 5, Button: tea.MouseLeft})
	if m.turnList.Index() != first || m.inspectedTurn().Number != 30-first || !m.turnsFocused {
		t.Fatal("mouse did not select visible turn")
	}
	view := m.View().Content
	if strings.Count(view, "╭") != 3 || strings.Count(view, "╯") != 3 {
		t.Fatal("expected three separately bordered panes")
	}
	copied := ""
	m.copyText = func(text string) error { copied = text; return nil }
	_, cmd := m.Update(tea.MouseClickMsg{X: m.turnListWidth() + 8, Y: m.list.Height() + 4, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("detail-pane title is not copyable")
	}
	m.Update(cmd())
	if copied != m.selected().humanID {
		t.Fatal("wrong ID copied")
	}
	var selected bytes.Buffer
	turnDelegate{}.Render(&selected, m.turnList, first, m.turnList.SelectedItem())
	if lipgloss.Width(selected.String()) != m.turnList.Width() || !strings.Contains(selected.String(), "48;2;36;79;82") || strings.Contains(selected.String(), ">") {
		t.Fatal("turn selection does not fill row")
	}
	m.selected().expanded = true
	m.refreshLogs(false)
	m.Update(tea.MouseClickMsg{X: 70, Y: m.list.Height() + 7, Button: tea.MouseLeft})
	m.Update(tea.MouseWheelMsg{X: 70, Y: m.list.Height() + 7, Button: tea.MouseWheelDown})
	if m.turnsFocused || m.logs.YOffset() == 0 {
		t.Fatal("details did not scroll independently")
	}
	offset := m.logs.YOffset()
	m.Update(inspectorTick{})
	if m.logs.YOffset() != offset {
		t.Fatal("live refresh moved details scrollback")
	}
	m.switchMode(false)
	m.switchMode(true)
	if m.logs.YOffset() != offset {
		t.Fatal("view switch lost detail scroll position")
	}
	for _, size := range [][2]int{{36, 12}, {80, 24}, {120, 40}} {
		m.resize(size[0], size[1])
		m.refreshLogs(false)
		view := m.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("inspector exceeds terminal %dx%d", size[0], size[1])
		}
	}
}
