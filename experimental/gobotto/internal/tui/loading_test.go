package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

func TestDetailLoadingIsDeferredPagedAndMergesLiveUpdates(t *testing.T) {
	m := newModel(nil)
	calls := 0
	m.apply(Event{Kind: Started, Session: 1, LoadDetails: func(before int) (inspection.Page, error) {
		calls++
		if before == 0 {
			return inspection.Page{Turns: []inspection.Turn{{Number: 2, State: "running", Response: "old"}}, Before: 2}, nil
		}
		if before != 2 {
			t.Fatal("wrong page cursor")
		}
		return inspection.Page{Turns: []inspection.Turn{{Number: 1, State: "delivered"}}}, nil
	}})
	if calls != 0 {
		t.Fatal("history load was synchronous")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || calls != 0 || !strings.Contains(m.View().Content, "Loading history") {
		t.Fatal("load was synchronous or has no loading state")
	}
	live := inspection.Turn{Number: 2, State: "delivered", Response: "new"}
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &live})
	m.Update(cmd())
	if m.selected().records[0].Response != "new" {
		t.Fatal("disk response overwrote newer live data")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'p'})
	if cmd == nil {
		t.Fatal("older page unavailable")
	}
	m.Update(cmd())
	if calls != 2 || len(m.selected().records) != 2 || m.selected().before != 0 {
		t.Fatal("older page failed")
	}
	if m.selected().records[1].Response != "new" {
		t.Fatal("older page regressed live state")
	}
}

func TestRapidNavigationSerializesLoadsAndBoundsCache(t *testing.T) {
	m := newModel(nil)
	load := func(int) (inspection.Page, error) {
		return inspection.Page{Turns: []inspection.Turn{{Number: 1, State: "delivered"}}}, nil
	}
	for id := uint64(1); id <= 5; id++ {
		m.apply(Event{Kind: Started, Session: id, LoadDetails: load})
	}
	first := m.loadSelected()
	if first == nil {
		t.Fatal("missing command")
	}
	if cmd := m.moveSessions(1); cmd != nil {
		t.Fatal("concurrent history reads are unbounded")
	}
	current := m.activeSession
	_, next := m.Update(first())
	if m.activeSession != current || next == nil {
		t.Fatal("late response stole navigation or failed to load current view")
	}
	m.Update(next())
	for id := uint64(1); id <= 5; id++ {
		for i, s := range m.sessions {
			if s.id == id {
				if cmd := m.moveSessions(i - m.sessionIndex()); cmd != nil {
					m.Update(cmd())
				}
				break
			}
		}
	}
	cached := 0
	for _, s := range m.sessions {
		if s.detailsLoaded {
			cached++
		} else if len(s.records) > 0 || len(s.liveRecords) > 0 {
			t.Fatal("evicted detail cache retained private content")
		}
	}
	if cached != 3 {
		t.Fatal("detail cache is not bounded", cached)
	}
}

func TestDetailFailureRetriesAndEvictedResponsesAreIgnored(t *testing.T) {
	m := newModel(nil)
	fail := true
	m.apply(Event{Kind: Started, Session: 1, LoadDetails: func(int) (inspection.Page, error) {
		if fail {
			return inspection.Page{}, errors.New("private error")
		}
		return inspection.Page{}, nil
	}})
	cmd := m.loadSelected()
	m.Update(cmd())
	if !strings.Contains(m.View().Content, "Unable to load history") || strings.Contains(m.View().Content, "private error") {
		t.Fatal("unsafe or missing load error")
	}
	fail = false
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'r'})
	if cmd == nil {
		t.Fatal("retry unavailable")
	}
	m.Update(cmd())
	if !m.selected().detailsLoaded || m.selected().detailsError != "" {
		t.Fatal("retry failed")
	}
	cmd = m.loadPage(m.selected(), 0)
	m.apply(Event{Kind: Evicted, Session: 1})
	m.Update(cmd())
	if len(m.sessions) != 0 || len(m.rows) != 0 || m.detailBusy {
		t.Fatal("late load resurrected evicted session")
	}
}
