package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

// nodeKey identifies a row independently of its position in the visible tree.
// Steps are append-only within a turn; session/turn retention removes parents.
type nodeKey struct {
	session uint64
	turn    int
	step    int // zero for a parent; positive for steps, negative for message/outcome rows
}

type treeRow struct {
	key                  nodeKey
	s                    *session
	t                    *inspection.Turn
	step                 *inspection.Step
	label, detail, state string // synthetic input, output, failure, or operational event
}

// hasChildren keeps disclosure controls consistent with actual retained data.
func (row treeRow) hasChildren() bool {
	if row.key.step != 0 {
		return false
	}
	if row.t != nil {
		return len(row.t.Steps) > 0 || len(row.t.Inputs) > 0 || row.t.Response != "" || row.t.Emoji != "" || row.t.Failure != ""
	}
	return len(row.s.records) > 0
}

func (m *model) currentRow() *treeRow {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m *model) rebuildTree() {
	key := nodeKey{}
	if row := m.currentRow(); row != nil {
		key = row.key
	}
	valid := make(map[nodeKey]bool)
	if m.activeSession != 0 {
		found := false
		for _, s := range m.sessions {
			found = found || s.id == m.activeSession
		}
		if !found {
			m.activeSession = 0
			m.followActive = false
		}
	}
	m.rows = nil
	for _, s := range m.sessions {
		sk := nodeKey{session: s.id}
		valid[sk] = true
		if m.activeSession == 0 {
			m.rows = append(m.rows, treeRow{key: sk, s: s})
		}
		if m.operationalLog && m.activeSession == s.id {
			for i, entry := range s.logs {
				m.rows = append(m.rows, treeRow{key: nodeKey{session: s.id, step: i + 1}, s: s, label: "Log", detail: entry})
			}
		}
		for i := len(s.records) - 1; i >= 0; i-- {
			t := &s.records[i]
			tk := nodeKey{session: s.id, turn: t.Number}
			valid[tk] = true
			if _, exists := m.folds[tk]; !exists {
				m.folds[tk] = true
			}
			if m.activeSession != s.id || m.operationalLog {
				continue
			}
			m.rows = append(m.rows, treeRow{key: tk, s: s, t: t})
			if m.folds[tk] {
				for j, input := range t.Inputs {
					m.rows = append(m.rows, treeRow{key: nodeKey{session: s.id, turn: t.Number, step: -j - 1}, s: s, t: t, label: "← Input", detail: input.Text})
				}
				for j := range t.Steps {
					m.rows = append(m.rows, treeRow{key: nodeKey{session: s.id, turn: t.Number, step: j + 1}, s: s, t: t, step: &t.Steps[j]})
				}
				if t.Response != "" {
					m.rows = append(m.rows, treeRow{key: nodeKey{session: s.id, turn: t.Number, step: -1000001}, s: s, t: t, label: "→ Output", detail: t.Response, state: t.State})
				}
				if t.Emoji != "" {
					m.rows = append(m.rows, treeRow{key: nodeKey{session: s.id, turn: t.Number, step: -1000002}, s: s, t: t, label: "→ Reaction", detail: ":" + t.Emoji + ":", state: t.State})
				}
				if t.Failure != "" {
					m.rows = append(m.rows, treeRow{key: nodeKey{session: s.id, turn: t.Number, step: -1000003}, s: s, t: t, label: "! Failure", detail: t.Failure, state: t.State})
				}
			}
		}
	}
	for k := range m.folds {
		if !valid[k] {
			delete(m.folds, k)
		}
	}
	// If retention or a fold removes a selected child, select its nearest parent.
	index := -1
	for index < 0 {
		for i, row := range m.rows {
			if row.key == key {
				index = i
				break
			}
		}
		if index >= 0 || key == (nodeKey{}) {
			break
		}
		if key.step != 0 {
			key.step = 0
		} else if key.turn != 0 {
			key.turn = 0
		} else {
			key = nodeKey{}
		}
	}
	if index < 0 {
		index = min(m.cursor, len(m.rows)-1)
	}
	m.cursor = max(0, index)
	if m.activeSession == 0 && !m.overviewNavigated {
		m.cursor, m.offset = 0, 0
	}
	m.resize(m.width, m.height)
}

