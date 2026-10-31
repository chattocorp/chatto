package bot

import (
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
)

// inspect is called only by the active turn's owner. The UI gets detached copies.
func (c *conversation) inspect(update func(*inspection.Turn)) {
	if len(c.records) == 0 {
		return
	}
	t := &c.records[len(c.records)-1]
	update(t)
	if c.emitInspection != nil {
		c.emitInspection(t.Clone())
	}
}

func (c *conversation) step(kind, name string) int {
	if runes := []rune(name); len(runes) > 80 {
		name = string(runes[:80]) + "…"
	}
	index := -1
	c.inspect(func(t *inspection.Turn) {
		index = len(t.Steps)
		t.Steps = append(t.Steps, inspection.Step{Kind: kind, Name: name, State: "running", Started: time.Now()})
	})
	return index
}

func (c *conversation) finishStep(index int, state, detail string) {
	c.inspect(func(t *inspection.Turn) {
		if index < 0 || index >= len(t.Steps) {
			return
		}
		s := &t.Steps[index]
		s.State = state
		s.Detail = detail
		s.Duration = time.Since(s.Started)
	})
}

func (c *conversation) recordUsage(u agent.Usage) {
	c.inspect(func(t *inspection.Turn) {
		if len(t.Steps) == 0 {
			return
		}
		s := &t.Steps[len(t.Steps)-1]
		if s.Kind != "model" && s.Kind != "title" {
			return
		}
		s.Usage = inspection.Usage{Input: u.Input, Cached: u.CachedInput, Output: u.Output, Known: true, CacheKnown: u.CacheKnown, CostMissing: u.Cost == nil}
		s.Usage.Reasoning, s.Usage.ReasoningKnown = u.Reasoning, u.ReasoningKnown
		s.Provider, s.RequestID = u.Provider, u.RequestID
		if u.Cost != nil {
			s.Usage.CostKnown = true
			s.Usage.Cost = *u.Cost
		}
	})
}

// send keeps operational logs independently from private inspector records.
// The mutex covers manager invalidations arriving while the worker reports logs.
func (c *conversation) send(emit tui.Emit, event tui.Event) {
	if event.At.IsZero() {
		event.At = time.Now()
	}
	if event.Text != "" {
		c.logMu.Lock()
		c.logs = append(c.logs, inspection.Log{At: event.At, Text: event.Text})
		if len(c.logs) > 200 {
			c.logs = c.logs[len(c.logs)-200:]
		}
		c.logMu.Unlock()
	}
	emit(event)
}

func (c *conversation) storedLogs() []inspection.Log {
	c.logMu.Lock()
	defer c.logMu.Unlock()
	return append([]inspection.Log(nil), c.logs...)
}
