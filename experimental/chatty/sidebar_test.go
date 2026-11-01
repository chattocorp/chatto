package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestSidebarPersistenceAcrossRestartAndImmediateQuit(t *testing.T) {
	m := groupedModel()
	m.e.path = filepath.Join(t.TempDir(), "session.json")
	m.e.state.Session.AccessToken = "synthetic-token"
	oldSave := m.activateSidebar(0) // collapse Team
	newSave := m.activateSidebar(0) // expand Team again
	if result := newSave().(sidebarSaved); result.err != nil {
		t.Fatal(result.err)
	}
	if result := oldSave().(sidebarSaved); result.err != nil {
		t.Fatal(result.err)
	}
	// Quit before the next asynchronous save runs. close must flush its state.
	m.sidebarKey = "dms"
	m.foldSidebar(true)
	m.e.close()
	data, err := os.ReadFile(m.e.path)
	if err != nil {
		t.Fatal(err)
	}
	e := &engine{ctx: context.Background()}
	if err := json.Unmarshal(data, &e.state); err != nil {
		t.Fatal(err)
	}
	restored := newModel(e, "")
	if restored.collapsed["group:team"] || !restored.collapsed["dms"] {
		t.Fatal("restart lost section states or an older save won")
	}
	restored.collapsed["dms"] = false
	if !e.state.CollapsedSections["dms"] {
		t.Fatal("UI shares the persisted map")
	}
}

func groupedModel() *model {
	m := exampleModel()
	m.rooms = []chatto.RoomEntry{
		{Room: chatto.Room{ID: "a", Name: "alpha", Kind: "ROOM_KIND_CHANNEL"}},
		{Room: chatto.Room{ID: "b", Name: "beta", Kind: "ROOM_KIND_CHANNEL"}},
		{Room: chatto.Room{ID: "c", Name: "other", Kind: "ROOM_KIND_CHANNEL"}},
		{Room: chatto.Room{ID: "dm", Name: "Friends", Kind: "ROOM_KIND_DM"}},
	}
	m.rooms[1].ViewerState.HasUnread = true
	m.groups = []chatto.RoomGroup{{ID: "team", Name: "Team", Items: []chatto.RoomGroupItem{
		{Room: &m.rooms[1]}, {SidebarLink: &chatto.SidebarLink{ID: "link", Label: "Website"}},
		{Room: &m.rooms[0]}, {Room: &m.rooms[1]}, {Room: &chatto.RoomEntry{Room: chatto.Room{ID: "no-access"}}},
	}}}
	m.room = "a"
	m.roomIndex = 0
	m.sidebarKey = "room:a"
	m.syncSidebar()
	return m
}

func rowKeys(m *model) []string {
	var keys []string
	for _, r := range m.sidebarRows() {
		keys = append(keys, r.key)
	}
	return keys
}

func TestSidebarUsesServerOrderAndKeepsDMsSeparate(t *testing.T) {
	m := groupedModel()
	want := []string{"group:team", "room:b", "room:a", "rooms", "room:c", "dms", "room:dm"}
	if !reflect.DeepEqual(rowKeys(m), want) {
		t.Fatal(rowKeys(m))
	}
	if row := m.sidebarRows()[0]; row.unread != 1 {
		t.Fatal("wrong section summary")
	}
	if label := m.roomName(m.rooms[3]); strings.HasPrefix(label, "#") {
		t.Fatal("DM styled as channel")
	}
	m.groups = nil
	if want := []string{"rooms", "room:a", "room:b", "room:c", "dms", "room:dm"}; !reflect.DeepEqual(rowKeys(m), want) {
		t.Fatal("fallback omitted rooms", rowKeys(m))
	}
}

