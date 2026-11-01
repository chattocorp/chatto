package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

type animationTick struct{ generation int }
type changed struct{}
type sidebarSaved struct{ err error }
type loaded struct {
	Generation int
	Snapshot   snapshot
}
type rowRange struct {
	Start, End int
	Images     []imageHit
	Meta       []metaHit
}

// Match message identity as well as screen position: live updates can move rows
// between clicks. Release events must not clear the first click.
type messageClick struct {
	room, root, id         string
	pane, x, y, generation int
	at                     time.Time
}

// A scroll anchor keeps the same message line visible when older rows are prepended.
type scrollAnchor struct {
	id   string
	line int
}

func captureAnchor(p chatto.TimelinePage, rows []rowRange, offset int) scrollAnchor {
	for i, row := range rows {
		if row.End > offset {
			return scrollAnchor{p.Messages[i].ID, max(0, offset-row.Start)}
		}
	}
	return scrollAnchor{}
}
func restoreAnchor(v *viewport.Model, p chatto.TimelinePage, rows []rowRange, a scrollAnchor) {
	for i, msg := range p.Messages {
		if msg.ID == a.id {
			v.SetYOffset(max(0, rows[i].Start+a.line))
			return
		}
	}
}

type model struct {
	images                              imageCache
	quitting                            bool
	e                                   *engine
	width, height                       int
	logged, busy                        bool
	status                              string
	serverName                          string
	fields                              [3]textinput.Model
	field                               int
	compose                             textinput.Model
	focus                               int // Navigation target: 0 rooms, 1 timeline, 2 thread. Input is always active.
	rooms                               []chatto.RoomEntry
	groups                              []chatto.RoomGroup
	notifications                       map[string]chatto.NotificationCount
	collapsed                           map[string]bool
	sidebarKey                          string
	sidebarTop                          int
	sidebarHidden                       bool
	roomIndex, selected, threadSelected int
	room, root                          string
	page, thread                        chatto.TimelinePage
	users                               map[string]chatto.Profile
	viewer                              chatto.Profile
	main, side                          viewport.Model
	mainRows, sideRows                  []rowRange
	generation                          int
	drafts                              map[string]string
	pending                             bool
	history, dirty                      bool
	sentDraftKey                        string
	sentBody                            string
	loginCancel                         context.CancelFunc
	sidebarWidth, threadWidth           int
	dragSeparator                       int // 1 sidebar, 2 thread; zero when not dragging
	animationFrame                      int
	animationGeneration                 int
	loadingOlder                        bool
	loadingRoot                         string
	loadingGeneration                   int
	lastMessageClick                    messageClick
}

func newModel(e *engine, origin string) *model {
	m := &model{e: e, width: 100, height: 32, users: map[string]chatto.Profile{}, drafts: map[string]string{}, logged: e.state.Session.AccessToken != "", status: "Ready"}
	m.collapsed = make(map[string]bool, len(e.state.CollapsedSections))
	for key, value := range e.state.CollapsedSections {
		m.collapsed[key] = value
	}
	for i := range m.fields {
		m.fields[i] = textinput.New()
		m.fields[i].SetWidth(50)
		m.fields[i].CharLimit = 2000
	}
	m.fields[0].Placeholder = "https://chat.example.com"
	m.fields[0].SetValue(origin)
	m.fields[1].Placeholder = "Username"
	m.fields[2].Placeholder = "Password"
	m.fields[2].EchoMode = textinput.EchoPassword
	m.fields[0].Focus()
	m.compose = textinput.New()
	m.compose.Focus()
	m.focus = 1
	m.compose.Prompt = ""
	m.compose.Placeholder = "Write a message…"
	m.compose.CharLimit = 10000
	m.main = viewport.New()
	m.side = viewport.New()
	return m
}
func animate(generation int) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return animationTick{generation} })
}
func (m *model) spinner() string {
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	return string(frames[m.animationFrame%len(frames)])
}
func (m *model) watch() tea.Cmd {
	return func() tea.Msg {
		select {
		case <-m.e.events:
			return changed{}
		case <-m.e.ctx.Done():
			return nil
		}
	}
}
func (m *model) Init() tea.Cmd {
	if m.logged {
		return tea.Batch(m.reload(""), textinput.Blink, m.watch())
	}
	return tea.Batch(textinput.Blink, m.watch())
}
func (m *model) reload(before string) tea.Cmd {
	return m.reloadTimeline(before, m.root)
}
func (m *model) reloadTimeline(before, root string) tea.Cmd {
	if m.busy {
		return nil
	}
	m.busy = true
	m.loadingOlder, m.loadingRoot, m.loadingGeneration = before != "", root, m.generation
	m.animationGeneration++
	m.history = before != ""
	m.dirty = false
	g, room := m.generation, m.room
	m.status = "Loading room…"
	if before != "" {
		m.status = "Scrollback · Alt+R returns to live messages"
	}
	return tea.Batch(animate(m.animationGeneration), func() tea.Msg { return loaded{g, m.e.load(room, root, before)} })
}

// timelinePane binds the shared navigation behavior to either conversation pane.
type timelinePane struct {
	view     *viewport.Model
	page     *chatto.TimelinePage
	selected *int
	rows     *[]rowRange
	root     string
}

