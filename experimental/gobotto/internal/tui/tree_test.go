package tui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

func treeFixture() *model {
	m := newModel(nil)
	m.resize(100, 24)
	m.apply(Event{Kind: Started, Session: 1, HumanID: "quiet-fox", Title: "Plan the release", At: time.Unix(1, 0)})
	for n := 1; n <= 3; n++ {
		turn := inspection.Turn{Number: n, State: "delivered", Model: "hidden-model", Inputs: []inspection.Input{{Text: "hidden-input"}}, Response: "hidden-response", Steps: []inspection.Step{
			{Name: "Completion", Kind: "model", Detail: "hidden-detail", Provider: "hidden-provider", Usage: inspection.Usage{Known: true, Input: 1234, Output: 567, CostKnown: true, Cost: 1.23}},
			{Name: "memory_search", Kind: "tool"},
		}}
		m.apply(Event{Kind: Inspected, Session: 1, Inspection: &turn})
	}
	return m
}

func key(m *model, code rune) { m.Update(tea.KeyPressMsg{Code: code}) }

func TestSplitViewContainsOnlySessionAndEventLabels(t *testing.T) {
	m := treeFixture()
	if len(m.rows) != 15 || m.rows[0].label != "Turn 3" || m.rows[1].label != "Output" || m.rows[2].label != "memory_search" || m.rows[3].label != "Completion" || m.rows[4].label != "Input" {
		t.Fatal("turn grouping incorrect", m.rows)
	}
	view := ansi.Strip(m.View().Content)
	t.Log("\n" + view)
	for _, want := range []string{"Sessions", "Events", "Plan the release", "Turn 3", "memory_search", "Output"} {
		if !strings.Contains(view, want) {
			t.Fatal("missing label", want)
		}
	}
	listView := strings.Join(strings.Split(view, "\n")[:m.eventHeight()+1], "\n")
	for _, unwanted := range []string{"hidden-", "Tokens", "Cost", "Runtime", "Overview", "1234", "1.23", "delivered"} {
		if strings.Contains(listView, unwanted) {
			t.Fatal("extra data in baseline", unwanted)
		}
	}
	key(m, tea.KeyEnter)
	if !m.eventsFocused || !strings.Contains(m.View().Content, "Sessions") {
		t.Fatal("enter replaced split view")
	}
	key(m, tea.KeyEnter)
	key(m, ' ')
	if len(m.rows) != 15 {
		t.Fatal("group header hid events")
	}
	turn := m.selected().records[2].Clone()
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &turn})
	if len(m.rows) != 15 {
		t.Fatal("live update hid events")
	}
	key(m, tea.KeyEscape)
	if m.eventsFocused || m.activeSession != 1 {
		t.Fatal("escape lost session")
	}
}

func TestSessionsSortByTimeAndPreserveManualSelection(t *testing.T) {
	m := newModel(nil)
	for _, id := range []uint64{2, 1, 3} {
		m.apply(Event{Kind: Started, Session: id, At: time.Unix(int64(id), 0)})
		if m.activeSession != m.sessions[0].id {
			t.Fatal("startup cursor is not newest")
		}
	}
	if m.sessions[0].id != 3 || m.sessions[1].id != 2 || m.sessions[2].id != 1 {
		t.Fatal("restore order used instead of timestamps")
	}
	key(m, tea.KeyDown)
	m.apply(Event{Kind: Started, Session: 4, At: time.Unix(4, 0)})
	if m.activeSession != 2 {
		t.Fatal("new session stole selection")
	}
	m.apply(Event{Kind: Evicted, Session: 2})
	if m.selected() == nil || m.activeSession != 4 {
		t.Fatal("eviction left stale selection")
	}
}

func TestDividerDragResizeAndMouseHitTesting(t *testing.T) {
	m := treeFixture()
	m.apply(Event{Kind: Started, Session: 2, Title: "Second session", At: time.Unix(2, 0)})
	left, _ := m.paneWidths()
	m.Update(tea.MouseClickMsg{X: left, Y: 5, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: 60, Y: 5, Button: tea.MouseLeft})
	if l, _ := m.paneWidths(); l != 60 {
		t.Fatal("divider did not track pointer", l)
	}
	m.Update(tea.MouseReleaseMsg{X: 60, Y: 5, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: 70, Y: 5})
	if l, _ := m.paneWidths(); l != 60 || m.dragging {
		t.Fatal("drag continued after release")
	}
	m.Update(tea.MouseClickMsg{X: 4, Y: 2, Button: tea.MouseLeft})
	if m.activeSession != 1 || m.eventsFocused {
		t.Fatal("left row hit area incorrect")
	}
	m.Update(tea.MouseClickMsg{X: 62, Y: 1, Button: tea.MouseLeft})
	if !m.eventsFocused || len(m.rows) != 15 {
		t.Fatal("group header click hid events after drag")
	}
	m.resize(60, 12)
	left, right := m.paneWidths()
	if left < 16 || right < 16 || left+right+1 != 60 {
		t.Fatal("resize broke pane limits")
	}
	m.dragDivider(-100)
	if left, _ := m.paneWidths(); left != 16 {
		t.Fatal("left minimum not enforced")
	}
	m.dragDivider(10000)
	if _, right := m.paneWidths(); right != 16 {
		t.Fatal("right minimum not enforced")
	}
}

func TestSplitViewFitsTerminalAndSanitizesLabels(t *testing.T) {
	m := treeFixture()
	m.selected().title = strings.Repeat("界🙂", 100) + "\x1b[31m\u202e\nmalicious"
	for _, size := range [][2]int{{40, 8}, {60, 12}, {80, 24}, {140, 40}, {20, 5}} {
		m.resize(size[0], size[1])
		for _, split := range []float64{0.01, 0.35, 0.99} {
			m.split = split
			view := m.View().Content
			if lipgloss.Height(view) > size[1] {
				t.Fatal("view exceeds height", size)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size[0] {
					t.Fatal("view exceeds width", size)
				}
			}
			if strings.Contains(view, "\u202e") {
				t.Fatal("bidi override rendered")
			}
		}
	}
}

func TestEventSelectionSurvivesUpdatesAndOtherSessions(t *testing.T) {
	m := treeFixture()
	key(m, tea.KeyTab)
	key(m, tea.KeyDown)
	selected := m.currentRow().key
	turn := inspection.Turn{Number: 4, Steps: []inspection.Step{{Name: "Completion"}}}
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &turn})
	m.apply(Event{Kind: Started, Session: 2, At: time.Unix(2, 0)})
	if m.activeSession != 1 || m.currentRow().key != selected {
		t.Fatal("update stole event selection")
	}
}

func TestQuitCancelsAndJoinsWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stopped := make(chan struct{})
	err := Run(ctx, func(ctx context.Context, emit Emit) error {
		emit(Event{Kind: Started, Session: 1})
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	}, tea.WithInput(strings.NewReader("Q")), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("worker survived UI exit")
	}
}
