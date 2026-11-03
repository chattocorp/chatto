// Package tui presents sessions and their turn events in a split terminal view.
// Conversation bodies are confined to inspector records, never operational logs.
package tui

import (
	"context"
	"fmt"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

// Kind identifies a runtime event. Only Started creates a session.
type Kind int

const (
	Connection Kind = iota
	Started
	Log
	Completed
	Failed
	Queued
	TurnStarted
	Tokens
	Evicted
	TitleChanged
	Inspected
)

// Event carries host-authored operational text. Text must never contain chat
// content, credentials, or unfiltered external errors. Session is a local counter.
type Event struct {
	// LoadDetails reads a detached page from persisted storage, never worker
	// state. The UI invokes it asynchronously only for opened sessions.
	LoadDetails func(before int) (inspection.Page, error)
	Kind        Kind
	Session     uint64
	// Background keeps session activity visible between parent conversation turns.
	Background bool
	// HumanID matches the persistent snapshot name; it contains no user data.
	HumanID string
	// Title is private model-generated display text, never an operational log.
	Title string
	Text  string
	At    time.Time
	// Provider-reported token deltas. CachedInput is included in Input.
	Input, CachedInput, Output       int64
	Reasoning                        int64
	ReasoningKnown, ReasoningMissing bool
	// Cost is the reported account charge for this request, or nil if unknown.
	Cost *float64
	// Restore metadata; ordinary events leave these at their zero values.
	Turns       int
	CostMissing bool
	Unreported  bool
	// Inspection data is private and must never be appended to log text.
	Inspection *inspection.Turn
	Records    []inspection.Turn
	Logs       []inspection.Log
	Pending    int
}

const maxSessions = 100

type session struct {
	loadDetails                   func(int) (inspection.Page, error)
	detailsLoaded, detailsLoading bool
	detailsError                  string
	before                        int
	loadGeneration                uint64
	lastViewed                    uint64
	liveRecords                   map[int]inspection.Turn
	id                            uint64
	humanID                       string
	title                         string
	started                       time.Time
	busy                          bool
	records                       []inspection.Turn
}

type model struct {
	detailBusy     bool // serialize disk reads across rapid session navigation
	viewCounter    uint64
	requestQuit    func()
	quitting       bool
	workerStopped  bool
	events         <-chan tea.Msg
	sessions       []*session // newest first; all mutation belongs to the UI goroutine
	width, height  int
	activeSession  uint64
	sessionOffset  int
	navigated      bool // startup restores stay on the newest session until navigation
	eventsFocused  bool
	split          float64 // fraction of available width assigned to sessions
	dragging       bool
	detailSplit    float64 // fraction of right-side content assigned to events
	draggingDetail bool
	status         string
	rows           []treeRow
	cursor, offset int
	detailOffset   int
	activityFrame  int
	showBrains     bool
	brains         brainsView
}

func newModel(events <-chan tea.Msg) *model {
	m := &model{events: events, status: "Connecting to Chatto…", width: 80, height: 24, split: 0.35, detailSplit: 0.65}
	m.brains = brainsView{ctx: context.Background(), split: 0.35}
	m.resize(80, 24)
	return m
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(waitEvent(m.events), activityTick(), waitBrainChanges(m.brains.changes))
}

func waitEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-events }
}

func (m *model) selected() *session {
	for _, s := range m.sessions {
		if s.id == m.activeSession {
			return s
		}
	}
	return nil
}

func (m *model) apply(event Event) {
	if event.At.IsZero() {
		event.At = time.Now()
	}
	if event.Kind == Connection {
		m.status = event.Text
		return
	}
	var target *session
	defer func() { m.syncSession(); m.rebuildTree() }()
	for _, s := range m.sessions {
		if s.id == event.Session {
			target = s
			break
		}
	}
	if event.Kind == Started && target == nil {
		target = &session{id: event.Session, started: event.At}
		target.humanID = event.HumanID
		target.loadDetails = event.LoadDetails
		target.detailsLoaded = event.LoadDetails == nil
		target.title = event.Title
		for _, t := range event.Records {
			target.records = append(target.records, t.Clone())
		}
		if target.humanID == "" {
			target.humanID = fmt.Sprintf("#%03d", event.Session)
		}
		m.sessions = append([]*session{target}, m.sessions...)
		sort.SliceStable(m.sessions, func(i, j int) bool {
			if m.sessions[i].started.Equal(m.sessions[j].started) {
				return m.sessions[i].id > m.sessions[j].id
			}
			return m.sessions[i].started.After(m.sessions[j].started)
		})
		if len(m.sessions) > maxSessions {
			m.sessions = m.sessions[:maxSessions]
		}
	}
	if target == nil {
		return
	}
	if event.Kind == Evicted {
		for i, s := range m.sessions {
			if s == target {
				m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
				break
			}
		}

		return
	}
	switch event.Kind {
	case TurnStarted:
		target.busy = true
	case Completed:
		target.busy = event.Background
	case Failed, Queued:
		target.busy = false
	case Inspected:
		if event.Inspection != nil {
			if target.loadDetails != nil && !target.detailsLoaded && !target.detailsLoading {
				break
			}
			t := event.Inspection.Clone()
			if target.loadDetails != nil {
				if target.liveRecords == nil {
					target.liveRecords = make(map[int]inspection.Turn)
				}
				target.liveRecords[t.Number] = t
				live := make([]inspection.Turn, 0, len(target.liveRecords))
				for _, turn := range target.liveRecords {
					live = append(live, turn)
				}
				sort.Slice(live, func(i, j int) bool { return live[i].Number < live[j].Number })
				clear(target.liveRecords)
				for _, turn := range inspection.Retain(live) {
					target.liveRecords[turn.Number] = turn
				}
			}
			found := false
			for i, old := range target.records {
				if old.Number == t.Number {
					target.records[i] = t
					found = true
					break
				}
			}
			if !found {
				target.records = inspection.Retain(append(target.records, t))
			}
			// Existing turns grow on every completion, not only when appended.
			target.records = inspection.Retain(target.records)
		}
	case TitleChanged:
		target.title = event.Title
	}

}
