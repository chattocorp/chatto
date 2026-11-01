package main

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
	"strings"
	"testing"
)

func exampleModel() *model {
	m := newModel(&engine{ctx: context.Background()}, "https://example.test")
	m.logged = true
	m.room = "room"
	m.rooms = []chatto.RoomEntry{{Room: chatto.Room{ID: "room", Name: "general"}}, {Room: chatto.Room{ID: "other", Name: "random"}}}
	m.users = map[string]chatto.Profile{"alice": {ID: "alice", Login: "alice"}}
	m.viewer = m.users["alice"]
	m.page = chatto.TimelinePage{Messages: []chatto.Message{{ID: "m", ActorID: "alice", Body: "**Hello** from Chatty.\n\nA small terminal client.", CreatedAt: "2026-11-01T12:00:00Z"}}}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	return m
}

func TestModifiedShortcutsAndSeparatorDragging(t *testing.T) {
	m := exampleModel()
	for _, key := range "qirohjkl" {
		m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
		if m.focus != 1 || m.busy {
			t.Fatalf("plain %c triggered an action", key)
		}
	}
	if m.compose.Value() != "qirohjkl" {
		t.Fatal("plain text did not reach input")
	}
	left, _, _ := m.columns()
	m.Update(tea.MouseClickMsg{X: left - 1, Y: 5, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: 35, Y: 5, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft})
	left, _, _ = m.columns()
	if left != 36 || m.dragSeparator != 0 {
		t.Fatal("sidebar drag failed")
	}
	m.root, m.thread = "m", m.page
	m.layout()
	left, center, _ := m.columns()
	m.Update(tea.MouseClickMsg{X: left + center - 1, Y: 5, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: 85, Y: 5, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft})
	_, _, right := m.columns()
	if right != 34 {
		t.Fatal("thread separator drag failed", right)
	}
	m.Update(tea.WindowSizeMsg{Width: 55, Height: 16})
	if lipgloss.Width(m.View().Content) > 55 || lipgloss.Height(m.View().Content) != 16 {
		t.Fatal("resized panes overflow")
	}
}

func TestScrollbackLoadsAndPreservesAnchor(t *testing.T) {
	for _, thread := range []bool{false, true} {
		m := exampleModel()
		p := chatto.TimelinePage{StartCursor: "older", HasOlder: true}
		for i := 10; i < 50; i++ {
			p.Messages = append(p.Messages, chatto.Message{ID: fmt.Sprint(i), ActorID: "alice", Body: "A message", CreatedAt: fmt.Sprintf("2026-11-01T12:%02d:00Z", i)})
		}
		m.page = p
		root := ""
		if thread {
			root = "10"
			m.root, m.thread = root, p
		}
		m.layout()
		v, rows := &m.main, m.mainRows
		if thread {
			v, rows = &m.side, m.sideRows
		}
		v.SetYOffset(0)
		anchor := captureAnchor(p, rows, 0)
		if cmd := m.scrollTimeline(thread, -3); cmd == nil || !m.busy || !m.history {
			t.Fatal("top did not request older page")
		}
		if cmd := m.scrollTimeline(thread, -3); cmd != nil {
			t.Fatal("duplicate page request")
		}
		older := chatto.TimelinePage{StartCursor: "end", Messages: []chatto.Message{{ID: "old", ActorID: "alice", Body: "Older", CreatedAt: "2026-11-01T11:00:00Z"}}}
		older.Messages = append(older.Messages, p.Messages...) // SDK returns its merged store.
		m.Update(loaded{Generation: m.generation, Snapshot: snapshot{Room: m.room, Root: root, Before: "older", Users: map[string]chatto.Profile{"new": {ID: "new", Login: "new-author"}}, Page: older}})
		if len(m.rooms) != 2 || m.viewer.ID != "alice" || m.users["alice"].Login != "alice" || m.users["new"].Login != "new-author" {
			t.Fatal("partial history response replaced directory or existing profiles")
		}
		if thread && len(m.page.Messages) != 40 {
			t.Fatal("thread pagination replaced the room timeline")
		}
		p, rows = m.page, m.mainRows
		if thread {
			p, rows = m.thread, m.sideRows
		}
		if got := captureAnchor(p, rows, v.YOffset()); got != anchor {
			t.Fatalf("reading position moved: %#v -> %#v", anchor, got)
		}
		m.Update(changed{})
		if m.busy || !m.history {
			t.Fatal("ordinary live event interrupted scrollback")
		}
		if cmd := m.scrollTimeline(thread, -1000); cmd != nil {
			t.Fatal("requested page after history exhausted")
		}
	}
}

func TestRoomLoadingIsVisible(t *testing.T) {
	m := exampleModel()
	m.chooseRoom(1)
	if !strings.Contains(ansi.Strip(m.View().Content), "Loading room") {
		t.Fatal("empty room has no loading indicator")
	}
}

