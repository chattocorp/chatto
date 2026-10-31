// Package tui presents operational logs and a separate private turn inspector.
// Conversation bodies are confined to inspector records, never operational logs.
package tui

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
	"github.com/chatto-chat/chatto/experimental/gobotto/memorytree"
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
	MemoryChanged
)

// Event carries host-authored operational text. Text must never contain chat
// content, credentials, or unfiltered external errors. Session is a local counter.
type Event struct {
	// Memory is private browser data, never operational log text.
	Memory  *memorytree.Snapshot
	Kind    Kind
	Session uint64
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
	// Inspection data is private and must never be appended to log text.
	Inspection *inspection.Turn
	Records    []inspection.Turn
	Logs       []inspection.Log
	Pending    int
}

const maxSessions = 100
const maxLogs = 200

type session struct {
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
	expanded                         bool
	logOffset, inspectOffset         int
	pending                          int
}

func (s *session) FilterValue() string { return "" }

type delegate struct{}

func (delegate) Height() int                         { return 1 }
func (delegate) Spacing() int                        { return 0 }
func (delegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (delegate) Render(w io.Writer, l list.Model, index int, item list.Item) {
	s := item.(*session)
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#AEB8C5"))
	if index == l.Index() {
		base = base.Background(lipgloss.Color("#244F52")).Foreground(lipgloss.Color("#E8FFFF"))
	}
	cell := func(text string, width int, style lipgloss.Style) string {
		return style.Width(width).MaxWidth(width).MaxHeight(1).Render(text)
	}
	statusColor := "#8593A4"
	switch s.state {
	case "running":
		statusColor = "#5EEAD4"
	case "queued":
		statusColor = "#F2C879"
	case "failed":
		statusColor = "#F38BA8"
	}
	status := cell(s.state, 7, base.Foreground(lipgloss.Color(statusColor)))
	gap := base.Render("  ")
	columns := []string{status}
	reserved := 11 // leading padding, status, and one gap
	if l.Width() >= 58 {
		columns = append(columns, cell(fmt.Sprintf("%d turns", s.turns), 7, base))
		reserved += 9
		if l.Width() >= 74 {
			tokens := "tokens —"
			if s.usageKnown {
				tokens = compactTokens(s.input+s.output) + " tokens"
			}
			columns = append(columns, cell(tokens, 11, base))
			reserved += 13
		}
		cost := "cost —"
		if s.costKnown {
			cost = fmt.Sprintf("$%.6f", s.cost)
			if s.costMissing {
				cost += "+"
			}
		}
		columns = append(columns, cell(cost, 10, base))
		reserved += 12
		if l.Width() >= 108 {
			columns = append(columns, cell(s.started.Format("15:04:05"), 8, base))
			reserved += 10
		}
	}
	label := s.title
	if label == "" {
		label = s.humanID
	}
	name := cell(label, max(1, l.Width()-reserved), base.Bold(true))
	line := base.Render("  ") + name + gap + strings.Join(columns, gap)
	fmt.Fprint(w, base.Width(l.Width()).MaxWidth(l.Width()).MaxHeight(1).Render(line))
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
	memoryMode      bool
	typesMode       bool
	types           typeBrowser
	memory          memoryBrowser
	requestQuit     func()
	quitting        bool
	workerStopped   bool
	copyText        func(string) error
	clipboardNotice string
	clipboardUntil  time.Time
	events          <-chan tea.Msg
	list            list.Model
	turnList        list.Model
	logs            viewport.Model
	stats           viewport.Model
	sessions        []*session // newest first; all mutation belongs to the UI goroutine
	width, height   int
	logsFocused     bool
	status          string
	inspectMode     bool
	turnsFocused    bool
	// split is the session pane's share of usable rows. Dragging changes this
	// ratio; terminal resizes retain it while leaving both panes usable.
	split           float64
	splitCustomized bool
	dragging        bool
	dragY, dragTop  int
}

func newModel(events <-chan tea.Msg) *model {
	l := list.New(nil, delegate{}, 76, 6)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	v := viewport.New(viewport.WithWidth(76), viewport.WithHeight(12))
	v.SoftWrap = true
	v.FillHeight = true
	m := &model{events: events, list: l, logs: v, status: "Connecting to Chatto…", width: 80, height: 24, split: 1.0 / 3, copyText: clipboard.WriteAll}
	m.turnList = newTurnList()
	m.stats = viewport.New()
	m.stats.SoftWrap = true
	m.stats.FillHeight = true
	m.resize(80, 24)
	m.refreshLogs(true)
	return m
}

type inspectorTick struct{}

func tickInspector() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return inspectorTick{} })
}

