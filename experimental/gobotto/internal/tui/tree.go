package tui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// nodeKey keeps selection stable when live events append rows or history loads.
type nodeKey struct {
	session uint64
	turn    int
	step    int // zero is a turn; positive steps and negative synthetic events
}

type treeRow struct {
	key   nodeKey
	label string
	busy  bool
}

func (m *model) currentRow() *treeRow {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m *model) rebuildTree() {
	var selected nodeKey
	if row := m.currentRow(); row != nil {
		selected = row.key
	}
	m.rows = nil
	for _, s := range m.sessions {
		for i := len(s.records) - 1; i >= 0; i-- {
			t := &s.records[i]
			key := nodeKey{session: s.id, turn: t.Number}
			if s.id != m.activeSession {
				continue
			}
			m.rows = append(m.rows, treeRow{key: key, label: fmt.Sprintf("Turn %d", t.Number), busy: t.State == "running" || t.State == "waiting"})
			childrenStart := len(m.rows)
			add := func(step int, label string) {
				m.rows = append(m.rows, treeRow{key: nodeKey{session: s.id, turn: t.Number, step: step}, label: label})
			}
			for j := range t.Inputs {
				add(-j-1, "Input")
			}
			for j, step := range t.Steps {
				label := step.Name
				if step.Kind == "title" {
					label = "Title"
				} else if label == "" {
					label = "Event"
				}
				add(j+1, label)
			}
			if t.Response != "" {
				add(-1000001, "Output")
			}
			if t.Emoji != "" {
				add(-1000002, "Reaction")
			}
			if t.Failure != "" {
				add(-1000003, "Failure")
			}
			slices.Reverse(m.rows[childrenStart:])
		}
	}
	index := -1
	for i, row := range m.rows {
		if row.key == selected {
			index = i
			break
		}
	}
	if index < 0 && selected.step != 0 {
		selected.step = 0
		for i, row := range m.rows {
			if row.key == selected {
				index = i
				break
			}
		}
	}
	if index >= 0 {
		m.cursor = index
	}
	if !m.navigated {
		m.cursor, m.offset = 0, 0
	}
	m.clampNavigation()
	if row := m.currentRow(); row == nil || row.key != selected {
		m.detailOffset = 0
	}
}

func (m *model) sessionIndex() int {
	for i, s := range m.sessions {
		if s.id == m.activeSession {
			return i
		}
	}
	return -1
}

// syncSession pins startup restores to the newest timestamp, then respects the
// user's selection until that session is removed by retention or eviction.
func (m *model) syncSession() {
	if len(m.sessions) == 0 {
		m.activeSession = 0
	} else if !m.navigated || m.selected() == nil {
		m.activate(m.sessions[0])
	}
	m.clampNavigation()
}

func (m *model) activate(s *session) {
	if s.id == m.activeSession {
		return
	}
	m.activeSession = s.id
	m.rows = nil
	m.cursor, m.offset = 0, 0
	m.detailOffset = 0
	m.viewCounter++
	s.lastViewed = m.viewCounter
	m.trimDetails()
}

func (m *model) moveSessions(delta int) tea.Cmd {
	m.navigated = true
	if len(m.sessions) == 0 {
		return nil
	}
	i := max(0, min(len(m.sessions)-1, m.sessionIndex()+delta))
	m.activate(m.sessions[i])
	m.rebuildTree()
	return m.loadSelected()
}

func (m *model) loadSelected() tea.Cmd {
	if s := m.selected(); s != nil && !s.detailsLoaded && !s.detailsLoading && s.detailsError == "" {
		return m.loadPage(s, 0)
	}
	return nil
}

func (m *model) treeHeight() int { return max(1, m.height-3) }

// paneWidths is shared by rendering and mouse hit testing. The divider has
// exactly one terminal cell; minimum widths keep both panes usable after drag.
func (m *model) paneWidths() (int, int) {
	available := max(2, m.width-1)
	minimum := min(16, available/2)
	left := max(minimum, min(available-minimum, int(float64(available)*m.split)))
	return left, available - left
}

func visibleOffset(cursor, offset, count, height int) int {
	offset = max(0, min(offset, max(0, count-height)))
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+height {
		return cursor - height + 1
	}
	return offset
}

func (m *model) clampNavigation() {
	m.cursor = max(0, min(m.cursor, len(m.rows)-1))
	m.offset = visibleOffset(m.cursor, m.offset, len(m.rows), m.eventHeight())
	m.sessionOffset = visibleOffset(max(0, m.sessionIndex()), m.sessionOffset, len(m.sessions), m.treeHeight())
}

func (m *model) resize(width, height int) {
	m.width, m.height = width, height
	m.clampNavigation()
}