func (m *model) timeline(thread bool) timelinePane {
	if thread {
		return timelinePane{&m.side, &m.thread, &m.threadSelected, &m.sideRows, m.root}
	}
	return timelinePane{&m.main, &m.page, &m.selected, &m.mainRows, ""}
}

// loadOlder is the single pagination gate for mouse, page keys, and cursor keys.
func (m *model) loadOlder(p timelinePane) tea.Cmd {
	if m.busy || !p.page.HasOlder || p.page.StartCursor == "" {
		return nil
	}
	return m.reloadTimeline(p.page.StartCursor, p.root)
}

func (m *model) moveTimeline(thread bool, delta int) tea.Cmd {
	p := m.timeline(thread)
	*p.selected = max(0, min(len(p.page.Messages)-1, *p.selected+delta))
	m.renderMessages()
	revealTimelineSelection(p)
	if delta < 0 {
		m.history = true
		m.status = "Scrollback · Alt+R returns to live messages"
		if *p.selected == 0 {
			return m.loadOlder(p)
		}
	}
	return nil
}

// Scrollback remains stable across ordinary live events. Access invalidations
// still clear the view immediately; Alt+R explicitly returns to live data.
func (m *model) scrollTimeline(thread bool, delta int) tea.Cmd {
	p := m.timeline(thread)
	v := p.view
	if delta < 0 {
		m.history = true
		v.ScrollUp(-delta)
		m.status = "Scrollback · Alt+R returns to live messages"
		if v.YOffset() == 0 {
			return m.loadOlder(p)
		}
	} else {
		v.ScrollDown(delta)
		if m.main.AtBottom() && (m.root == "" || m.side.AtBottom()) {
			m.history = false
			if !m.busy {
				m.status = "Latest messages"
			}
		}
	}
	return nil
}
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || r == 0x7f || r == '\U0010EEEE' {
			return -1
		}
		return r
	}, s)
}
func oneLine(s string) string { return strings.Join(strings.Fields(clean(s)), " ") }

// composeRoot follows navigation focus; an open thread alone is not a destination.
func (m *model) composeRoot() string {
	if m.focus == 2 {
		return m.root
	}
	return ""
}
func (m *model) draftKey() string { return m.room + "/" + m.composeRoot() }
func (m *model) saveDraft()       { m.drafts[m.draftKey()] = m.compose.Value() }
func (m *model) restoreDraft()    { m.compose.SetValue(m.drafts[m.draftKey()]) }
func (m *model) roomName(r chatto.RoomEntry) string {
	if r.Room.Name != "" {
		if r.Room.Kind == "ROOM_KIND_DM" {
			return oneLine(r.Room.Name)
		}
		return "#" + oneLine(r.Room.Name)
	}
	names := []string{}
	for _, id := range r.MemberUserIDs {
		if id != m.viewer.ID {
			names = append(names, m.userName(id))
		}
	}
	if len(names) == 0 {
		return "Saved messages"
	}
	return strings.Join(names, ", ")
}
func (m *model) userName(id string) string {
	if u, ok := m.users[id]; ok && u.Login != "" {
		return "@" + oneLine(u.Login)
	}
	return "Unknown user"
}
func (m *model) setFocus(f int) tea.Cmd {
	if f == 0 && m.sidebarHidden {
		f = 1
		if _, center, _ := m.columns(); center == 0 {
			f = 2
		}
	}
	m.saveDraft()
	m.focus = f
	m.restoreDraft()
	return m.compose.Focus()
}
func (m *model) chooseRoom(i int) tea.Cmd {
	if i < 0 || i >= len(m.rooms) {
		return nil
	}
	m.saveDraft()
	m.roomIndex = i
	m.room = m.rooms[i].Room.ID
	m.sidebarKey = "room:" + m.room
	m.syncSidebar()
	m.root = ""
	m.page = chatto.TimelinePage{}
	m.thread = chatto.TimelinePage{}
	m.selected = 0
	m.generation++
	m.restoreDraft()
	m.renderMessages()
	return m.reload("")
}
func (m *model) openThread() tea.Cmd {
	if len(m.page.Messages) == 0 {
		return nil
	}
	return m.openThreadRoot(m.page.Messages[m.selected].ThreadRootID())
}

func (m *model) openThreadRoot(root string) tea.Cmd {
	m.saveDraft()
	m.root = root
	m.thread = chatto.TimelinePage{}
	m.threadSelected = 0
	m.focus = 2
	m.generation++
	m.restoreDraft()
	m.renderMessages()
	return m.reload("")
}
func (m *model) closeThread() tea.Cmd {
	m.saveDraft()
	m.root = ""
	m.focus = 1
	m.generation++
	m.restoreDraft()
	m.renderMessages()
	return m.reload("")
}

func keepSelection(old, fresh []chatto.Message, index int) int {
	if len(fresh) == 0 {
		return 0
	}
	if len(old) == 0 {
		return len(fresh) - 1
	}
	if index >= 0 && index < len(old) {
		for i, msg := range fresh {
			if msg.ID == old[index].ID {
				return i
			}
		}
	}
	return min(index, len(fresh)-1)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	result, cmd := m.update(msg)
	var images tea.Cmd
	if !m.quitting {
		images = m.syncImages()
	}
	m.renderImageViewer()
	if m.images.raw.Len() > 0 {
		packet := m.images.raw.String()
		m.images.raw.Reset()
		// Keep each upload and its placements together in one renderer write.
		cmd = tea.Sequence(tea.Raw(packet), cmd)
	}
	return result, tea.Batch(cmd, images)
}

