// Package bot owns retained thread conversations and debounced message batches.
// It keeps model state separate from Chatto's transport and from the terminal UI.
package bot

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/memory"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/subagent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

const batchWindow = 4 * time.Second
const sessionLimit = 100
const pendingLimit = 128
const transcriptLimit = 1 << 20

// Client is the public SDK surface used by the bot. One Run is bound to one
// server and authenticated bot identity, so thread keys cannot cross accounts.
type Client interface {
	ObserveMessages(context.Context, string, func(), func(context.Context, chatto.MessageEvent) error) error
	Thread(context.Context, string, string, string) (chatto.Page, error)
	Reply(context.Context, string, string, string, string, string) error
	React(context.Context, string, string, string) error
	RefreshTypingIndicator(context.Context, string, string) error
}

// ModelFactory supplies one stable provider session and usage reporter per thread.
type ModelFactory func(string, func(agent.Usage)) agent.Complete

type threadKey struct{ room, root string }
type conversation struct {
	// Shared membership index allows intake to scope invalidations while the
	// worker owns the transcript. Ready is false while a snapshot is loading.
	contextIDs        sync.Map
	contextReady      atomic.Bool
	id                uint64
	humanID           string
	title             string
	titleAttempted    bool
	records           []inspection.Turn // owned by the manager when idle, by the worker during a turn
	emitInspection    func(inspection.Turn)
	logMu             sync.Mutex
	logs              []inspection.Log
	queuedAt          time.Time
	identity          agent.Identity
	ownerUserIDs      []string // current startup config; never inherited from a session or thread
	memory            *memory.Manager
	tools             []agent.Tool // immutable startup capabilities, shared by conversations
	subagents         *subagent.Manager
	childRuns         map[string]*subagent.Run // private child transcripts, scoped to this thread
	jobs              *subagent.Background
	childReady        []string
	childSteps        map[string]int
	childUsageApplied map[string]int
	accountUsage      func(agent.Usage)
	reportUsage       func(agent.Usage)
	limits            agent.Limits
	maxDelegations    int
	systemPrompt      string
	key               threadKey
	model             *agent.Session
	complete          agent.Complete
	seen              map[string]bool   // worker-owned IDs already supplied to the model
	current           map[string]string // fingerprints for offline edit/deletion detection
	// editCheck prevents delivery while intake verifies a metadata-only edit.
	editCheck            sync.RWMutex
	pending              []chatto.MessageEvent // manager-owned while the worker runs
	deadline             time.Time
	used                 time.Time
	reset                bool
	blocked              bool
	turns                int
	provider             string
	started              time.Time
	usage                totals // worker-owned while a turn is active
	store                *Store
	storeErr             error
	resumed, interrupted bool
}
type incoming struct {
	event    chatto.MessageEvent
	at       time.Time
	accepted chan struct{}
	state    string
}
type result struct {
	c   *conversation
	err error
}

// Options supplies optional persistence and operator-authored instructions.
type Options struct {
	// Shutdown requests graceful exit after the active turn, without starting
	// queued turns. Context cancellation still interrupts work immediately.
	Shutdown <-chan struct{}
	// Tools contains operator-selected external capabilities for every turn.
	Tools []agent.Tool
	// Subagents supplies isolated read-only workers; nil disables delegation.
	Subagents *subagent.Manager
	// Zero execution limits leave work unbounded; cancellation still applies.
	TurnTimeout    time.Duration
	Limits         agent.Limits
	MaxDelegations int
	// Debounce is the quiet period before a turn. Zero uses four seconds.
	Debounce time.Duration
	Memory   *memory.Manager
	// OwnerUserIDs applies to each message author, not to an entire conversation.
	OwnerUserIDs []string
	Store        *Store
	SystemPrompt string
	Titles       agent.TitleGenerator
	Model        string
	TitleModel   string
}

