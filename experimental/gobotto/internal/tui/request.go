package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type detailSection struct{ name, text string }
type detailView struct {
	session     uint64
	title       string
	sections    []detailSection
	tab, offset int
}

func prettyJSON(raw string) string {
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return raw
	}
	b, _ := json.MarshalIndent(value, "", "  ")
	return string(b)
}

// requestSizes measures compact inspection JSON, not tokens or wire bytes:
// image payloads and provider reasoning have already been removed.
func requestSizes(request map[string]json.RawMessage) string {
	compact, _ := json.Marshal(request)
	var messages []map[string]json.RawMessage
	_ = json.Unmarshal(request["messages"], &messages)
	lastUser := -1
	for i, msg := range messages {
		if string(msg["role"]) == `"user"` {
			lastUser = i
		}
	}
	var system, input, history, results int
	for i, msg := range messages {
		b, _ := json.Marshal(msg)
		switch {
		case string(msg["role"]) == `"system"`:
			system += len(b)
		case string(msg["role"]) == `"tool"`:
			results += len(b)
		case i == lastUser:
			input += len(b)
		default:
			history += len(b)
		}
	}
	var schemas int
	if raw := request["tools"]; len(raw) > 0 {
		b, _ := json.Marshal(raw)
		schemas = len(b)
	}
	other := len(compact) - system - input - history - results - schemas
	return fmt.Sprintf("Compact inspection JSON (bytes; not tokens; images/reasoning omitted)\nSystem: %d\nTool schemas: %d\nLatest input: %d\nOther history: %d\nTool results: %d\nSettings and framing: %d\nTotal: %d", system, schemas, input, history, results, other, len(compact))
}

// openDetail takes a private snapshot so live events cannot move the selected
// request while the user scrolls or copies it. Reopen to refresh running work.
func (m *model) openDetail() {
	row := m.currentRow()
	if row == nil {
		return
	}
	d := &detailView{title: "Session inspection"}
	if row.s != nil {
		d.session = row.s.id
	}
	if row.step != nil {
		s := row.step
		d.title = fmt.Sprintf("Turn %d · %s", row.t.Number, s.Name)
		d.sections = append(d.sections, detailSection{"Details", fmt.Sprintf("%s · %s\nRuntime: %s\nProvider: %s\nRequest: %s\n\n%s", s.Name, s.State, s.Duration, s.Provider, s.RequestID, s.Detail)})
		if s.Kind == "model" {
			var request map[string]json.RawMessage
			if s.Request != "" && json.Unmarshal([]byte(s.Request), &request) == nil {
				d.sections[0].text += "\n\n" + requestSizes(request)
				var messages []map[string]json.RawMessage
				_ = json.Unmarshal(request["messages"], &messages)
				var text strings.Builder
				for i, msg := range messages {
					var role, content string
					_ = json.Unmarshal(msg["role"], &role)
					fmt.Fprintf(&text, "── %d · %s ──\n", i+1, role)
					if json.Unmarshal(msg["content"], &content) == nil {
						text.WriteString(prettyJSON(content))
					} else {
						text.WriteString(prettyJSON(string(msg["content"])))
					}
					for _, key := range []string{"name", "tool_call_id", "tool_calls"} {
						if v := msg[key]; len(v) > 0 {
							fmt.Fprintf(&text, "\n%s: %s", key, prettyJSON(string(v)))
						}
					}
					text.WriteString("\n\n")
				}
				d.sections = append(d.sections, detailSection{"Messages", text.String()}, detailSection{"Tools", prettyJSON(string(request["tools"]))})
				delete(request, "messages")
				delete(request, "tools")
				b, _ := json.MarshalIndent(request, "", "  ")
				d.sections = append(d.sections, detailSection{"Settings", string(b)})
			} else {
				text := s.Request
				if text == "" {
					text = "Request unavailable. Full request inspection is live-only and bounded; it is not saved across restarts or history reloads."
				}
				d.sections = append(d.sections, detailSection{"Request", text})
			}
			response := s.Response
			if response == "" {
				response = "No live response available (running, failed, evicted, or reloaded record)."
			}
			d.sections = append(d.sections, detailSection{"Response", response})
		}
	} else if row.key.step != 0 {
		d.title = row.label
		d.sections = []detailSection{{"Content", row.detail}}
	} else if row.t != nil {
		t := row.t
		d.title = fmt.Sprintf("Turn %d", t.Number)
		var inputs []string
		for _, i := range t.Inputs {
			inputs = append(inputs, i.Text)
		}
		d.sections = []detailSection{{"Summary", fmt.Sprintf("State: %s\nModel: %s\nStarted: %s\nFinished: %s\nSteps: %d\nFailure: %s\n\nSelect a completion and press Enter to inspect its prompt and tools.", t.State, t.Model, t.Started, t.Finished, len(t.Steps), t.Failure)}, {"Input", strings.Join(inputs, "\n\n")}, {"Output", t.Response}}
	} else if row.s != nil {
		s := row.s
		d.sections = []detailSection{{"Summary", fmt.Sprintf("Session: %s\nTitle: %s\nState: %s\nTurns: %d\nLoaded turns: %d\n\nEnter this session to inspect individual turns and model calls.", s.humanID, s.title, s.state, s.turns, len(s.records))}}
	}
	if len(d.sections) > 0 {
		m.detail = d
	}
}