func (m *model) Init() tea.Cmd { return tea.Batch(waitEvent(m.events), tickInspector()) }

func waitEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-events }
}

func (m *model) selected() *session {
	s, _ := m.list.SelectedItem().(*session)
	return s
}

func (m *model) resize(width, height int) {
	m.width, m.height = width, height
	m.logs.SoftWrap = !m.inspectMode
	m.logs.SetHorizontalStep(8)
	// Header, footer, and the two pane borders use six rows in total.
	available := max(4, height-6)
	top := min(available-2, max(2, int(math.Round(float64(available)*m.split))))
	if !m.splitCustomized {
		top = min(top, 8)
	}
	if m.inspectMode {
		top = min(top, max(1, available-4))
	}
	m.list.SetSize(max(1, width-4), top)
	m.logs.SetWidth(max(1, width-4))
	m.logs.SetHeight(max(1, available-top))
	if m.inspectMode {
		left := m.turnListWidth()
		m.turnList.SetSize(left, max(1, available-top))
		m.logs.SetWidth(max(1, width-left-9))
		// The extra pair of borders separates fixed turn statistics from the log.
		body := max(2, available-top-2)
		m.stats.SetWidth(m.logs.Width())
		m.stats.SetHeight(max(1, min(6, body/3)))
		m.logs.SetHeight(max(1, body-m.stats.Height()))
	}
}

