package tui

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/chatto-chat/chatto/experimental/gobotto/memorytree"
)

// typeBrowser owns a stable version selection and a read-only schema viewport.
type typeBrowser struct {
	rows                  []memorytree.Type
	selected              string
	offset                int
	ready, detailsFocused bool
	detail                viewport.Model
}

func typeKey(t memorytree.Type) string { return fmt.Sprintf("%s v%d", t.Name, t.Version) }

func typeCounts(s memorytree.Snapshot, typ memorytree.Type) (active, archived int) {
	for _, n := range s.Nodes {
		if n.Type == typ.Name && n.Version == typ.Version {
			if s.Visible(n.ID) {
				active++
			} else {
				archived++
			}
		}
	}
	return
}

func (b *typeBrowser) index() int {
	for i, typ := range b.rows {
		if typeKey(typ) == b.selected {
			return i
		}
	}
	return 0
}

func (b *typeBrowser) refresh(s memorytree.Snapshot, width, height int) {
	if !b.ready {
		b.detail = viewport.New()
		b.detail.SoftWrap, b.detail.FillHeight = true, true
		b.ready = true
	}
	b.rows = append([]memorytree.Type(nil), s.Types...)
	sort.Slice(b.rows, func(i, j int) bool {
		if b.rows[i].Name == b.rows[j].Name {
			return b.rows[i].Version > b.rows[j].Version
		}
		return b.rows[i].Name < b.rows[j].Name
	})
	_, right := memoryWidths(width)
	b.detail.SetWidth(max(1, right-4))
	b.detail.SetHeight(max(1, height-4))
	if len(b.rows) == 0 {
		b.selected, b.offset = "", 0
		b.detail.SetContent("No schemas available. Enable tree memory to inspect its types.")
		return
	}
	i := b.index()
	b.selected = typeKey(b.rows[i])
	b.offset = max(0, min(b.offset, len(b.rows)-max(1, height-4)))
	if i < b.offset {
		b.offset = i
	}
	if i >= b.offset+max(1, height-4) {
		b.offset = i - max(1, height-4) + 1
	}
	current := b.rows[i]
	var previous *memorytree.Type
	for j := range b.rows {
		t := &b.rows[j]
		if t.Name == current.Name && t.Version < current.Version && (previous == nil || t.Version > previous.Version) {
			previous = t
		}
	}
	active, archived := typeCounts(s, current)
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5EEAD4"))
	var content strings.Builder
	content.WriteString(accent.Render(displayText(typeKey(current))) + fmt.Sprintf("\n%d active · %d archived records\nEnter opens these records in Memory\n\n", active, archived))
	content.WriteString(accent.Render("Fields") + "\n" + schemaFields(current.Schema, b.detail.Width()) + "\n\n")
	content.WriteString(accent.Render("Changes") + "\n")
	if previous == nil {
		content.WriteString("First stored version.\n")
	} else {
		content.WriteString(fmt.Sprintf("Compared with v%d · structural diff, not a compatibility verdict\n", previous.Version))
		var before, after any
		_ = json.Unmarshal(previous.Schema, &before)
		_ = json.Unmarshal(current.Schema, &after)
		changes := schemaChanges("", before, after)
		if len(changes) == 0 {
			content.WriteString("No schema changes.\n")
		} else {
			content.WriteString(strings.Join(changes, "\n") + "\n")
		}
	}
	raw, err := json.MarshalIndent(current.Schema, "", "  ")
	if err != nil {
		raw = []byte("Schema could not be decoded.")
	}
	content.WriteString("\n" + accent.Render("JSON Schema") + "\n" + displayText(string(raw)))
	b.detail.SetContent(content.String())
}

func schemaFields(raw json.RawMessage, width int) string {
	var schema map[string]any
	if json.Unmarshal(raw, &schema) != nil || schema == nil {
		return "No object field table; inspect the full schema below."
	}
	props, _ := schema["properties"].(map[string]any)
	if len(props) == 0 {
		return "No top-level fields declared; constraints may be defined elsewhere in the schema."
	}
	required := map[string]bool{}
	if names, ok := schema["required"].([]any); ok {
		for _, name := range names {
			if s, ok := name.(string); ok {
				required[s] = true
			}
		}
	}
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	nameWidth := max(6, min(24, (width-7)/2))
	typeWidth := max(5, width-nameWidth-7)
	row := func(name, kind, required string) string {
		name = ansi.Truncate(name, nameWidth, "…")
		kind = ansi.Truncate(kind, typeWidth, "…")
		return name + strings.Repeat(" ", max(0, nameWidth-lipgloss.Width(name))) + " " + kind + strings.Repeat(" ", max(0, typeWidth-lipgloss.Width(kind))) + " " + required
	}
	lines := []string{row("Field", "Type", "Req.")}
	for _, name := range names {
		field, _ := props[name].(map[string]any)
		kind := "schema"
		if value, ok := field["type"]; ok {
			kind = schemaValue(value)
		}
		if kind == "array" {
			if items, ok := field["items"].(map[string]any); ok {
				if t, ok := items["type"].(string); ok {
					kind = "[]" + t
				}
			}
		}
		must := "no"
		if required[name] {
			must = "yes"
		}
		lines = append(lines, row(displayText(name), kind, must))
		if desc, ok := field["description"].(string); ok && desc != "" {
			lines = append(lines, "  "+displayText(desc))
		}
	}
	return strings.Join(lines, "\n")
}

