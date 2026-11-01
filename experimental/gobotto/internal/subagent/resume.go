package subagent

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
)

func (b *Background) canResume(r *Run) bool {
	if b.manager == nil || r == nil || r.Status != "partial" {
		return false
	}
	w, ok := b.manager.workers[r.Agent]
	if !ok || w.config.Model != r.Model || w.config.Dir != r.Dir {
		return false
	}
	_, _, limit := w.config.RequestPolicy()
	if r.AutoResumes >= limit {
		return false
	}
	switch r.Reason {
	case "cancelled", "deadline", "context_limit", "checkpoint_failed", "output_limit":
		return false
	}
	return r.Reason == "work_limit" || (r.LastFailure != nil && r.LastFailure.Retryable && r.Reason != "checkpoint_failed")
}

// RecoveryTools exposes only eligible runs from this completion event. Neither
// a new task nor another conversation's run can be selected through this tool.
func (b *Background) RecoveryTools(ids []string, maxRuns, turn int, target string, before func(context.Context)) []agent.Tool {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	allowed := map[string]bool{}
	var names []string
	for _, id := range ids {
		if b.canResume(b.runs[id]) {
			allowed[id] = true
			names = append(names, id)
		}
	}
	b.mu.Unlock()
	if len(names) == 0 {
		return nil
	}
	tools := b.tools(maxRuns, turn, target, before, true)
	if len(tools) == 0 {
		return nil
	}
	delegate := tools[0]
	return []agent.Tool{{Name: "resume_investigation", Description: "Continue an eligible incomplete read-only investigation from this completion event, retaining its transcript and evidence. Cannot create a replacement task or expand capabilities. Optional focus identifies the next check within the original task. Wait for the start receipt before claiming it resumed.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"run_id": map[string]any{"type": "string", "enum": names}, "focus": map[string]any{"type": "string", "maxLength": 4000}}, "required": []string{"run_id"}, "additionalProperties": false}, Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
		var args struct {
			ID    string `json:"run_id"`
			Focus string `json:"focus"`
		}
		d := json.NewDecoder(strings.NewReader(string(raw)))
		d.DisallowUnknownFields()
		if d.Decode(&args) != nil || d.Decode(new(any)) != io.EOF || !allowed[args.ID] || len(args.Focus) > 4000 {
			return rejected("Select an eligible run_id and a focused continuation of its original task")
		}
		b.mu.Lock()
		r := b.runs[args.ID]
		eligible := b.canResume(r)
		name := ""
		if r != nil {
			name = r.Agent
		}
		b.mu.Unlock()
		if !eligible {
			return rejected("This investigation cannot auto-resume: it is running, its recovery allowance is exhausted, or its failure needs user action. Report the retained findings and precise blocker.")
		}
		next := map[string]string{"agent": name, "resume_id": args.ID, "task": "Continue the original investigation using retained evidence. Do not restart broad discovery. Resolve the remaining uncertainty or return a useful partial answer. Next check: " + args.Focus}
		encoded, _ := json.Marshal(next)
		return delegate.Execute(ctx, encoded)
	}}}
}