func (m *model) detailLines() []string {
	return strings.Split(ansi.Hardwrap(displayText(m.detail.sections[m.detail.tab].text), max(1, m.width-4), true), "\n")
}
func (m *model) detailKey(key string) tea.Cmd {
	d := m.detail
	switch key {
	case "esc", "i":
		m.detail = nil
	case "tab", "right":
		d.tab = (d.tab + 1) % len(d.sections)
		d.offset = 0
	case "shift+tab", "left":
		d.tab = (d.tab + len(d.sections) - 1) % len(d.sections)
		d.offset = 0
	case "up", "k":
		d.offset--
	case "down", "j":
		d.offset++
	case "pgdown", "ctrl+d", "space":
		d.offset += max(1, m.height-6)
	case "pgup", "ctrl+u":
		d.offset -= max(1, m.height-6)
	case "home", "g":
		d.offset = 0
	case "end", "G":
		d.offset = len(m.detailLines())
	case "c":
		text := d.sections[d.tab].text
		copyText := m.copyText
		return func() tea.Msg { return clipboardResult{id: "inspection section", err: copyText(text)} }
	}
	if m.detail != nil {
		d.offset = min(max(0, d.offset), max(0, len(m.detailLines())-max(1, m.height-6)))
	}
	return nil
}
func (m *model) detailContent() string {
	d := m.detail
	var tabs []string
	for i, s := range d.sections {
		label := s.name
		if i == d.tab {
			label = "[" + label + "]"
		}
		tabs = append(tabs, label)
	}
	lines := m.detailLines()
	height := max(1, m.height-6)
	start := min(d.offset, max(0, len(lines)-height))
	end := min(len(lines), start+height)
	body := strings.Join(lines[start:end], "\n")
	body += strings.Repeat("\n", max(0, height-(end-start)))
	foot := fmt.Sprintf("%d–%d/%d · Tab sections · ↑↓ scroll · c copy · Esc back · q quit", start+1, end, len(lines))
	if m.clipboardNotice != "" {
		foot = m.clipboardNotice + " · Esc back"
	}
	if m.quitting {
		foot = "Waiting for active work · Shift-Q quit immediately"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8F9")).Render(ansi.Truncate(displayText(d.title), m.width, "…")) + "\n" + ansi.Truncate(strings.Join(tabs, "  "), m.width, "…") + "\n" + renderPane("Private snapshot · images and provider reasoning omitted", body, m.width, true) + "\n" + ansi.Truncate(foot, m.width, "…")
}
