package subagent

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
)

// Background owns independent child tasks for a single conversation. Parent
// turns only see detached snapshots. Children never mutate the parent session.
type Background struct {
	mu      sync.Mutex
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
	manager *Manager
	runs    map[string]*Run
	active  map[string]bool
	updates []Activity
	done    []string
	persist func(map[string]*Run) error
	notify  func()
	err     error
}

// Activity routes telemetry to the originating turn, even while another turn runs.
type Activity struct {
	At    time.Time
	RunID string
	Turn  int
	Event Event
	Usage *agent.Usage
}

// NewBackground creates a conversation-owned supervisor. Persist is serialized;
// notify must not block. Background work does not send typing indicators.
func NewBackground(ctx context.Context, m *Manager, runs map[string]*Run, persist func(map[string]*Run) error, notify func()) *Background {
	ctx, cancel := context.WithCancel(ctx)
	if runs == nil {
		runs = map[string]*Run{}
	}
	b := &Background{ctx: ctx, cancel: cancel, manager: m, runs: cloneRuns(runs), active: map[string]bool{}, persist: persist, notify: notify}
	return b
}

func cloneRuns(runs map[string]*Run) map[string]*Run {
	data, _ := json.Marshal(runs)
	var out map[string]*Run
	_ = json.Unmarshal(data, &out)
	return out
}

func (b *Background) signal() {
	if b.notify != nil {
		b.notify()
	}
}

func (b *Background) publish(r *Run) error {
	b.mu.Lock()
	copy := cloneRuns(map[string]*Run{r.ID: r})[r.ID]
	if previous := b.runs[r.ID]; previous != nil {
		copy.UsageAcknowledged = previous.UsageAcknowledged
	}
	b.runs[r.ID] = copy
	Binding{Runs: b.runs}.retain(r.ID)
	if b.persist != nil {
		if err := b.persist(b.runs); err != nil {
			b.err = err
		}
	}
	err := b.err
	b.mu.Unlock()
	b.signal()
	return err
}

func (b *Background) activity(a Activity) {
	a.At = time.Now()
	b.mu.Lock()
	// Inspector events are bounded. Usage is never discarded.
	if len(b.updates) >= 512 && a.Usage == nil {
		for n, old := range b.updates {
			if old.Usage == nil {
				b.updates = append(b.updates[:n], b.updates[n+1:]...)
				break
			}
		}
	}
	b.updates = append(b.updates, a)
	b.mu.Unlock()
	b.signal()
}

// Drain transfers pending telemetry and completion notifications to the owner.
func (b *Background) Drain() (map[string]*Run, []Activity, []string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	runs := cloneRuns(b.runs)
	updates, done := b.updates, b.done
	b.updates, b.done = nil, nil
	return runs, updates, done, b.err
}

// CanEvict prevents the coordinator from dropping a just-completed job before
// its completion event or usage has reached the parent snapshot.
func (b *Background) CanEvict() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.active) > 0 || len(b.done) > 0 {
		return false
	}
	for _, r := range b.runs {
		if r.NotificationPending || len(r.Usage) > r.UsageAcknowledged {
			return false
		}
	}
	return true
}

// Active reports whether the supervisor still owns running work.
func (b *Background) Active() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.active) > 0
}

// Cancel stops the conversation's children without waiting for their cleanup.
func (b *Background) Cancel() {
	if b != nil {
		b.cancel()
	}
}

// Close cancels and joins children before directory handles or stores are closed.
func (b *Background) Close() {
	if b != nil {
		b.cancel()
		b.wg.Wait()
	}
}

// Tools starts jobs without waiting for their completions. Work inherits the
// conversation lifetime, not the short parent turn deadline.
func (b *Background) Tools(maxRuns, turn int, target string, before func(context.Context)) []agent.Tool {
	return b.tools(maxRuns, turn, target, before, false)
}

