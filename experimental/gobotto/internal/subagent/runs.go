package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
)

// Run is private, resumable child state owned by one parent conversation.
// History contains source and provider metadata; never put it in operational logs.
type Run struct {
	ID, Agent, Model, Dir, Task string
	Status, Reason, Report      string
	Updated                     time.Time
	History                     agent.Snapshot
	Evidence                    []Evidence
	// Updates is a bounded public activity buffer; reasoning fields never enter it.
	Updates    []UpdateNote
	Target     string
	OriginTurn int
	// Usage is durable before the parent accounts for it. Entries identify the
	// originating attempt so resumed jobs retain correct per-turn attribution.
	Usage               []RunUsage
	UsageAcknowledged   int
	NotificationPending bool
	LastFailure         *agent.FailureInfo
	Recoveries          int
	AutoResumes         int
	// Phase is host-observed activity, not a claim about investigative progress.
	Phase      string
	PhaseSince time.Time
}

// RunUsage associates a reported request charge with its original parent turn.
type RunUsage struct {
	Turn  int
	Usage agent.Usage
}

// UpdateNote contains public model content or a bounded tool receipt, never reasoning.
type UpdateNote struct {
	At         time.Time
	Kind, Text string
}

func (r *Run) note(kind, text string) {
	if text == "" {
		return
	}
	r.Updates = append(r.Updates, UpdateNote{At: time.Now(), Kind: kind, Text: clip(text, 2000)})
	for len(r.Updates) > 32 {
		r.Updates = r.Updates[1:]
	}
	for {
		size := 0
		for _, n := range r.Updates {
			size += len(n.Text)
		}
		if size <= 64<<10 || len(r.Updates) == 0 {
			break
		}
		r.Updates = r.Updates[1:]
	}
}

func (r *Run) phase(value string) { r.Phase = value; r.PhaseSince = time.Now() }

// Evidence preserves bounded completed tool receipts, not model reasoning.
type Evidence struct{ Tool, Arguments, Result string }

// Binding scopes runs to one parent and checkpoints them through its private
// store. The parent must serialize access. Nil Checkpoint means in-memory only.
type Binding struct {
	Runs       map[string]*Run
	Checkpoint func() error
	// BeforeRun announces validated work, after its initial checkpoint.
	BeforeRun func(context.Context)
	// Launch transfers execution to a background owner. It must not share Run
	// with readers; publish detached snapshots through Checkpoint instead.
	Launch  func(*Run, func(context.Context) string)
	Publish func(*Run) error
}

func (b Binding) saveRun(r *Run) error {
	if b.Publish != nil {
		return b.Publish(r)
	}
	return b.save()
}

func (b Binding) save() error {
	if b.Checkpoint != nil {
		return b.Checkpoint()
	}
	return nil
}

func clip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "\n[truncated]"
	}
	return text
}

// investigationVerification states limits of the worker's host-assigned tools,
// independently of model claims or execution status. Derive it on disclosure so
// retained runs also receive these limits without rewriting private history.
type investigationVerification struct {
	Method            string `json:"method"`
	RuntimeChecked    bool   `json:"runtime_checked"`
	TestsRun          bool   `json:"tests_run"`
	GitHistoryChecked bool   `json:"git_history_checked"`
	CompletionMeaning string `json:"completion_meaning"`
}

func verificationLimits() investigationVerification {
	return investigationVerification{
		Method:            "static inspection of a live checkout using file tools only",
		CompletionMeaning: "complete means the worker finished its report, not that its diagnosis was reproduced or independently verified",
	}
}

func (r *Run) result() map[string]any {
	report := r.Report
	if report == "" {
		report = "Investigation unfinished. No final diagnosis was produced. Use the completed evidence below, preserve uncertainty, and resume only if more investigation is needed."
	}
	unknown := 0
	for _, u := range r.Usage {
		if u.Usage.Unreported {
			unknown++
		}
	}
	return map[string]any{"run_id": r.ID, "agent": r.Agent, "status": r.Status, "phase": r.Phase, "phase_since": r.PhaseSince, "reason": r.Reason, "failure": r.LastFailure, "request_recoveries": r.Recoveries, "unreported_requests": unknown, "auto_resumes": r.AutoResumes, "report": report, "report_truncated": false, "verification": verificationLimits(), "evidence": r.Evidence, "updates": r.Updates, "scope": "Read-only live checkout; evidence may be incomplete or stale. No files were changed."}
}

func (r *Run) receipt(e agent.ToolEvent) {
	if e.State != "complete" {
		return
	}
	r.note("tool", e.Name+" "+clip(e.Arguments, 500)+"\n"+clip(e.Result, 1000))
	r.Evidence = append(r.Evidence, Evidence{Tool: e.Name, Arguments: clip(e.Arguments, 500), Result: clip(e.Result, 1500)})
	if len(r.Evidence) > 8 {
		r.Evidence = r.Evidence[len(r.Evidence)-8:]
	}
}

var errContext = errors.New("child context limit")

func stopReason(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, errContext):
		return "context_limit"
	case errors.Is(err, agent.ErrOutputLimit):
		return "output_limit"
	default:
		return agent.ModelFailureInfo(err).Code
	}
}

func (b Binding) statusTool() agent.Tool {
	return agent.Tool{Name: "investigation_status", Description: "List retained investigations without starting work. Supply run_id for a report and evidence. Use for status questions before considering another delegation. Internal run IDs must not appear in user replies.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"run_id": map[string]any{"type": "string"}}, "additionalProperties": false}, Execute: func(_ context.Context, raw json.RawMessage) (string, error) {
		var args map[string]json.RawMessage
		if json.Unmarshal(raw, &args) != nil || args == nil || len(args) > 1 {
			return rejected("Use an empty object or a run_id")
		}
		if len(args) == 1 {
			var id string
			if json.Unmarshal(args["run_id"], &id) != nil || b.Runs[id] == nil {
				return rejected("Unknown run_id")
			}
			return result(b.Runs[id].result()), nil
		}
		ids := make([]string, 0, len(b.Runs))
		for id := range b.Runs {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return b.Runs[ids[i]].Updated.After(b.Runs[ids[j]].Updated) })
		var reports []map[string]any
		for _, id := range ids {
			r := b.Runs[id]
			reports = append(reports, map[string]any{"run_id": id, "agent": r.Agent, "task": clip(r.Task, 300), "status": r.Status, "phase": r.Phase, "phase_since": r.PhaseSince, "reason": r.Reason, "failure": r.LastFailure, "report": clip(r.Report, 500), "report_truncated": len([]rune(r.Report)) > 500, "verification": verificationLimits(), "completed_evidence": len(r.Evidence)})
		}
		return result(map[string]any{"runs": reports}), nil
	}}
}

// retain bounds private child history separately from the parent transcript.
func (b Binding) retain(activeID string) {
	for len(b.Runs) > 8 {
		oldest := ""
		for id, r := range b.Runs {
			if id == activeID || r.Status == "running" || r.NotificationPending || len(r.Usage) > r.UsageAcknowledged {
				continue
			}
			if oldest == "" || r.Updated.Before(b.Runs[oldest].Updated) || (r.Updated.Equal(b.Runs[oldest].Updated) && id < oldest) {
				oldest = id
			}
		}
		if oldest == "" {
			break
		}
		delete(b.Runs, oldest)
	}
}