func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyPressMsg, tea.MouseWheelMsg, tea.WindowSizeMsg:
		m.lastMessageClick = messageClick{}
	}
	switch v := msg.(type) {
	case notificationJump:
		return m, m.acceptNotificationJump(v)
	case imageLoaded:
		m.acceptImage(v)
		return m, nil
	case sidebarSaved:
		if v.err != nil {
			m.status = "Could not save sidebar preferences"
		}
		return m, nil
	case animationTick:
		if v.generation != m.animationGeneration {
			return m, nil
		}
		m.animationFrame++
		if m.busy {
			return m, animate(m.animationGeneration)
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
		m.layout()
		if m.images.enabled {
			return m, tea.Raw(ansi.WindowOp(ansi.RequestCellSizeWinOp))
		}
		return m, nil
	case uv.CellSizeEvent:
		if v.Width > 0 && v.Height > 0 && v.Width <= 1000 && v.Height <= 1000 {
			m.images.cellWidth, m.images.cellHeight = v.Width, v.Height
			m.renderMessages()
		}
		return m, nil
	case changed:
		var cmd tea.Cmd
		if m.e.invalidated.Swap(false) {
			m.notifications = nil
			m.images.clear()
			m.history = false
			m.generation++
			m.page = chatto.TimelinePage{}
			m.thread = chatto.TimelinePage{}
			m.rooms = nil
			m.groups = nil
			m.users = map[string]chatto.Profile{}
			m.renderMessages()
		}
		if m.logged {
			if m.busy {
				m.dirty = true
			} else {
				r := m.e.current(m.room, m.root)
				if r.Ready && (r.HasTimeline || len(r.Rooms) == 0) {
					_, cmd = m.Update(loaded{Generation: m.generation, Snapshot: r})
				} else if r.Ready {
					m.page, m.thread = chatto.TimelinePage{}, chatto.TimelinePage{}
					m.renderMessages()
					cmd = m.reload("")
				} else if r.Err != nil {
					m.status = clean(r.Err.Error())
				} else {
					m.status = "Synchronizing server…"
				}
			}
		}
		return m, tea.Batch(m.watch(), cmd)
	case loaded:
		m.busy = false
		m.loadingOlder = false
		if v.Generation != m.generation {
			return m, m.reload("")
		}
		r := v.Snapshot
		if r.ServerGeneration != 0 && r.ServerGeneration != m.e.current(m.room, m.root).ServerGeneration {
			return m, m.reload("")
		}
		m.notifications = r.Notifications
		if name := oneLine(r.ServerName); name != "" {
			m.serverName = name
		}
		m.pending = r.Pending != nil
		if r.Err != nil {
			m.status = clean(r.Err.Error())
			if r.Before != "" {
				return m, nil
			}
			m.page = chatto.TimelinePage{}
			m.thread = chatto.TimelinePage{}
			m.rooms = r.Rooms
			m.groups = r.Groups
			m.syncSidebar()
			m.users = map[string]chatto.Profile{}
			m.renderMessages()
			return m, nil
		}
		if r.Before == "" {
			m.viewer = r.Viewer
			m.rooms = r.Rooms
			m.groups = r.Groups
			m.users = r.Users
		} else {
			if m.users == nil {
				m.users = map[string]chatto.Profile{}
			}
			for id, user := range r.Users {
				m.users[id] = user
			}
		}
		if r.Room != m.room {
			m.room = r.Room
			m.root = r.Root
			m.restoreDraft()
		}
		m.roomIndex = 0
		for i, room := range m.rooms {
			if room.Room.ID == m.room {
				m.roomIndex = i
			}
		}
		m.syncSidebar()
		p := r.Page
		mainAnchor := captureAnchor(m.page, m.mainRows, m.main.YOffset())
		sideAnchor := captureAnchor(m.thread, m.sideRows, m.side.YOffset())
		// Follow each pane independently. Fetching older history must preserve
		// its anchor even when the previous page fitted entirely on screen.
		followMain := (len(m.page.Messages) == 0 || m.main.AtBottom()) && !(r.Before != "" && r.Root == "")
		followSide := (len(m.thread.Messages) == 0 || m.side.AtBottom()) && !(r.Before != "" && r.Root != "")
		selectMainLast := followMain && m.selected >= len(m.page.Messages)-1
		selectSideLast := followSide && m.threadSelected >= len(m.thread.Messages)-1
		if m.root == "" || (r.Before != "" && r.Root == "") {
			m.selected = keepSelection(m.page.Messages, p.Messages, m.selected)
			m.page = p
		} else {
			if r.Before == "" {
				m.selected = keepSelection(m.page.Messages, r.RoomPage.Messages, m.selected)
				m.page = r.RoomPage
			}
			m.threadSelected = keepSelection(m.thread.Messages, p.Messages, m.threadSelected)
			m.thread = p
		}
		m.status = "Connected · live updates"
		if !r.Live {
			m.status = "Disconnected · reconnecting"
		}
		if m.history {
			m.status = "Scrollback · Alt+R returns to live messages"
		}
		if m.pending {
			m.status = "Unconfirmed send retained · Ctrl+R retry exactly · Alt+D discard local attempt"
		}
		if selectMainLast {
			m.selected = max(0, len(m.page.Messages)-1)
		}
		if selectSideLast {
			m.threadSelected = max(0, len(m.thread.Messages)-1)
		}
		m.renderMessages()
		if followMain {
			m.main.GotoBottom()
		} else {
			restoreAnchor(&m.main, m.page, m.mainRows, mainAnchor)
		}
		if followSide {
			m.side.GotoBottom()
		} else {
			restoreAnchor(&m.side, m.thread, m.sideRows, sideAnchor)
		}
		if m.dirty {
			m.dirty = false
			current := m.e.current(m.room, m.root)
			if current.Ready && current.HasTimeline {
				g := m.generation
				return m, func() tea.Msg { return loaded{Generation: g, Snapshot: current} }
			}
		}
		return m, nil
	case actionDone:
		m.busy = false
		if v.Kind == "login" && m.loginCancel != nil {
			m.loginCancel()
			m.loginCancel = nil
		}
		if v.Err != nil {
			m.status = clean(v.Err.Error())
			if v.Kind == "send" {
				m.pending = true
			}
			return m, nil
		}
		switch v.Kind {
		case "login":
			m.logged = true
			m.fields[2].Reset()
			m.focus = 1
			m.status = "Signed in"
			return m, m.reload("")
		case "logout":
			m.notifications = nil
			m.images.clear()
			m.serverName = ""
			m.logged = false
			m.rooms = nil
			m.groups = nil
			m.collapsed = nil
			m.sidebarKey = ""
			m.sidebarTop = 0
			m.page = chatto.TimelinePage{}
			m.thread = chatto.TimelinePage{}
			m.users = map[string]chatto.Profile{}
			m.compose.Reset()
			m.drafts = map[string]string{}
			m.room = ""
			m.root = ""
			m.status = "Signed out"
			m.fields[0].Focus()
			return m, nil
		case "send":
			if m.sentDraftKey != "" {
				if m.drafts[m.sentDraftKey] == m.sentBody {
					delete(m.drafts, m.sentDraftKey)
				}
				if m.draftKey() == m.sentDraftKey && m.compose.Value() == m.sentBody {
					m.compose.Reset()
				}
			}
			m.pending = false
		case "discard":
			m.pending = false
			m.status = "Local attempt discarded; a delivered message is not deleted"
		}
		return m, m.reload("")
	case tea.KeyPressMsg:
		key := v.String()
		if key == "ctrl+d" {
			m.quitting = true
			m.images.clear()
			return m, tea.Quit
		}
		if key == "ctrl+c" {
			return m, nil
		}
		if m.images.expanded != "" {
			if key == "esc" || key == "alt+esc" || key == "alt+escape" || key == "alt+v" || key == "ctrl+o" {
				m.images.expanded = ""
			}
			return m, nil
		}
		if !m.logged {
			return m, m.loginKey(v)
		}
		if key == "ctrl+b" {
			main := captureAnchor(m.page, m.mainRows, m.main.YOffset())
			side := captureAnchor(m.thread, m.sideRows, m.side.YOffset())
			followMain, followSide := m.main.AtBottom(), m.side.AtBottom()
			m.sidebarHidden = !m.sidebarHidden
			m.dragSeparator = 0
			var cmd tea.Cmd
			if m.sidebarHidden && m.focus == 0 {
				cmd = m.setFocus(0)
			}
			m.layout()
			if followMain {
				m.main.GotoBottom()
			} else {
				restoreAnchor(&m.main, m.page, m.mainRows, main)
			}
			if followSide {
				m.side.GotoBottom()
			} else {
				restoreAnchor(&m.side, m.thread, m.sideRows, side)
			}
			return m, cmd
		}
		if key == "ctrl+l" && !m.busy {
			m.busy = true
			return m, func() tea.Msg { return m.e.logout() }
		}
		if key == "ctrl+r" && m.pending && !m.busy {
			m.busy = true
			m.sentDraftKey = ""
			return m, func() tea.Msg { return m.e.send("", "", "", true) }
		}
		if key == "alt+d" && m.pending && !m.busy {
			m.busy = true
			return m, func() tea.Msg { return m.e.discard() }
		}
		if key == "tab" || key == "shift+tab" || key == "ctrl+n" {
			panes := []int{}
			left, center, right := m.columns()
			if left > 0 {
				panes = append(panes, 0)
			}
			if center > 0 {
				panes = append(panes, 1)
			}
			if right > 0 {
				panes = append(panes, 2)
			}
			index := 0
			for i, f := range panes {
				if f == m.focus {
					index = i
				}
			}
			delta := 1
			if key == "shift+tab" {
				delta = -1
			}
			return m, m.setFocus(panes[(index+delta+len(panes))%len(panes)])
		}
		if key == "esc" || key == "alt+escape" || key == "alt+esc" {
			if m.root != "" {
				return m, m.closeThread()
			}
			return m, m.setFocus(0)
		}
		if key == "alt+r" {
			if m.busy {
				return m, nil
			}
			m.e.releaseHistory(m.room, m.root)
			m.page, m.thread = chatto.TimelinePage{}, chatto.TimelinePage{}
			m.renderMessages()
			return m, m.reload("")
		}
		if key == "alt+v" || key == "ctrl+o" {
			m.toggleImage()
			return m, nil
		}
		if key == "alt+enter" || (key == "enter" && strings.TrimSpace(m.compose.Value()) != "") {
			if !m.busy && m.room != "" && strings.TrimSpace(m.compose.Value()) != "" {
				root := m.composeRoot()
				// Sending returns its destination to live history. Subsequent
				// realtime delivery follows it unless the user scrolls back again.
				m.history = false
				if root == "" {
					m.selected = max(0, len(m.page.Messages)-1)
				} else {
					m.threadSelected = max(0, len(m.thread.Messages)-1)
				}
				m.renderMessages()
				if root == "" {
					m.main.GotoBottom()
				} else {
					m.side.GotoBottom()
				}
				m.busy = true
				room, body := m.room, m.compose.Value()
				m.sentDraftKey = m.draftKey()
				m.sentBody = body
				m.status = "Sending…"
				return m, func() tea.Msg { return m.e.send(room, root, body, false) }
			}
			return m, nil
		}
		if m.focus == 0 {
			switch key {
			case "enter":
				return m, m.activateSidebar(m.sidebarIndex(m.sidebarRows()))
			case "left", "alt+left":
				return m, m.foldSidebar(true)
			case "right", "alt+right":
				return m, m.foldSidebar(false)
			}
		}
		if key == "enter" && m.focus == 1 {
			return m, m.openThread()
		}
		if key == "alt+pgup" {
			return m, m.scrollTimeline(m.focus == 2, -m.bodyHeight())
		}
		if key == "alt+pgdown" {
			return m, m.scrollTimeline(m.focus == 2, m.bodyHeight())
		}
		delta := 0
		if key == "up" || key == "alt+up" {
			delta = -1
		}
		if key == "down" || key == "alt+down" {
			delta = 1
		}
		if delta != 0 {
			if m.focus == 0 {
				return m, m.moveSidebar(delta)
			}
			return m, m.moveTimeline(m.focus == 2, delta)
		}
		var cmd tea.Cmd
		m.compose, cmd = m.compose.Update(v)
		return m, cmd
	case tea.MouseMotionMsg:
		if m.dragSeparator != 0 {
			left, _, _ := m.columns()
			if m.dragSeparator == 1 {
				m.sidebarWidth = max(18, min(m.width-30, v.X+1))
			} else {
				m.threadWidth = max(24, min(m.width-left-24, m.width-v.X-1))
			}
			m.layout()
		}
		return m, nil
	case tea.MouseReleaseMsg:
		m.dragSeparator = 0
		return m, nil
	case tea.MouseClickMsg:
		previousClick := m.lastMessageClick
		m.lastMessageClick = messageClick{}
		if !m.logged || v.Button != tea.MouseLeft {
			return m, nil
		}
		if m.images.expanded != "" {
			m.images.expanded = ""
			return m, nil
		}
		left, center, _ := m.columns()
		if v.Y == m.height-2 {
			return m, m.compose.Focus()
		}
		if v.Y < 2 || v.Y >= m.height-3 {
			return m, nil
		}
		if left > 0 && v.X == left-1 {
			m.dragSeparator = 1
			return m, nil
		}
		if m.root != "" && center > 0 && v.X == left+center-1 {
			m.dragSeparator = 2
			return m, nil
		}
		if v.X < left {
			i := v.Y - 2 + m.sidebarTop
			if i < len(m.sidebarRows()) {
				row := m.sidebarRows()[i]
				if !row.header {
					room := m.rooms[row.roomIndex].Room.ID
					count := m.notifications[room]
					if important, hit := badgeAt(v.X-1, left-3, count.Unread, count.Important); hit {
						return m, m.jumpNotification(room, important)
					}
				}
				m.setFocus(0)
				return m, m.activateSidebar(i)
			}
			return m, nil
		}
		rows, offset := m.mainRows, m.main.YOffset()
		paneX := left
		focus := 1
		if m.root != "" && v.X >= left+center {
			rows, offset = m.sideRows, m.side.YOffset()
			focus = 2
			paneX += center
		}
		m.setFocus(focus)
		y := v.Y - 2 + offset
		for i, row := range rows {
			if y >= row.Start && y < row.End {
				if m.focus == 1 {
					m.selected = i
				} else {
					m.threadSelected = i
				}
				for _, hit := range row.Images {
					x := v.X - paneX - 1 // terminalPane's left padding
					if x >= hit.x && x < hit.x+hit.width && y >= hit.y && y < hit.y+hit.height {
						m.openImage(hit.key, hit.title)
						break
					}
				}
				for _, hit := range row.Meta {
					x := v.X - paneX - 1
					if y == hit.y && x >= hit.x && x < hit.x+hit.width {
						return m, m.openThreadRoot(hit.root)
					}
				}
				var message chatto.Message
				if focus == 2 {
					message = m.thread.Messages[i]
				} else {
					message = m.page.Messages[i]
				}
				if m.images.expanded == "" && (message.Thread != nil || message.EchoFromThreadRootEventID != "") {
					click := messageClick{room: m.room, root: m.root, id: message.ID, pane: focus, x: v.X, y: v.Y, generation: m.generation, at: time.Now()}
					elapsed := click.at.Sub(previousClick.at)
					if previousClick.room == click.room && previousClick.root == click.root && previousClick.id == click.id &&
						previousClick.pane == click.pane && previousClick.generation == click.generation &&
						previousClick.x == click.x && previousClick.y == click.y && elapsed >= 0 && elapsed <= 400*time.Millisecond {
						return m, m.openThreadRoot(message.ThreadRootID())
					}
					m.lastMessageClick = click
				}
				m.renderMessages()
				break
			}
		}
		return m, nil
	case tea.MouseWheelMsg:
		if m.images.expanded != "" {
			return m, nil
		}
		left, center, _ := m.columns()
		if v.X < left {
			delta := 3
			if v.Button == tea.MouseWheelUp {
				delta = -3
			}
			m.sidebarTop = max(0, min(max(0, len(m.sidebarRows())-m.bodyHeight()), m.sidebarTop+delta))
			return m, nil
		}
		delta := 3
		if v.Button == tea.MouseWheelUp {
			delta = -3
		}
		return m, m.scrollTimeline(m.root != "" && v.X >= left+center, delta)
	}
	// Text inputs also need paste, clipboard-result, and cursor messages.
	if m.images.expanded != "" {
		return m, nil
	}
	var cmd tea.Cmd
	if !m.logged && !m.busy {
		m.fields[m.field], cmd = m.fields[m.field].Update(msg)
	} else if m.logged {
		m.compose, cmd = m.compose.Update(msg)
	}
	return m, cmd
}