func schemaValue(value any) string {
	if s, ok := value.(string); ok {
		return displayText(s)
	}
	b, _ := json.Marshal(value)
	return displayText(string(b))
}

// schemaChanges compares JSON structure, including required arrays and nested
// constraints. Paths are JSON pointers; this is not semantic compatibility analysis.
func schemaChanges(path string, before, after any) []string {
	if reflect.DeepEqual(before, after) {
		return nil
	}
	a, aok := before.(map[string]any)
	b, bok := after.(map[string]any)
	if aok && bok {
		keys := map[string]bool{}
		for k := range a {
			keys[k] = true
		}
		for k := range b {
			keys[k] = true
		}
		names := make([]string, 0, len(keys))
		for k := range keys {
			names = append(names, k)
		}
		sort.Strings(names)
		var result []string
		for _, k := range names {
			p := path + "/" + strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
			av, hasA := a[k]
			bv, hasB := b[k]
			switch {
			case !hasA:
				result = append(result, "+ "+displayText(p)+": "+schemaValue(bv))
			case !hasB:
				result = append(result, "− "+displayText(p)+": "+schemaValue(av))
			default:
				result = append(result, schemaChanges(p, av, bv)...)
			}
		}
		return result
	}
	if path == "" {
		path = "/"
	}
	return []string{"~ " + displayText(path) + ": " + schemaValue(before) + " → " + schemaValue(after)}
}

func (b *typeBrowser) update(msg tea.Msg, s memorytree.Snapshot, width, height int) {
	left, _ := memoryWidths(width)
	selectRow := func(i int) {
		if len(b.rows) > 0 {
			b.selected = typeKey(b.rows[max(0, min(i, len(b.rows)-1))])
			b.detail.GotoTop()
		}
	}
	switch v := msg.(type) {
	case tea.MouseClickMsg:
		if v.Button != tea.MouseLeft {
			return
		}
		b.detailsFocused = v.X > left
		if !b.detailsFocused && v.Y >= 2 && v.Y < height-2 && b.offset+v.Y-2 < len(b.rows) {
			selectRow(b.offset + v.Y - 2)
		}
	case tea.MouseWheelMsg:
		if v.X > left {
			b.detail, _ = b.detail.Update(msg)
		} else if v.Button == tea.MouseWheelDown {
			selectRow(b.index() + 1)
		} else {
			selectRow(b.index() - 1)
		}
	case tea.KeyPressMsg:
		switch v.String() {
		case "tab", "shift+tab":
			b.detailsFocused = !b.detailsFocused
		case "down", "j":
			if b.detailsFocused {
				b.detail, _ = b.detail.Update(msg)
			} else {
				selectRow(b.index() + 1)
			}
		case "up", "k":
			if b.detailsFocused {
				b.detail, _ = b.detail.Update(msg)
			} else {
				selectRow(b.index() - 1)
			}
		default:
			b.detail, _ = b.detail.Update(msg)
		}
	}
	b.refresh(s, width, height)
}

func (m *model) showTypeRecords() {
	if len(m.types.rows) == 0 {
		return
	}
	typ := m.types.rows[m.types.index()]
	m.memory.filterType, m.memory.filterVersion = typ.Name, typ.Version
	m.memory.archived = true
	m.memory.offset = 0
	m.memory.detailsFocused = false
	m.memory.refresh(m.width, m.height)
	m.memory.detail.GotoTop()
	m.typesMode, m.memoryMode = false, true
}

func (m *model) typesView() tea.View {
	left, right := memoryWidths(m.width)
	height := max(1, m.height-4)
	lines := []string{}
	for i := m.types.offset; i < min(len(m.types.rows), m.types.offset+height); i++ {
		typ := m.types.rows[i]
		active, archived := typeCounts(m.memory.snapshot, typ)
		label := fmt.Sprintf("%s  · %d", displayText(typeKey(typ)), active+archived)
		style := lipgloss.NewStyle().Width(max(1, left-4)).MaxWidth(max(1, left-4)).MaxHeight(1)
		if typeKey(typ) == m.types.selected {
			style = style.Background(lipgloss.Color("#294457")).Foreground(lipgloss.Color("#E8FFFF"))
		}
		lines = append(lines, style.Render(label))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, renderPane(fmt.Sprintf("Types · %d versions", len(m.types.rows)), strings.Join(lines, "\n"), left, !m.types.detailsFocused), " ", renderPane("Schema", m.types.detail.View(), right, m.types.detailsFocused))
	footer := " t activity · m memory · Enter records · Tab focus · q quit"
	if m.quitting {
		footer = " Waiting for active turn · Shift-Q quit immediately"
	}
	content := m.navigation() + "\n" + body + "\n" + footer
	if m.width < 50 || m.height < 12 {
		content = "Types · enlarge terminal to 50×12\nt activity · m memory · q quit"
		if m.quitting {
			content = "Finishing active turn…\nShift-Q quit now"
		}
	}
	v := tea.NewView(lipgloss.NewStyle().MaxWidth(max(1, m.width)).MaxHeight(max(1, m.height)).Render(content))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}
