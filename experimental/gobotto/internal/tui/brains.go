package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/brainview"
)

type brainGroup struct {
	brainview.Brain
	page      brainview.Page
	offset    int
	loaded    bool
	collapsed bool
	err       string
}
type brainRow struct {
	key, id, label string
	header         bool
}
type brainRequest struct {
	kind, key, id string
	offset        int
	generation    uint64
}
type brainResult struct {
	request brainRequest
	catalog []brainview.Brain
	page    brainview.Page
	doc     brainview.Document
	err     error
}
type brainsView struct {
	source                  brainview.Source
	ctx                     context.Context
	groups                  []brainGroup
	rows                    []brainRow
	cursor, offset, scroll  int
	contentFocused          bool
	split                   float64
	dragging                bool
	ready, loading          bool
	generation              uint64
	catalogError            string
	docKey, docID, docError string
	doc                     brainview.Document
	lines                   []string
	lineWidth               int
}

func (b *brainsView) selected() *brainRow {
	if b.cursor < 0 || b.cursor >= len(b.rows) {
		return nil
	}
	return &b.rows[b.cursor]
}
func (b *brainsView) group(key string) *brainGroup {
	for i := range b.groups {
		if b.groups[i].Key == key {
			return &b.groups[i]
		}
	}
	return nil
}
func (b *brainsView) rebuild() {
	var selected brainRow
	if row := b.selected(); row != nil {
		selected = *row
	}
	b.rows = nil
	for _, group := range b.groups {
		b.rows = append(b.rows, brainRow{key: group.Key, label: group.Name, header: true})
		if group.collapsed {
			continue
		}
		for _, entry := range group.page.Entries {
			b.rows = append(b.rows, brainRow{key: group.Key, id: entry.ID, label: entry.Title})
		}
	}
	b.cursor = 0
	// A deleted document or page replacement falls back to its own collection.
	for i, row := range b.rows {
		if row.key == selected.key && row.header {
			b.cursor = i
			break
		}
	}
	for i, row := range b.rows {
		if row.key == selected.key && row.id == selected.id {
			b.cursor = i
			break
		}
	}
	b.lines = nil
}

func (b *brainsView) request(req brainRequest) tea.Cmd {
	if b.loading || b.source == nil {
		return nil
	}
	b.loading = true
	req.generation = b.generation
	source, parent := b.source, b.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		defer cancel()
		result := brainResult{request: req}
		switch req.kind {
		case "catalog":
			result.catalog, result.err = source.Catalog(ctx)
		case "list":
			result.page, result.err = source.List(ctx, req.key, req.offset)
		case "read":
			result.doc, result.err = source.Read(ctx, req.key, req.id)
		}
		return result
	}
}
func (b *brainsView) loadSelected() tea.Cmd {
	if b.loading || b.source == nil {
		return nil
	}
	if !b.ready {
		if b.catalogError != "" {
			return nil
		}
		return b.request(brainRequest{kind: "catalog"})
	}
	row := b.selected()
	if row == nil {
		return nil
	}
	group := b.group(row.key)
	if !group.loaded && group.err == "" {
		return b.request(brainRequest{kind: "list", key: row.key, offset: group.offset})
	}
	if !row.header && (b.docKey != row.key || b.docID != row.id) {
		return b.request(brainRequest{kind: "read", key: row.key, id: row.id})
	}
	return nil
}
func (b *brainsView) finish(result brainResult) {
	b.loading = false
	if result.request.generation != b.generation {
		return
	}
	b.lines = nil
	safeError := ""
	if result.err != nil {
		safeError = "Unable to load · r retry"
	}
	switch result.request.kind {
	case "catalog":
		b.catalogError = safeError
		if result.err == nil {
			b.ready = true
			collapsed := map[string]bool{}
			for _, group := range b.groups {
				collapsed[group.Key] = group.collapsed
			}
			b.groups = nil
			for _, brain := range result.catalog {
				folded, known := collapsed[brain.Key]
				if !known {
					folded = true
				}
				b.groups = append(b.groups, brainGroup{Brain: brain, collapsed: folded})
			}
			b.rebuild()
		}
	case "list":
		if group := b.group(result.request.key); group != nil {
			group.err = safeError
			if result.err == nil {
				group.page, group.loaded, group.offset = result.page, true, result.request.offset
			}
			b.rebuild()
		}
	case "read":
		b.docKey, b.docID, b.docError, b.doc = result.request.key, result.request.id, safeError, result.doc
	}
}