func (m *model) loginKey(v tea.KeyPressMsg) tea.Cmd {
	if m.busy {
		if v.String() == "alt+esc" && m.loginCancel != nil {
			m.loginCancel()
		}
		return nil
	}
	switch v.String() {
	case "ctrl+n":
		m.fields[m.field].Blur()
		m.field = (m.field + 1) % 3
		return m.fields[m.field].Focus()
	case "ctrl+b", "alt+enter":
		browser := v.String() == "ctrl+b"
		origin, login, password := m.fields[0].Value(), m.fields[1].Value(), m.fields[2].Value()
		m.fields[2].Reset()
		ctx, cancel := context.WithTimeout(m.e.ctx, 5*time.Minute)
		m.loginCancel = cancel
		m.busy = true
		m.status = "Signing in…"
		if browser {
			m.status = "Finish sign-in in your browser · Alt+Esc cancels"
		}
		return func() tea.Msg {
			return m.e.login(ctx, origin, login, password, browser, func(u string) error {
				name := "xdg-open"
				args := []string{u}
				if runtime.GOOS == "darwin" {
					name = "open"
				}
				return exec.CommandContext(ctx, name, args...).Run()
			})
		}
	}
	var cmd tea.Cmd
	m.fields[m.field], cmd = m.fields[m.field].Update(v)
	return cmd
}

