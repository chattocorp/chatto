package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestActivityTracksRuntimeWithoutMovingLabels(t *testing.T) {
	m := treeFixture()
	m.apply(Event{Kind: TurnStarted, Session: 1})
	turn := m.selected().records[2].Clone()
	turn.State = "running"
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &turn})
	if !m.selected().busy || !m.rows[0].busy {
		t.Fatal("active session or turn missing activity")
	}
	first := m.activityPrefix(true)
	_, cmd := m.Update(activityTickMsg{})
	if cmd == nil || m.activityPrefix(true) == first {
		t.Fatal("activity did not animate")
	}
	if ansi.StringWidth(m.activityPrefix(true)) != 2 || m.activityPrefix(false) != "  " {
		t.Fatal("activity moved label indentation")
	}
	turn.State = "delivered"
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &turn})
	m.apply(Event{Kind: Completed, Session: 1, Background: true})
	if !m.selected().busy || m.rows[0].busy {
		t.Fatal("background work confused session and turn activity")
	}
	m.apply(Event{Kind: Completed, Session: 1})
	if m.selected().busy {
		t.Fatal("completed session still busy")
	}
	m.apply(Event{Kind: TurnStarted, Session: 1})
	m.apply(Event{Kind: Failed, Session: 1})
	if m.selected().busy {
		t.Fatal("failed session still busy")
	}
	m.workerStopped = true
	if m.activityPrefix(true) != "  " {
		t.Fatal("stopped worker still animates")
	}
}