func (b *brainsView) content(width int) []string {
	if b.lines != nil && b.lineWidth == width {
		return b.lines
	}
	message := "Select a brain"
	var fields []detailField
	if b.source == nil {
		message = "Brain browser unavailable"
	} else if b.catalogError != "" {
		message = b.catalogError
	} else if !b.ready {
		message = "Loading brains…"
	} else if row := b.selected(); row != nil {
		group := b.group(row.key)
		message = "Loading…"
		if group.err != "" {
			message = group.err
		} else if row.header && group.loaded {
			fields = []detailField{{"Selected", group.page.Definition.Title}, {"Kind", group.Kind}, {"Brain", group.Key}, {"Instructions", group.page.Definition.Body}, {"Metadata", group.page.Definition.Metadata}}
			if group.page.Skipped > 0 {
				fields = append(fields, detailField{"Skipped", fmt.Sprintf("%d invalid documents", group.page.Skipped)})
			}
			if len(group.page.Entries) == 0 {
				fields = append(fields, detailField{"Documents", "No documents"})
			}
		} else if !row.header && b.docKey == row.key && b.docID == row.id {
			if b.docError != "" {
				message = b.docError
			} else {
				fields = []detailField{{"Selected", b.doc.Title}, {"Brain", group.Key}, {"ID", row.id}, {"Body", b.doc.Body}, {"Metadata", b.doc.Metadata}}
			}
		}
	}
	if fields != nil {
		b.lines = renderDetails(fields, width)
	} else {
		b.lines = []string{insetLine(message, width)}
	}
	b.lineWidth = width
	return b.lines
}

func (m *model) brainWidths() (int, int) {
	return splitWidths(m.width, m.brains.split)
}
func (m *model) clampBrains() {
	b := &m.brains
	b.cursor = max(0, min(b.cursor, len(b.rows)-1))
	b.offset = visibleOffset(b.cursor, b.offset, len(b.rows), m.treeHeight())
	_, right := m.brainWidths()
	b.scroll = max(0, min(b.scroll, len(b.content(right-1))-m.treeHeight()))
}
func (m *model) moveBrains(delta int) tea.Cmd {
	b := &m.brains
	if b.contentFocused {
		b.scroll += delta
	} else {
		b.cursor = max(0, min(b.cursor+delta, len(b.rows)-1))
		b.scroll, b.lines = 0, nil
	}
	m.clampBrains()
	return b.loadSelected()
}

// foldBrain preserves the collection selection when its document rows disappear.
func (m *model) foldBrain(collapsed bool) tea.Cmd {
	b := &m.brains
	if row := b.selected(); row != nil {
		b.group(row.key).collapsed = collapsed
		b.rebuild()
		b.scroll = 0
		m.clampBrains()
	}
	return b.loadSelected()
}