func TestTabCyclesVisiblePanesAndEscapeClosesThread(t *testing.T) {
	m := exampleModel()
	m.busy = true // navigation must not start HTTP work
	m.focus = 0
	for _, want := range []int{1, 0} {
		m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if m.focus != want {
			t.Fatal("Tab did not cycle sidebar and timeline")
		}
	}
	m.root = "m"
	for _, want := range []int{1, 2, 0} {
		m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if m.focus != want {
			t.Fatal("Tab skipped the open thread")
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.focus != 2 {
		t.Fatal("Shift+Tab did not cycle backward")
	}
	m.width = 80
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focus != 2 {
		t.Fatal("Tab selected a hidden timeline")
	}
	m.compose.SetValue("thread draft")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.root != "" || m.focus != 1 || m.drafts["room/m"] != "thread draft" {
		t.Fatal("Escape did not close thread and preserve draft")
	}
}

func TestHistoryProgressAtTopAndBrailleAnimation(t *testing.T) {
	m := exampleModel()
	m.root, m.thread = "m", m.page
	m.layout()
	m.reloadTimeline("older", "m")
	view := ansi.Strip(m.View().Content)
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[1], "⠋ Loading older") || strings.Contains(lines[len(lines)-3], "Loading older") {
		t.Fatal("history progress must be in the requesting pane's top bar")
	}
	if m.timelineTitle("Room", "") != "Room" {
		t.Fatal("wrong pane shows progress")
	}
	_, cmd := m.Update(animationTick{m.animationGeneration})
	if m.spinner() != "⠙" || cmd == nil {
		t.Fatal("braille spinner did not advance")
	}
	m.busy = false
	_, cmd = m.Update(animationTick{m.animationGeneration})
	if cmd != nil {
		t.Fatal("animation kept running while idle")
	}
	if lipgloss.Height(view) != m.height {
		t.Fatal("loading shifted layout")
	}
}

func TestPaneHorizontalPadding(t *testing.T) {
	text := ansi.Strip(terminalPane("Title", "hello", 20, 4, true))
	lines := strings.Split(text, "\n")
	if !strings.HasPrefix(lines[1], " hello") || !strings.HasSuffix(lines[1], " │") || lipgloss.Width(text) != 20 {
		t.Fatalf("pane padding or separator position wrong: %q", lines[1])
	}
	m := exampleModel()
	left, center, _ := m.columns()
	if m.main.Width() != center-3 || lipgloss.Width(m.sidebarContent(left-3, m.bodyHeight())) > left-3 {
		t.Fatal("content does not reserve padding columns")
	}
}
func TestResponsiveViewAndTerminalSafety(t *testing.T) {
	m := exampleModel()
	m.root = "m"
	m.thread = m.page
	for _, width := range []int{55, 80, 120, 180} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 32})
		view := m.View().Content
		if lipgloss.Width(view) > width || lipgloss.Height(view) != 32 {
			t.Fatalf("view overflows at %d: %dx%d", width, lipgloss.Width(view), lipgloss.Height(view))
		}
		if !strings.Contains(view, "THREAD") || !strings.Contains(view, "example.test") {
			t.Fatal("missing panes")
		}
	}
	if strings.ContainsRune(clean("hello\x1b]52;c;secret\a"), '\x1b') {
		t.Fatal("terminal escape survived")
	}
}

func TestIRCMessageRowsAndMouseSelection(t *testing.T) {
	m := exampleModel()
	m.page.Messages[0].Body = "Hello, terminal."
	m.page.Messages = append(m.page.Messages, chatto.Message{ID: "next", ActorID: "alice", Body: strings.Repeat("wrapped words ", 20), CreatedAt: m.page.Messages[0].CreatedAt})
	m.renderMessages()
	text, rows := m.renderPage(m.page, -1, m.main.Width())
	lines := strings.Split(ansi.Strip(text), "\n")
	if len(rows) != 2 || rows[0].Start != 1 || rows[0].End != rows[1].Start || rows[0].End-rows[0].Start != 1 {
		t.Fatalf("short messages must use one row, with no gaps: %#v", rows)
	}
	if !strings.Contains(lines[1], "         <alice> Hello, terminal.") {
		t.Fatalf("nickname and body not inline: %q", lines[1])
	}
	if lipgloss.Width(text) > m.main.Width() || !strings.Contains(lines[0], "2026") {
		t.Fatal("message width or date separator incorrect")
	}
	left, _, _ := m.columns()
	m.Update(tea.MouseClickMsg{X: left + 10, Y: 2 + rows[1].Start, Button: tea.MouseLeft})
	if m.selected != 1 {
		t.Fatal("message hit target did not track compact layout")
	}
	m.Update(tea.MouseClickMsg{X: 4, Y: m.height - 2, Button: tea.MouseLeft})
	if m.focus != 1 || !m.compose.Focused() {
		t.Fatal("input line is not clickable")
	}
}
func TestStaleLoadsAndRoomDrafts(t *testing.T) {
	m := exampleModel()
	m.busy = true
	m.compose.SetValue("first draft")
	m.chooseRoom(1)
	m.compose.SetValue("second draft")
	_, cmd := m.Update(loaded{Generation: 0, Snapshot: snapshot{Room: "room", Page: chatto.TimelinePage{Messages: []chatto.Message{{ID: "stale"}}}}})
	if m.room != "other" || len(m.page.Messages) != 0 || cmd == nil {
		t.Fatal("stale response won navigation")
	}
	m.busy = false
	m.sentDraftKey = "room/"
	m.Update(actionDone{Kind: "send"})
	if m.compose.Value() != "second draft" {
		t.Fatal("send completion erased a different room draft")
	}
}
func TestLoginHidesPassword(t *testing.T) {
	m := newModel(&engine{ctx: context.Background()}, "https://example.test")
	m.fields[2].SetValue("secret-password")
	if strings.Contains(m.View().Content, "secret-password") {
		t.Fatal("password visible")
	}
	for _, width := range []int{55, 80, 120} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 28})
		if lipgloss.Width(m.View().Content) > width || lipgloss.Height(m.View().Content) > 28 {
			t.Fatal("login overflows terminal")
		}
	}
}