func (m *model) columns() (int, int, int) {
	left := min(28, max(18, m.width/5))
	if m.sidebarWidth > 0 {
		left = max(18, min(m.width-30, m.sidebarWidth))
	}
	if m.sidebarHidden {
		left = 0
	}
	remaining := max(1, m.width-left)
	if m.root == "" {
		return left, remaining, 0
	}
	if m.width < 105 {
		return left, 0, remaining
	}
	right := remaining - remaining/2
	if m.threadWidth > 0 {
		right = max(24, min(remaining-24, m.threadWidth))
	}
	return left, remaining - right, right
}
func (m *model) layout() {
	m.syncSidebar()
	for i := range m.fields {
		m.fields[i].SetWidth(max(10, min(50, m.width-14)))
	}
	_, center, right := m.columns()
	m.main.SetWidth(max(1, center-3))
	m.side.SetWidth(max(1, right-3))
	m.main.SetHeight(m.bodyHeight())
	m.side.SetHeight(m.bodyHeight())
	m.compose.SetWidth(max(1, m.width-ansi.StringWidth(m.inputPrompt())-1))
	m.renderMessages()
}

// bodyHeight and the two header rows define both rendering and mouse geometry.
func (m *model) bodyHeight() int { return max(1, m.height-5) }

func nickColor(id string) lipgloss.Style {
	palette := []string{"#7DD3FC", "#C4B5FD", "#F9A8D4", "#FCD34D", "#86EFAC", "#FDBA74"}
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return lipgloss.NewStyle().Foreground(lipgloss.Color(palette[int(h.Sum32())%len(palette)])).Bold(true)
}