func (m *model) updateBrains(msg tea.Msg) tea.Cmd {
	b := &m.brains
	left, _ := m.brainWidths()
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft || msg.Y < 0 || msg.Y >= m.height-1 || msg.X < 0 || msg.X >= m.width {
			return nil
		}
		if msg.X == left {
			b.dragging = true
			return nil
		}
		b.contentFocused = msg.X > left
		if !b.contentFocused && msg.Y >= 1 && msg.Y <= m.treeHeight() {
			index := b.offset + msg.Y - 1
			if index < len(b.rows) {
				cmd := m.moveBrains(index - b.cursor)
				if row := b.selected(); row != nil && row.header && msg.X == 1 {
					foldCmd := m.foldBrain(!b.group(row.key).collapsed)
					return tea.Batch(cmd, foldCmd)
				}
				return cmd
			}
		}
	case tea.MouseMotionMsg:
		if b.dragging {
			b.split = dividerRatio(m.width, msg.X)
			m.clampBrains()
		}
	case tea.MouseReleaseMsg:
		if b.dragging {
			b.split = dividerRatio(m.width, msg.X)
			b.dragging = false
			m.clampBrains()
		}
	case tea.MouseWheelMsg:
		if msg.Y < 1 || msg.Y > m.treeHeight() || msg.X == left {
			return nil
		}
		b.contentFocused = msg.X > left
		if msg.Button == tea.MouseWheelUp {
			return m.moveBrains(-3)
		}
		if msg.Button == tea.MouseWheelDown {
			return m.moveBrains(3)
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "tab", "shift+tab":
			b.contentFocused = !b.contentFocused
		case "esc":
			b.contentFocused = false
		case "left":
			if b.contentFocused {
				b.contentFocused = false
			} else {
				return m.foldBrain(true)
			}
		case "right":
			if !b.contentFocused {
				return m.foldBrain(false)
			}
		case "enter", "space", " ":
			if !b.contentFocused {
				if row := b.selected(); row != nil && row.header {
					return m.foldBrain(!b.group(row.key).collapsed)
				}
				b.contentFocused = true
			} else if msg.String() != "enter" {
				return m.moveBrains(m.treeHeight())
			}
		case "up", "k":
			return m.moveBrains(-1)
		case "down", "j":
			return m.moveBrains(1)
		case "pgup":
			return m.moveBrains(-m.treeHeight())
		case "pgdown":
			return m.moveBrains(m.treeHeight())
		case "home", "g":
			return m.moveBrains(-1 << 30)
		case "end", "G":
			return m.moveBrains(1 << 30)
		case "r":
			b.generation++
			b.ready, b.catalogError, b.docKey, b.docID, b.docError = false, "", "", "", ""
			b.lines, b.scroll = nil, 0
			return b.loadSelected()
		case "[", "]":
			delta := 1
			if msg.String() == "[" {
				delta = -1
			}
			for i := b.cursor + delta; i >= 0 && i < len(b.rows); i += delta {
				if b.rows[i].header {
					b.contentFocused = false
					return m.moveBrains(i - b.cursor)
				}
			}
		case "n", "p":
			if row := b.selected(); row != nil && !b.loading {
				g := b.group(row.key)
				offset := max(0, g.offset-brainview.PageSize)
				if msg.String() == "n" {
					if !g.page.More {
						return nil
					}
					offset = g.offset + len(g.page.Entries)
				}
				if offset != g.offset {
					b.scroll = 0
					return b.request(brainRequest{kind: "list", key: g.Key, offset: offset})
				}
			}
		}
	}
	return nil
}

func (m *model) brainsContent() string {
	b := &m.brains
	left, right := m.brainWidths()
	border := lipgloss.NewStyle().Foreground(lipgloss.Color("#3C4654")).Render
	divider := border
	if b.dragging {
		divider = lipgloss.NewStyle().Foreground(lipgloss.Color("#5EEAD4")).Render
	}
	title := "Brains"
	if b.loading {
		title += " · loading"
	}
	lines := []string{border("╭") + panelTitle(title, left-1, !b.contentFocused, border) + divider("┬") + panelTitle("Contents", right-1, b.contentFocused, border) + border("╮")}
	contents := b.content(right - 1)
	scroll := min(b.scroll, max(0, len(contents)-m.treeHeight()))
	for i := 0; i < m.treeHeight(); i++ {
		l, r := fitLine("", left-1), fitLine("", right-1)
		if n := b.offset + i; n < len(b.rows) {
			row := b.rows[n]
			if row.header {
				prefix := contentIndent
				group := b.group(row.key)
				if !group.loaded || len(group.page.Entries) > 0 {
					prefix = "⌄ "
					if group.collapsed {
						prefix = "› "
					}
				}
				l = separatorBar(prefix+displayText(row.label), left-1, n == b.cursor && !b.contentFocused)
			} else {
				l = rowText(insetLine(displayText(row.label), left-1), left-1, n == b.cursor, !b.contentFocused)
			}
		} else if len(b.rows) == 0 && i == 0 {
			l = insetLine("No brains", left-1)
		}
		if n := scroll + i; n < len(contents) {
			r = contents[n]
		}
		lines = append(lines, border("│")+l+divider("│")+r+border("│"))
	}
	lines = append(lines, border("╰"+strings.Repeat("─", left-1))+divider("┴")+border(strings.Repeat("─", right-1)+"╯"))
	help := "1 Sessions · 2 Brains · Tab panes · [ ] brains · n/p pages · r refresh · q quit"
	if row := b.selected(); row != nil {
		g := b.group(row.key)
		if g.loaded && (g.offset > 0 || g.page.More) {
			help = fmt.Sprintf("Docs %d–%d · ", g.offset+1, g.offset+len(g.page.Entries)) + help
		}
	}
	if m.quitting {
		help = "Waiting for active work · Shift-Q quit immediately"
	}
	return strings.Join(lines, "\n") + "\n" + ansi.Truncate(help, m.width, "…")
}
