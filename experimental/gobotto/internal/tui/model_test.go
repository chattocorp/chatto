package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestMouseSelectsVisiblePageAndScrollsLogs(t *testing.T) {
	m := newModel(nil)
	for id := uint64(1); id <= 12; id++ {
		m.apply(Event{Kind: Started, Session: id, Text: fmt.Sprintf("Session %d", id)})
	}
	// Select a row on a later list page; mouse coordinates address visible rows.
	m.list.Select(7)
	first := m.list.Paginator.Page * m.list.Paginator.PerPage
	m.Update(tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft})
	if m.list.Index() != first || !strings.Contains(m.logs.GetContent(), fmt.Sprintf("Session %d", m.sessions[first].id)) {
		t.Fatal("clicked row did not select its session and logs")
	}
	for range 30 {
		m.apply(Event{Kind: Log, Session: m.selected().id, Text: "More output"})
	}
	m.Update(tea.MouseWheelMsg{X: 5, Y: m.list.Height() + 6, Button: tea.MouseWheelUp})
	if m.logs.AtBottom() {
		t.Fatal("mouse wheel did not scroll logs")
	}
	m.Update(tea.MouseClickMsg{X: 5, Y: m.list.Height() + 6, Button: tea.MouseLeft})
	if !m.logsFocused {
		t.Fatal("click did not focus log pane")
	}
}

func TestNewSessionRevealedFromLaterPage(t *testing.T) {
	m := newModel(nil)
	for id := uint64(1); id <= 20; id++ {
		m.apply(Event{Kind: Started, Session: id})
	}
	m.list.Select(15)
	m.refreshLogs(true)
	m.switchMode(true)
	if m.list.Paginator.Page == 0 {
		t.Fatal("test requires an old selection on another page")
	}
	id := "warm-wren-04c86adcdb4c"
	m.apply(Event{Kind: Started, Session: 21, HumanID: id})
	if m.selected().id != 21 || m.list.Paginator.Page != 0 || !strings.Contains(m.View().Content, id) || !strings.Contains(m.logs.GetContent(), "No recorded turns yet") {
		t.Fatal("new session and inspector did not become visible together")
	}
	copied := ""
	m.copyText = func(text string) error { copied = text; return nil }
	_, cmd := m.Update(tea.MouseClickMsg{X: m.turnListWidth() + 16, Y: m.list.Height() + 3, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("new session title cannot be copied")
	}
	m.Update(cmd())
	if copied != id {
		t.Fatal("copied an older session ID")
	}
	m.apply(Event{Kind: Log, Session: 1, Text: "Older conversation updated"})
	if m.selected().id != 21 {
		t.Fatal("ordinary activity changed selection")
	}
}

func TestDividerDragClampsAndSurvivesResize(t *testing.T) {
	m := newModel(nil)
	initial := m.list.Height()
	y := initial + 3 // lower pane's top border, immediately below the session pane
	m.Update(tea.MouseClickMsg{X: 10, Y: y, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: 10, Y: y + 3, Button: tea.MouseLeft})
	if m.list.Height() != initial+3 {
		t.Fatal("divider did not follow drag")
	}
	m.Update(tea.MouseReleaseMsg{X: 10, Y: y + 3, Button: tea.MouseLeft})
	if m.dragging {
		t.Fatal("drag did not end")
	}
	ratio := m.split
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if m.split != ratio {
		t.Fatal("terminal resize reset split")
	}
	m.Update(tea.MouseClickMsg{X: 10, Y: m.list.Height() + 2, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: 10, Y: 999, Button: tea.MouseLeft})
	if m.logs.Height() != 2 {
		t.Fatal("bottom pane collapsed")
	}
	m.Update(tea.MouseMotionMsg{X: 10, Y: -999, Button: tea.MouseLeft})
	if m.list.Height() != 2 {
		t.Fatal("top pane collapsed")
	}
	if lipgloss.Height(m.View().Content) != 40 {
		t.Fatal("drag changed terminal layout height")
	}
}

func TestSelectedRowFillsWidthWithoutMarker(t *testing.T) {
	m := newModel(nil)
	m.apply(Event{Kind: Started, Session: 1})
	var out bytes.Buffer
	delegate{}.Render(&out, m.list, 0, m.selected())
	if strings.Contains(out.String(), ">") {
		t.Fatal("selection marker remains")
	}
	if lipgloss.Width(out.String()) != m.list.Width() {
		t.Fatal("selected row does not fill pane width")
	}
	if !strings.Contains(out.String(), "48;2;36;79;82") {
		t.Fatalf("selected row has no background: %q", out.String())
	}
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("mouse tracking disabled")
	}
}