func (m *model) renderPage(p chatto.TimelinePage, selected, width int) (string, []rowRange) {
	nickWidth := min(14, max(5, width/4))
	gutter := 6 + nickWidth + 3
	bodyWidth := max(1, width-gutter)
	style := styles.DarkStyleConfig
	zero := uint(0)
	style.Document.Margin = &zero
	style.Document.BlockPrefix, style.Document.BlockSuffix = "", ""
	renderer, _ := glamour.NewTermRenderer(glamour.WithStyles(style), glamour.WithWordWrap(bodyWidth))
	lines := []string{}
	rows := []rowRange{}
	lastDate := ""
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B"))
	for i, msg := range p.Messages {
		stamp := "--:--"
		if t, err := time.Parse(time.RFC3339Nano, msg.CreatedAt); err == nil {
			stamp = t.Local().Format("15:04")
			date := t.Local().Format("Mon, 02 Jan 2006")
			if date != lastDate {
				label := "── " + date + " "
				lines = append(lines, dim.Render(ansi.Truncate(label+strings.Repeat("─", max(0, width-ansi.StringWidth(label))), width, "")))
				lastDate = date
			}
		}
		nick := ansi.Truncate(strings.TrimPrefix(m.userName(msg.ActorID), "@"), nickWidth, "…")
		padding := strings.Repeat(" ", max(0, nickWidth-ansi.StringWidth(nick)))
		prefix := dim.Render(stamp+" ") + padding + nickColor(msg.ActorID).Render("<"+nick+"> ")
		body := clean(msg.Body)
		if body == "" {
			if len(msg.Attachments) == 0 {
				body = "[Non-text message]"
			} else {
				body = "Attachment"
			}
		}
		if renderer != nil {
			if rendered, err := renderer.Render(body); err == nil {
				body = strings.Trim(rendered, "\n")
			}
		}
		body = ansi.Hardwrap(body, bodyWidth, true)
		bodyLines := strings.Split(body, "\n")
		for len(bodyLines) > 1 && strings.TrimSpace(ansi.Strip(bodyLines[len(bodyLines)-1])) == "" {
			bodyLines = bodyLines[:len(bodyLines)-1]
		}
		start := len(lines)
		for j, part := range bodyLines {
			lead := strings.Repeat(" ", gutter)
			if j == 0 {
				lead = prefix
			}
			text := ansi.Truncate(lead+part, width, "")
			rowStyle := lipgloss.NewStyle().Width(width)
			if i == selected {
				// Strip nested backgrounds so selection covers every terminal cell.
				text = ansi.Strip(text)
				rowStyle = rowStyle.Background(lipgloss.Color("#193D48")).Foreground(lipgloss.Color("#E2E8F0"))
			}
			lines = append(lines, rowStyle.Render(text))
		}
		attachments, hits := m.attachmentLines(msg, width, gutter)
		for i := range hits {
			hits[i].y += len(lines)
		}
		for _, line := range attachments {
			style := lipgloss.NewStyle().Width(width)
			if i == selected {
				style = style.Background(lipgloss.Color("#193D48"))
			}
			lines = append(lines, style.Render(line))
		}
		metadata, metaHits := m.messageMeta(msg, width, gutter, i == selected)
		for j := range metaHits {
			metaHits[j].y += len(lines)
		}
		lines = append(lines, metadata...)
		rows = append(rows, rowRange{Start: start, End: len(lines), Images: hits, Meta: metaHits})
	}
	return strings.Join(lines, "\n"), rows
}
func (m *model) renderMessages() {
	_, center, right := m.columns()
	m.main.SetWidth(max(1, center-3))
	m.side.SetWidth(max(1, right-3))
	m.compose.SetWidth(max(1, m.width-ansi.StringWidth(m.inputPrompt())-1))
	text, rows := m.renderPage(m.page, m.selected, m.main.Width())
	m.mainRows = rows
	m.main.SetContent(text)
	text, rows = m.renderPage(m.thread, m.threadSelected, m.side.Width())
	m.sideRows = rows
	m.side.SetContent(text)
}
func (m *model) revealSelection() {
	revealTimelineSelection(m.timeline(m.focus == 2))
}

