package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

func TestTurnRowResponsiveUsage(t *testing.T) {
	r := inspection.Turn{Steps: []inspection.Step{
		{Kind: "model", Usage: inspection.Usage{Known: true, Input: 22000, Cached: 20000, Output: 500, CostKnown: true, Cost: 0.01}},
		{Kind: "model", Name: "child / Completion", Usage: inspection.Usage{Known: true, Input: 100, Output: 100, CostKnown: true, Cost: 0.0023}},
	}}
	tokens, cost := turnUsageSummary(r)
	if tokens != "22.7k tok" || cost != "$0.0123" {
		t.Fatal("incorrect total", tokens, cost)
	}
	item := turnItem{number: 6, started: time.Now(), state: "Reacted", tokens: tokens, cost: cost}
	for _, tc := range []struct {
		width        int
		tokens, cost bool
	}{{48, true, true}, {34, false, true}, {24, false, false}} {
		l := newTurnList()
		l.SetSize(tc.width, 4)
		l.SetItems([]list.Item{item})
		var out bytes.Buffer
		turnDelegate{}.Render(&out, l, 0, item)
		row := out.String()
		if strings.Contains(row, tokens) != tc.tokens || strings.Contains(row, cost) != tc.cost || !strings.Contains(row, "Reacted") || lipgloss.Width(row) != tc.width {
			t.Fatalf("bad row at %d: %q", tc.width, row)
		}
	}
	r.Steps = append(r.Steps, inspection.Step{Kind: "model"})
	tokens, cost = turnUsageSummary(r)
	if tokens != "22.7k+ tok" || cost != "$0.0123+" {
		t.Fatal("partial accounting hidden", tokens, cost)
	}
	tokens, cost = turnUsageSummary(inspection.Turn{})
	if tokens != "— tok" || cost != "$—" {
		t.Fatal("missing accounting shown as zero")
	}
}

func TestTurnDividerDragAndResize(t *testing.T) {
	for _, grab := range []int{3, 4, 5} {
		m := newModel(nil)
		m.resize(160, 40)
		m.switchMode(true)
		m.apply(Event{Kind: Started, Session: 1})
		r := inspection.Turn{Number: 1, State: "delivered"}
		m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
		left := m.turnListWidth()
		x, y := left+grab, m.list.Height()+4
		m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		if !m.draggingTurns || m.dragging {
			t.Fatal("divider did not start horizontal drag")
		}
		m.Update(tea.MouseMotionMsg{X: x + 10, Y: y + 4, Button: tea.MouseLeft})
		if m.turnListWidth() != left+10 || m.inspectedTurn().Number != 1 {
			t.Fatal("drag changed wrong geometry or selection")
		}
		m.Update(tea.MouseReleaseMsg{X: x + 10, Y: y + 4, Button: tea.MouseLeft})
		if m.draggingTurns {
			t.Fatal("drag did not end")
		}
		ratio := m.turnSplit
		m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		if m.turnSplit != ratio || m.logs.Width() < 24 {
			t.Fatal("resize lost ratio or detail pane")
		}
		m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
		if m.turnListWidth() != left+10 {
			t.Fatal("resize did not restore chosen split")
		}
		m.dragX, m.dragLeft = 0, m.turnListWidth()
		m.dragTurnDivider(10000)
		if m.logs.Width() != 24 {
			t.Fatal("right pane not clamped")
		}
		m.dragTurnDivider(-10000)
		if m.turnListWidth() != 14 {
			t.Fatal("left pane not clamped")
		}
	}
}

func TestChildModelNameVisible(t *testing.T) {
	m := newModel(nil)
	m.resize(160, 40)
	m.apply(Event{Kind: Started, Session: 1})
	r := inspection.Turn{Number: 1, State: "running", Steps: []inspection.Step{{Kind: "model", Name: "code_investigator / Completion", State: "running", Started: time.Now()}}}
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	m.switchMode(true)
	if !strings.Contains(m.logs.GetContent(), "code_investigator / Completion") {
		t.Fatal("child model hidden behind parent label")
	}
}

func TestReasoningUsageDisplay(t *testing.T) {
	for _, tc := range []struct {
		known   bool
		tokens  int64
		partial bool
		want    string
	}{
		{false, 0, false, "reasoning n/a"}, {true, 0, false, "0 reasoning"}, {true, 75, false, "75 reasoning"}, {true, 75, true, "75 reasoning (partial)"},
	} {
		m := newModel(nil)
		m.resize(160, 40)
		m.apply(Event{Kind: Started, Session: 1})
		m.apply(Event{Kind: Tokens, Session: 1, Input: 100, Output: 100, Reasoning: tc.tokens, ReasoningKnown: tc.known, ReasoningMissing: tc.partial})
		if !strings.Contains(m.View().Content, tc.want) || m.selected().input+m.selected().output != 200 {
			t.Fatal("session reasoning display or total incorrect")
		}
		r := inspection.Turn{Number: 1, State: "delivered", Steps: []inspection.Step{{Kind: "model", Usage: inspection.Usage{Known: true, Input: 100, Output: 100, Reasoning: tc.tokens, ReasoningKnown: tc.known}}}}
		if tc.partial {
			r.Steps = append(r.Steps, inspection.Step{Kind: "title", Usage: inspection.Usage{Known: true, Output: 10}})
		}
		m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
		m.switchMode(true)
		if !strings.Contains(m.stats.GetContent(), tc.want) {
			t.Fatal("turn reasoning display incorrect")
		}
	}
}