func TestCollapseKeepsConversationDraftAndSurvivesRefresh(t *testing.T) {
	m := groupedModel()
	m.root = "thread"
	m.compose.SetValue("unfinished reply")
	m.activateSidebar(0)
	if m.room != "a" || m.root != "thread" || m.compose.Value() != "unfinished reply" || m.sidebarKey != "group:team" {
		t.Fatal("collapse changed conversation")
	}
	if got := rowKeys(m); !reflect.DeepEqual(got, []string{"group:team", "rooms", "room:c", "dms", "room:dm"}) {
		t.Fatal(got)
	}
	m.groups[0].Name = "Renamed group"
	m.syncSidebar()
	if !m.collapsed["group:team"] || m.sidebarKey != "group:team" {
		t.Fatal("refresh lost collapse/focus identity")
	}
	m.focus = 0
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
	if m.collapsed["group:team"] {
		t.Fatal("right did not expand")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	if !m.collapsed["group:team"] {
		t.Fatal("left did not collapse")
	}
}

func TestSidebarMouseHeadersRoomsAndScrolledHitTargets(t *testing.T) {
	m := groupedModel()
	m.busy = true // navigation must not start network work here
	m.Update(tea.MouseClickMsg{X: 3, Y: 2, Button: tea.MouseLeft})
	if !m.collapsed["group:team"] {
		t.Fatal("header click did not collapse")
	}
	// The next row is now Other rooms, then its channel.
	m.Update(tea.MouseClickMsg{X: 3, Y: 4, Button: tea.MouseLeft})
	if m.room != "c" {
		t.Fatal("collapsed rows broke mouse selection")
	}
	m.height = 12
	m.sidebarTop = 2
	m.Update(tea.MouseClickMsg{X: 3, Y: 3, Button: tea.MouseLeft})
	if m.sidebarKey != "dms" || !m.collapsed["dms"] {
		t.Fatal("scrolled header hit target is wrong")
	}
}

func TestSidebarClipsLongHeadersAndReportsHiddenUnread(t *testing.T) {
	m := groupedModel()
	m.groups[0].Name = strings.Repeat("Long group ", 20)
	m.activateSidebar(0)
	text := m.sidebarContent(18, 8)
	if lipgloss.Width(text) > 18 || !strings.Contains(text, "▸") || !strings.Contains(text, "●") || strings.Contains(ansi.Strip(text), "2") {
		t.Fatal("section summary overflowed or lost unread state")
	}
}

func TestSidebarSeparatesUnreadDotsAndNotificationAttention(t *testing.T) {
	m := exampleModel()
	m.rooms[0].ViewerState.HasUnread = true
	m.notifications = map[string]chatto.NotificationCount{"room": {Unread: 7, Important: 2}}
	for _, width := range []int{15, 32} {
		text := m.sidebarContent(width, 10)
		if lipgloss.Width(text) > width {
			t.Fatal("badges overflow sidebar")
		}
		line := strings.Split(text, "\n")[1]
		fields := strings.Fields(ansi.Strip(line))
		if fields[0] != "●" || fields[len(fields)-2] != "5" || fields[len(fields)-1] != "2" {
			t.Fatal("dot or exact attention counts missing")
		}
		buf := uv.NewScreenBuffer(width, 1)
		uv.NewStyledString(line).Draw(buf, buf.Bounds())
		white, orange := false, false
		for x := 0; x < width; x++ {
			cell := buf.CellAt(x, 0)
			if cell == nil || cell.Style.Bg == nil {
				continue
			}
			r, g, b, _ := cell.Style.Bg.RGBA()
			if cell.Content == "5" && r>>8 == 0xf8 && g>>8 == 0xfa && b>>8 == 0xfc {
				white = true
			}
			if cell.Content == "2" && r>>8 == 0xf5 && g>>8 == 0x9e && b>>8 == 0x0b {
				orange = true
			}
		}
		if !white || !orange {
			t.Fatal("selection overwrote badge colors")
		}
	}
	m.notifications = nil
	if strings.Contains(ansi.Strip(m.sidebarContent(32, 10)), "5") {
		t.Fatal("cleared notification badge remained")
	}
}