func revealTimelineSelection(p timelinePane) {
	v, rows, index := p.view, *p.rows, *p.selected
	if index >= 0 && index < len(rows) {
		r := rows[index]
		if r.Start < v.YOffset() {
			v.SetYOffset(r.Start)
		} else if r.End > v.YOffset()+v.Height() {
			v.SetYOffset(max(r.Start, r.End-v.Height()))
		}
	}
}
func panel(title, body string, width, height int, active bool) string {
	title = ansi.Truncate(oneLine(title), max(1, width-4), "…")
	color := "#30434B"
	if active {
		color = "#5EEAD4"
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(color)).Width(max(1, width-2)).Height(max(1, height-2)).Render(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5EEAD4")).Render(" "+title) + "\n" + body)
}

func terminalBar(text string, width int, active bool) string {
	bg, fg := "#203340", "#94A3B8"
	if active {
		bg, fg = "#285465", "#E0F2FE"
	}
	return lipgloss.NewStyle().Width(width).Background(lipgloss.Color(bg)).Foreground(lipgloss.Color(fg)).Bold(true).Render(" " + ansi.Truncate(oneLine(text), max(1, width-2), "…"))
}

type rowBackground func(int) string

func selectionBackground(rows []rowRange, selected, offset int) rowBackground {
	return func(y int) string {
		if selected >= 0 && selected < len(rows) && y+offset >= rows[selected].Start && y+offset < rows[selected].End {
			return "#193D48"
		}
		return ""
	}
}

// terminalPane reserves one column of padding on each side of its body. Row
// backgrounds include that padding, but never cover the draggable separator.
// The right separator remains at width-1 for mouse dragging.
func terminalPane(title, body string, width, height int, active bool, backgrounds ...rowBackground) string {
	inner := max(1, width-1)
	lines := strings.Split(body, "\n")
	padded := make([]string, max(0, height-1))
	for y := range padded {
		line := ""
		if y < len(lines) {
			line = ansi.Truncate(lines[y], max(1, inner-2), "")
		}
		style := lipgloss.NewStyle()
		if len(backgrounds) > 0 && backgrounds[0] != nil {
			if bg := backgrounds[0](y); bg != "" {
				style = style.Background(lipgloss.Color(bg))
			}
		}
		// Style gutters separately: a nested ANSI reset at the end of the
		// content must not clear the background of the trailing padding.
		padded[y] = style.Render(" ") + line + style.Render(strings.Repeat(" ", max(1, inner-1-ansi.StringWidth(line))))
	}
	content := strings.Join(padded, "\n")
	divider := lipgloss.NewStyle().Foreground(lipgloss.Color("#30434B")).Render(strings.TrimSuffix(strings.Repeat("│\n", height-1), "\n"))
	return terminalBar(" "+title, width, active) + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, content, divider)
}

