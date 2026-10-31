package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/chatto-chat/chatto/experimental/gobotto/memorytree"
)

// memoryBrowser owns selection independently from agent activity. It never
// changes memory itself. Archived nodes remain inspectable through an explicit toggle.
type memoryBrowser struct {
	snapshot       memorytree.Snapshot
	ready          bool
	selected       string
	expanded       map[string]bool
	rows           []memoryRow
	offset         int
	archived       bool
	detailsFocused bool
	detail         viewport.Model
	markdown       markdownPreview
	filterType     string
	filterVersion  int
}
type memoryRow struct {
	node  memorytree.Node
	depth int
}

func memoryWidths(width int) (int, int) {
	left := max(16, min(48, width/3))
	return left, max(1, width-left-1)
}

func (b *memoryBrowser) refresh(width, height int) {
	if b.expanded == nil {
		b.expanded = map[string]bool{"root": true}
		b.detail = viewport.New()
		b.detail.SoftWrap = true
		b.detail.FillHeight = true
	}
	if b.selected == "" {
		b.selected = "root"
	}
	b.rows = nil
	var visit func(string, int)
	visit = func(id string, depth int) {
		n, ok := b.snapshot.Find(id)
		if !ok || (!b.archived && !b.snapshot.Visible(id)) {
			return
		}
		b.rows = append(b.rows, memoryRow{n, depth})
		if b.expanded[id] {
			for _, child := range b.snapshot.Children(id, b.archived) {
				visit(child.ID, depth+1)
			}
		}
	}
	if b.filterType == "" {
		visit("root", 0)
	} else {
		for _, n := range b.snapshot.Nodes {
			if n.Type == b.filterType && n.Version == b.filterVersion && (b.archived || b.snapshot.Visible(n.ID)) {
				b.rows = append(b.rows, memoryRow{node: n})
			}
		}
	}
	index := -1
	for i, row := range b.rows {
		if row.node.ID == b.selected {
			index = i
			break
		}
	}
	if index < 0 {
		b.selected = "root"
		if len(b.rows) > 0 {
			b.selected = b.rows[0].node.ID
		}
		index = 0
	}
	visible := max(1, height-4)
	b.offset = min(b.offset, max(0, len(b.rows)-visible))
	if index < b.offset {
		b.offset = index
	}
	if index >= b.offset+visible {
		b.offset = index - visible + 1
	}
	_, right := memoryWidths(width)
	b.detail.SetWidth(max(1, right-4))
	b.detail.SetHeight(visible)
	n, ok := b.snapshot.Find(b.selected)
	if b.filterType != "" && len(b.rows) == 0 {
		b.detail.SetContent("No records use this version. Esc returns to the tree.")
		return
	}
	if !ok {
		b.detail.SetContent("Tree memory is disabled. Set memory.backend = \"tree\" and restart.")
		return
	}
	status := "active"
	if !b.snapshot.Visible(n.ID) {
		status = "archived subtree"
	}
	if n.ID == "root" || n.Instructions {
		status += " · owner-approved instructions"
	} else {
		status += " · data"
	}
	data, _ := json.MarshalIndent(n.Data, "", "  ")
	var schema string
	for _, t := range b.snapshot.Types {
		if t.Name == n.Type && t.Version == n.Version {
			s, _ := json.MarshalIndent(t.Schema, "", "  ")
			schema = string(s)
		}
	}
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5EEAD4"))
	content := accent.Render(displayText(n.Title)) + "\n" + displayText(n.Summary) + fmt.Sprintf("\n\n%s · v%d · revision %d · %s\n%d children · position %d\nID %s\n\n", displayText(n.Type), n.Version, n.Revision, status, len(b.snapshot.Children(n.ID, true)), n.Position, n.ID) + b.markdown.render(n.Body, b.detail.Width()) + "\n\n" + accent.Render("DATA") + "\n" + displayText(string(data)) + "\n\n" + accent.Render("TYPE SCHEMA") + "\n" + displayText(schema)
	b.detail.SetContent(content)
}

