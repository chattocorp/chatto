package tui

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

// displayText prevents terminal controls and bidi overrides in private content
// from affecting the surrounding interface. Clipboard actions use original text.
func displayText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, text)
}

func excerpt(text string, expanded bool) string {
	text = displayText(text)
	if expanded {
		return text
	}
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 140 {
		return string(runes[:140]) + "…"
	}
	return text
}

func (m *model) inspectedTurn() *inspection.Turn {
	s := m.selected()
	if s == nil {
		return nil
	}
	for i := range s.records {
		if s.records[i].Number == s.inspected {
			return &s.records[i]
		}
	}
	return nil
}

func (m *model) switchMode(inspect bool) {
	if m.inspectMode == inspect {
		return
	}
	if s := m.selected(); s != nil {
		if m.inspectMode {
			s.inspectOffset = m.logs.YOffset()
		} else {
			s.logOffset = m.logs.YOffset()
		}
	}
	m.inspectMode = inspect
	m.logsFocused = true
	m.turnsFocused = inspect
	m.resize(m.width, m.height)
	m.refreshLogs(false)
	if s := m.selected(); s != nil {
		if inspect {
			m.logs.SetYOffset(s.inspectOffset)
		} else {
			m.logs.SetYOffset(s.logOffset)
		}
	}
}

func (m *model) moveTurn(delta int) {
	s := m.selected()
	if s == nil || len(s.records) == 0 {
		return
	}
	index := len(s.records) - 1
	for i, t := range s.records {
		if t.Number == s.inspected {
			index = i
		}
	}
	index = max(0, min(len(s.records)-1, index+delta))
	s.inspected = s.records[index].Number
	m.refreshLogs(false)
	m.logs.GotoTop()
}

