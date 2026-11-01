package bot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/subagent"
)

// saveChildren is independent from the parent snapshot writer. One Background
// serializes this ledger; no child goroutine accesses conversation-owned state.
func (s *Store) saveChildren(id string, runs map[string]*subagent.Run) error {
	if s == nil {
		return nil
	}
	if !validHumanID.MatchString(id) {
		return errStore
	}
	dir := filepath.Join(s.dir, "children")
	if os.MkdirAll(dir, 0700) != nil {
		return errStore
	}
	f, err := os.CreateTemp(dir, ".snapshot-*")
	if err != nil {
		return errStore
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(runs)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil || os.Rename(f.Name(), filepath.Join(dir, id+".json")) != nil {
		return errStore
	}
	d, err := os.Open(dir)
	if err != nil {
		return errStore
	}
	defer d.Close()
	if d.Sync() != nil {
		return errStore
	}
	return nil
}

func (s *Store) loadChildren(c *conversation) error {
	data, err := os.ReadFile(filepath.Join(s.dir, "children", c.humanID+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || json.Unmarshal(data, &c.childRuns) != nil {
		return errStore
	}
	for _, r := range c.childRuns {
		if r == nil {
			return errStore
		}
		if r.Status == "running" {
			r.Status, r.Reason = "partial", "process_interrupted"
		}
		r.NotificationPending = false
	}
	return nil
}

// syncChildren is called only by the conversation owner: the coordinator when
// idle or the parent worker between requests. Completion wakes are never replayed
// after a restart, since the prior Chatto delivery may have been uncertain.
func (c *conversation) syncChildren() error {
	if c.jobs == nil {
		return nil
	}
	runs, events, done, err := c.jobs.Drain()
	c.childRuns = runs
	c.childReady = append(c.childReady, done...)
	if c.childSteps == nil {
		c.childSteps = map[string]int{}
	}
	for _, a := range events {
		var record *inspection.Turn
		for n := range c.records {
			if c.records[n].Number == a.Turn {
				record = &c.records[n]
				break
			}
		}
		index, ok := c.childSteps[a.RunID]
		if a.Usage != nil {
			u := *a.Usage
			if record != nil && ok && index < len(record.Steps) {
				s := &record.Steps[index]
				s.Usage = inspection.Usage{Input: u.Input, Cached: u.CachedInput, Output: u.Output, Known: true, CacheKnown: u.CacheKnown, Reasoning: u.Reasoning, ReasoningKnown: u.ReasoningKnown, CostMissing: u.Cost == nil}
				s.Usage.Known = !u.Unreported
				if u.Cost != nil {
					s.Usage.CostKnown = true
					s.Usage.Cost = *u.Cost
				}
				s.Provider, s.RequestID = u.Provider, u.RequestID
			}
		} else if record != nil {
			e := a.Event
			if e.State == "running" {
				index = len(record.Steps)
				c.childSteps[a.RunID] = index
				record.Steps = append(record.Steps, inspection.Step{Kind: e.Kind, Name: e.Name, State: e.State, Started: a.At, Detail: boundedDetail(e.Detail)})
			} else if ok && index < len(record.Steps) {
				s := &record.Steps[index]
				s.State = e.State
				s.Detail = boundedDetail(e.Detail)
				s.Duration = a.At.Sub(s.Started)
			}
		}
		if record != nil && c.emitInspection != nil {
			c.emitInspection(record.Clone())
		}
	}
	if c.childUsageApplied == nil {
		c.childUsageApplied = map[string]int{}
	}
	for id, r := range runs {
		for n := c.childUsageApplied[id]; n < len(r.Usage); n++ {
			c.childUsageApplied[id] = n + 1
			if c.accountUsage != nil {
				c.accountUsage(r.Usage[n].Usage)
			}
			if c.storeErr != nil {
				return c.storeErr
			}
		}
	}
	if ackErr := c.jobs.Acknowledge(c.childUsageApplied, nil); ackErr != nil {
		c.storeErr = errStore
		return errStore
	}
	if err != nil {
		c.storeErr = errStore
		return errStore
	}
	return nil
}

func boundedDetail(s string) string {
	r := []rune(s)
	if len(r) > 4000 {
		return string(r[:4000]) + "\n[truncated]"
	}
	return s
}

func deferredTool(allowed func() bool) agent.Tool {
	return agent.Tool{Name: "defer_response", Description: "End this turn without an additional message. Use after announcing background work, or when a completion notification needs no further reply. Background work continues and its completion will wake the conversation.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
		if !allowed() {
			return `{"status":"rejected","error":"There is no background work, progress reply, or completion notification. Answer the user."}`, nil
		}
		return deferExecute(ctx, raw)
	}}
}

func deferExecute(_ context.Context, raw json.RawMessage) (string, error) {
	var args map[string]json.RawMessage
	if json.Unmarshal(raw, &args) != nil || args == nil || len(args) != 0 {
		return `{"status":"rejected","error":"Use an empty object"}`, nil
	}
	return `{"status":"complete","deferred":true,"message":"Turn ended without a final Chatto message. Earlier progress receipts still apply."}`, nil
}