// Run keeps the socket reader responsive while one model turn runs at a time.
// Each thread has its own transcript. Its first addressed message starts a
// configurable debounce; each later message resets it. Busy-time windows elapse
// while the current turn runs. Once a session exists, every non-self message in
// its thread can start a turn, including after restoring the session.
// All goroutines stop before Run returns. An optional store retains transcripts.
func Run(ctx context.Context, client Client, profile chatto.Profile, factory ModelFactory, emit tui.Emit, options ...Options) (runErr error) {
	viewer := profile.ID
	var opts Options
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.Debounce < 0 {
		return errors.New("debounce duration must not be negative")
	}
	if opts.TurnTimeout < 0 || opts.Limits.MaxToolCalls < 0 || opts.Limits.MaxModelSteps < 0 || opts.MaxDelegations < 0 {
		return errors.New("execution limits must be nonnegative")
	}
	if opts.Debounce == 0 {
		opts.Debounce = batchWindow
	}
	store := opts.Store
	// Detach from the caller's mutable configuration slice.
	owners := append([]string(nil), opts.OwnerUserIDs...)
	loaded, err := store.load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	sessions := map[threadKey]*conversation{}
	defer func() {
		cancel()
		workers.Wait()
		for _, c := range sessions {
			c.jobs.Close()
			_ = c.syncChildren()
			if c.reset {
				c.model = nil
				c.seen = nil
				c.current = nil
				c.provider = rand.Text()
			}
			if err := store.save(c, false); err != nil {
				runErr = err
			}
		}
	}()
	input := make(chan incoming, 128)
	sourceDone := make(chan error, 1)
	turnDone := make(chan result, 1)
	childWake := make(chan struct{}, 1)
	resumable, canResume := client.(interface {
		ObserveResumable(context.Context, string, chatto.RecoveryOptions, func(), func(context.Context, chatto.MessageEvent) error) error
	})
	connected := !canResume
	cursor, pending, interrupted := store.pendingInbox()
	workers.Add(1)
	go func() {
		defer workers.Done()
		deliver := func(ctx context.Context, event chatto.MessageEvent) error {
			if !store.inboxPending(event) {
				return nil
			}
			ack := make(chan struct{})
			select {
			case input <- incoming{event: event, at: time.Now(), accepted: ack}:
			case <-ctx.Done():
				return ctx.Err()
			}
			select {
			case <-ack:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		status := func(text string) {
			select {
			case input <- incoming{state: text}:
			case <-ctx.Done():
			}
		}
		if interrupted > 0 {
			emit(tui.Event{Kind: tui.Connection, Text: "Interrupted inbox work retained; no automatic retry"})
		}
		for _, event := range pending {
			if err := deliver(ctx, event); err != nil {
				sourceDone <- err
				return
			}
		}
		if canResume {
			sourceDone <- resumable.ObserveResumable(ctx, viewer, chatto.RecoveryOptions{Cursor: cursor, Checkpoint: store.checkpoint, Status: status}, func() { status("Connected") }, deliver)
		} else {
			sourceDone <- client.ObserveMessages(ctx, viewer, func() { status("Connected") }, func(ctx context.Context, event chatto.MessageEvent) error {
				if err := store.checkpoint(cursor, &event); err != nil {
					return err
				}
				return deliver(ctx, event)
			})
		}
	}()
	var nextID uint64
	var active *conversation
	var stopTurn context.CancelCauseFunc
	startChildren := func(c *conversation) {
		human := c.humanID
		c.jobs = subagent.NewBackground(ctx, c.subagents, c.childRuns, func(runs map[string]*subagent.Run) error { return store.saveChildren(human, runs) }, func() {
			select {
			case childWake <- struct{}{}:
			default:
			}
		})
	}
	bind := func(c *conversation) {
		for id, hash := range c.current {
			c.contextIDs.Store(id, hash)
		}
		for id := range c.seen {
			c.contextIDs.LoadOrStore(id, true)
		}
		c.contextReady.Store(c.model != nil)
		c.emitInspection = func(turn inspection.Turn) { emit(tui.Event{Kind: tui.Inspected, Session: c.id, Inspection: &turn}) }
		c.systemPrompt = opts.SystemPrompt
		c.identity = agent.Identity{ID: profile.ID, Login: profile.Login, DisplayName: profile.DisplayName}
		c.ownerUserIDs = owners
		c.memory = opts.Memory
		c.tools = append([]agent.Tool(nil), opts.Tools...)
		c.subagents = opts.Subagents
		c.limits, c.maxDelegations = opts.Limits, opts.MaxDelegations
		c.store = store
		startChildren(c)
		c.accountUsage = func(u agent.Usage) {
			c.usage.addReasoning(u)
			if !u.Unreported {
				c.usage.Known = true
			}
			c.usage.Input += u.Input
			c.usage.Cached += u.CachedInput
			c.usage.Output += u.Output
			if u.Cost != nil {
				c.usage.CostKnown = true
				c.usage.Cost += *u.Cost
			} else {
				c.usage.CostMissing = true
			}
			inTurn := len(c.records) > 0 && c.records[len(c.records)-1].State == "running"
			c.storeErr = store.save(c, inTurn)
			emit(tui.Event{Kind: tui.Tokens, Session: c.id, Unreported: u.Unreported, Input: u.Input, CachedInput: u.CachedInput, Output: u.Output, Cost: u.Cost, Reasoning: u.Reasoning, ReasoningKnown: u.ReasoningKnown})
		}
		c.reportUsage = func(u agent.Usage) { c.recordUsage(u); c.accountUsage(u) }
		c.complete = factory(c.provider, c.reportUsage)
	}
	initialize := func(c *conversation) {
		if c.jobs != nil {
			c.jobs.Close()
			_ = c.syncChildren()
			c.childRuns = nil
			c.childReady = nil
			if err := store.saveChildren(c.humanID, nil); err != nil {
				c.storeErr = err
			}
		}
		c.contextIDs.Clear()
		c.contextReady.Store(false)
		c.model = nil
		c.seen = map[string]bool{}
		c.current = map[string]string{}
		c.reset = false
		c.blocked = false
		c.resumed = false
		c.provider = rand.Text()
		if c.humanID == "" {
			c.humanID = humanID(c.provider)
		}
		bind(c)
	}
	for _, c := range loaded {
		bind(c)
		sessions[c.key] = c
		nextID = max(nextID, c.id)
		c.send(emit, tui.Event{Kind: tui.Started, Session: c.id, HumanID: c.humanID, Title: c.title, At: c.started, Turns: c.turns, Records: c.records, Logs: c.storedLogs()})
		c.send(emit, tui.Event{Kind: tui.Log, Session: c.id, Text: "Session restored"})
		if c.usage.Known {
			var cost *float64
			if c.usage.CostKnown {
				value := c.usage.Cost
				cost = &value
			}
			emit(tui.Event{Kind: tui.Tokens, Session: c.id, Input: c.usage.Input, CachedInput: c.usage.Cached, Output: c.usage.Output, Cost: cost, CostMissing: c.usage.CostMissing, Reasoning: c.usage.Reasoning, ReasoningKnown: c.usage.ReasoningKnown, ReasoningMissing: c.usage.ReasoningMissing})
		}
		if c.interrupted {
			c.send(emit, tui.Event{Kind: tui.Failed, Session: c.id, Text: "Previous turn interrupted; no automatic retry"})
		} else {
			emit(tui.Event{Kind: tui.Completed, Session: c.id})
		}
	}
	var intake *incoming
	keepIntake := false
	var activeBatch []chatto.MessageEvent
	recoveryGap := false
	shutdown := opts.Shutdown
	draining := false
	for {
		// Check before scheduling so queued work cannot win a select race with
		// an already requested shutdown.
		if !draining {
			select {
			case <-shutdown:
				draining = true
				shutdown = nil
			default:
			}
		}
		if intake != nil {
			if !keepIntake {
				if err := store.finish([]chatto.MessageEvent{intake.event}); err != nil {
					return err
				}
			}
			close(intake.accepted)
			intake = nil
			keepIntake = false
		}
		childrenActive, childrenReady := false, false
		for _, c := range sessions {
			childrenActive = childrenActive || c.jobs.Active()
			if c != active {
				if err := c.syncChildren(); err != nil {
					return err
				}
				if len(c.childReady) > 0 && c.deadline.IsZero() && !c.reset {
					c.deadline = time.Now()
					c.queuedAt = c.deadline
				}
				childrenReady = childrenReady || len(c.childReady) > 0
			}
		}
		if draining && active == nil && !childrenActive && !childrenReady {
			return nil
		}
		var timer *time.Timer
		var wake <-chan time.Time
		var next *conversation
		if active == nil && connected {
			for _, c := range sessions {
				if draining && len(c.childReady) == 0 {
					continue
				}
				if !c.deadline.IsZero() && (next == nil || c.deadline.Before(next.deadline)) {
					next = c
				}
			}
			if next != nil {
				timer = time.NewTimer(max(0, time.Until(next.deadline)))
				wake = timer.C
			}
		}
		select {
		case <-childWake:
			if timer != nil {
				timer.Stop()
			}
			continue
		case <-shutdown:
			draining = true
			shutdown = nil
			if timer != nil {
				timer.Stop()
			}
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return ctx.Err()
		case err := <-sourceDone:
			if timer != nil {
				timer.Stop()
			}
			return err
		case in := <-input:
			if timer != nil {
				timer.Stop()
			}
			if in.state != "" {
				connected = in.state == "Connected"
				if connected && recoveryGap {
					in.state = "Connected · replay gap; offline messages may be missing"
				}
				if connected && interrupted > 0 {
					in.state += fmt.Sprintf(" · %d interrupted inbox messages not retried", interrupted)
				}
				emit(tui.Event{Kind: tui.Connection, Text: in.state})
				continue
			}
			intake = &in
			e := in.event
			if e.RecoveryGap {
				connected = false
				recoveryGap = true
				for _, c := range sessions {
					c.jobs.Cancel()
					if active == c {
						c.reset = true
						stopTurn(invalidatedTurn("Cancelled: realtime replay gap requires a context refresh"))
					} else {
						c.jobs.Close()
						if err := c.syncChildren(); err != nil {
							return err
						}
						c.childReady = nil
						startChildren(c)
						c.resumed = true
					}
					c.send(emit, tui.Event{Kind: tui.Log, Session: c.id, Text: "Replay unavailable; thread context will refresh. Offline messages may be missing."})
				}
				emit(tui.Event{Kind: tui.Connection, Text: "Replay unavailable; refreshing retained threads. Offline messages may be missing."})
				continue
			}
			if e.InvalidatedRoom != "" || e.InvalidateAll {
				for _, c := range sessions {
					if c.affectedBy(e, active == c, activeBatch) {
						if e.InvalidationReason == chatto.MessageEdited {
							c.editCheck.Lock()
							unchanged := c.unchangedEdit(ctx, client, e, activeBatch)
							if !unchanged && active == c {
								stopTurn(invalidationCause(e))
							}
							c.editCheck.Unlock()
							if unchanged {
								continue
							}
						}
						if err := store.finish(c.pending); err != nil {
							return err
						}
						c.jobs.Cancel()
						c.pending = nil
						c.deadline = time.Time{}
						c.reset = true
						if active == c {
							stopTurn(invalidationCause(e))
						} else {
							initialize(c)
							if err := store.save(c, false); err != nil {
								return err
							}
						}
						c.send(emit, tui.Event{Kind: tui.Failed, Session: c.id, Text: invalidationCause(e).Error() + "; context reset. Waiting for a new message."})
					}
				}
				continue
			}
			m := e.Message
			if m.ID == "" || m.ActorID == viewer {
				continue
			}
			key := threadKey{m.RoomID, m.ThreadRootID()}
			c := sessions[key]
			if c == nil {
				if draining {
					keepIntake = true
					continue
				}
				if !e.Addressed {
					continue
				}
				if len(sessions) >= sessionLimit {
					var oldest *conversation
					for _, candidate := range sessions {
						if candidate != active && candidate.jobs.CanEvict() && candidate.deadline.IsZero() && (oldest == nil || candidate.used.Before(oldest.used)) {
							oldest = candidate
						}
					}
					if oldest == nil {
						return errors.New("all conversation slots have pending turns")
					}
					delete(sessions, oldest.key)
					oldest.jobs.Close()
					if err := store.remove(oldest); err != nil {
						return err
					}
					emit(tui.Event{Kind: tui.Evicted, Session: oldest.id})
				}
				nextID++
				c = &conversation{id: nextID, key: key, started: in.at}
				initialize(c)
				sessions[key] = c
				if err := store.save(c, false); err != nil {
					return err
				}
				emit(tui.Event{Kind: tui.Started, Session: c.id, HumanID: c.humanID})
				// Saving the new session before this best-effort side effect prevents
				// replay on restart. Keep intake and model work independent of it.
				workers.Add(1)
				go func(c *conversation, message chatto.Message) {
					defer workers.Done()
					ackCtx, cancelAck := context.WithTimeout(ctx, 10*time.Second)
					defer cancelAck()
					text := "Joined thread · eyes reaction added"
					if err := client.React(ackCtx, message.RoomID, message.ID, "eyes"); err != nil {
						if ctx.Err() != nil {
							return
						}
						text = "Thread acknowledgement not confirmed; continuing without retry"
					}
					c.send(emit, tui.Event{Kind: tui.Log, Session: c.id, Text: text})
				}(c, m)
			}
			c.used = in.at
			if c.blocked {
				continue
			}
			duplicate := false
			for _, pending := range c.pending {
				if pending.Message.ID == m.ID {
					duplicate = true
					break
				}
			}
			if duplicate {
				keepIntake = true
				continue
			}
			if len(c.pending) >= pendingLimit {
				return errors.New("thread input buffer is full")
			}
			if len(c.pending) == 0 {
				c.queuedAt = in.at
			}
			c.pending = append(c.pending, e)
			keepIntake = true
			c.deadline = in.at.Add(opts.Debounce)
			kind := tui.Queued
			if active == c {
				kind = tui.Log
			}
			emit(tui.Event{Kind: kind, Session: c.id, Pending: len(c.pending)})
		case <-wake:
			select {
			case <-shutdown:
				draining = true
				shutdown = nil
				continue
			default:
			}
			c := next
			batch := c.pending
			if draining {
				batch = nil
			}
			// Persist the no-retry boundary before any model or delivery side effect.
			if err := store.claim(batch); err != nil {
				return err
			}
			activeBatch = batch
			if !draining {
				c.pending = nil
			}
			c.deadline = time.Time{}
			active = c
			deadlineCtx := ctx
			deadlineCancel := func() {}
			if opts.TurnTimeout > 0 {
				deadlineCtx, deadlineCancel = context.WithTimeout(ctx, opts.TurnTimeout)
			}
			turnCtx, turnCancel := context.WithCancelCause(deadlineCtx)
			stopTurn = turnCancel
			c.turns++
			record := inspection.Turn{Number: c.turns, Started: time.Now(), Wait: time.Since(c.queuedAt), State: "running", Model: opts.Model}
			for _, e := range batch {
				body := []rune(e.Message.Body)
				if len(body) > 10000 {
					body = append(body[:10000], []rune("\n[truncated]")...)
				}
				record.Inputs = append(record.Inputs, inspection.Input{Text: string(body), Addressed: e.Addressed})
			}
			if len(batch) == 0 {
				record.Inputs = append(record.Inputs, inspection.Input{Text: "Background investigation completed (host event)"})
			}
			c.records = inspection.Retain(append(c.records, record))
			c.inspect(func(*inspection.Turn) {})
			if err := store.save(c, true); err != nil {
				turnCancel(nil)
				deadlineCancel()
				return err
			}
			c.send(emit, tui.Event{Kind: tui.TurnStarted, Session: c.id, Text: fmt.Sprintf("Turn %d started · %d incoming messages", c.turns, len(batch))})
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer deadlineCancel()
				defer turnCancel(nil)
				err := respond(turnCtx, client, c, batch, func(text string) { c.send(emit, tui.Event{Kind: tui.Log, Session: c.id, Text: text}) })
				if err == nil && opts.Titles != nil && c.title == "" && !c.titleAttempted && turnCtx.Err() == nil {
					c.titleAttempted = true
					if saveErr := store.save(c, false); saveErr != nil {
						err = saveErr
					} else {
						titleCtx, cancelTitle := context.WithTimeout(turnCtx, 10*time.Second)
						name := opts.TitleModel
						if name == "" {
							name = "Session title"
						}
						titleStep := c.step("title", name)
						title, titleErr := opts.Titles(titleCtx, c.model.Snapshot())
						cancelTitle()
						if u := title.Usage; u != nil {
							c.recordUsage(*u)
							c.usage.addReasoning(*u)
							c.usage.Known = true
							c.usage.Input += u.Input
							c.usage.Output += u.Output
							c.usage.Cached += u.CachedInput
							if u.Cost != nil {
								c.usage.CostKnown = true
								c.usage.Cost += *u.Cost
							} else {
								c.usage.CostMissing = true
							}
							emit(tui.Event{Kind: tui.Tokens, Session: c.id, Input: u.Input, Output: u.Output, CachedInput: u.CachedInput, Cost: u.Cost, Reasoning: u.Reasoning, ReasoningKnown: u.ReasoningKnown})
						}
						if titleErr == nil {
							c.finishStep(titleStep, "complete", "Title generated")
							c.title = agent.CleanTitle(title.Title)
							emit(tui.Event{Kind: tui.TitleChanged, Session: c.id, Title: c.title})
						} else {
							c.finishStep(titleStep, "failed", "Title unavailable")
							c.send(emit, tui.Event{Kind: tui.Log, Session: c.id, Text: "Session title unavailable"})
						}
						if saveErr := store.save(c, false); saveErr != nil {
							err = saveErr
						}
					}
				}
				c.inspect(func(t *inspection.Turn) { t.Finished = time.Now() })
				if saveErr := store.save(c, false); saveErr != nil {
					err = saveErr
				}
				turnDone <- result{c, err}
			}()
		case done := <-turnDone:
			if timer != nil {
				timer.Stop()
			}
			active = nil
			if err := store.finish(activeBatch); err != nil {
				return err
			}
			activeBatch = nil
			stopTurn = nil
			c := done.c
			c.used = time.Now()
			if c.reset {
				initialize(c)
				if err := store.save(c, false); err != nil {
					return err
				}
				continue
			}
			if errors.Is(done.err, errStore) {
				return done.err
			}
			if done.err != nil {
				if errors.Is(done.err, errBudget) {
					c.blocked = true
					if err := store.finish(c.pending); err != nil {
						return err
					}
					c.pending = nil
					c.deadline = time.Time{}
				}
				c.send(emit, tui.Event{Kind: tui.Failed, Session: c.id, Text: "Turn failed; no automatic retry. Check delivery before requesting a retry."})
			} else {
				emit(tui.Event{Kind: tui.Completed, Session: c.id, Background: c.jobs.Active()})
			}
			if !c.deadline.IsZero() {
				emit(tui.Event{Kind: tui.Queued, Session: c.id})
			}
		}
	}
}

var errBudget = errors.New("conversation context budget reached")

func respond(ctx context.Context, client Client, c *conversation, batch []chatto.MessageEvent, report func(string)) (turnErr error) {
	if err := c.syncChildren(); err != nil {
		return err
	}
	completed := append([]string(nil), c.childReady...)
	c.childReady = nil
	stopTyping := startTyping(ctx, client, c.key)
	defer stopTyping()
	tools := append(c.memory.Tools(c.ownerOnlyBatch(batch)), c.tools...)
	// A completion event authorizes reporting, not a new user-directed operation.
	if len(batch) == 0 {
		tools = nil
	}
	updates := newProgress(c, client, batch)
	if updates.target == "" {
		for _, id := range completed {
			if r := c.childRuns[id]; r != nil {
				updates.target = r.Target
			}
		}
	}
	tools = append(tools, updates.tool(), deferredTool(func() bool { return c.jobs.Active() || len(completed) > 0 || !updates.last.IsZero() }))
	childTools := c.jobs.Tools(c.maxDelegations, c.turns, updates.target, func(ctx context.Context) {
		if updates.last.IsZero() && !updates.uncertain {
			updates.send(ctx, "I'll investigate the relevant code and report what I can verify.")
		}
	})
	if len(batch) == 0 {
		reportTools := childTools[:0]
		for _, tool := range childTools {
			if tool.Name == "investigation_status" {
				reportTools = append(reportTools, tool)
			}
		}
		childTools = reportTools
		childTools = append(childTools, c.jobs.RecoveryTools(completed, c.maxDelegations, c.turns, updates.target, nil)...)
	}
	tools = append(tools, childTools...)
	ctx = agent.WithTools(ctx, tools)
	ctx = agent.WithLimits(ctx, c.limits)
	defer func() {
		if err := c.syncChildren(); err != nil {
			turnErr = err
		}
		c.inspect(func(t *inspection.Turn) {
			t.Finished = time.Now()
			if turnErr != nil && t.State == "running" {
				t.State = "failed"
				t.Failure = "Turn failed before final delivery; inspect completed actions and progress updates"
				if detail := modelFailureDetail(ctx, turnErr); detail != "Model request failed" {
					t.Failure = detail
				}
				if errors.Is(turnErr, context.Canceled) || errors.Is(turnErr, context.DeadlineExceeded) {
					t.State = "cancelled"
					t.Failure = "Turn cancelled or timed out"
					var cause invalidatedTurn
					if errors.As(context.Cause(ctx), &cause) {
						t.Failure = cause.Error()
					}
				}
				if errors.Is(turnErr, errBudget) {
					t.Failure = "Context budget reached"
				}
				if errors.Is(turnErr, errStore) {
					t.Failure = "Session checkpoint failed; output was not sent"
				}
			}
		})
		c.records = inspection.Retain(c.records)
		if err := c.store.save(c, false); err != nil {
			turnErr = err
		}
		if turnErr == nil {
			ack := []string{}
			for _, id := range completed {
				if r := c.childRuns[id]; r != nil && r.OriginTurn != c.turns {
					ack = append(ack, id)
				}
			}
			if err := c.jobs.Acknowledge(c.childUsageApplied, ack); err != nil {
				turnErr = errStore
			}
		}
	}()
	if report == nil {
		report = func(string) {}
	}
	if c.seen == nil {
		c.seen = map[string]bool{}
	}
	// Only deduplicate a refresh after its tool result is committed. A failed
	// model turn rolls back tool exchanges and must not hide later realtime input.
	var refreshed []string
	var latest map[string]string
	initializing := true
	read := func(ctx context.Context) (string, error) {
		c.contextReady.Store(false)
		index := -1
		if initializing {
			index = c.step("context", "Load thread context")
		}
		page, err := client.Thread(ctx, c.key.room, c.key.root, "")
		if err != nil {
			c.finishStep(index, "failed", "Thread read failed")
			report("Thread read failed")
			return "", err
		}
		c.finishStep(index, "complete", fmt.Sprintf("%d messages · older omitted: %t", len(page.Messages), page.HasOlder))
		latest = map[string]string{}
		for _, m := range page.Messages {
			if m.ID != "" {
				refreshed = append(refreshed, m.ID)
				latest[m.ID] = fingerprint(m)
				c.contextIDs.Store(m.ID, latest[m.ID])
			}
		}
		c.contextReady.Store(true)
		if page.HasOlder {
			report("Older thread history omitted")
		}
		data, err := json.Marshal(struct {
			Messages     []chatto.Message `json:"messages"`
			OlderOmitted bool             `json:"olderOmitted"`
		}{page.Messages, page.HasOlder})
		return string(data), err
	}
	if c.model == nil {
		snapshot, err := read(ctx)
		if err != nil {
			return err
		}
		c.model = agent.NewSession(snapshot)
		c.current = latest
		c.resumed = false
		for _, id := range refreshed {
			c.seen[id] = true
		}
		refreshed = nil
	} else if c.resumed {
		// Reauthorize and refresh after restart or an unrecoverable replay gap.
		snapshot, err := read(ctx)
		if err != nil {
			return err
		}
		changed := false
		for id, value := range c.current {
			if latest[id] != value {
				changed = true
				break
			}
		}
		if changed {
			c.model = agent.NewSession(snapshot)
			c.seen = map[string]bool{}
			report("Thread changed while offline; context reset")
		} else {
			c.model.Refresh(snapshot)
		}
		c.current = latest
		for _, id := range refreshed {
			c.seen[id] = true
		}
		refreshed = nil
		c.resumed = false
	}
	initializing = false
	var target chatto.Message
	for _, id := range completed {
		if r := c.childRuns[id]; r != nil {
			target.ID = r.Target
			updates.target = r.Target
		}
	}
	c.model.SetInstructions(c.identity, c.systemPrompt, c.ownerUserIDs...)
	input := struct {
		CurrentTime    string           `json:"currentTime"`
		Messages       []chatto.Message `json:"newMessages"`
		Addressed      []string         `json:"addressedMessageIds"`
		Triggers       []string         `json:"triggerMessageIds"`
		Authors        []triggerAuthor  `json:"triggerAuthors"`
		Brains         json.RawMessage  `json:"brainCatalog"`
		Investigations []map[string]any `json:"investigations,omitempty"`
		Completed      []string         `json:"completedInvestigations,omitempty"`
	}{CurrentTime: time.Now().UTC().Format(time.RFC3339)}
	input.Investigations = c.jobs.Context()
	input.Completed = completed
	catalog, catalogErr := c.memory.Catalog(ctx)
	if catalogErr != nil {
		return catalogErr
	}
	input.Brains = json.RawMessage(catalog)
	for _, event := range batch {
		m := event.Message
		if c.current == nil {
			c.current = map[string]string{}
		}
		c.current[m.ID] = fingerprint(m)
		c.contextIDs.Store(m.ID, c.current[m.ID])
		if !c.seen[m.ID] {
			input.Messages = append(input.Messages, m)
			c.seen[m.ID] = true
		}
		if event.Addressed {
			input.Addressed = append(input.Addressed, m.ID)
		}
		input.Triggers = append(input.Triggers, m.ID)
		input.Authors = append(input.Authors, triggerAuthor{MessageID: m.ID, UserID: m.ActorID, IsOwner: c.isOwner(m.ActorID)})
		target = m
	}
	if target.ID == "" {
		return errors.New("batch has no message")
	}
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	complete := func(ctx context.Context, messages []agent.Message) (agent.Message, error) {
		if err := c.syncChildren(); err != nil {
			return nil, err
		}
		if c.storeErr != nil {
			return nil, c.storeErr
		}
		encoded, _ := json.Marshal(messages)
		if len(encoded) > transcriptLimit || len(c.seen) > 4096 {
			report("Context budget reached; start a new thread. Automatic compaction is not enabled.")
			return nil, errBudget
		}
		index := c.step("model", "Completion")
		// Preserve per-completion controls, including the final text-only
		// budget wrap-up, instead of replacing them with the turn context.
		result, err := c.complete(ctx, messages)
		if c.storeErr != nil {
			c.finishStep(index, "failed", "Usage checkpoint failed")
			return nil, c.storeErr
		}
		if err != nil {
			detail := modelFailureDetail(ctx, err)
			state := "failed"
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				state = "cancelled"
			}
			c.finishStep(index, state, detail)
			report(detail)
			return nil, err
		}
		c.finishStep(index, "complete", "Response received")
		return result, nil
	}
	toolIndex := -1
	outcome, err := c.model.Turn(ctx, complete, read, string(data), func(e agent.ToolEvent) {
		if e.State == "running" {
			name := e.Name
			if e.Name == "mcp_call" || e.Name == "mcp_discover" {
				var args struct {
					Name string `json:"name"`
				}
				if json.Unmarshal([]byte(e.Arguments), &args) == nil && args.Name != "" {
					name += " · " + args.Name
				}
			}
			toolIndex = c.step("tool", name)
		}
		detail := e.Arguments
		if e.Result != "" {
			detail += "\n" + e.Result
		}
		runes := []rune(detail)
		if len(runes) > 4000 {
			detail = string(runes[:4000]) + "\n[truncated]"
		}
		if e.State != "running" {
			c.finishStep(toolIndex, e.State, detail)
			if strings.HasPrefix(e.Name, "memory_") {
				// Capability receipts are installed before this callback.
				c.storeErr = c.store.save(c, true)
			}
		} else {
			c.inspect(func(t *inspection.Turn) {
				if toolIndex >= 0 {
					t.Steps[toolIndex].Detail = detail
				}
			})
		}
	})
	if err != nil {
		c.inspect(func(t *inspection.Turn) {
			if len(t.Steps) > 0 && t.Steps[len(t.Steps)-1].State == "complete" {
				t.Steps = append(t.Steps, inspection.Step{Kind: "validation", Name: "Agent result", State: "failed", Started: time.Now(), Detail: "Invalid or incomplete model action; no final answer delivered. Earlier tool and progress receipts still apply."})
			}
		})
		return err
	}
	if outcome.Waiting {
		c.inspect(func(t *inspection.Turn) { t.State = "waiting" })
		return nil
	}
	c.inspect(func(t *inspection.Turn) { t.Response = outcome.Body; t.Emoji = outcome.Emoji })
	for _, id := range refreshed {
		c.seen[id] = true
	}
	if len(refreshed) > 0 {
		for id, value := range latest {
			c.current[id] = value
		}
	}
	c.editCheck.RLock()
	defer c.editCheck.RUnlock()
	if err := ctx.Err(); err != nil {
		c.model.RecordDelivery(err)
		return err
	}
	// Commit the proposed action before sending it. A crash here leaves an
	// interrupted checkpoint, never an instruction to replay the action.
	if err := c.store.save(c, true); err != nil {
		c.model.RecordDelivery(err)
		return err
	}
	// Drain the refresh worker before delivery; no late refresh may resurrect
	// typing after the reply clears it. Title generation does not show typing.
	stopTyping()
	delivery := c.step("delivery", "Post reply")
	if outcome.Emoji != "" {
		c.inspect(func(t *inspection.Turn) {
			if delivery >= 0 {
				t.Steps[delivery].Name = "Add reaction"
			}
		})
		err = client.React(ctx, c.key.room, target.ID, outcome.Emoji)
	} else {
		err = client.Reply(ctx, c.key.room, c.key.root, target.ID, outcome.Body, "")
	}
	c.model.RecordDelivery(err)
	if err != nil {
		c.finishStep(delivery, "uncertain", "Delivery failed or is uncertain; no automatic retry")
		c.inspect(func(t *inspection.Turn) {
			t.State = "uncertain"
			t.Failure = "Delivery may have reached Chatto; check before retrying"
		})
		report("Delivery failed or is uncertain")
		return err
	}
	c.finishStep(delivery, "complete", "Delivered to Chatto")
	c.inspect(func(t *inspection.Turn) {
		t.State = "delivered"
		// Record the actual delivered action at the delivery boundary too.
		t.Response, t.Emoji = outcome.Body, outcome.Emoji
	})
	if outcome.Emoji != "" {
		report("Reaction added")
	} else {
		report("Reply posted")
	}
	return nil
}