func (m *model) moveTree(delta int) {
	m.detailOffset = 0
	m.cursor += delta
	m.clampNavigation()
}

func (m *model) dragDivider(x int) {
	available := max(2, m.width-1)
	minimum := min(16, available/2)
	x = max(minimum, min(available-minimum, x))
	// The half cell prevents floating-point rounding from shifting the divider.
	m.split = (float64(x) + 0.5) / float64(available)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case activityTickMsg:
		m.activityFrame = (m.activityFrame + 1) % len(activityFrames)
		return m, activityTick()
	case detailsResult:
		return m, m.finishDetails(msg)
	case Event:
		m.apply(msg)
		return m, tea.Batch(waitEvent(m.events), m.loadSelected())
	case finished:
		m.workerStopped = true
		if m.quitting || msg.err != nil {
			return m, tea.Quit
		}
		m.status = "Stopped · q to exit"
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft || msg.Y < 0 || msg.Y >= m.height-1 {
			break
		}
		left, _ := m.paneWidths()
		if msg.X == left {
			m.dragging = true
			break
		}
		if msg.X < 0 || msg.X >= m.width {
			break
		}
		if msg.X > left && msg.Y == m.eventHeight()+1 {
			m.draggingDetail = true
			break
		}
		m.eventsFocused = msg.X > left
		m.navigated = true
		if msg.Y < 1 || msg.Y > m.treeHeight() {
			break
		}
		if !m.eventsFocused {
			i := m.sessionOffset + msg.Y - 1
			if i < len(m.sessions) {
				return m, m.moveSessions(i - m.sessionIndex())
			}
		} else {
			if msg.Y > m.eventHeight() {
				break
			}
			i := m.offset + msg.Y - 1
			if i < len(m.rows) {
				m.moveTree(i - m.cursor)
			}
		}
	case tea.MouseMotionMsg:
		if m.dragging {
			m.dragDivider(msg.X)
		} else if m.draggingDetail {
			m.dragDetailDivider(msg.Y)
		}
	case tea.MouseReleaseMsg:
		if m.dragging {
			m.dragDivider(msg.X)
			m.dragging = false
		}
		if m.draggingDetail {
			m.dragDetailDivider(msg.Y)
			m.draggingDetail = false
		}
	case tea.MouseWheelMsg:
		if msg.Y < 1 || msg.Y > m.treeHeight() {
			break
		}
		left, _ := m.paneWidths()
		if msg.X == left {
			break
		}
		delta := 0
		if msg.Button == tea.MouseWheelUp {
			delta = -1
		} else if msg.Button == tea.MouseWheelDown {
			delta = 1
		}
		if delta == 0 {
			break
		}
		m.navigated = true
		m.eventsFocused = msg.X > left
		if m.eventsFocused && msg.Y > m.eventHeight()+1 {
			m.scrollDetails(delta * 3)
			break
		}
		if !m.eventsFocused {
			return m, m.moveSessions(delta)
		}
		m.moveTree(delta)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.workerStopped || m.requestQuit == nil {
				return m, tea.Quit
			}
			if !m.quitting {
				m.quitting = true
				m.requestQuit()
			}
		case "Q", "shift+q":
			return m, tea.Quit
		case "tab", "shift+tab":
			m.navigated = true
			m.eventsFocused = !m.eventsFocused
		case "left", "esc":
			m.navigated = true
			m.eventsFocused = false
		case "right":
			m.navigated = true
			m.eventsFocused = true
		case "enter", "space", " ":
			m.navigated = true
			if !m.eventsFocused {
				m.eventsFocused = true
				return m, m.loadSelected()
			}
		case "p", "r":
			if s := m.selected(); s != nil {
				if msg.String() == "r" {
					return m, m.loadPage(s, 0)
				}
				if s.before > 0 {
					return m, m.loadPage(s, s.before)
				}
			}
		case "[":
			m.scrollDetails(-3)
		case "]":
			m.scrollDetails(3)
		default:
			delta := 0
			switch msg.String() {
			case "up", "k":
				delta = -1
			case "down", "j":
				delta = 1
			case "pgup":
				delta = -m.treeHeight()
				if m.eventsFocused {
					delta = -m.eventHeight()
				}
			case "pgdown":
				delta = m.treeHeight()
				if m.eventsFocused {
					delta = m.eventHeight()
				}
			case "home", "g":
				delta = -max(len(m.rows), len(m.sessions))
			case "end", "G":
				delta = max(len(m.rows), len(m.sessions))
			}
			if delta != 0 {
				m.navigated = true
				if !m.eventsFocused {
					return m, m.moveSessions(delta)
				}
				m.moveTree(delta)
			}
		}
	}
	return m, nil
}

// displayText prevents terminal controls and bidi overrides from affecting UI.
func displayText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, text)
}