func TestSessionSelectionAndLiveLogs(t *testing.T) {
	m := newModel(nil)
	m.apply(Event{Kind: Started, Session: 1, Text: "First session"})
	m.apply(Event{Kind: Started, Session: 2, Text: "Second session"})
	if m.selected().id != 2 {
		t.Fatal("new session was not selected")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.selected().id != 2 || !strings.Contains(m.logs.GetContent(), "Second session") {
		t.Fatal("selection did not switch logs")
	}
	m.apply(Event{Kind: Log, Session: 1, Text: "Hidden update"})
	if strings.Contains(m.logs.GetContent(), "Hidden update") {
		t.Fatal("logs leaked between sessions")
	}
	m.apply(Event{Kind: Log, Session: 2, Text: "Model request completed"})
	m.apply(Event{Kind: Completed, Session: 2, Text: "Session completed"})
	if !strings.Contains(m.logs.GetContent(), "Model request completed") || m.selected().state != "idle" {
		t.Fatal("selected session did not update")
	}
}

func TestHumanIDLabelsSessionAndLogs(t *testing.T) {
	m := newModel(nil)
	id := "calm-otter-012345abcdef"
	m.apply(Event{Kind: Started, Session: 7, HumanID: id})
	var row bytes.Buffer
	delegate{}.Render(&row, m.list, 0, m.selected())
	if !strings.Contains(row.String(), id) || !strings.Contains(m.View().Content, id+" · running") {
		t.Fatal("readable ID missing from session row or log title")
	}
	m.apply(Event{Kind: TitleChanged, Session: 7, Title: "Understanding cache hits"})
	row.Reset()
	delegate{}.Render(&row, m.list, 0, m.selected())
	if !strings.Contains(row.String(), "Understanding cache hits") || !strings.Contains(m.View().Content, id+" · running") || len(m.selected().logs) != 0 {
		t.Fatal("title changed the stable ID or leaked into logs")
	}
}

func TestClickSessionTitleCopiesSelectedID(t *testing.T) {
	m := newModel(nil)
	id := "calm-otter-012345abcdef"
	m.apply(Event{Kind: Started, Session: 1, HumanID: id})
	m.apply(Event{Kind: Started, Session: 2, HumanID: "quiet-fox-abcdef012345"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	copied := ""
	m.copyText = func(text string) error { copied = text; return nil }
	for _, height := range []int{24, 40} {
		m.resize(80, height)
		_, cmd := m.Update(tea.MouseClickMsg{X: 3, Y: m.list.Height() + 3, Button: tea.MouseLeft})
		if cmd == nil || m.dragging {
			t.Fatal("name click started dragging instead of copying")
		}
		m.Update(cmd())
		if copied != id || !strings.Contains(m.View().Content, "Copied "+id) {
			t.Fatal("wrong selected ID or missing confirmation")
		}
	}
	m.copyText = func(string) error { return fmt.Errorf("clipboard failure") }
	_, cmd := m.Update(tea.MouseClickMsg{X: 3, Y: m.list.Height() + 3, Button: tea.MouseLeft})
	m.Update(cmd())
	if !strings.Contains(m.View().Content, "Clipboard unavailable") {
		t.Fatal("clipboard failure reported as success")
	}
	_, cmd = m.Update(tea.MouseClickMsg{X: 3 + len(id), Y: m.list.Height() + 3, Button: tea.MouseLeft})
	if cmd != nil || !m.dragging {
		t.Fatal("separator outside name no longer drags")
	}
}

func TestThreadTurnsAccumulateUsageInOneRow(t *testing.T) {
	m := newModel(nil)
	m.apply(Event{Kind: Started, Session: 1})
	cost := 0.00125
	for range 2 {
		m.apply(Event{Kind: TurnStarted, Session: 1})
		m.apply(Event{Kind: Tokens, Session: 1, Input: 100, CachedInput: 80, Output: 10, Cost: &cost})
		m.apply(Event{Kind: Completed, Session: 1})
	}
	s := m.selected()
	if s.cost != 0.0025 || !strings.Contains(m.View().Content, "220 tokens · $0.002500") || len(s.logs) != 0 {
		t.Fatalf("incorrect totals or noisy usage logs: %+v", s)
	}
	m.apply(Event{Kind: Tokens, Session: 1})
	if !strings.Contains(m.View().Content, "(partial)") {
		t.Fatal("missing cost was treated as zero")
	}
	if len(m.sessions) != 1 || s.turns != 2 || s.input != 200 || s.cached != 160 || s.output != 20 || s.state != "idle" {
		t.Fatalf("session=%+v", s)
	}
	m.apply(Event{Kind: Started, Session: 2})
	m.apply(Event{Kind: Evicted, Session: 1})
	if m.selected().id != 2 || len(m.sessions) != 1 {
		t.Fatal("registry eviction did not remove the correct row")
	}
}

func TestScrollbackIsStableUntilFollow(t *testing.T) {
	m := newModel(nil)
	m.apply(Event{Kind: Started, Session: 1})
	for i := range 40 {
		m.apply(Event{Kind: Log, Session: 1, Text: fmt.Sprintf("Entry %d", i)})
	}
	if !m.logs.AtBottom() {
		t.Fatal("new logs should follow")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	offset := m.logs.YOffset()
	if m.logs.AtBottom() {
		t.Fatal("page up did not enter scrollback")
	}
	m.apply(Event{Kind: Log, Session: 1, Text: "New entry"})
	if m.logs.YOffset() != offset {
		t.Fatal("incoming log moved scrollback")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if !m.logs.AtBottom() {
		t.Fatal("end did not resume following")
	}
}

func TestRetentionAndResize(t *testing.T) {
	m := newModel(nil)
	for id := uint64(1); id <= maxSessions+1; id++ {
		m.apply(Event{Kind: Started, Session: id, Text: "Started"})
	}
	if len(m.sessions) != maxSessions || m.selected().id != maxSessions+1 {
		t.Fatal("pruned selection was not replaced")
	}
	for range maxLogs + 20 {
		m.apply(Event{Kind: Log, Session: maxSessions + 1, Text: "entry"})
	}
	if len(m.selected().logs) != maxLogs {
		t.Fatal("logs are not bounded")
	}
	for _, size := range [][2]int{{80, 24}, {120, 40}, {36, 12}, {20, 5}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View()
		if lipgloss.Width(view.Content) > size[0] || lipgloss.Height(view.Content) > size[1] {
			t.Fatalf("view %dx%d exceeds terminal %dx%d:\n%s", lipgloss.Width(view.Content), lipgloss.Height(view.Content), size[0], size[1], view.Content)
		}
		if !view.AltScreen {
			t.Fatal("alternate screen disabled")
		}
	}
}

func TestQuitCancelsAndJoinsWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stopped := make(chan struct{})
	err := Run(ctx, func(ctx context.Context, emit Emit) error {
		emit(Event{Kind: Started, Session: 1, Text: "Started"})
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
