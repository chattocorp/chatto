package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

func TestRequestSizeBreakdown(t *testing.T) {
	var request map[string]json.RawMessage
	_ = json.Unmarshal([]byte(`{"messages":[{"role":"system","content":"rules"},{"role":"user","content":"old"},{"role":"tool","content":"result"},{"role":"user","content":"new"}],"tools":[],"model":"test"}`), &request)
	compact, _ := json.Marshal(request)
	got := requestSizes(request)
	for _, want := range []string{"System: 35", "Tool schemas: 2", "Latest input: 31", "Other history: 31", "Tool results: 34", fmt.Sprintf("Total: %d", len(compact))} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
}

func TestCompletionInspectorSectionsAndNavigation(t *testing.T) {
	m := treeFixture()
	m.resize(100, 24)
	m.enterSession()
	turn := inspection.Turn{Number: 1, Steps: []inspection.Step{{Kind: "model", Name: "Completion", State: "complete", Request: `{"messages":[{"role":"system","content":"system instructions"},{"role":"user","content":"{\"toolGuidance\":\"injected guidance\"}"}],"tools":[{"name":"schedule_list"}],"tool_choice":"none"}`, Response: `{"content":"result"}`}}}
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &turn})
	for i, r := range m.rows {
		if r.step != nil && r.step.Request != "" {
			m.cursor = i
			break
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.detail == nil {
		t.Fatal("Enter did not open completion")
	}
	m.detailKey("tab")
	for _, want := range []string{"system instructions", "injected guidance", "toolGuidance"} {
		if !strings.Contains(m.View().Content, want) {
			t.Fatal("missing readable prompt", want)
		}
	}
	m.detailKey("tab")
	if !strings.Contains(m.View().Content, "schedule_list") {
		t.Fatal("missing tools")
	}
	m.detailKey("tab")
	if !strings.Contains(m.View().Content, "tool_choice") {
		t.Fatal("missing settings")
	}
	m.detailKey("tab")
	var copied string
	m.copyText = func(s string) error { copied = s; return nil }
	m.detailKey("c")()
	if !strings.Contains(copied, "result") {
		t.Fatal("copied wrong section")
	}
	m.detail.sections[m.detail.tab].text = strings.Repeat("long line\n", 100) + "tail\x1b\u202e"
	m.detailKey("end")
	if !strings.Contains(m.View().Content, "tail") || strings.Contains(m.View().Content, "\u202e") {
		t.Fatal("scrolling or display sanitization failed")
	}
	for _, width := range []int{36, 60, 100} {
		m.resize(width, 12)
		view := m.View().Content
		if len(strings.Split(view, "\n")) > 12 {
			t.Fatal("detail view overflowed height")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatal("detail view overflowed width")
			}
		}
	}
	m.detailKey("esc")
	if m.detail != nil {
		t.Fatal("Esc did not return to tree")
	}
}