type clipboardResult struct {
	id  string
	err error
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if e, ok := msg.(Event); ok && e.Kind == MemoryChanged {
		if e.Memory != nil {
			m.memory.snapshot = *e.Memory
			m.memory.ready = true
			m.memory.refresh(m.width, m.height)
			m.types.refresh(m.memory.snapshot, m.width, m.height)
		}
		return m, waitEvent(m.events)
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "m" {
		m.typesMode = false
		m.memoryMode = !m.memoryMode
		m.memory.refresh(m.width, m.height)
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "t" {
		m.typesMode = !m.typesMode
		m.memoryMode = false
		m.types.refresh(m.memory.snapshot, m.width, m.height)
		return m, nil
	}
	if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft && click.Y == 0 {
		if mode, hit := m.modeAt(click.X); hit {
			m.memoryMode, m.typesMode = mode == 1, mode == 2
			m.memory.refresh(m.width, m.height)
			m.types.refresh(m.memory.snapshot, m.width, m.height)
			return m, nil
		}
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.memory.refresh(size.Width, size.Height)
		m.types.refresh(m.memory.snapshot, size.Width, size.Height)
	}
	if m.typesMode {
		switch v := msg.(type) {
		case tea.KeyPressMsg:
			if v.String() != "q" && v.String() != "Q" && v.String() != "shift+q" && v.String() != "ctrl+c" {
				if v.String() == "enter" {
					m.showTypeRecords()
				} else {
					m.types.update(msg, m.memory.snapshot, m.width, m.height)
				}
				return m, nil
			}
		case tea.MouseClickMsg, tea.MouseWheelMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
			m.types.update(msg, m.memory.snapshot, m.width, m.height)
			return m, nil
		}
	}
	if m.memoryMode {
		switch v := msg.(type) {
		case tea.KeyPressMsg:
			if v.String() != "q" && v.String() != "Q" && v.String() != "shift+q" && v.String() != "ctrl+c" {
				m.memory.update(msg, m.width, m.height)
				return m, nil
			}
		case tea.MouseClickMsg, tea.MouseWheelMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
			m.memory.update(msg, m.width, m.height)
			return m, nil
		}
	}
	switch msg := msg.(type) {
	case inspectorTick:
		if time.Now().After(m.clipboardUntil) {
			m.clipboardNotice = ""
		}
		if m.inspectMode {
			m.refreshLogs(false)
		}
		return m, tickInspector()
	case clipboardResult:
		m.clipboardUntil = time.Now().Add(3 * time.Second)
		m.clipboardNotice = "Copied " + msg.id
		if msg.err != nil {
			m.clipboardNotice = "Clipboard unavailable"
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.dragging = false
		follow := m.logs.AtBottom()
		m.resize(msg.Width, msg.Height)
		m.refreshLogs(follow)
		return m, nil
	case tea.MouseClickMsg:
		if m.width < 36 || m.height < 12 || msg.Button != tea.MouseLeft || msg.X < 1 || msg.X >= m.width-1 {
			return m, nil
		}
		divider := m.list.Height() + 2
		if m.inspectMode && msg.Y >= divider+2 && msg.Y < m.height-2 {
			m.turnsFocused = msg.X < 2+m.turnListWidth()
			if m.turnsFocused {
				row := msg.Y - (divider + 2)
				index := m.turnList.Paginator.Page*m.turnList.Paginator.PerPage + row
				if row >= 0 && index < len(m.turnList.Items()) {
					m.turnList.Select(index)
					m.selectTurnRow()
				}
			}
			m.logsFocused = true
			return m, nil
		}
		// The lower border title starts after the corner, dash, and padding.
		// Give the visible name priority over separator dragging.
		nameX, nameWidth := 3, m.width-6
		if m.inspectMode {
			nameX = m.turnListWidth() + 16
			nameWidth = max(0, m.logs.Width()-10)
		}
		if s := m.selected(); s != nil && msg.Y == divider+1 && msg.X >= nameX && msg.X < nameX+min(lipgloss.Width(s.humanID), nameWidth) {
			id, copyText := s.humanID, m.copyText
			return m, func() tea.Msg { return clipboardResult{id: id, err: copyText(id)} }
		}
		if msg.Y == divider || msg.Y == divider+1 {
			m.dragging = true
			m.dragY, m.dragTop = msg.Y, m.list.Height()
			return m, nil
		}
		if msg.Y >= 2 && msg.Y < divider {
			m.logsFocused = false
			row := msg.Y - 2
			index := m.list.Paginator.Page*m.list.Paginator.PerPage + row
			if row >= 0 && index < len(m.sessions) {
				m.list.Select(index)
				m.refreshLogs(true)
			}
		} else if msg.Y > divider+1 && msg.Y < m.height-2 {
			m.logsFocused = true
		}
		return m, nil
	case tea.MouseMotionMsg:
		if m.dragging {
			m.dragDivider(msg.Y)
		}
		return m, nil
	case tea.MouseReleaseMsg:
		if m.dragging {
			m.dragDivider(msg.Y)
			m.dragging = false
		}
		return m, nil
	case tea.MouseWheelMsg:
		if m.width < 36 || m.height < 12 || msg.X < 1 || msg.X >= m.width-1 || m.dragging {
			return m, nil
		}
		divider := m.list.Height() + 2
		if msg.Y >= 2 && msg.Y < divider {
			previous := m.selected()
			if msg.Button == tea.MouseWheelUp {
				m.list.CursorUp()
			} else if msg.Button == tea.MouseWheelDown {
				m.list.CursorDown()
			}
			if previous != m.selected() {
				m.refreshLogs(true)
			}
		} else if msg.Y > divider+1 && msg.Y < m.height-2 {
			if m.inspectMode && msg.X < 2+m.turnListWidth() {
				if msg.Button == tea.MouseWheelUp {
					m.turnList.CursorUp()
				} else if msg.Button == tea.MouseWheelDown {
					m.turnList.CursorDown()
				}
				m.selectTurnRow()
				return m, nil
			}
			var cmd tea.Cmd
			if m.inspectMode && msg.Y < divider+2+m.stats.Height() {
				m.stats, cmd = m.stats.Update(msg)
				return m, cmd
			}
			m.logs, cmd = m.logs.Update(msg)
			return m, cmd
		}
		return m, nil
	case Event:
		m.apply(msg)
		return m, waitEvent(m.events)
	case finished:
		m.workerStopped = true
		if m.quitting {
			return m, tea.Quit
		}
		m.status = "Stopped · q to exit"
		if msg.err != nil {
			m.status = "Connection stopped · q to exit and restart"
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "i":
			m.switchMode(true)
			return m, nil
		case "l":
			m.switchMode(false)
			return m, nil
		case "[":
			if m.inspectMode {
				m.moveTurn(-1)
				return m, nil
			}
		case "]":
			if m.inspectMode {
				m.moveTurn(1)
				return m, nil
			}
		case "enter":
			if m.inspectMode {
				if s := m.selected(); s != nil {
					s.expanded = !s.expanded
					m.refreshLogs(false)
				}
				return m, nil
			}
		case "c", "y":
			if m.inspectMode {
				if t := m.inspectedTurn(); t != nil {
					content, label := t.Response, "response"
					if t.Emoji != "" {
						content = ":" + t.Emoji + ":"
					}
					if msg.String() == "y" {
						var parts []string
						for _, input := range t.Inputs {
							parts = append(parts, input.Text)
						}
						content = strings.Join(parts, "\n\n")
						label = "input"
					}
					copyText := m.copyText
					return m, func() tea.Msg { return clipboardResult{id: label, err: copyText(content)} }
				}
				return m, nil
			}
		case "q", "ctrl+c":
			if m.workerStopped || m.requestQuit == nil {
				return m, tea.Quit
			}
			if !m.quitting {
				m.quitting = true
				m.requestQuit()
			}
			return m, nil
		case "Q", "shift+q":
			return m, tea.Quit
		case "tab", "shift+tab":
			if m.inspectMode {
				if msg.String() == "shift+tab" {
					if !m.logsFocused {
						m.logsFocused = true
						m.turnsFocused = false
					} else if !m.turnsFocused {
						m.turnsFocused = true
					} else {
						m.logsFocused = false
					}
				} else {
					if !m.logsFocused {
						m.logsFocused = true
						m.turnsFocused = true
					} else if m.turnsFocused {
						m.turnsFocused = false
					} else {
						m.logsFocused = false
					}
				}
			} else {
				m.logsFocused = !m.logsFocused
			}
			return m, nil
		case "end", "G":
			if m.inspectMode {
				m.moveTurn(1 << 20)
				return m, nil
			}
			m.logs.GotoBottom()
			return m, nil
		case "home", "g":
			if m.logsFocused {
				m.logs.GotoTop()
				return m, nil
			}
		}
		if m.logsFocused && m.inspectMode && m.turnsFocused && msg.String() != "pgup" && msg.String() != "pgdown" {
			var cmd tea.Cmd
			m.turnList, cmd = m.turnList.Update(msg)
			m.selectTurnRow()
			return m, cmd
		}
		if m.logsFocused || msg.String() == "pgup" || msg.String() == "pgdown" {
			var cmd tea.Cmd
			m.logs, cmd = m.logs.Update(msg)
			return m, cmd
		}
		previous := m.selected()
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		if previous != m.selected() {
			m.refreshLogs(true)
		}
		return m, cmd
	}
	return m, nil
}

// dragDivider uses the original grab position so either border row can be
// grabbed without jumping. Out-of-bounds motion clamps to the pane minimums.
func (m *model) dragDivider(y int) {
	available := max(4, m.height-6)
	top := min(available-2, max(2, m.dragTop+y-m.dragY))
	follow := m.logs.AtBottom()
	m.split = float64(top) / float64(available)
	m.splitCustomized = true
	m.resize(m.width, m.height)
	m.refreshLogs(follow)
}

func (m *model) apply(event Event) {
	if event.At.IsZero() {
		event.At = time.Now()
	}
	if event.Kind == Connection {
		m.status = event.Text
		return
	}
	selected := m.selected()
	follow := !m.inspectMode && m.logs.AtBottom()
	var target *session
	added := false
	for _, s := range m.sessions {
		if s.id == event.Session {
			target = s
			break
		}
	}
	if event.Kind == Started && target == nil {
		added = true
		target = &session{id: event.Session, started: event.At, state: "running", turns: event.Turns}
		target.humanID = event.HumanID
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
		items := make([]list.Item, len(m.sessions))
		index := 0
		for i, s := range m.sessions {
			items[i] = s
			if s == selected {
				index = i
			}
		}
		m.list.SetItems(items)
		m.list.Select(index)
		if target == selected {
			m.refreshLogs(true)
		}
		return
	}
	switch event.Kind {
	case Inspected:
		if event.Inspection != nil {
			t := event.Inspection.Clone()
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
		}
	case TitleChanged:
		target.title = event.Title
	case Completed:
		target.state = "idle"
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
		target.usageKnown = true
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
	items := make([]list.Item, len(m.sessions))
	index := 0
	for i, s := range m.sessions {
		items[i] = s
		if s == selected {
			index = i
		}
	}
	m.list.SetItems(items) // filtering is disabled, so this requires no command
	// Bring a newly added conversation into view, including when the old
	// selection was on a later page. Ordinary updates keep manual selection.
	if added {
		index = 0
	}
	m.list.Select(index)
	if m.selected() != selected {
		m.refreshLogs(true)
	} else if target == selected {
		m.refreshLogs(follow)
	}
}

func (m *model) refreshLogs(follow bool) {
	text := ""
	if m.selected() == nil {
		m.stats.SetContent("")
	}
	if s := m.selected(); s != nil {
		if m.inspectMode {
			text = m.inspectorContent(s)
		} else {
			text = strings.Join(s.logs, "\n")
		}
	}
	m.logs.SetContent(text)
	if follow {
		if m.inspectMode {
			m.logs.GotoTop()
			m.stats.GotoTop()
		} else {
			m.logs.GotoBottom()
		}
	}
}

func (m *model) View() tea.View {
	if m.typesMode {
		return m.typesView()
	}
	if m.memoryMode {
		return m.memoryView()
	}
	var content string
	if m.width < 36 || m.height < 12 {
		message := "Gobotto · enlarge terminal to 36×12\nq quit"
		if m.quitting {
			message = "Finishing active turn…\nShift-Q quit now"
		}
		content = lipgloss.NewStyle().MaxWidth(max(1, m.width)).MaxHeight(max(1, m.height)).Render(message)
	} else {
		header := m.navigation()
		topTitle := fmt.Sprintf("Sessions · %d retained", len(m.sessions))
		if s := m.selected(); s != nil && s.pending > 0 {
			topTitle += fmt.Sprintf(" · %d messages pending", s.pending)
		}
		bottomTitle := "Session logs"
		if s := m.selected(); s != nil {
			bottomTitle = fmt.Sprintf("%s · %s", s.humanID, s.state)
			if s.usageKnown {
				bottomTitle += fmt.Sprintf(" · %d tokens", s.input+s.output)
			} else {
				bottomTitle += " · tokens n/a"
			}
			if s.costKnown {
				bottomTitle += fmt.Sprintf(" · $%.6f", s.cost)
				if s.costMissing {
					bottomTitle += " (partial)"
				}
			} else {
				bottomTitle += " · cost n/a"
			}
			bottomTitle += " · " + reasoningLabel(s.reasoning, s.reasoningKnown, s.reasoningMissing)
		}
		if m.inspectMode {
			if s := m.selected(); s != nil {
				bottomTitle = s.humanID
			}
		} else if m.logs.AtBottom() {
			bottomTitle += " · following"
		} else {
			bottomTitle += " · scrollback"
		}
		topBody := m.list.View()
		if len(m.sessions) == 0 {
			topBody = lipgloss.NewStyle().Height(m.list.Height()).Render("")
		}
		help := " i inspector · l log · tab pane · pgup/pgdn scroll · q quit"
		if m.inspectMode {
			help = " [ ] turn · enter expand · c copy reply · y copy input · l log · q quit"
		}
		if m.clipboardNotice != "" {
			help = " " + m.clipboardNotice + " · click name to copy · q quit"
		}
		if m.quitting {
			help = " Waiting for active turn · Shift-Q quit immediately"
		}
		footer := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).MaxWidth(m.width).MaxHeight(1).Render(help)
		lowerBody := renderPane(bottomTitle, m.logs.View(), m.width, m.logsFocused)
		if m.inspectMode {
			lowerBody = m.inspectorPanes(bottomTitle)
		}
		content = lipgloss.JoinVertical(lipgloss.Left, header, renderPane(topTitle, topBody, m.width, !m.logsFocused), lowerBody, footer)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
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
