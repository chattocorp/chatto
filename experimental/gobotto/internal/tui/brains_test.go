package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/brainview"
)

type fakeBrains struct {
	calls []string
	fail  bool
}

func (f *fakeBrains) Catalog(context.Context) ([]brainview.Brain, error) {
	f.calls = append(f.calls, "catalog")
	if f.fail {
		return nil, errors.New("private diagnostic")
	}
	return []brainview.Brain{{Key: "memory", Name: "Memory", Kind: "Built-in"}, {Key: "apps/tasks", Name: "Tasks", Kind: "App"}}, nil
}
func (f *fakeBrains) List(_ context.Context, key string, offset int) (brainview.Page, error) {
	f.calls = append(f.calls, fmt.Sprintf("list %s %d", key, offset))
	return brainview.Page{Definition: brainview.Document{Title: key, Body: "# Instructions\nKeep useful facts.", Metadata: `{"schema":{"type":"object"}}`}, Entries: []brainview.Entry{{ID: fmt.Sprint(offset + 1), Title: "Synthetic document"}}, More: offset == 0}, nil
}
func (f *fakeBrains) Read(_ context.Context, key, id string) (brainview.Document, error) {
	f.calls = append(f.calls, "read "+key+" "+id)
	return brainview.Document{Title: "Synthetic document", Body: strings.Repeat("Private synthetic fact\n", 40), Metadata: `{"title":"Synthetic document"}`}, nil
}
func finishBrainCommands(t *testing.T, m *model, cmd tea.Cmd) {
	t.Helper()
	for i := 0; cmd != nil; i++ {
		if i > 10 {
			t.Fatal("request loop")
		}
		msg := cmd()
		if _, ok := msg.(brainResult); !ok {
			t.Fatalf("unexpected browser command %T", msg)
		}
		_, cmd = m.Update(msg)
	}
}
func openTestBrains(t *testing.T) (*model, *fakeBrains) {
	t.Helper()
	m := treeFixture()
	source := &fakeBrains{}
	m.brains.source = source
	_, cmd := m.Update(tea.KeyPressMsg{Code: '2'})
	if len(source.calls) != 0 {
		t.Fatal("browser read synchronously")
	}
	finishBrainCommands(t, m, cmd)
	key(m, tea.KeyRight)
	return m, source
}

func TestBrainsNavigationPaginationAndViewState(t *testing.T) {
	m, source := openTestBrains(t)
	if !m.showBrains || len(m.brains.rows) != 3 || m.brains.selected().key != "memory" {
		t.Fatal("initial browser layout")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	finishBrainCommands(t, m, cmd)
	if m.brains.docID != "1" || !strings.Contains(ansi.Strip(m.View().Content), "Private synthetic fact") {
		t.Fatal("selected document not shown")
	}
	key(m, tea.KeyTab)
	key(m, tea.KeyDown)
	if m.brains.scroll != 1 {
		t.Fatal("content did not scroll independently")
	}
	selected := m.brains.cursor
	key(m, '1')
	if m.showBrains || m.activeSession != 1 {
		t.Fatal("session selection lost")
	}
	key(m, '2')
	if m.brains.cursor != selected || m.brains.scroll != 1 {
		t.Fatal("brain view state lost")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'n'})
	finishBrainCommands(t, m, cmd)
	if m.brains.group("memory").offset != 1 || !m.brains.selected().header {
		t.Fatal("page replacement did not select group")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'p'})
	finishBrainCommands(t, m, cmd)
	if m.brains.group("memory").offset != 0 {
		t.Fatal("previous page failed")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: ']'})
	finishBrainCommands(t, m, cmd)
	if m.brains.selected().key != "apps/tasks" || source.calls[len(source.calls)-1] != "list apps/tasks 0" {
		t.Fatal("brain navigation failed", source.calls)
	}
}

func TestBrainsLateResultsAndRefresh(t *testing.T) {
	m, _ := openTestBrains(t)
	_, read := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, next := m.Update(tea.KeyPressMsg{Code: ']'})
	if next != nil {
		t.Fatal("overlapping disk reads")
	}
	finishBrainCommands(t, m, read)
	if m.brains.selected().key != "apps/tasks" || strings.Contains(ansi.Strip(m.View().Content), "Private synthetic fact") {
		t.Fatal("late read replaced selected brain")
	}
	key(m, tea.KeyRight)
	_, read = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, refresh := m.Update(tea.KeyPressMsg{Code: 'r'})
	if refresh != nil {
		t.Fatal("refresh overlapped read")
	}
	finishBrainCommands(t, m, read)
	if m.brains.docKey != "" || m.brains.loading {
		t.Fatal("stale document survived refresh")
	}
}