func (m *model) syncTreeSelection(reset bool) {
	if row := m.currentRow(); row != nil {
		if row.t != nil {
			row.s.inspected = row.t.Number
		}
	}
	m.offset = max(0, min(m.offset, max(0, len(m.rows)-m.treeHeight())))
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.treeHeight() {
		m.offset = m.cursor - m.treeHeight() + 1
	}
}

func (m *model) headerHeight() int { return 7 }
func (m *model) tableHeaderHeight() int {
	if m.activeSession != 0 && !m.operationalLog {
		return 1
	}
	return 0
}
func (m *model) treeHeight() int { return max(1, m.height-10-m.tableHeaderHeight()) }
func (m *model) treeWidth() int  { return m.width }

func (m *model) resize(width, height int) {
	m.width, m.height = width, height
	m.syncTreeSelection(false)
}

func (m *model) moveTree(delta int) {
	m.overviewNavigated = true
	m.followActive = false
	m.cursor = max(0, min(len(m.rows)-1, m.cursor+delta))
	m.syncTreeSelection(true)
}

func (m *model) foldTree(open bool) {
	if m.activeSession == 0 {
		return
	}
	row := m.currentRow()
	if row == nil {
		return
	}
	m.followActive = false
	if row.hasChildren() && m.folds[row.key] != open {
		m.folds[row.key] = open
		m.rebuildTree()
	} else if open {
		if row.hasChildren() && m.cursor+1 < len(m.rows) && m.rows[m.cursor+1].key.session == row.key.session && (row.t == nil || m.rows[m.cursor+1].key.turn == row.key.turn) {
			m.moveTree(1)
		}
	} else {
		parent := nodeKey{session: row.key.session}
		if row.key.step != 0 {
			parent.turn = row.key.turn
		}
		for i, r := range m.rows {
			if r.key == parent {
				m.cursor = i
				break
			}
		}
		m.syncTreeSelection(true)
	}
}