func (b *memoryBrowser) update(msg tea.Msg, width, height int) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "esc" && b.filterType != "" {
		b.filterType, b.filterVersion = "", 0
		for id := b.selected; id != ""; {
			n, ok := b.snapshot.Find(id)
			if !ok {
				break
			}
			b.expanded[n.Parent] = true
			id = n.Parent
		}
		b.refresh(width, height)
		return
	}
	if len(b.rows) == 0 {
		if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "a" {
			b.archived = !b.archived
			b.refresh(width, height)
		}
		return
	}
	index := 0
	for i, r := range b.rows {
		if r.node.ID == b.selected {
			index = i
		}
	}
	selectRow := func(i int) { i = max(0, min(i, len(b.rows)-1)); b.selected = b.rows[i].node.ID; b.detail.GotoTop() }
	left, _ := memoryWidths(width)
	switch v := msg.(type) {
	case tea.MouseClickMsg:
		if v.Button != tea.MouseLeft {
			return
		}
		b.detailsFocused = v.X > left
		if !b.detailsFocused && v.Y >= 2 && v.Y < height-2 {
			i := b.offset + v.Y - 2
			if i < len(b.rows) {
				selectRow(i)
				if b.filterType == "" {
					b.expanded[b.selected] = !b.expanded[b.selected]
				}
			}
		}
	case tea.MouseWheelMsg:
		if v.X > left {
			b.detail, _ = b.detail.Update(msg)
		} else if v.Button == tea.MouseWheelDown {
			selectRow(index + 1)
		} else {
			selectRow(index - 1)
		}
	case tea.KeyPressMsg:
		switch v.String() {
		case "tab", "shift+tab":
			b.detailsFocused = !b.detailsFocused
		case "a":
			b.archived = !b.archived
		case "enter", "right":
			if !b.detailsFocused && b.filterType == "" {
				b.expanded[b.selected] = !b.expanded[b.selected]
			} else {
				b.detail, _ = b.detail.Update(msg)
			}
		case "left":
			if !b.detailsFocused && b.filterType == "" {
				if b.expanded[b.selected] {
					b.expanded[b.selected] = false
				} else if n, ok := b.snapshot.Find(b.selected); ok && n.Parent != "" {
					b.selected = n.Parent
				}
			} else {
				b.detail, _ = b.detail.Update(msg)
			}
		case "up", "k":
			if !b.detailsFocused {
				selectRow(index - 1)
			} else {
				b.detail, _ = b.detail.Update(msg)
			}
		case "down", "j":
			if !b.detailsFocused {
				selectRow(index + 1)
			} else {
				b.detail, _ = b.detail.Update(msg)
			}
		default:
			b.detail, _ = b.detail.Update(msg)
		}
	}
	b.refresh(width, height)
}

func (m *model) memoryView() tea.View {
	left, right := memoryWidths(m.width)
	height := max(1, m.height-4)
	lines := []string{}
	for i := m.memory.offset; i < min(len(m.memory.rows), m.memory.offset+height); i++ {
		r := m.memory.rows[i]
		mark := "·"
		if m.memory.filterType == "" && len(m.memory.snapshot.Children(r.node.ID, m.memory.archived)) > 0 {
			mark = "▸"
			if m.memory.expanded[r.node.ID] {
				mark = "▾"
			}
		}
		text := strings.Repeat("  ", min(r.depth, 12)) + mark + " " + displayText(r.node.Title)
		if !m.memory.snapshot.Visible(r.node.ID) {
			text += " [archived]"
		}
		style := lipgloss.NewStyle().Width(max(1, left-4)).MaxWidth(max(1, left-4)).MaxHeight(1)
		if r.node.ID == m.memory.selected {
			style = style.Background(lipgloss.Color("#294457")).Foreground(lipgloss.Color("#E8FFFF"))
		}
		lines = append(lines, style.Render(text))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	title := fmt.Sprintf("Memory · %d nodes", len(m.memory.snapshot.Nodes))
	if m.memory.filterType != "" {
		title = fmt.Sprintf("%s v%d · %d records", displayText(m.memory.filterType), m.memory.filterVersion, len(m.memory.rows))
	}
	if m.memory.archived {
		title += " · archives"
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, renderPane(title, strings.Join(lines, "\n"), left, !m.memory.detailsFocused), " ", renderPane("Node", m.memory.detail.View(), right, m.memory.detailsFocused))
	header := m.navigation()
	footer := " m activity · Enter expand · Tab focus · a archives · q quit"
	if m.memory.filterType != "" {
		footer = " Esc full tree · t types · Tab focus · a archives · q quit"
	}
	if m.quitting {
		footer = " Waiting for active turn · Shift-Q quit immediately"
	}
	content := header + "\n" + body + "\n" + footer
	if m.width < 50 || m.height < 12 {
		content = "Memory · enlarge terminal to 50×12\nm activity · q quit"
		if m.quitting {
			content = "Finishing active turn…\nShift-Q quit now"
		}
	}
	v := tea.NewView(lipgloss.NewStyle().MaxWidth(max(1, m.width)).MaxHeight(max(1, m.height)).Render(content))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}