func TestBrainsErrorsRetryAndQuit(t *testing.T) {
	m := newModel(nil)
	m.brains.source = &fakeBrains{fail: true}
	_, cmd := m.Update(tea.KeyPressMsg{Code: '2'})
	finishBrainCommands(t, m, cmd)
	if !strings.Contains(m.View().Content, "Unable to load") || strings.Contains(m.View().Content, "private diagnostic") {
		t.Fatal("unsafe or hidden error")
	}
	m.brains.source = &fakeBrains{}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'r'})
	finishBrainCommands(t, m, cmd)
	if !m.brains.ready {
		t.Fatal("refresh failed")
	}
	requested := false
	m.requestQuit = func() { requested = true }
	key(m, 'q')
	if !requested || !strings.Contains(m.View().Content, "Waiting for active work") {
		t.Fatal("brain view bypassed graceful quit")
	}
}

func TestBrainsDividerAndSmallLayouts(t *testing.T) {
	m, _ := openTestBrains(t)
	left, _ := m.brainWidths()
	m.Update(tea.MouseClickMsg{X: left, Y: 4, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: 60, Y: 4, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: 60, Y: 4, Button: tea.MouseLeft})
	if left, _ := m.brainWidths(); left != 60 || m.brains.dragging {
		t.Fatal("brain divider did not move")
	}
	for _, size := range [][2]int{{40, 8}, {80, 24}, {140, 40}, {20, 5}} {
		m.resize(size[0], size[1])
		view := m.View().Content
		if lipgloss.Height(view) > size[1] {
			t.Fatal("view exceeds height")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("view exceeds width")
			}
		}
	}
	m.resize(100, 24)
	t.Log("\n" + ansi.Strip(m.View().Content))
}

func TestBrainFoldingFromDocumentsAndContents(t *testing.T) {
	m, _ := openTestBrains(t)
	_, read := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	key(m, tea.KeyLeft)
	if !m.brains.group("memory").collapsed || !m.brains.selected().header || len(m.brains.rows) != 2 {
		t.Fatal("left did not fold the document's brain")
	}
	finishBrainCommands(t, m, read)
	if len(m.brains.rows) != 2 {
		t.Fatal("late read unfolded brain")
	}
	key(m, tea.KeyRight)
	if m.brains.contentFocused || m.brains.group("memory").collapsed || len(m.brains.rows) != 3 {
		t.Fatal("right did not unfold in place")
	}
	key(m, tea.KeyTab)
	key(m, tea.KeyLeft)
	if m.brains.contentFocused || m.brains.group("memory").collapsed {
		t.Fatal("left from contents folded instead of moving focus")
	}
	key(m, tea.KeyEnter)
	_, refresh := m.Update(tea.KeyPressMsg{Code: 'r'})
	finishBrainCommands(t, m, refresh)
	if !m.brains.group("memory").collapsed || len(m.brains.rows) != 2 {
		t.Fatal("refresh lost fold choice")
	}
	m.Update(tea.MouseClickMsg{X: 1, Y: 1, Button: tea.MouseLeft})
	if m.brains.group("memory").collapsed || len(m.brains.rows) != 3 {
		t.Fatal("caret click did not unfold")
	}
}

func TestBrainSelectionDoesNotExpandGroups(t *testing.T) {
	m := treeFixture()
	m.brains.source = &fakeBrains{}
	_, cmd := m.Update(tea.KeyPressMsg{Code: '2'})
	finishBrainCommands(t, m, cmd)
	if len(m.brains.rows) != 2 || !m.brains.group("memory").collapsed {
		t.Fatal("initial selection expanded a brain")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	finishBrainCommands(t, m, cmd)
	if len(m.brains.rows) != 2 || !m.brains.group("apps/tasks").collapsed {
		t.Fatal("navigation or loading expanded a brain")
	}
	key(m, tea.KeyRight)
	if len(m.brains.rows) != 3 || m.brains.group("apps/tasks").collapsed {
		t.Fatal("explicit expansion failed")
	}
}
