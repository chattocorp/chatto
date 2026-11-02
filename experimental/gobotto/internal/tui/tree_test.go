package tui

import (
	"context"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

func treeFixture() *model {
	m := newModel(nil)
	m.resize(140, 30)
	m.apply(Event{Kind: Started, Session: 1, HumanID: "calm-test-123", Title: "Example session", Text: "Session restored"})
	for n := 1; n <= 3; n++ {
		r := inspection.Turn{Number: n, Started: time.Now(), Finished: time.Now(), State: "delivered", Inputs: []inspection.Input{{Text: "private input"}}, Response: "private response", Steps: []inspection.Step{
			{Kind: "model", Name: "Completion", State: "complete", Usage: inspection.Usage{Known: true, Input: 100, Output: 10, CostKnown: true, Cost: 0.01}},
			{Kind: "delivery", Name: "Post reply", State: "complete"},
		}}
		if n == 3 {
			r.State = "running"
			r.Finished = time.Time{}
			r.Steps[1].State = "running"
			r.Steps[1].Started = time.Now()
		}
		m.apply(Event{Kind: Inspected, Session: 1, Inspection: &r})
	}
	return m
}

func key(m *model, code rune) { m.Update(tea.KeyPressMsg{Code: code}) }

func TestTreeColumnsAndContinuousBackground(t *testing.T) {
	m := treeFixture()
	sgr := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	for _, width := range []int{32, 65, 90, 120} {
		for _, selected := range []bool{false, true} {
			for _, count := range []int64{10, 39500, 1600000} {
				s := m.rows[0].s
				s.state, s.input, s.usageKnown, s.costKnown = "idle", count, true, true
				s.title = strings.Repeat("Long title ", 20)
				s.turns = 256
				line := m.renderTreeRow(m.rows[0], width, selected)
				plain := sgr.ReplaceAllString(line, "")
				if lipgloss.Width(plain) != width || strings.Contains(plain, "idle") || !strings.HasPrefix(plain, "  Long title") {
					t.Fatal("idle title slot or row width incorrect")
				}
				if width >= 65 && lipgloss.Width(plain[:strings.Index(plain, "tok")+3]) != width-13 {
					t.Fatal("token values are not right aligned")
				}
				if width >= 65 && !strings.Contains(plain, "256 turns") {
					t.Fatal("session turn count missing")
				}
				background := "48;2;37;42;51"
				if selected {
					background = "48;2;36;79;82"
				}
				painted, end := false, 0
				for _, span := range sgr.FindAllStringIndex(line, -1) {
					if span[0] > end && !painted {
						t.Fatal("row contains an unpainted text or padding segment")
					}
					code := line[span[0]:span[1]]
					if code == "\x1b[m" || code == "\x1b[0m" {
						painted = false
					}
					if strings.Contains(code, background) {
						painted = true
					}
					end = span[1]
				}
				if end < len(line) && !painted {
					t.Fatal("trailing padding has no background")
				}
			}
		}
	}
}

func TestOverviewSpinnerPrecedesTitle(t *testing.T) {
	m := treeFixture()
	sgr := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	for _, state := range []string{"running", "queued", "investigating", "idle", "failed", "ready"} {
		m.rows[0].s.state = state
		plain := sgr.ReplaceAllString(m.renderTreeRow(m.rows[0], 90, false), "")
		prefix := "  "
		if state == "running" || state == "queued" || state == "investigating" {
			prefix = m.busy.View() + " "
		}
		if !strings.HasPrefix(plain, prefix+"Example session") || strings.Contains(plain, state) {
			t.Fatal("incorrect session activity placement", state)
		}
	}
}

func TestSessionUsageAndBackgroundState(t *testing.T) {
	m := treeFixture()
	cost := 0.00125
	for range 2 {
		m.apply(Event{Kind: TurnStarted, Session: 1})
		m.apply(Event{Kind: Tokens, Session: 1, Input: 100, CachedInput: 80, Output: 10, Cost: &cost})
		m.apply(Event{Kind: Completed, Session: 1, Background: true})
	}
	s := m.selected()
	if s.input != 200 || s.cached != 160 || s.output != 20 || s.cost != 0.0025 || s.state != "investigating" {
		t.Fatal("session totals or state incorrect")
	}
	m.apply(Event{Kind: Tokens, Session: 1, Unreported: true})
	if !strings.Contains(m.sessionHeader(), "partial") {
		t.Fatal("unknown charge hidden")
	}
}

func TestQuitCancelsAndJoinsWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stopped := make(chan struct{})
	err := Run(ctx, func(ctx context.Context, emit Emit) error {
		emit(Event{Kind: Started, Session: 1})
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	}, tea.WithInput(strings.NewReader("Q")), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("worker survived UI exit")
	}
}