func (b *Background) tools(maxRuns, turn int, target string, before func(context.Context), automatic bool) []agent.Tool {
	if b == nil || b.manager == nil {
		return nil
	}
	started := 0
	base := b.manager.Tools(0, nil, nil)
	if len(base) == 0 {
		return nil
	}
	delegate := base[0]
	delegate.Description = "Start or resume a read-only investigation in the background. Returns immediately; completion wakes this conversation. Use investigation_status for updates, and defer_response after announcing work. " + delegate.Description
	delegate.Execute = func(ctx context.Context, raw json.RawMessage) (string, error) {
		if ctx.Err() != nil || b.ctx.Err() != nil {
			return rejected("Conversation stopped")
		}
		if maxRuns > 0 && started >= maxRuns {
			return rejected("Delegation limit reached for this turn")
		}
		b.mu.Lock()
		local := cloneRuns(b.runs)
		b.mu.Unlock()
		var id string
		firstPublish := true
		binding := Binding{Runs: local, BeforeRun: before, Publish: func(r *Run) error {
			if automatic && firstPublish {
				r.AutoResumes++
			}
			firstPublish = false
			id = r.ID
			r.Target = target
			r.OriginTurn = turn
			r.NotificationPending = true
			return b.publish(r)
		}, Launch: func(r *Run, execute func(context.Context) string) {
			started++
			b.mu.Lock()
			b.active[r.ID] = true
			b.mu.Unlock()
			b.wg.Add(1)
			go func() {
				defer b.wg.Done()
				execute(b.ctx)
				b.mu.Lock()
				delete(b.active, r.ID)
				b.done = append(b.done, r.ID)
				b.mu.Unlock()
				b.signal()
			}()
		}}
		tools := b.manager.Tools(0, func(u agent.Usage) { b.activity(Activity{RunID: id, Turn: turn, Usage: &u}) }, func(e Event) { b.activity(Activity{RunID: id, Turn: turn, Event: e}) }, binding)
		return tools[0].Execute(ctx, raw)
	}
	status := Binding{Runs: map[string]*Run{}}.statusTool()
	status.Execute = func(ctx context.Context, raw json.RawMessage) (string, error) {
		b.mu.Lock()
		runs := cloneRuns(b.runs)
		b.mu.Unlock()
		return (Binding{Runs: runs}).statusTool().Execute(ctx, raw)
	}
	return []agent.Tool{delegate, status}
}

// Acknowledge follows a successful parent snapshot, so a crash cannot lose
// unaccounted usage or silently evict an unreported result.
func (b *Background) Acknowledge(usage map[string]int, completed []string) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	changed := false
	for id, n := range usage {
		if r := b.runs[id]; r != nil && n > r.UsageAcknowledged {
			r.UsageAcknowledged = n
			changed = true
		}
	}
	for _, id := range completed {
		if r := b.runs[id]; r != nil && r.NotificationPending {
			r.NotificationPending = false
			changed = true
		}
	}
	if !changed {
		return nil
	}
	Binding{Runs: b.runs}.retain("")
	if b.persist != nil {
		if err := b.persist(b.runs); err != nil {
			b.err = err
		}
	}
	return b.err
}

// Context is bounded disclosure of each child's recent public output, for new
// user turns. Full provider history and private reasoning remain in storage.
func (b *Background) Context() []map[string]any {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	ids := make([]string, 0, len(b.runs))
	for id := range b.runs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := b.runs[id]
		notes := r.Updates
		if len(notes) > 4 {
			notes = notes[len(notes)-4:]
		}
		out = append(out, map[string]any{"run_id": r.ID, "agent": r.Agent, "status": r.Status, "phase": r.Phase, "phase_since": r.PhaseSince, "reason": r.Reason, "failure": r.LastFailure, "can_auto_resume": b.canResume(r), "request_recoveries": r.Recoveries, "task": clip(r.Task, 300), "report": clip(r.Report, 2000), "report_truncated": len([]rune(r.Report)) > 2000, "verification": verificationLimits(), "recent_updates": append([]UpdateNote(nil), notes...)})
	}
	return out
}