func (m *model) inspectorContent(s *session) string {
	items := make([]list.Item, 0, len(s.records))
	selected := 0
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].Number == s.inspected {
			selected = len(items)
		}
		record := s.records[i]
		label := record.State
		if record.State == "delivered" {
			label = "Replied"
			if record.Emoji != "" {
				label = "Reacted"
			}
		}
		items = append(items, turnItem{record.Number, record.Started, label})
	}
	m.turnList.SetItems(items)
	m.turnList.Select(selected)
	if len(s.records) == 0 {
		return "No recorded turns yet. Older sessions gain inspection records on their next turn."
	}
	var parts []string
	wrap := lipgloss.NewStyle().Width(m.logs.Width()).MaxWidth(m.logs.Width())
	for i := len(s.records) - 1; i >= 0; i-- {
		t := s.records[i]
		if t.Number != s.inspected {
			continue
		}
		duration := t.Finished.Sub(t.Started)
		if t.Finished.IsZero() {
			duration = time.Since(t.Started)
		}
		if t.State == "interrupted" && t.Finished.IsZero() {
			duration = 0
		}
		heading := fmt.Sprintf("Turn %d    %s    %s", t.Number, t.Started.Format("15:04:05"), duration.Round(time.Millisecond))
		style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#AEB8C5"))
		block := style.Width(m.logs.Width()).MaxWidth(m.logs.Width()).MaxHeight(1).Render(heading)
		if t.Number == s.inspected {
			var body strings.Builder
			inputLabel := "Incoming message"
			if len(t.Inputs) != 1 {
				inputLabel = fmt.Sprintf("%d incoming messages", len(t.Inputs))
			}
			fmt.Fprintf(&body, "\n%s\n", inspectorSection(inputLabel, m.logs.Width()))
			for _, input := range t.Inputs {
				body.WriteString(inspectorQuote(excerpt(input.Text, s.expanded), m.logs.Width(), "#8593A4") + "\n")
			}
			outcome := "Working"
			if t.State != "running" {
				outcome = "No response"
			}
			if t.Response != "" {
				outcome = "Reply"
			}
			if t.Emoji != "" {
				outcome = "Reaction"
			}
			fmt.Fprintf(&body, "\n%s\n", inspectorSection(outcome+"  /  "+t.State, m.logs.Width()))
			if t.Response != "" {
				body.WriteString(inspectorQuote(excerpt(t.Response, s.expanded), m.logs.Width(), "#5EEAD4") + "\n")
			}
			if t.Emoji != "" {
				fmt.Fprintf(&body, "  %s  :%s:\n", reactionGlyph(t.Emoji), displayText(t.Emoji))
			}
			if t.Failure != "" {
				body.WriteString(inspectorQuote(displayText(t.Failure), m.logs.Width(), "#F38BA8") + "\n")
			}
			fmt.Fprintf(&body, "\n%s\n", inspectorSection("Execution", m.logs.Width()))
			fmt.Fprintf(&body, "  ◷  Batch wait    %s\n", t.Wait.Round(time.Millisecond))
			if t.Model != "" {
				fmt.Fprintf(&body, "  %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("#8593A4")).Render(displayText(t.Model)))
			}
			var usage inspection.Usage
			cacheComplete := true
			calls, reads, titleCalls := 0, 0, 0
			for _, step := range t.Steps {
				elapsed := step.Duration
				if step.State == "running" {
					elapsed = time.Since(step.Started)
				}
				name := step.Name
				if step.Kind == "model" {
					name = "Generate response"
				}
				if step.Kind == "tool" && step.Name == "react_to_ping" {
					name = "Choose reaction"
				}
				mark, color := "✓", "#8593A4"
				if step.State == "running" {
					mark, color = "◌", "#5EEAD4"
				}
				if step.State == "failed" || step.State == "uncertain" || step.State == "interrupted" || step.State == "rejected" {
					mark, color = "!", "#F38BA8"
					name += " (" + step.State + ")"
				}
				elapsedText := elapsed.Round(time.Millisecond).String()
				if elapsed < time.Millisecond {
					elapsedText = "<1ms"
				}
				fmt.Fprintf(&body, "  %s  %s    %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(mark), displayText(name), elapsedText)
				if step.Provider != "" {
					fmt.Fprintf(&body, "     %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("#8593A4")).Render("via "+displayText(step.Provider)))
				}
				if s.expanded && step.RequestID != "" {
					fmt.Fprintf(&body, "      Request: %s\n", displayText(step.RequestID))
				}
				if s.expanded && step.Detail != "" {
					fmt.Fprintf(&body, "      %s\n", displayText(step.Detail))
				}
				if step.Kind == "model" {
					calls++
				}
				if step.Kind == "title" {
					titleCalls++
				}
				if step.Kind == "tool" && step.Name == "read_thread" {
					reads++
				}
				u := step.Usage
				if (step.Kind == "model" || step.Kind == "title") && !u.CacheKnown && u.Cached == 0 {
					cacheComplete = false
				}
				usage.Input += u.Input
				usage.Cached += u.Cached
				usage.Output += u.Output
				usage.Cost += u.Cost
				usage.Known = usage.Known || u.Known
				usage.CostKnown = usage.CostKnown || u.CostKnown
				usage.CostMissing = usage.CostMissing || u.CostMissing
			}
			fmt.Fprintf(&body, "\n%s\n", inspectorSection("Usage", m.logs.Width()))
			if usage.Known {
				percent := float64(0)
				if usage.Input > 0 {
					percent = 100 * float64(usage.Cached) / float64(usage.Input)
				}
				cache := fmt.Sprintf("%d cached (%.0f%%)", usage.Cached, percent)
				if !cacheComplete {
					cache = "cache n/a"
					if usage.Cached > 0 {
						cache = fmt.Sprintf("%d cached (partial)", usage.Cached)
					}
				}
				fmt.Fprintf(&body, "  %d input    %d output    %d %s\n  %s", usage.Input, usage.Output, calls+titleCalls, pluralCalls(calls+titleCalls), cache)
				if usage.CostKnown {
					fmt.Fprintf(&body, " · $%.6f", usage.Cost)
					if usage.CostMissing {
						body.WriteString(" (partial)")
					}
				} else {
					body.WriteString(" · cost n/a")
				}
				body.WriteString("\n")
			} else {
				body.WriteString("  Not reported\n")
			}
			if reads > 0 {
				fmt.Fprintf(&body, "  NOTE  %d thread refresh(es) requested despite supplied context\n", reads)
			}
			if i > 0 && usage.Known && usage.Input > 0 && cacheComplete {
				var previousInput, previousCached int64
				previousComplete := true
				for _, step := range s.records[i-1].Steps {
					if (step.Kind == "model" || step.Kind == "title") && !step.Usage.CacheKnown && step.Usage.Cached == 0 {
						previousComplete = false
					}
					previousInput += step.Usage.Input
					previousCached += step.Usage.Cached
				}
				if previousComplete && previousInput > 0 && float64(previousCached)/float64(previousInput)-float64(usage.Cached)/float64(usage.Input) >= 0.2 {
					body.WriteString("  NOTE  Cache hit rate fell by at least 20 percentage points\n")
				}
			}
			if t.Emoji != "" && calls > 1 {
				body.WriteString("  NOTE  Multiple model calls for a reaction\n")
			}
			content := body.String()
			block += "\n" + wrap.Render(content)
		}
		parts = append(parts, block)
	}
	return strings.Join(parts, "\n")
}

type turnItem struct {
	number  int
	started time.Time
	state   string
}

// inspectorSection separates conversation, execution, and accounting without
// applying syntax-like highlighting to user-controlled message text.
func inspectorSection(title string, width int) string {
	label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#DCE5EF")).Render(title)
	rule := lipgloss.NewStyle().Foreground(lipgloss.Color("#465363")).Render(strings.Repeat("─", max(0, width-lipgloss.Width(label)-3)))
	return label + "  " + rule
}

func inspectorQuote(text string, width int, color string) string {
	return lipgloss.NewStyle().BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(color)).PaddingLeft(1).Width(max(1, width)).Render(text)
}

func pluralCalls(n int) string {
	if n == 1 {
		return "call"
	}
	return "calls"
}

// Unknown and custom emoji retain their exact name beside a neutral glyph.
func reactionGlyph(name string) string {
	if glyph, ok := map[string]string{"thumbsup": "👍", "heart": "♥", "eyes": "👀", "brain": "🧠", "grinning": "😀", "white_check_mark": "✅", "tada": "🎉"}[name]; ok {
		return glyph
	}
	return "◇"
}

func (t turnItem) FilterValue() string { return "" }

type turnDelegate struct{}

func (turnDelegate) Height() int                         { return 1 }
func (turnDelegate) Spacing() int                        { return 0 }
func (turnDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (turnDelegate) Render(w io.Writer, l list.Model, index int, item list.Item) {
	t := item.(turnItem)
	text := fmt.Sprintf(" %02d  %s  %s", t.number, t.started.Format("15:04"), t.state)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#AEB8C5"))
	if index == l.Index() {
		style = style.Background(lipgloss.Color("#244F52")).Foreground(lipgloss.Color("#E8FFFF")).Bold(true)
	}
	fmt.Fprint(w, style.Width(l.Width()).MaxWidth(l.Width()).MaxHeight(1).Render(text))
}

func newTurnList() list.Model {
	l := list.New(nil, turnDelegate{}, 28, 10)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	return l
}

func (m *model) turnListWidth() int { return min(28, max(14, (m.width-4)/4)) }

func (m *model) selectTurnRow() {
	item, ok := m.turnList.SelectedItem().(turnItem)
	s := m.selected()
	if !ok || s == nil || s.inspected == item.number {
		return
	}
	s.inspected = item.number
	m.refreshLogs(false)
	m.logs.GotoTop()
}

func (m *model) inspectorPanes(title string) string {
	leftWidth := m.turnListWidth()
	left := m.turnList.View()
	if len(m.turnList.Items()) == 0 {
		left = lipgloss.NewStyle().Width(leftWidth).Height(m.logs.Height()).Render("")
	}
	left = renderPane(fmt.Sprintf("Turns · %d", len(m.turnList.Items())), left, leftWidth+4, m.logsFocused && m.turnsFocused)
	right := renderPane(title, m.logs.View(), m.width-leftWidth-5, m.logsFocused && !m.turnsFocused)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}