func TestInvalidationRejectsAlreadyRunningRead(t *testing.T) {
	m := exampleModel()
	m.busy = true
	m.e.invalidated.Store(true)
	m.Update(changed{})
	if len(m.page.Messages) != 0 || len(m.rooms) != 0 {
		t.Fatal("revoked data stayed visible")
	}
	_, cmd := m.Update(loaded{Generation: 0, Snapshot: snapshot{Room: "room", Page: chatto.TimelinePage{Messages: []chatto.Message{{ID: "old"}}}}})
	if len(m.page.Messages) != 0 || cmd == nil {
		t.Fatal("old in-flight read restored invalidated data")
	}
}

func TestPasteReachesLoginAndComposer(t *testing.T) {
	m := newModel(&engine{ctx: context.Background()}, "")
	m.Update(tea.PasteMsg{Content: "https://example.test"})
	if m.fields[0].Value() != "https://example.test" {
		t.Fatal("login paste dropped")
	}
	m.logged = true
	m.setFocus(1)
	m.Update(tea.PasteMsg{Content: "hello"})
	if m.compose.Value() != "hello" {
		t.Fatal("composer paste dropped")
	}
}

func TestCursorNavigationWithAlwaysActiveInput(t *testing.T) {
	m := exampleModel()
	m.page.Messages = append(m.page.Messages, chatto.Message{ID: "second", ActorID: "alice", Body: "Second message"})
	m.renderMessages()
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.selected != 1 {
		t.Fatal("Down did not select next message")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.selected != 0 {
		t.Fatal("Up did not select previous message")
	}
	m.Update(tea.PasteMsg{Content: "room draft"})
	m.busy = true // prevent actual network commands in this interaction test
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.root != "" || m.compose.Value() != "room draft" {
		t.Fatal("Enter with a busy draft must not open a thread")
	}
	m.openThread()
	if m.root != "m" || m.focus != 2 || m.drafts["room/"] != "room draft" {
		t.Fatal("Opening a thread did not retain the room draft")
	}
	m.Update(tea.PasteMsg{Content: "thread reply"})
	if m.compose.Value() != "thread reply" || !m.compose.Focused() {
		t.Fatal("input stopped accepting text in thread")
	}
	m.busy = false
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.busy || m.sentBody != "thread reply" || m.sentDraftKey != "room/m" {
		t.Fatal("Enter did not submit the thread draft")
	}
}

func TestEnterSendsRoomDraftOrOpensThreadWhenEmpty(t *testing.T) {
	m := exampleModel()
	m.compose.SetValue("a room message")
	m.setFocus(0) // Sidebar focus sends to the room.
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || m.sentBody != "a room message" || m.sentDraftKey != "room/" || m.root != "" {
		t.Fatal("Enter did not send the visible room draft")
	}
	m = exampleModel()
	m.busy = true // Do not run a network request in this key-binding test.
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.root != "m" || m.focus != 2 {
		t.Fatal("empty Enter did not open selected thread")
	}
}

func TestSendDestinationFollowsFocusWithThreadOpen(t *testing.T) {
	for _, focus := range []int{0, 1, 2} {
		m := exampleModel()
		m.compose.SetValue("room draft")
		m.openThread()
		m.compose.SetValue("thread draft")
		m.setFocus(focus)
		wantKey, wantBody := "room/", "room draft"
		if focus == 2 {
			wantKey, wantBody = "room/m", "thread draft"
		}
		if m.compose.Value() != wantBody {
			t.Fatalf("focus %d restored wrong draft", focus)
		}
		m.busy = false
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil || m.sentDraftKey != wantKey || m.sentBody != wantBody {
			t.Fatalf("focus %d sent to wrong destination", focus)
		}
		if m.root != "m" {
			t.Fatal("sending closed the thread")
		}
		m.setFocus(2)
		if m.compose.Value() != "thread draft" {
			t.Fatal("thread draft lost")
		}
		m.setFocus(1)
		if m.compose.Value() != "room draft" {
			t.Fatal("room draft lost")
		}
	}
}
