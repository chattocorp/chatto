package main

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Sidebar identity is separate from the open conversation: collapsing its
// section must not change the composer destination or discard the draft.
type sidebarRow struct {
	key, parent, label string
	header             bool
	roomIndex, unread  int
}

func (m *model) sidebarRows() []sidebarRow {
	byID := map[string]int{}
	for i, r := range m.rooms {
		byID[r.Room.ID] = i
	}
	used := map[string]bool{}
	rows := []sidebarRow{}
	section := func(key, label string, indices []int) {
		header := sidebarRow{key: key, label: label, header: true}
		for _, i := range indices {
			if m.rooms[i].ViewerState.HasUnread {
				header.unread++
			}
		}
		rows = append(rows, header)
		if m.collapsed[key] {
			return
		}
		for _, i := range indices {
			rows = append(rows, sidebarRow{key: "room:" + m.rooms[i].Room.ID, parent: key, label: m.roomName(m.rooms[i]), roomIndex: i})
		}
	}
	for _, group := range m.groups {
		indices := []int{}
		for _, item := range group.Items {
			if item.Room == nil {
				continue
			}
			id := item.Room.Room.ID
			i, ok := byID[id]
			// Intersect independently read snapshots. Never restore a room that
			// disappeared from the current authorized directory.
			if !ok || used[id] || m.rooms[i].Room.Kind == "ROOM_KIND_DM" {
				continue
			}
			indices = append(indices, i)
			used[id] = true
		}
		if len(indices) > 0 {
			label := group.Name
			if strings.TrimSpace(label) == "" {
				label = "Rooms"
			}
			section("group:"+group.ID, label, indices)
		}
	}
	other, dms := []int{}, []int{}
	for i, r := range m.rooms {
		if r.Room.Kind == "ROOM_KIND_DM" {
			dms = append(dms, i)
		} else if !used[r.Room.ID] {
			other = append(other, i)
		}
	}
	if len(other) > 0 || len(rows) == 0 {
		label := "Rooms"
		if len(rows) > 0 {
			label = "Other rooms"
		}
		section("rooms", label, other)
	}
	section("dms", "Direct messages", dms)
	return rows
}

func (m *model) sidebarIndex(rows []sidebarRow) int {
	for i, r := range rows {
		if r.key == m.sidebarKey {
			return i
		}
	}
	for i, r := range rows {
		if r.key == "room:"+m.room {
			return i
		}
	}
	return 0
}

func (m *model) syncSidebar() {
	rows := m.sidebarRows()
	i := m.sidebarIndex(rows)
	m.sidebarKey = rows[i].key
	visible := m.bodyHeight()
	if i < m.sidebarTop {
		m.sidebarTop = i
	}
	if i >= m.sidebarTop+visible {
		m.sidebarTop = i - visible + 1
	}
	m.sidebarTop = min(m.sidebarTop, max(0, len(rows)-visible))
}

func (m *model) activateSidebar(index int) tea.Cmd {
	rows := m.sidebarRows()
	if index < 0 || index >= len(rows) {
		return nil
	}
	r := rows[index]
	m.sidebarKey = r.key
	if r.header {
		if m.collapsed == nil {
			m.collapsed = map[string]bool{}
		}
		m.collapsed[r.key] = !m.collapsed[r.key]
		m.syncSidebar()
		return m.persistSidebar()
	}
	m.syncSidebar()
	if m.room == m.rooms[r.roomIndex].Room.ID {
		return nil
	}
	return m.chooseRoom(r.roomIndex)
}

func (m *model) moveSidebar(delta int) tea.Cmd {
	rows := m.sidebarRows()
	i := max(0, min(len(rows)-1, m.sidebarIndex(rows)+delta))
	m.sidebarKey = rows[i].key
	m.syncSidebar()
	if !rows[i].header && m.room != m.rooms[rows[i].roomIndex].Room.ID {
		return m.chooseRoom(rows[i].roomIndex)
	}
	return nil
}