// followSession is used only in explicit follow mode. Manual navigation stops it.
func (m *model) followSession(id uint64) {
	if m.activeSession != 0 && id != m.activeSession {
		return
	}
	if m.operationalLog {
		m.cursor = max(0, len(m.rows)-1)
		m.syncTreeSelection(false)
		return
	}
	for _, s := range m.sessions {
		if s.id != id {
			continue
		}
		key := nodeKey{session: id}
		if m.activeSession != 0 && len(s.records) > 0 {
			t := s.records[len(s.records)-1]
			key.turn = t.Number
			m.folds[key] = true
			key.step = len(t.Steps)
		}
		m.rebuildTree()
		for i, r := range m.rows {
			if r.key == key {
				m.cursor = i
				break
			}
		}
		m.syncTreeSelection(true)
		return
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case detailsResult:
		return m, m.finishDetails(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.busy, cmd = m.busy.Update(msg)
		return m, cmd
	case Event:
		m.apply(msg)
		if m.followActive && (msg.Kind == Started || msg.Kind == TurnStarted || msg.Kind == Inspected) {
			m.followSession(msg.Session)
		}
		return m, waitEvent(m.events)
	case inspectorTick:
		if time.Now().After(m.clipboardUntil) {
			m.clipboardNotice = ""
		}
		return m, tickInspector()
	case clipboardResult:
		m.clipboardUntil = time.Now().Add(3 * time.Second)
		m.clipboardNotice = "Copied " + msg.id
		if msg.err != nil {
			m.clipboardNotice = "Clipboard unavailable"
		}
	case finished:
		m.workerStopped = true
		if m.quitting || msg.err != nil {
			return m, tea.Quit
		}
		m.status = "Stopped · q to exit"
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
	case tea.MouseClickMsg:
		if m.detail != nil {
			return m, nil
		}
		if msg.Button != tea.MouseLeft || msg.Y < 0 || msg.Y >= m.height-1 {
			break
		}
		if msg.Y == 0 && m.activeSession != 0 {
			return m, m.copySessionID()
		}
		x := m.treeWidth()
		y := msg.Y - m.headerHeight() - m.tableHeaderHeight()
		if msg.X < 1 || msg.X >= x-1 || y < 1 || y > m.treeHeight() {
			break
		}
		index := m.offset + y - 1
		if index < len(m.rows) {
			m.moveTree(index - m.cursor)
			row := m.currentRow()
			depth := 0
			if row.key.step != 0 {
				depth = 1
			}
			if m.activeSession != 0 && row.hasChildren() && msg.X == 2+depth*2 {
				m.foldTree(!m.folds[row.key])
			}
		}
	case tea.MouseWheelMsg:
		if m.detail != nil {
			if msg.Button == tea.MouseWheelUp {
				return m, m.detailKey("up")
			}
			if msg.Button == tea.MouseWheelDown {
				return m, m.detailKey("down")
			}
			return m, nil
		}
		if msg.Y < m.headerHeight() {
			break
		}
		if msg.Button == tea.MouseWheelUp {
			m.moveTree(-1)
		} else if msg.Button == tea.MouseWheelDown {
			m.moveTree(1)
		}
	case tea.KeyPressMsg:
		if m.detail != nil && msg.String() != "q" && msg.String() != "ctrl+c" && msg.String() != "Q" && msg.String() != "shift+q" {
			return m, m.detailKey(msg.String())
		}
		switch msg.String() {
		case "i":
			m.openDetail()
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
		case "esc":
			m.leaveSession()
		case "enter":
			if m.activeSession == 0 {
				return m, m.enterSession()
			} else if row := m.currentRow(); row != nil {
				if row.key.step != 0 {
					m.openDetail()
				} else {
					m.foldTree(!m.folds[row.key])
				}
			}
		case "[":
			m.textOffset = max(0, m.textOffset-16)
		case "]":
			m.textOffset += 16
		case "l":
			if m.activeSession != 0 {
				m.operationalLog = !m.operationalLog
				m.rows = nil
				m.cursor = 0
				m.offset = 0
				m.rebuildTree()
			}
		case "f":
			m.overviewNavigated = true
			m.followActive = !m.followActive
			if m.followActive && len(m.sessions) > 0 {
				id := m.activeSession
				if id == 0 {
					id = m.sessions[0].id
				}
				m.followSession(id)
			}
		case "p", "r":
			if s := m.selected(); m.activeSession != 0 && s != nil {
				if msg.String() == "r" {
					return m, m.loadPage(s, 0)
				}
				if s.before > 0 {
					return m, m.loadPage(s, s.before)
				}
			}
		case "c", "y":
			if row := m.currentRow(); msg.String() == "c" && row != nil && row.key.step != 0 {
				content := row.detail
				if row.step != nil {
					content = row.step.Name + "\n" + row.step.Provider + "\n" + row.step.RequestID + "\n" + row.step.Detail
				}
				copyText := m.copyText
				return m, func() tea.Msg { return clipboardResult{id: "event", err: copyText(content)} }
			}
			if row := m.currentRow(); row != nil && row.t != nil {
				content, label := row.t.Response, "response"
				if row.t.Emoji != "" {
					content = ":" + row.t.Emoji + ":"
				}
				if msg.String() == "y" {
					var parts []string
					for _, input := range row.t.Inputs {
						parts = append(parts, input.Text)
					}
					content = strings.Join(parts, "\n\n")
					label = "input"
				}
				copyText := m.copyText
				return m, func() tea.Msg { return clipboardResult{id: label, err: copyText(content)} }
			}
			return m, m.copySessionID()
		default:
			switch msg.String() {
			case "up", "k":
				m.moveTree(-1)
			case "down", "j":
				m.moveTree(1)
			case "right":
				m.foldTree(true)
			case "left":
				m.foldTree(false)
			case " ", "space":
				if r := m.currentRow(); r != nil && r.step == nil {
					m.foldTree(!m.folds[r.key])
				}
			case "home", "g":
				m.moveTree(-len(m.rows))
			case "end", "G":
				m.moveTree(len(m.rows))
			case "pgup":
				m.moveTree(-m.treeHeight())
			case "pgdown":
				m.moveTree(m.treeHeight())
			}
		}
	}
	return m, nil
}

func (m *model) copySessionID() tea.Cmd {
	if s := m.selected(); s != nil {
		id, copyText := s.humanID, m.copyText
		return func() tea.Msg { return clipboardResult{id: id, err: copyText(id)} }
	}
	return nil
}

func (m *model) renderTreeRow(row treeRow, width int, selected bool) string {
	elapsed := ""
	description := ""
	label, state, tokens, cost, depth := row.s.title, row.s.state, "— tok", "$—", 0
	if label == "" {
		label = row.s.humanID
	}
	if row.s.usageKnown {
		tokens = compactTokens(row.s.input+row.s.output) + " tok"
	}
	if row.s.costKnown {
		cost = fmt.Sprintf("$%.4f", row.s.cost)
		if row.s.costMissing {
			cost += "+"
		}
	}
	if row.t != nil {
		depth = 0
		label = fmt.Sprintf("Turn %d", row.t.Number)
		description = row.t.Model
		state = row.t.State
		if state == "delivered" {
			state = "Replied"
			if row.t.Emoji != "" {
				state = "Reacted"
			}
		}
		tokens, cost = turnUsageSummary(*row.t)
		duration := row.t.Finished.Sub(row.t.Started)
		if row.t.Finished.IsZero() && row.t.State == "running" {
			duration = time.Since(row.t.Started)
		}
		if !row.t.Started.IsZero() && duration >= 0 {
			elapsed = duration.Round(time.Millisecond).String()
		}
	}
	if row.step != nil {
		depth = 1
		label = row.step.Name
		description = ""
		switch row.step.Kind {
		case "title":
			label, description = "Title", row.step.Name
		case "context":
			label, description = "Context", row.step.Name
		case "delivery":
			label, description = "Delivery", row.step.Name
		}
		state = row.step.State
		duration := row.step.Duration
		if state == "running" && !row.step.Started.IsZero() {
			duration = time.Since(row.step.Started)
		}
		elapsed = duration.Round(time.Millisecond).String()
		tokens, cost = turnUsageSummary(inspection.Turn{Steps: []inspection.Step{*row.step}})
		if row.step.Kind != "model" && row.step.Kind != "title" && !row.step.Usage.Known {
			tokens, cost = "", ""
		}
		if row.step.Provider != "" {
			description += " · " + row.step.Provider
		}
		if row.step.Kind == "model" || row.step.Kind == "title" {
			if row.step.Usage.Known {
				description += fmt.Sprintf(" · %s in / %s out", compactTokens(row.step.Usage.Input), compactTokens(row.step.Usage.Output))
			}
			if row.step.Usage.CacheKnown {
				description += fmt.Sprintf(" · %s cached", compactTokens(row.step.Usage.Cached))
			} else {
				description += " · cache n/a"
			}
			description += " · " + reasoningLabel(row.step.Usage.Reasoning, row.step.Usage.ReasoningKnown, false)
		}
		if row.step.Detail != "" && row.step.Detail != "Response received" {
			description += " · " + row.step.Detail
		}
		if row.step.RequestID != "" {
			description += " · request " + row.step.RequestID
		}
	}
	if row.label != "" {
		label, description = row.label, row.detail
		state = row.state
		tokens, cost, elapsed = "", "", ""
		depth = 1
	}
	mark := " "
	if row.t != nil && row.hasChildren() {
		mark = "›"
		if m.folds[row.key] {
			mark = "⌄"
		}
	}
	style := lipgloss.NewStyle().Background(lipgloss.Color("#252A33"))
	if selected {
		style = style.Background(lipgloss.Color("#244F52")).Foreground(lipgloss.Color("#E8FFFF"))
	}
	nameColor := "#89B4FA"
	if row.t != nil {
		nameColor = "#C4B5FD"
	}
	if row.key.step != 0 {
		nameColor = "#AEB8C5"
	}
	if row.label == "← Input" {
		nameColor = "#89B4FA"
	} else if row.label == "→ Output" || row.label == "→ Reaction" {
		nameColor = "#A6E3A1"
	} else if row.label == "! Failure" {
		nameColor = "#F38BA8"
	}
	stateColor := "#8593A4"
	switch state {
	case "running", "investigating":
		stateColor = "#5EEAD4"
	case "queued", "partial":
		stateColor = "#F2C879"
	case "Replied", "Reacted", "complete", "delivered":
		stateColor = "#A6E3A1"
	case "failed", "uncertain", "rejected", "interrupted", "cancelled":
		stateColor = "#F38BA8"
	}
	prefix := strings.Repeat("  ", depth) + mark + " "
	left := strings.Join(strings.Fields(displayText(label)), " ")
	if row.t == nil && row.label == "" {
		prefix = ""
	}
	left = prefix + left
	activity := ""
	if row.key.step == 0 && (state == "running" || (row.t == nil && (state == "queued" || state == "investigating"))) {
		activity = m.busy.View() + " "
	}
	// Render plain text in each cell with the same background, including all
	// padding and gutters. Nested ANSI resets otherwise leave gaps unpainted.
	cell := func(text, color string, size int, align lipgloss.Position) string {
		return style.Foreground(lipgloss.Color(color)).Width(size).MaxWidth(size).MaxHeight(1).Align(align).Render(ansi.Truncate(text, size, "…"))
	}
	if m.activeSession == 0 {
		nameWidth := width - 2
		if width >= 65 {
			nameWidth -= 39
		}
		line := cell(activity, stateColor, 2, lipgloss.Left) + cell(left, nameColor, nameWidth, lipgloss.Left)
		if width >= 65 {
			gap := cell("", "#94A3B8", 2, lipgloss.Left)
			line += gap + cell(fmt.Sprintf("%d turns", row.s.turns), "#C4B5FD", 10, lipgloss.Right)
			line += gap + cell(tokens, "#94A3B8", 12, lipgloss.Right)
			line += gap + cell(cost, "#94A3B8", 11, lipgloss.Right)
		}
		return line
	}
	description = strings.Join(strings.Fields(displayText(strings.TrimPrefix(description, " · "))), " ")
	if m.textOffset > 0 {
		text := []rune(description)
		description = string(text[min(m.textOffset, len(text)):])
	}
	if m.operationalLog {
		return cell(left+" · "+description, nameColor, width, lipgloss.Left)
	}
	if activity == "" {
		activity = "  "
	}
	return renderEventCells(width, [6]string{left, description, elapsed, activity + state, tokens, cost}, [6]string{nameColor, "#AEB8C5", "#94A3B8", stateColor, "#94A3B8", "#94A3B8"}, style)
}

func (m *model) View() tea.View {
	content := "Gobotto · enlarge terminal to 36×12\nq quit"
	if m.width >= 36 && m.height >= 12 {
		w := m.treeWidth()
		lines := make([]string, 0, m.treeHeight())
		if len(m.rows) == 0 {
			empty := "Waiting for sessions…"
			if m.activeSession != 0 {
				empty = "No recorded turns yet."
				if s := m.selected(); s != nil && s.loadDetails != nil && !s.detailsLoaded {
					empty = m.busy.View() + " Loading history…"
					if s.detailsError != "" {
						empty = s.detailsError
					}
				}
			}
			lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#8593A4")).MaxWidth(w-4).Render(empty))
		}
		for i := m.offset; i < len(m.rows) && len(lines) < m.treeHeight(); i++ {
			lines = append(lines, m.renderTreeRow(m.rows[i], w-4, i == m.cursor))
		}
		for len(lines) < m.treeHeight() {
			lines = append(lines, strings.Repeat(" ", max(1, w-4)))
		}
		title := fmt.Sprintf("Sessions · %d", len(m.sessions))
		if m.activeSession != 0 {
			if s := m.selected(); s != nil {
				title = s.title
				if title == "" {
					title = s.humanID
				}
				title = "Turns · " + displayText(title)
			}
			if m.operationalLog {
				title = "Operational log"
			}
		}
		if m.followActive {
			title += " · following"
		}
		if s := m.selected(); m.activeSession != 0 && s != nil {
			if s.detailsLoading {
				title += " · loading"
			} else if s.detailsError != "" {
				title += " · load failed · r retry"
			} else if s.before > 0 {
				title += " · p older turns"
			}
		}
		if m.tableHeaderHeight() != 0 {
			lines = append([]string{renderEventHeader(w - 4)}, lines...)
		}
		tree := renderPane(title, strings.Join(lines, "\n"), w, true)
		header := m.overviewHeader()
		if m.activeSession != 0 {
			header = m.sessionHeader()
		}
		content = header + "\n" + tree
		help := " ↑↓ select · enter session · f follow · q quit"
		if m.activeSession != 0 {
			help = " esc overview · enter/i inspect · ←→ fold · c copy · l logs · q quit"
		}
		if m.clipboardNotice != "" {
			help = " " + m.clipboardNotice
		}
		if m.quitting {
			help = " Waiting for active work · Shift-Q quit immediately"
		} else if m.clipboardNotice == "" {
			status := " GOBOTTO · " + displayText(m.status)
			if lipgloss.Width(status+" ·"+help) > m.width {
				help = " · enter details · q quit"
			}
			help = status + " ·" + help
		}
		content += "\n" + lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(1).Render(help)
	}
	if m.detail != nil && m.width >= 36 && m.height >= 12 {
		content = m.detailContent()
	}
	v := tea.NewView(lipgloss.NewStyle().MaxWidth(max(1, m.width)).MaxHeight(max(1, m.height)).Render(content))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