// Show pagination progress in the requesting pane's title, without shifting
// the scroll anchor or the message hit targets while the request runs.
func (m *model) timelineTitle(title, root string) string {
	if m.busy && m.loadingOlder && m.loadingGeneration == m.generation && m.loadingRoot == root {
		return m.spinner() + " Loading older messages…"
	}
	return title
}

func (m *model) inputPrompt() string {
	nick := ansi.Truncate(strings.TrimPrefix(m.userName(m.viewer.ID), "@"), 16, "…")
	target := "room"
	if m.composeRoot() != "" {
		target = "thread"
	}
	return " [" + target + "] <" + nick + "> "
}

func (m *model) loadingView(label string, width int) string {
	text := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render(
		ansi.Truncate(m.spinner()+" "+label, max(1, width), "…"),
	)
	return lipgloss.Place(max(1, width), m.bodyHeight(), lipgloss.Center, lipgloss.Center, text)
}

func (m *model) serverLabel() string {
	if m.serverName != "" {
		return m.serverName
	}
	if origin, err := url.Parse(m.fields[0].Value()); err == nil && origin.Host != "" {
		return oneLine(origin.Host)
	}
	return "Server"
}

func (m *model) View() tea.View {
	var content string
	if m.width < 55 || m.height < 16 || (!m.logged && m.height < 28) {
		content = "Chatty · enlarge terminal to 55×28 for login · Ctrl+D quit"
	} else if m.images.expanded != "" {
		content = m.images.preview
	} else if !m.logged {
		body := "\n  CHATTY\n  Your conversations. Your terminal.\n\n"
		for i, label := range []string{"Server", "Username", "Password"} {
			body += "  " + label + "\n  " + m.fields[i].View() + "\n\n"
		}
		body += "  Alt+Enter  sign in with password\n  Ctrl+B  sign in through browser\n  Ctrl+N  next field · Ctrl+D quit\n\n  " + m.status
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel("chatty / sign in", body, min(68, m.width-4), min(27, m.height-2), true))
	} else {
		left, center, right := m.columns()
		h := m.height - 4
		panes := []string{}
		if left > 0 {
			panes = append(panes, terminalPane("CONVERSATIONS", m.sidebarContent(left-3, m.bodyHeight()), left, h, m.focus == 0, m.sidebarBackground(m.bodyHeight())))
		}
		title := "TIMELINE"
		if len(m.rooms) > m.roomIndex {
			title = m.roomName(m.rooms[m.roomIndex])
		}
		if center > 0 {
			body := m.main.View()
			if m.busy && len(m.page.Messages) == 0 {
				body = m.loadingView("Loading room…", center-3)
			}
			panes = append(panes, terminalPane(m.timelineTitle(title, ""), body, center, h, m.focus == 1, selectionBackground(m.mainRows, m.selected, m.main.YOffset())))
		}
		if right > 0 {
			body := m.side.View()
			if m.busy && len(m.thread.Messages) == 0 {
				body = m.loadingView("Loading thread…", right-3)
			}
			panes = append(panes, terminalPane(m.timelineTitle("THREAD · Esc closes", m.root), body, right, h, m.focus == 2, selectionBackground(m.sideRows, m.threadSelected, m.side.YOffset())))
		}
		status := m.status
		if m.busy && !m.loadingOlder {
			status = m.spinner() + " " + status
		}
		position := fmt.Sprintf("%d/%d", min(m.selected+1, len(m.page.Messages)), len(m.page.Messages))
		if m.root != "" {
			position = fmt.Sprintf("thread %d/%d", min(m.threadSelected+1, len(m.thread.Messages)), len(m.thread.Messages))
		}
		header := terminalBar(m.serverLabel()+" / "+m.userName(m.viewer.ID)+" / "+title, m.width, false)
		statusLine := terminalBar(" ["+position+"] "+status, m.width, true)
		prompt := nickColor(m.viewer.ID).Render(m.inputPrompt())
		input := lipgloss.NewStyle().Width(m.width).Render(prompt + m.compose.View())
		help := " ↑↓ select · Enter send / thread · Tab pane · Ctrl+B sidebar · Esc close · Ctrl+O image"
		content = header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, panes...) + "\n" + statusLine + "\n" + input + "\n" + ansi.Truncate(help, m.width, "")
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