func (m *model) foldSidebar(collapse bool) tea.Cmd {
	rows := m.sidebarRows()
	r := rows[m.sidebarIndex(rows)]
	key := r.key
	if !r.header {
		key = r.parent
	}
	if m.collapsed == nil {
		m.collapsed = map[string]bool{}
	}
	m.collapsed[key] = collapse
	m.sidebarKey = key
	m.syncSidebar()
	return m.persistSidebar()
}

func (m *model) persistSidebar() tea.Cmd {
	m.e.queueSidebar(m.collapsed)
	return func() tea.Msg { return sidebarSaved{m.e.saveSidebar()} }
}

func (m *model) sidebarContent(width, height int) string {
	rows := m.sidebarRows()
	top := min(m.sidebarTop, max(0, len(rows)-height))
	cursor := m.sidebarIndex(rows)
	var out strings.Builder
	for i := top; i < len(rows) && i < top+height; i++ {
		r := rows[i]
		style := lipgloss.NewStyle().Width(width).MaxWidth(width)
		label := "  " + r.label
		if r.header {
			mark := "▾"
			if m.collapsed[r.key] {
				mark = "▸"
			}
			suffix := ""
			if r.unread > 0 {
				suffix += " ●"
			}
			label = " " + mark + " " + ansi.Truncate(oneLine(r.label), max(1, width-3-lipgloss.Width(suffix)), "…")
			label += strings.Repeat(" ", max(0, width-lipgloss.Width(label)-lipgloss.Width(suffix))) + suffix
			style = style.Bold(true).Foreground(lipgloss.Color("#5EEAD4")).Background(lipgloss.Color("#203340"))
		} else {
			if m.rooms[r.roomIndex].ViewerState.HasUnread {
				label = "● " + r.label
				style = style.Bold(true).Foreground(lipgloss.Color("#F8FAFC"))
			}
			if m.rooms[r.roomIndex].Room.ID == m.room {
				style = style.Background(lipgloss.Color("#193D48")).Foreground(lipgloss.Color("#E7FFF8"))
			}
		}
		if i == cursor && m.focus == 0 {
			style = style.Background(lipgloss.Color("#285465")).Foreground(lipgloss.Color("#E7FFF8"))
		}
		badges := ""
		if !r.header {
			count := m.notifications[m.rooms[r.roomIndex].Room.ID]
			badges = notificationBadges(count.Unread, count.Important)
		}
		if badges == "" {
			out.WriteString(style.Render(ansi.Truncate(label, width, "…")) + "\n")
		} else {
			remaining := max(0, width-ansi.StringWidth(badges)-1)
			label = ansi.Truncate(label, remaining, "…")
			label += strings.Repeat(" ", max(0, width-ansi.StringWidth(label)-ansi.StringWidth(badges)))
			out.WriteString(style.Render(label+badges) + "\n")
		}
	}
	return out.String()
}

// Notification badges count occurrences, independently of the neutral unread dot.
// Ordinary attention comes first; important attention stays at the row edge.
func notificationBadges(total, important int) string {
	important = max(0, min(total, important))
	var badges []string
	for _, badge := range []struct {
		count int
		color string
	}{{max(0, total-important), "#F8FAFC"}, {important, "#F59E0B"}} {
		if badge.count == 0 {
			continue
		}
		label := strconv.Itoa(badge.count)
		if badge.count > 99 {
			label = "99+"
		}
		badges = append(badges, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#17212B")).Background(lipgloss.Color(badge.color)).Render(" "+label+" "))
	}
	return strings.Join(badges, " ")
}

// sidebarBackground extends each highlighted row through the pane gutters.
func (m *model) sidebarBackground(height int) rowBackground {
	rows := m.sidebarRows()
	top := min(m.sidebarTop, max(0, len(rows)-height))
	cursor := m.sidebarIndex(rows)
	return func(y int) string {
		i := y + top
		if i < 0 || i >= len(rows) {
			return ""
		}
		r := rows[i]
		if i == cursor && m.focus == 0 {
			return "#285465"
		}
		if r.header {
			return "#203340"
		}
		if m.rooms[r.roomIndex].Room.ID == m.room {
			return "#193D48"
		}
		return ""
	}
}
