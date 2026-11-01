package main

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func longTimeline() chatto.TimelinePage {
	p := chatto.TimelinePage{}
	for i := range 50 {
		p.Messages = append(p.Messages, chatto.Message{ID: fmt.Sprintf("m%d", i), ActorID: "alice", Body: "A timeline message"})
	}
	return p
}

func receiveTimelines(m *model, room, thread chatto.TimelinePage) {
	p := room
	if m.root != "" {
		p = thread
	}
	m.Update(loaded{Generation: m.generation, Snapshot: snapshot{Room: m.room, Root: m.root, Rooms: m.rooms, Groups: m.groups, Viewer: m.viewer, Users: m.users, Page: p, RoomPage: room, Live: true}})
}

func TestLiveTimelineFollowsBottomButPreservesScrollback(t *testing.T) {
	for _, actor := range []string{"alice", "other"} {
		for _, scrollback := range []bool{false, true} {
			m := exampleModel()
			m.page = longTimeline()
			m.selected = len(m.page.Messages) - 1
			m.renderMessages()
			m.main.GotoBottom()
			if scrollback {
				m.main.ScrollUp(10)
			}
			anchor := captureAnchor(m.page, m.mainRows, m.main.YOffset())
			p := m.page
			p.Messages = append(p.Messages, chatto.Message{ID: "new", ActorID: actor, Body: "New arrival"})
			receiveTimelines(m, p, chatto.TimelinePage{})
			if scrollback {
				if got := captureAnchor(m.page, m.mainRows, m.main.YOffset()); got != anchor {
					t.Fatal("incoming message moved scrollback")
				}
			} else if !m.main.AtBottom() || m.selected != len(p.Messages)-1 {
				t.Fatal("incoming message was not followed")
			}
		}
	}
}

func TestRoomAndThreadFollowIndependently(t *testing.T) {
	for _, readingThread := range []bool{false, true} {
		m := exampleModel()
		m.root = "thread"
		m.page, m.thread = longTimeline(), longTimeline()
		m.selected, m.threadSelected = 49, 49
		m.layout()
		m.main.GotoBottom()
		m.side.GotoBottom()
		reading := &m.main
		if readingThread {
			reading = &m.side
		}
		reading.ScrollUp(10)
		offset := reading.YOffset()
		room, thread := m.page, m.thread
		room.Messages = append(room.Messages, chatto.Message{ID: "new-room", Body: "Room arrival"})
		thread.Messages = append(thread.Messages, chatto.Message{ID: "new-thread", Body: "Thread arrival"})
		receiveTimelines(m, room, thread)
		if reading.YOffset() != offset {
			t.Fatal("inactive scrollback pane moved")
		}
		if readingThread && !m.main.AtBottom() || !readingThread && !m.side.AtBottom() {
			t.Fatal("live pane did not follow")
		}
	}
}

func TestSendingReturnsDestinationToBottomBeforeRealtimeArrival(t *testing.T) {
	for _, thread := range []bool{false, true} {
		m := exampleModel()
		m.page = longTimeline()
		if thread {
			m.root = "thread"
			m.thread = longTimeline()
			m.setFocus(2)
		}
		m.layout()
		m.main.SetYOffset(2)
		m.side.SetYOffset(2)
		m.compose.SetValue("My new message")
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("send command missing")
		}
		if thread {
			if !m.side.AtBottom() || m.main.YOffset() != 2 {
				t.Fatal("send scrolled the wrong pane")
			}
		} else if !m.main.AtBottom() {
			t.Fatal("send did not return to live history")
		}
		// Commands remain unexecuted: this test sends no real messages.
	}
}

func TestCursorAtFirstMessageLoadsOlderInEitherPane(t *testing.T) {
	for _, thread := range []bool{false, true} {
		m := exampleModel()
		m.page = longTimeline()
		if thread {
			m.root = "thread"
			m.thread = longTimeline()
			m.focus = 2
		}
		p := m.timeline(thread)
		p.page.HasOlder, p.page.StartCursor = true, "older-page"
		*p.selected = 1
		m.layout()
		first := p.page.Messages[0].ID
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		if cmd == nil || !m.busy || !m.loadingOlder || m.loadingRoot != p.root || *p.selected != 0 {
			t.Fatal("reaching first message did not load the correct pane")
		}
		_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		if cmd != nil {
			t.Fatal("held cursor key started duplicate request")
		}
		merged := chatto.TimelinePage{Messages: append([]chatto.Message{{ID: "older", Body: "Earlier message"}}, p.page.Messages...)}
		anchor := captureAnchor(*p.page, *p.rows, p.view.YOffset())
		m.Update(loaded{Generation: m.generation, Snapshot: snapshot{Room: m.room, Root: p.root, Before: "older-page", Page: merged}})
		if p.page.Messages[*p.selected].ID != first || *p.selected != 1 {
			t.Fatal("pagination changed selected message")
		}
		if captureAnchor(*p.page, *p.rows, p.view.YOffset()) != anchor {
			t.Fatal("pagination moved reading position")
		}
		_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		if cmd != nil || p.page.Messages[*p.selected].ID != "older" {
			t.Fatal("navigation did not reach older message or ignored history exhaustion")
		}
	}
}
