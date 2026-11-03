// Package tui presents a session overview and private turn event tables.
// Conversation bodies are confined to inspector records, never operational logs.
package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
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
const maxLogs = 200

type session struct {
	loadDetails                      func(int) (inspection.Page, error)
	detailsLoaded, detailsLoading    bool
	detailsError                     string
	before                           int
	loadGeneration                   uint64
	lastViewed                       uint64
	liveRecords                      map[int]inspection.Turn
	id                               uint64
	humanID                          string
	title                            string
	started                          time.Time
	state                            string
	logs                             []string
	turns                            int
	input, cached, output            int64
	reasoning                        int64
	reasoningKnown, reasoningMissing bool
	usageKnown                       bool
	cost                             float64
	costKnown, costMissing           bool
	records                          []inspection.Turn
	inspected                        int
	pending                          int
}

// compactTokens keeps cumulative usage legible without widening table columns.
func compactTokens(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprint(n)
}

type model struct {
	detail            *detailView
	detailBusy        bool // serialize disk reads across rapid session navigation
	viewCounter       uint64
	textOffset        int // horizontal offset of event text; numeric columns stay fixed
	operationalLog    bool
	requestQuit       func()
	quitting          bool
	workerStopped     bool
	copyText          func(string) error
	clipboardNotice   string
	clipboardUntil    time.Time
	events            <-chan tea.Msg
	busy              spinner.Model // one clock keeps visible busy rows in sync
	sessions          []*session    // newest first; all mutation belongs to the UI goroutine
	width, height     int
	activeSession     uint64 // zero selects the overview
	overviewSelection uint64
	overviewOffset    int
	overviewNavigated bool // until first navigation, keep startup restores on the newest row
	runtimeStarted    time.Time
	status            string
	rows              []treeRow
	cursor, offset    int
	folds             map[nodeKey]bool // choices survive snapshot updates
	followActive      bool
}

func newModel(events <-chan tea.Msg) *model {
	m := &model{events: events, status: "Connecting to Chatto…", width: 80, height: 24, folds: make(map[nodeKey]bool), copyText: clipboard.WriteAll}
	m.runtimeStarted = time.Now()
	m.busy = spinner.New(spinner.WithSpinner(spinner.MiniDot))
	m.resize(80, 24)
	return m
}

type inspectorTick struct{}

func tickInspector() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return inspectorTick{} })
}

func (m *model) Init() tea.Cmd { return tea.Batch(waitEvent(m.events), tickInspector(), m.busy.Tick) }

func waitEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-events }
}

func (m *model) selected() *session {
	if row := m.currentRow(); row != nil {
		return row.s
	}
	for _, s := range m.sessions {
		if s.id == m.activeSession {
			return s
		}
	}
	return nil
}

type clipboardResult struct {
	id  string
	err error
}

func (m *model) apply(event Event) {
	if event.Kind == Evicted && m.detail != nil && m.detail.session == event.Session {
		m.detail = nil
	}
	if event.At.IsZero() {
		event.At = time.Now()
	}
	if event.Kind == Connection {
		m.status = event.Text
		return
	}
	var target *session
	defer m.rebuildTree()
	for _, s := range m.sessions {
		if s.id == event.Session {
			target = s
			break
		}
	}
	if event.Kind == Started && target == nil {
		target = &session{id: event.Session, started: event.At, state: "running", turns: event.Turns}
		target.humanID = event.HumanID
		target.loadDetails = event.LoadDetails
		target.detailsLoaded = event.LoadDetails == nil
		target.title = event.Title
		for _, t := range event.Records {
			target.records = append(target.records, t.Clone())
			target.inspected = t.Number
		}
		for _, entry := range event.Logs {
			target.logs = append(target.logs, entry.At.Format("15:04:05")+"  "+displayText(entry.Text))
		}
		if target.humanID == "" {
			target.humanID = fmt.Sprintf("#%03d", event.Session)
		}
		m.sessions = append([]*session{target}, m.sessions...)
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
				followTurn := len(target.records) == 0 || target.inspected == target.records[len(target.records)-1].Number
				target.records = inspection.Retain(append(target.records, t))
				if followTurn {
					target.inspected = t.Number
				}
			}
			// Existing turns grow on every completion, not only when appended.
			target.records = inspection.Retain(target.records)
		}
	case TitleChanged:
		target.title = event.Title
	case Completed:
		target.state = "idle"
		if event.Background {
			target.state = "investigating"
		}
	case Failed:
		target.state = "failed"
	case Queued:
		target.state = "queued"
		target.pending = event.Pending
	case TurnStarted:
		target.pending = 0
		target.state = "running"
		target.turns++
	case Tokens:
		target.reasoning += event.Reasoning
		target.reasoningMissing = target.reasoningMissing || event.ReasoningMissing || !event.ReasoningKnown
		target.reasoningKnown = target.reasoningKnown || event.ReasoningKnown
		target.costMissing = target.costMissing || event.CostMissing
		if !event.Unreported {
			target.usageKnown = true
		}
		target.input += event.Input
		target.cached += event.CachedInput
		target.output += event.Output
		if event.Cost != nil {
			target.cost += *event.Cost
			target.costKnown = true
		} else {
			target.costMissing = true
		}
	}
	if event.Pending > 0 {
		target.pending = event.Pending
	}
	if event.Text != "" {
		// Bound each entry and strip terminal controls as a second line of defense.
		text := strings.Map(func(r rune) rune {
			if r < 32 || (r >= 127 && r <= 159) {
				return -1
			}
			return r
		}, event.Text)
		runes := []rune(text)
		if len(runes) > 1000 {
			text = string(runes[:1000]) + "…"
		}
		target.logs = append(target.logs, event.At.Format("15:04:05")+"  "+text)
		if len(target.logs) > maxLogs {
			target.logs = target.logs[len(target.logs)-maxLogs:]
		}
	}

}

// renderPane places a padded title inside a pane's own top border.
func renderPane(title, body string, width int, focused bool) string {
	color := lipgloss.Color("240")
	if focused {
		color = lipgloss.Color("86")
	}
	label := " " + lipgloss.NewStyle().Bold(true).MaxWidth(max(1, width-6)).MaxHeight(1).Render(title) + " "
	border := lipgloss.NewStyle().Foreground(color)
	top := border.Render("╭─") + label + border.Render(strings.Repeat("─", max(0, width-3-lipgloss.Width(label)))+"╮")
	frame := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(color).Padding(0, 1).Width(width).Render(body)
	_, rest, _ := strings.Cut(frame, "\n")
	return top + "\n" + rest
}
