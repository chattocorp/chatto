package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestDetailsFollowSelectionAndScrollIndependently(t *testing.T) {
	m := treeFixture()
	if text := ansi.Strip(strings.Join(m.detailLines(60), "\n")); !strings.Contains(text, "hidden-model") {
		t.Fatal("missing turn details", text)
	}
	key(m, tea.KeyTab)
	m.selected().records[2].Steps[0].Detail = "hidden-detail\n" + strings.Repeat("additional recorded detail\n", 20)
	m.moveTree(3)
	if text := ansi.Strip(strings.Join(m.detailLines(60), "\n")); !strings.Contains(text, "hidden-detail") || !strings.Contains(text, "1234 input") {
		t.Fatal("missing step details", text)
	}
	selected := m.currentRow().key
	m.scrollDetails(10000)
	if m.detailOffset == 0 {
		t.Fatal("details did not scroll")
	}
	end := m.detailOffset
	m.scrollDetails(-3)
	if m.detailOffset >= end || m.currentRow().key != selected {
		t.Fatal("detail scrolling changed selection or exceeded bounds")
	}
	left, _ := m.paneWidths()
	m.Update(tea.MouseClickMsg{X: left + 3, Y: m.eventHeight() + 2, Button: tea.MouseLeft})
	if m.currentRow().key != selected {
		t.Fatal("detail click selected an event")
	}
	m.moveTree(1)
	if m.detailOffset != 0 || !strings.Contains(ansi.Strip(strings.Join(m.detailLines(60), "\n")), "hidden-input") {
		t.Fatal("input selection did not reset details")
	}
}

func TestFormattedDetailsFitAndSanitizeRecordedContent(t *testing.T) {
	fields := []detailField{
		{"Selected", "Turn 2"}, {"State", "delivered"},
		{"Model", "example/model"}, {"Runtime", "13.353s"},
		{"Output", "# Result\n- Created a diagram\n```svg rendered\n<svg>" + strings.Repeat("界", 80) + "</svg>\n```\n\x1b[2J\u202eunsafe"},
	}
	for _, width := range []int{13, 40, 64, 100} {
		lines := renderDetails(fields, width)
		for _, line := range lines {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d exceeded: %q", width, line)
			}
		}
		text := ansi.Strip(strings.Join(lines, "\n"))
		if strings.Contains(text, "\u202e") || strings.Contains(strings.Join(lines, "\n"), "\x1b[2J") {
			t.Fatal("unsafe terminal control")
		}
		if !strings.Contains(text, "OUTPUT") || !strings.Contains(text, "DELIVERED") || strings.Contains(text, "```") {
			t.Fatal("missing formatting", text)
		}
	}
}

func TestDetailsDividerDragAndResize(t *testing.T) {
	m := treeFixture()
	left, _ := m.paneWidths()
	selected := m.currentRow().key
	m.Update(tea.MouseClickMsg{X: left + 5, Y: m.eventHeight() + 1, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: left + 5, Y: 9, Button: tea.MouseLeft})
	if m.eventHeight() != 8 || !m.draggingDetail || m.currentRow().key != selected {
		t.Fatal("divider did not track mouse independently of selection")
	}
	m.Update(tea.MouseReleaseMsg{X: left + 5, Y: 9, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: left + 5, Y: 15})
	if m.eventHeight() != 8 || m.draggingDetail {
		t.Fatal("drag continued after release")
	}
	for _, height := range []int{8, 12, 24, 40} {
		m.resize(80, height)
		for _, y := range []int{-100, 10000} {
			m.dragDetailDivider(y)
			if m.eventHeight() < 2 || m.treeHeight()-m.eventHeight()-1 < 2 {
				t.Fatal("divider hid a pane")
			}
		}
	}
}

func TestDetailHeadersIncludeContentPaddingInBackground(t *testing.T) {
	lines := renderDetails([]detailField{{"Selected", "Turn 3"}, {"Output", "Recorded reply"}}, 80)
	checked := 0
	for _, line := range lines {
		plain := ansi.Strip(line)
		if strings.Contains(plain, "Turn 3") || strings.Contains(plain, "OUTPUT") {
			if !strings.HasPrefix(plain, contentIndent) || strings.HasPrefix(plain, contentIndent+" ") {
				t.Fatal("header does not have exactly two spaces of indentation")
			}
			if line != separatorBar(plain, 80, false) {
				t.Fatal("header has padding outside its background")
			}
			checked++
		}
	}
	if checked != 2 {
		t.Fatal("missing detail header or section header")
	}
}