func TestInspectorIsSeparateSelectableAndCopyable(t *testing.T) {
	m := newModel(nil)
	m.resize(100, 40)
	m.apply(Event{Kind: Started, Session: 1, HumanID: "calm-otter-012345abcdef", Text: "Session restored"})
	private := "Private input " + strings.Repeat("details ", 40) + "end-marker"
	for n := 1; n <= 2; n++ {
		r := inspection.Turn{Number: n, Started: time.Now(), Finished: time.Now(), State: "delivered", Inputs: []inspection.Input{{Text: private}}, Response: "Private reply", Steps: []inspection.Step{{Kind: "model", Name: "Completion", State: "complete", Usage: inspection.Usage{Input: 100, Cached: 80, Output: 10, Known: true, Cost: 0.001, CostKnown: true}}}}
		m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	}
	if strings.Contains(m.logs.GetContent(), "Private") {
		t.Fatal("inspector content appeared in log")
	}
	m.Update(tea.KeyPressMsg{Code: 'i'})
	if strings.Contains(m.View().Content, " Inspector ") {
		t.Fatal("redundant inspector switcher is still visible")
	}
	if !m.inspectMode || !strings.Contains(m.logs.GetContent(), "Private reply") || !strings.Contains(m.stats.GetContent(), "80 cached (80%)") || strings.Contains(m.logs.GetContent(), "end-marker") {
		t.Fatal("inspector preview or usage missing")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(m.logs.GetContent(), "end-marker") {
		t.Fatal("expand did not show full input")
	}
	copied := ""
	m.copyText = func(text string) error { copied = text; return nil }
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})
	m.Update(cmd())
	if copied != private || strings.Contains(m.clipboardNotice, private) {
		t.Fatal("copy input failed or content entered notice")
	}
	m.Update(tea.KeyPressMsg{Code: '['})
	if m.inspectedTurn().Number != 1 {
		t.Fatal("previous turn not selected")
	}
	// Diagnostic logs remain accessible by keyboard; the tab switcher is gone.
	m.Update(tea.KeyPressMsg{Code: 'l'})
	if m.inspectMode || strings.Contains(m.logs.GetContent(), "Private") {
		t.Fatal("log tab did not restore operational log")
	}
	m.Update(tea.KeyPressMsg{Code: 'i'})
	if !m.inspectMode || m.inspectedTurn().Number != 1 {
		t.Fatal("inspector lost turn selection")
	}
}

func TestInspectorStripsTerminalControls(t *testing.T) {
	if got := displayText("safe\x1b\u202e\x00\ntext"); got != "safe\ntext" {
		t.Fatal("unsafe display characters remain")
	}
}

func TestInspectorLeadsWithLatestOutcomeAndKeepsFailureVisible(t *testing.T) {
	m := newModel(nil)
	m.resize(120, 40)
	m.apply(Event{Kind: Started, Session: 1})
	r := inspection.Turn{Number: 1, State: "delivered", Emoji: "custom_reaction", Inputs: []inspection.Input{{Text: "Good night"}}, Steps: []inspection.Step{{Kind: "delivery", Name: "Add reaction", State: "complete"}}}
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	m.switchMode(true)
	content := m.logs.GetContent()
	if strings.Index(content, ":custom_reaction:") > strings.Index(content, "Good night") || !strings.Contains(content, ":custom_reaction:") || !strings.Contains(content, "Good night") {
		t.Fatal("reaction must precede input, including custom emoji")
	}
	if m.turnList.SelectedItem().(turnItem).state != "Reacted" {
		t.Fatal("turn lacks outcome label")
	}
	r.State = "uncertain"
	r.Failure = "Delivery was not confirmed"
	r.Steps[0].State = "uncertain"
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	if !strings.Contains(strings.Split(m.logs.GetContent(), "\n")[0], "Delivery was not confirmed") || !strings.Contains(m.logs.GetContent(), "Add reaction (uncertain)") {
		t.Fatal("uncertain delivery hidden")
	}
}

func TestInspectorDistinguishesMissingCacheUsage(t *testing.T) {
	m := newModel(nil)
	m.resize(120, 40)
	m.apply(Event{Kind: Started, Session: 1})
	m.switchMode(true)
	for _, known := range []bool{false, true} {
		r := inspection.Turn{Number: 1, State: "delivered", Steps: []inspection.Step{{Kind: "model", Provider: "Test provider", RequestID: "gen-test", Usage: inspection.Usage{Input: 100, Known: true, CacheKnown: known}}}}
		m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
		content := m.stats.GetContent()
		if strings.Contains(content, "0 cached (0%)") != known || strings.Contains(content, "cache n/a") == known {
			t.Fatalf("incorrect cache display for known=%t: %s", known, content)
		}
		if !strings.Contains(m.logs.GetContent(), "Test provider") {
			t.Fatal("provider missing")
		}
		m.selected().expanded = true
		m.refreshLogs(false)
		if !strings.Contains(m.logs.GetContent(), "gen-test") {
			t.Fatal("request ID missing")
		}
	}
}