func rowText(label string, width int, selected, focused bool) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#B8C1CC")).Width(width).MaxWidth(width).MaxHeight(1)
	if selected {
		style = style.Background(lipgloss.Color("#293542"))
		if focused {
			style = style.Background(lipgloss.Color("#244F52")).Foreground(lipgloss.Color("#E8FFFF"))
		}
	}
	return style.Render(ansi.Truncate(displayText(label), width, "…"))
}

func (m *model) View() tea.View {
	content := "Gobotto · enlarge terminal to 40×8\nq quit"
	if m.width >= 40 && m.height >= 8 {
		left, right := m.paneWidths()
		sessionLines, eventLines := []string{}, []string{}
		for i := m.sessionOffset; i < len(m.sessions) && len(sessionLines) < m.treeHeight(); i++ {
			s := m.sessions[i]
			label := s.title
			if label == "" {
				label = s.humanID
			}
			label = m.activityPrefix(s.busy) + ansi.Truncate(displayText(label), contentWidth(left-1), "…")
			sessionLines = append(sessionLines, rowText(label, left-1, s.id == m.activeSession, !m.eventsFocused))
		}
		if len(m.sessions) == 0 {
			sessionLines = append(sessionLines, rowText(contentIndent+"No sessions", left-1, false, false))
		}
		for i := m.offset; i < len(m.rows) && len(eventLines) < m.eventHeight(); i++ {
			row := m.rows[i]
			if row.key.step == 0 {
				eventLines = append(eventLines, separatorBar(m.activityPrefix(row.busy)+row.label, right-1, i == m.cursor && m.eventsFocused))
			} else {
				label := insetLine(displayText(row.label), right-1)
				eventLines = append(eventLines, rowText(label, right-1, i == m.cursor, m.eventsFocused))
			}
		}
		if len(eventLines) == 0 {
			text := "No events"
			if s := m.selected(); s != nil {
				if s.detailsError != "" {
					text = s.detailsError
				} else if !s.detailsLoaded {
					text = "Loading history…"
				}
			}
			eventLines = append(eventLines, rowText(contentIndent+text, right-1, false, false))
		}
		for len(sessionLines) < m.treeHeight() {
			sessionLines = append(sessionLines, strings.Repeat(" ", left-1))
		}
		for len(eventLines) < m.eventHeight() {
			eventLines = append(eventLines, strings.Repeat(" ", right-1))
		}
		eventTitle := "Events"
		if s := m.selected(); s != nil {
			if s.detailsLoading {
				eventTitle += " · loading"
			} else if s.detailsError != "" {
				eventTitle += " · r retry"
			} else if s.before > 0 {
				eventTitle += " · p older"
			}
		}
		border := lipgloss.NewStyle().Foreground(lipgloss.Color("#3C4654")).Render
		activeDivider := lipgloss.NewStyle().Foreground(lipgloss.Color("#5EEAD4")).Render
		vertical, horizontal := border, border
		if m.dragging {
			vertical = activeDivider
		}
		if m.draggingDetail {
			horizontal = activeDivider
		}
		lines := []string{border("╭") + panelTitle("Sessions", left-1, !m.eventsFocused, border) + vertical("┬") + panelTitle(eventTitle, right-1, m.eventsFocused, border) + border("╮")}
		details := m.detailLines(right - 1)
		detailHeight := m.treeHeight() - m.eventHeight() - 1
		detailOffset := min(m.detailOffset, max(0, len(details)-detailHeight))
		for i := range sessionLines {
			join, end := vertical("│"), border("│")
			var rightLine string
			switch {
			case i < m.eventHeight():
				rightLine = eventLines[i]
			case i == m.eventHeight():
				join, end = vertical("├"), horizontal("┤")
				if m.draggingDetail {
					join = horizontal("├")
				}
				rightLine = panelTitle("Details", right-1, false, horizontal)
			default:
				label := ""
				if n := detailOffset + i - m.eventHeight() - 1; n < len(details) {
					label = details[n]
				}
				rightLine = fitLine(label, right-1)
			}
			lines = append(lines, border("│")+sessionLines[i]+join+rightLine+end)
		}
		lines = append(lines, border("╰"+strings.Repeat("─", left-1))+vertical("┴")+border(strings.Repeat("─", right-1)+"╯"))
		content = strings.Join(lines, "\n")
		help := "Tab panes · ↑↓ select · drag divider · q quit"
		if m.quitting {
			help = "Waiting for active work · Shift-Q quit immediately"
		} else if m.status != "" {
			help = displayText(m.status) + " · " + help
		}
		content += "\n" + ansi.Truncate(help, m.width, "…")
	}
	v := tea.NewView(lipgloss.NewStyle().MaxWidth(max(1, m.width)).MaxHeight(max(1, m.height)).Render(content))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
