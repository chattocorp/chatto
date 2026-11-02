package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

func TestSessionEventsAndSummaryShareOneTable(t *testing.T) {
	m := treeFixture()
	m.resize(240, 35)
	m.enterSession()
	r := inspection.Turn{Number: 4, State: "uncertain", Started: time.Now(), Inputs: []inspection.Input{{Text: "private input\x1b\u202e"}}, Response: "private reply", Failure: "Delivery was not confirmed", Steps: []inspection.Step{
		{Kind: "model", Name: "Generate", Provider: "Provider", RequestID: "request-123", Detail: "model details", State: "complete", Duration: time.Second, Usage: inspection.Usage{Known: true, Input: 100, Output: 10, CacheKnown: true, Cached: 80, ReasoningKnown: true, Reasoning: 5, CostKnown: true, Cost: 0.01}},
		{Kind: "delivery", Name: "Post reply", State: "uncertain", Detail: "delivery details"},
	}}
	m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	if strings.Count(m.View().Content, "╭") != 2 || m.treeWidth() != 240 {
		t.Fatal("expected summary and one full-width tree")
	}
	var rows []string
	for _, row := range m.rows {
		if row.key.turn == 4 {
			rows = append(rows, m.renderTreeRow(row, 236, false))
		}
	}
	text := strings.Join(rows, "\n")
	for _, want := range []string{"← Input", "private input", "Generate", "Provider", "request-123", "80 cached", "5 reasoning", "1s", "110 tok", "$0.0100", "Post reply", "uncertain", "→ Output", "private reply", "! Failure", "Delivery was not confirmed"} {
		if !strings.Contains(text, want) {
			t.Fatal("missing table data", want)
		}
	}
	if strings.Contains(text, "\u202e") {
		t.Fatal("unsafe terminal text")
	}
	key(m, 'l')
	if strings.Contains(m.View().Content, "private") {
		t.Fatal("operational log includes message bodies")
	}
	if !strings.Contains(m.View().Content, "Session restored") {
		t.Fatal("operational log inaccessible")
	}
	key(m, 'l')
	if len(m.rows) < 6 {
		t.Fatal("turn tree not restored")
	}
}

func TestTurnUsageIncludesChildrenAndUnknownCharges(t *testing.T) {
	r := inspection.Turn{Steps: []inspection.Step{
		{Kind: "model", Usage: inspection.Usage{Known: true, Input: 22000, Cached: 20000, Output: 500, CostKnown: true, Cost: 0.01}},
		{Kind: "model", Name: "child / Completion", Usage: inspection.Usage{Known: true, Input: 100, Output: 100, CostKnown: true, Cost: 0.0023}},
	}}
	tokens, cost := turnUsageSummary(r)
	if tokens != "22.7k tok" || cost != "$0.0123" {
		t.Fatal("incorrect total", tokens, cost)
	}
	r.Steps = append(r.Steps, inspection.Step{Kind: "model"})
	tokens, cost = turnUsageSummary(r)
	if tokens != "22.7k+ tok" || cost != "$0.0123+" {
		t.Fatal("partial accounting hidden", tokens, cost)
	}
	tokens, cost = turnUsageSummary(inspection.Turn{})
	if tokens != "— tok" || cost != "$—" {
		t.Fatal("missing usage shown as zero")
	}
}
