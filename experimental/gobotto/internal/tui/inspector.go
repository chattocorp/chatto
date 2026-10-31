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

// reasoningLabel distinguishes absent telemetry from a reported zero.
func reasoningLabel(tokens int64, known, partial bool) string {
	if !known {
		return "reasoning n/a"
	}
	label := fmt.Sprintf("%d reasoning", tokens)
	if partial {
		label += " (partial)"
	}
	return label
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
	m.stats.GotoTop()
}

func (m *model) inspectorContent(s *session) string {
	m.stats.SetContent("")
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
		block := ""
		stats := turnStats{input: "—", output: "—", cache: "cache n/a", cost: "cost n/a", reasoning: "reasoning n/a"}
		if t.Number == s.inspected {
			var body strings.Builder
			for _, input := range t.Inputs {
				body.WriteString(logEvent("←", "Input", excerpt(input.Text, s.expanded), "#8593A4") + "\n")
			}
			writeOutput := func() {
				if t.Response != "" {
					body.WriteString(logEvent("→", "Output ("+t.State+")", excerpt(t.Response, s.expanded), "#5EEAD4") + "\n")
				}
				if t.Emoji != "" {
					body.WriteString(logEvent("→", "Reaction ("+t.State+")", reactionGlyph(t.Emoji)+" :"+t.Emoji+":", "#5EEAD4") + "\n")
				}
			}
			outputWritten := false
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
				if step.State == "failed" || step.State == "uncertain" || step.State == "interrupted" || step.State == "rejected" || step.State == "cancelled" {
					mark, color = "!", "#F38BA8"
					name += " (" + step.State + ")"
				}
				elapsedText := elapsed.Round(time.Millisecond).String()
				if elapsed < time.Millisecond {
					elapsedText = "<1ms"
				}
				detail := elapsedText
				if step.Provider != "" {
					detail += " · via " + step.Provider
				}
				if s.expanded && step.RequestID != "" {
					detail += " · Request: " + step.RequestID
				}
				if s.expanded && step.Detail != "" {
					detail += " · " + step.Detail
				}
				body.WriteString(logEvent(mark, name, detail, color) + "\n")
				if step.Kind == "delivery" && !outputWritten {
					writeOutput()
					outputWritten = true
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
				usage.Reasoning += u.Reasoning
				usage.ReasoningKnown = usage.ReasoningKnown || u.ReasoningKnown
				usage.Cost += u.Cost
				usage.Known = usage.Known || u.Known
				usage.CostKnown = usage.CostKnown || u.CostKnown
				usage.CostMissing = usage.CostMissing || u.CostMissing
			}
			if !outputWritten {
				writeOutput()
			}
			if t.Failure != "" {
				body.WriteString(logEvent("!", "Failure", t.Failure, "#F38BA8") + "\n")
			}
			if usage.Known {
				percent := float64(0)
				if usage.Input > 0 {
					percent = 100 * float64(usage.Cached) / float64(usage.Input)
				}
				cache := fmt.Sprintf("%d cached (%.0f%%)", usage.Cached, percent)
				stats.cacheValue = fmt.Sprintf("%.0f%%", percent)
				if !cacheComplete {
					stats.cacheValue = "—"
					cache = "cache n/a"
					if usage.Cached > 0 {
						cache = fmt.Sprintf("%d cached (partial)", usage.Cached)
					}
				}
				reasoningMissing := false
				for _, step := range t.Steps {
					if (step.Kind == "model" || step.Kind == "title") && !step.Usage.ReasoningKnown {
						reasoningMissing = true
					}
				}
				stats.input = groupedTokens(usage.Input)
				stats.output = groupedTokens(usage.Output)
				stats.cache = cache
				stats.reasoning = reasoningLabel(usage.Reasoning, usage.ReasoningKnown, reasoningMissing)
				if usage.CostKnown {
					stats.cost = fmt.Sprintf("$%.6f", usage.Cost)
					if usage.CostMissing {
						stats.costDetail = "Partial · some unreported"
					} else {
						stats.costDetail = "Reported by provider"
					}
				}
			}
			stats.calls = calls + titleCalls
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
			block = strings.TrimSuffix(content, "\n")
		}
		m.stats.SetContent(renderTurnStats(t, duration, stats, m.stats.Width()))
		parts = append(parts, block)
	}
	return strings.Join(parts, "\n")
}

type turnItem struct {
	number  int
	started time.Time
	state   string
}

// logEvent keeps each event on one physical row. Full content stays copyable;
// the viewport supports horizontal scrolling for long expanded entries.
func logEvent(mark, label, detail, color string) string {
	flat := func(text string) string { return strings.Join(strings.Fields(displayText(text)), " ") }
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	return style.Render(mark+" "+flat(label)) + "  ·  " + flat(detail)
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
	m.stats.GotoTop()
}

func (m *model) inspectorPanes(title string) string {
	leftWidth := m.turnListWidth()
	left := m.turnList.View()
	if len(m.turnList.Items()) == 0 {
		left = lipgloss.NewStyle().Width(leftWidth).Height(m.turnList.Height()).Render("")
	}
	left = renderPane(fmt.Sprintf("Turns · %d", len(m.turnList.Items())), left, leftWidth+4, m.logsFocused && m.turnsFocused)
	rightWidth := m.width - leftWidth - 5
	focused := m.logsFocused && !m.turnsFocused
	right := lipgloss.JoinVertical(lipgloss.Left,
		renderPane("Stats · "+title, m.stats.View(), rightWidth, focused),
		renderPane("Log", m.logs.View(), rightWidth, focused))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}
