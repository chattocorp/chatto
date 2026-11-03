package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// eventHeight shares divider geometry with hit testing and keeps both panes usable.
func (m *model) eventHeight() int {
	available := max(2, m.treeHeight()-1)
	minimum := min(3, available/2)
	return max(minimum, min(available-minimum, int(float64(available)*m.detailSplit)))
}

func (m *model) dragDetailDivider(y int) {
	available := max(2, m.treeHeight()-1)
	minimum := min(3, available/2)
	height := max(minimum, min(available-minimum, y-1))
	m.detailSplit = (float64(height) + 0.5) / float64(available)
	m.clampNavigation()
	m.scrollDetails(0)
}

func (m *model) scrollDetails(delta int) {
	_, width := m.paneWidths()
	height := m.treeHeight() - m.eventHeight() - 1
	m.detailOffset = max(0, min(m.detailOffset+delta, len(m.detailLines(width-1))-height))
}

func (m *model) detailLines(width int) []string {
	row, session := m.currentRow(), m.selected()
	if row == nil || session == nil {
		return []string{insetLine("Select a turn or event", width)}
	}
	var parts []detailField
	add := func(label, value string) {
		if value != "" {
			parts = append(parts, detailField{label, value})
		}
	}
	for _, turn := range session.records {
		if turn.Number != row.key.turn {
			continue
		}
		if row.key.step == 0 {
			add("Selected", row.label)
		} else {
			add("Selected", fmt.Sprintf("Turn %d · %s", turn.Number, row.label))
		}
		switch step := row.key.step; {
		case step == 0:
			add("State", turn.State)
			add("Model", turn.Model)
			if !turn.Started.IsZero() {
				add("Started", turn.Started.Local().Format("2006-01-02 15:04:05"))
			}
			if !turn.Finished.IsZero() {
				add("Runtime", turn.Finished.Sub(turn.Started).Round(time.Millisecond).String())
			}
			add("Events", fmt.Sprintf("%d inputs · %d steps", len(turn.Inputs), len(turn.Steps)))
			add("Output", turn.Response)
			add("Reaction", turn.Emoji)
			add("Failure", turn.Failure)
		case step > 0 && step <= len(turn.Steps):
			s := turn.Steps[step-1]
			add("Type", s.Kind)
			add("State", s.State)
			add("Runtime", s.Duration.Round(time.Millisecond).String())
			add("Provider", s.Provider)
			add("Request ID", s.RequestID)
			if s.Usage.Known {
				add("Tokens", fmt.Sprintf("%d input · %d output", s.Usage.Input, s.Usage.Output))
			}
			if s.Usage.CostKnown {
				cost := fmt.Sprintf("$%.6f", s.Usage.Cost)
				if s.Usage.CostMissing {
					cost += " (partial)"
				}
				add("Cost", cost)
			}
			add("Details", s.Detail)
			add("Request", s.Request)
			add("Response", s.Response)
		case step == -1000001:
			add("State", turn.State)
			add("Output", turn.Response)
		case step == -1000002:
			add("Reaction", turn.Emoji)
		case step == -1000003:
			add("Failure", turn.Failure)
		case step < 0 && -step <= len(turn.Inputs):
			input := turn.Inputs[-step-1]
			add("Addressed", fmt.Sprint(input.Addressed))
			add("Input", input.Text)
		}
		break
	}
	return renderDetails(parts, width)
}

type detailField struct{ label, value string }

// renderDetails keeps metadata compact and gives recorded content a separate
// visual hierarchy. Sanitize source text before adding our own terminal styles.
func renderDetails(fields []detailField, width int) []string {
	width = max(1, width)
	fullWidth := width
	width = contentWidth(width)
	bands := make(map[int]bool)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#8593A4"))
	bright := lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0"))
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("#5EEAD4"))
	var title, state string
	var metadata, content []detailField
	for _, field := range fields {
		switch field.label {
		case "Selected":
			title = displayText(field.value)
		case "State":
			state = displayText(field.value)
		case "Input", "Output", "Details", "Request", "Response", "Failure":
			content = append(content, field)
		default:
			metadata = append(metadata, field)
		}
	}
	var lines []string
	header := title
	if state != "" {
		badge := strings.ToUpper(state)
		if ansi.StringWidth(header)+ansi.StringWidth(badge)+2 <= width {
			header += strings.Repeat(" ", width-ansi.StringWidth(header)-ansi.StringWidth(badge)) + badge
		} else {
			header += "\n" + badge
		}
	}
	for _, line := range strings.Split(ansi.Hardwrap(header, width, true), "\n") {
		bands[len(lines)] = true
		lines = append(lines, line)
	}
	lines = append(lines, "")
	// Wide inspectors use two aligned metadata columns; narrow ones stack fields.
	columns := 1
	if width >= 64 {
		columns = 2
	}
	cellWidth := (width - (columns-1)*3) / columns
	for i := 0; i < len(metadata); i += columns {
		cells := make([][]string, columns)
		height := 0
		for c := 0; c < columns && i+c < len(metadata); c++ {
			f := metadata[i+c]
			label := muted.Render(fmt.Sprintf("%-9s", f.label))
			value := bright.Render(displayText(f.value))
			cells[c] = strings.Split(ansi.Hardwrap(label+" "+value, cellWidth, true), "\n")
			height = max(height, len(cells[c]))
		}
		for y := 0; y < height; y++ {
			var row []string
			for c := 0; c < columns; c++ {
				cell := ""
				if y < len(cells[c]) {
					cell = cells[c][y]
				}
				row = append(row, cell+strings.Repeat(" ", max(0, cellWidth-ansi.StringWidth(cell))))
			}
			lines = append(lines, strings.Join(row, "   "))
		}
	}
	for _, field := range content {
		lines = append(lines, "")
		label := strings.ToUpper(field.label)
		bands[len(lines)] = true
		lines = append(lines, label)
		value := field.value
		var pretty bytes.Buffer
		isJSON := json.Indent(&pretty, []byte(value), "", "  ") == nil
		if isJSON {
			value = pretty.String()
		}
		code := isJSON
		for _, raw := range strings.Split(value, "\n") {
			line := displayText(ansi.Strip(raw))
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				code = !code
				language := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "```"))
				if code && language != "" {
					lines = append(lines, muted.Italic(true).Render(ansi.Truncate(language, width, "…")))
				}
				continue
			}
			style := bright
			prefix := ""
			if code && width > 3 {
				prefix = muted.Render("│ ")
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("#C4B5FD")).Background(lipgloss.Color("#202733"))
			} else if strings.HasPrefix(line, "#") {
				line = strings.TrimLeft(line, "# ")
				style = accent.Bold(true)
			} else if strings.HasPrefix(line, "- ") {
				line = "• " + line[2:]
			}
			bodyWidth := max(1, width-ansi.StringWidth(prefix))
			for _, wrapped := range strings.Split(ansi.Hardwrap(line, bodyWidth, true), "\n") {
				if code {
					wrapped += strings.Repeat(" ", max(0, bodyWidth-ansi.StringWidth(wrapped)))
				}
				lines = append(lines, prefix+style.Render(wrapped))
			}
		}
	}
	for i, line := range lines {
		if bands[i] {
			lines[i] = separator(line, fullWidth, false)
		} else {
			lines[i] = insetLine(line, fullWidth)
		}
	}
	return lines
}