func TestTurnListMouseSelectionAndIndependentScroll(t *testing.T) {
	m := newModel(nil)
	m.resize(120, 32)
	m.apply(Event{Kind: Started, Session: 1})
	for n := 1; n <= 30; n++ {
		r := inspection.Turn{Number: n, Started: time.Now(), State: "delivered", Response: strings.Repeat("long response\n", 60)}
		for range 20 {
			r.Steps = append(r.Steps, inspection.Step{Kind: "tool", Name: "memory_search", State: "complete"})
		}
		m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	}
	m.switchMode(true)
	// Select a row on a later page; mouse coordinates stay relative to that page.
	m.turnList.Select(22)
	first := m.turnList.Paginator.Page * m.turnList.Paginator.PerPage
	m.Update(tea.MouseClickMsg{X: 4, Y: m.list.Height() + 3, Button: tea.MouseLeft})
	if m.turnList.Index() != first || m.inspectedTurn().Number != 30-first || !m.turnsFocused {
		t.Fatal("mouse did not select visible turn")
	}
	view := m.View().Content
	if strings.Count(view, "╭") != 4 || strings.Count(view, "╯") != 4 {
		t.Fatal("expected four separately bordered panes")
	}
	copied := ""
	m.copyText = func(text string) error { copied = text; return nil }
	_, cmd := m.Update(tea.MouseClickMsg{X: m.turnListWidth() + 16, Y: m.list.Height() + 2, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("detail-pane title is not copyable")
	}
	m.Update(cmd())
	if copied != m.selected().humanID {
		t.Fatal("wrong ID copied")
	}
	var selected bytes.Buffer
	turnDelegate{}.Render(&selected, m.turnList, first, m.turnList.SelectedItem())
	if lipgloss.Width(selected.String()) != m.turnList.Width() || !strings.Contains(selected.String(), "48;2;36;79;82") || strings.Contains(selected.String(), ">") {
		t.Fatal("turn selection does not fill row")
	}
	m.selected().expanded = true
	m.refreshLogs(false)
	m.Update(tea.MouseClickMsg{X: 70, Y: m.list.Height() + 7, Button: tea.MouseLeft})
	statsBefore := m.stats.View()
	m.Update(tea.MouseWheelMsg{X: 70, Y: m.list.Height() + m.stats.Height() + 8, Button: tea.MouseWheelDown})
	if m.stats.View() != statsBefore || m.logs.Height() <= m.stats.Height() {
		t.Fatal("stats must stay fixed above the larger scrolling log")
	}
	if m.turnsFocused || m.logs.YOffset() == 0 {
		t.Fatal("details did not scroll independently")
	}
	offset := m.logs.YOffset()
	m.Update(inspectorTick{})
	if m.logs.YOffset() != offset {
		t.Fatal("live refresh moved details scrollback")
	}
	m.switchMode(false)
	m.switchMode(true)
	if m.logs.YOffset() != offset {
		t.Fatal("view switch lost detail scroll position")
	}
	for _, size := range [][2]int{{36, 12}, {80, 24}, {120, 40}} {
		m.resize(size[0], size[1])
		m.refreshLogs(false)
		view := m.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("inspector exceeds terminal %dx%d", size[0], size[1])
		}
	}
}

func TestInspectorLogHasOneRowPerEvent(t *testing.T) {
	m := newModel(nil)
	m.resize(120, 40)
	m.apply(Event{Kind: Started, Session: 1})
	r := inspection.Turn{Number: 1, State: "delivered",
		Inputs: []inspection.Input{{Text: "first\nsecond"}}, Response: strings.Repeat("reply ", 60),
		Steps: []inspection.Step{
			{Kind: "model", State: "complete", Provider: "Test provider", Detail: "multiple\nlines"},
			{Kind: "delivery", Name: "Post reply", State: "complete"},
			{Kind: "title", Name: "Generate title", State: "complete"},
		}}
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	m.switchMode(true)
	m.selected().expanded = true
	m.refreshLogs(false)
	rows := strings.Split(m.logs.GetContent(), "\n")
	if len(rows) != 5 || !strings.Contains(rows[4], "first second") || !strings.Contains(rows[3], "via Test provider") || !strings.Contains(rows[3], "multiple lines") || !strings.Contains(rows[1], "Output") || !strings.Contains(rows[0], "Generate title") {
		t.Fatal("events must occupy individual rows in reverse execution order")
	}
	if m.logs.SoftWrap {
		t.Fatal("event rows must not wrap")
	}
	m.logsFocused = true
	m.turnsFocused = false
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if m.logs.XOffset() == 0 {
		t.Fatal("long events cannot be scrolled horizontally")
	}
}
