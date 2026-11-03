package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/journal"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/scheduler"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/testmeans"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func journalTestConversation(t *testing.T) *conversation {
	t.Helper()
	s, err := journal.Open(t.TempDir(), "account", testmeans.Command)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	c := &conversation{journals: s, humanID: "journal-test", key: threadKey{"room", "root"}}
	c.complete = journalTestCompletion(t, false)
	return c
}

func journalTestCompletion(t *testing.T, fail bool) agent.Complete {
	t.Helper()
	return func(_ context.Context, ms []agent.Message) (agent.Message, error) {
		var system string
		_ = json.Unmarshal(ms[0]["content"], &system)
		if !strings.HasPrefix(system, "Maintain a private conversation journal") {
			return answer(), nil
		}
		if fail {
			return nil, errors.New("summary failed")
		}
		var raw string
		_ = json.Unmarshal(ms[1]["content"], &raw)
		var input struct {
			Entries []journal.Entry `json:"entries"`
		}
		if json.Unmarshal([]byte(raw), &input) != nil {
			t.Fatal("bad summary input")
		}
		notes := make([]string, len(input.Entries))
		for i := range notes {
			notes[i] = "A scoped decision was recorded; prior actions must not be repeated."
		}
		b, _ := json.Marshal(map[string]any{"notes": notes})
		content, _ := json.Marshal(string(b))
		return agent.Message{"role": json.RawMessage(`"assistant"`), "content": content}, nil
	}
}

func journalTestTurn(t *testing.T, c *conversation, f Client, body string, topLevel bool) {
	t.Helper()
	c.turns++
	c.records = append(c.records, inspection.Turn{Number: c.turns, Started: time.Now(), State: "running"})
	e := input(fmt.Sprintf("message-%d", c.turns), body, true)
	if topLevel {
		e.Message.ThreadRootEventID = ""
	}
	if err := respond(context.Background(), f, c, []chatto.MessageEvent{e}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestJournalTurnAndRolloverPreserveRecentContext(t *testing.T) {
	c := journalTestConversation(t)
	f := newFake()
	for range 6 {
		journalTestTurn(t, c, f, strings.Repeat("history ", 500), false)
	}
	d, err := c.journals.Read(c.journalScope())
	if err != nil || strings.Count(d.Summary, "A scoped decision") != 6 || len(d.Entries) != 0 {
		t.Fatal("journal missing", err)
	}
	before := c.model
	old, _ := json.Marshal(c.model.Snapshot())
	if err := c.maybeRollover(context.Background(), f, 16000); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(c.model.Snapshot())
	if c.model == before || len(b) >= len(old) || !strings.Contains(string(b), "A scoped decision") || !strings.Contains(string(b), "message-6") {
		t.Fatal("rollover failed to retain scoped context and last turn")
	}
	if c.contextModel != nil || !c.seen["message-6"] || c.seen["message-1"] {
		t.Fatal("rollover failed to reset context deltas")
	}
	journalTestTurn(t, c, f, "continue", false)
	d, err = c.journals.Read(c.journalScope())
	if err != nil || strings.Count(d.Summary, "A scoped decision") != 7 {
		t.Fatal("journal continuation failed", err)
	}
}

func TestJournalSummaryFailureKeepsHistoryAndDelivery(t *testing.T) {
	c := journalTestConversation(t)
	c.complete = journalTestCompletion(t, true)
	f := newFake()
	journalTestTurn(t, c, f, "remember the decision", false)
	if c.records[0].State != "delivered" || len(f.posts) != 1 || c.journalCheckpoint != "" {
		t.Fatal("summary failure changed delivery or permitted rollover")
	}
	before := c.model
	if err := c.maybeRollover(context.Background(), f, 1); err != nil || c.model != before {
		t.Fatal("failed journal rolled over")
	}
	d, _ := c.journals.Read(c.journalScope())
	if d.Through != 0 || len(d.Entries) != 1 || len(d.Entries[0].Inputs) == 0 {
		t.Fatal("failed summary lost pending input")
	}
	c.complete = journalTestCompletion(t, false)
	journalTestTurn(t, c, f, "continue", false)
	d, _ = c.journals.Read(c.journalScope())
	if strings.Count(d.Summary, "A scoped decision") != 2 || c.journalCheckpoint == "" {
		t.Fatal("next update did not recover pending journal")
	}
}

func TestJournalRecallCannotSelectAnotherScope(t *testing.T) {
	c := journalTestConversation(t)
	f := newFake()
	for _, scope := range []journal.Scope{{Room: "other", Thread: "root"}, {Room: "room", Thread: "other"}, {Room: "room"}} {
		if err := c.journals.Append(scope, journal.Entry{Key: "secret", Note: "MUST_NOT_LEAK"}); err != nil {
			t.Fatal(err)
		}
		d, _ := c.journals.Read(scope)
		if err := c.journals.Summarize(scope, d.Revision, d.Generation, "", []string{"MUST_NOT_LEAK"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.journals.Append(c.journalScope(), journal.Entry{Key: "local", Note: "LOCAL_NOTE"}); err != nil {
		t.Fatal(err)
	}
	d, _ := c.journals.Read(c.journalScope())
	if err := c.journals.Summarize(c.journalScope(), d.Revision, d.Generation, "", []string{"LOCAL_NOTE"}); err != nil {
		t.Fatal(err)
	}
	tool := c.journalTool(f)[0]
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"query":""}`))
	if err != nil || !strings.Contains(out, "LOCAL_NOTE") || strings.Contains(out, "MUST_NOT_LEAK") {
		t.Fatal("scope isolation failed", err)
	}
	for _, raw := range []string{`{"query":"","room":"other"}`, `{"query":"","thread":"other"}`, `null`, `{}`, `{"query":""} {}`} {
		out, err = tool.Execute(context.Background(), json.RawMessage(raw))
		if err != nil || !strings.Contains(out, "rejected") {
			t.Fatal("invalid selector accepted", raw)
		}
	}
	tool = c.journalTool(deniedScheduleClient{f})[0]
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"query":""}`)); err == nil {
		t.Fatal("journal read despite access loss")
	}
}

func TestRoomJournalNeverIncludesThreadReplies(t *testing.T) {
	c := journalTestConversation(t)
	f := newFake()
	journalTestTurn(t, c, f, "top-level request", true)
	d, err := c.journals.Read(journal.Scope{Room: "room"})
	if err != nil || strings.Count(d.Summary, "A scoped decision") != 1 || len(d.Entries) != 0 {
		t.Fatal("room journal included thread output", err)
	}
	journalTestTurn(t, c, f, "PRIVATE_THREAD_REPLY", false)
	d, _ = c.journals.Read(journal.Scope{Room: "room"})
	if strings.Contains(d.Summary, "PRIVATE_THREAD_REPLY") || strings.Count(d.Summary, "A scoped decision") != 1 {
		t.Fatal("thread input copied to room")
	}
}

func TestJournalInvalidationResetsRestoredTranscript(t *testing.T) {
	c := journalTestConversation(t)
	f := newFake()
	journalTestTurn(t, c, f, "old context", false)
	if err := c.journals.Forget("room"); err != nil {
		t.Fatal(err)
	}
	if err := c.prepareJournal(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if c.model != nil || c.journalCheckpoint != "" || c.journalSummary != "" {
		t.Fatal("invalidated journal survived in model history")
	}
}

func TestJournalReceiptsRemainExact(t *testing.T) {
	c := journalTestConversation(t)
	f := newFake()
	stage := 0
	summary := journalTestCompletion(t, false)
	c.tools = []agent.Tool{{Name: "write_example", Execute: func(context.Context, json.RawMessage) (string, error) {
		return `{"status":"complete","receipt":"EXACT_RECEIPT"}`, nil
	}}}
	c.complete = func(ctx context.Context, ms []agent.Message) (agent.Message, error) {
		var system string
		_ = json.Unmarshal(ms[0]["content"], &system)
		if strings.HasPrefix(system, "Maintain a private") {
			return summary(ctx, ms)
		}
		if stage == 0 {
			stage++
			return wireMessage(`{"role":"assistant","tool_calls":[{"id":"write","type":"function","function":{"name":"write_example","arguments":"{}"}}]}`), nil
		}
		return answer(), nil
	}
	journalTestTurn(t, c, f, "write it", false)
	d, _ := c.journals.Read(c.journalScope())
	b, _ := json.Marshal(c.model.Snapshot())
	if len(d.Entries) != 0 || strings.Contains(d.Summary, "EXACT_RECEIPT") || !strings.Contains(string(b), "EXACT_RECEIPT") {
		t.Fatal("receipt must stay in session history, not the notebook")
	}
	if len(f.posts) != 1 {
		t.Fatal("action replayed during summary")
	}
}

func TestFailedRolloverCheckpointRetainsOriginalTranscript(t *testing.T) {
	c := journalTestConversation(t)
	f := newFake()
	for range 6 {
		journalTestTurn(t, c, f, strings.Repeat("history ", 500), false)
	}
	before, checkpoint, offset := c.model, c.journalCheckpoint, c.journalOffset
	c.store = &Store{dir: filepath.Join(t.TempDir(), "missing")}
	if err := c.maybeRollover(context.Background(), f, 16000); err == nil {
		t.Fatal("rollover checkpoint unexpectedly succeeded")
	}
	if c.model != before || c.journalCheckpoint != checkpoint || c.journalOffset != offset {
		t.Fatal("failed checkpoint discarded original context")
	}
}

func TestUncertainDeliveryPreventsRollover(t *testing.T) {
	c := journalTestConversation(t)
	f := newFake()
	for range 6 {
		journalTestTurn(t, c, f, strings.Repeat("history ", 500), false)
	}
	scope := c.journalScope()
	if err := c.journals.Append(scope, journal.Entry{Key: "uncertain", State: "uncertain", Note: "Delivery may have happened"}); err != nil {
		t.Fatal(err)
	}
	d, _ := c.journals.Read(scope)
	if err := c.summarizeDocument(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	before := c.model
	if err := c.maybeRollover(context.Background(), f, 16000); err != nil || c.model != before {
		t.Fatal("uncertain delivery allowed rollover")
	}
}

func TestJournalCheckpointSurvivesSessionReload(t *testing.T) {
	c := journalTestConversation(t)
	f := newFake()
	c.id = 1
	c.provider = "provider"
	c.humanID = humanID(c.provider)
	store, err := OpenStore(t.TempDir(), "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c.store = store
	journalTestTurn(t, c, f, "Remember this decision", false)
	loaded, err := store.load()
	if err != nil || len(loaded) != 1 {
		t.Fatal(err)
	}
	r := loaded[0]
	r.journals = c.journals
	if r.journalCheckpoint != c.journalCheckpoint || r.journalOffset != c.journalOffset {
		t.Fatal("journal checkpoint lost at restart")
	}
	if err := c.journals.Forget("room"); err != nil {
		t.Fatal(err)
	}
	if err := r.prepareJournal(context.Background(), f); err != nil || r.model != nil {
		t.Fatal("restored transcript retained invalidated journal", err)
	}
}

func TestJournalInvalidationDistinguishesPreviewFromTextEdit(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				journals, err := journal.Open(t.TempDir(), "account", testmeans.Command)
				if err != nil {
					t.Fatal(err)
				}
				defer journals.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				f := &editClient{fakeClient: newFake(), current: input("first", "start", true).Message}
				started := make(chan context.Context, 1)
				release := make(chan struct{})
				done := make(chan error, 1)
				summary := journalTestCompletion(t, false)
				go func() {
					done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
						return func(ctx context.Context, ms []agent.Message) (agent.Message, error) {
							var system string
							_ = json.Unmarshal(ms[0]["content"], &system)
							if strings.HasPrefix(system, "Maintain a private") {
								return summary(ctx, ms)
							}
							started <- ctx
							select {
							case <-ctx.Done():
								return nil, ctx.Err()
							case <-release:
								return answer(), nil
							}
						}
					}, func(tui.Event) {}, Options{Journals: journals})
				}()
				<-f.ready
				f.events <- input("first", "start", true)
				time.Sleep(batchWindow)
				synctest.Wait()
				modelCtx := <-started
				if changed {
					f.mu.Lock()
					f.current.Body = "changed"
					f.mu.Unlock()
				}
				f.events <- chatto.MessageEvent{InvalidatedRoom: "room", InvalidatedMessage: "first", InvalidationReason: chatto.MessageEdited}
				synctest.Wait()
				if (modelCtx.Err() != nil) != changed {
					t.Fatal("incorrect cancellation")
				}
				d, err := journals.Read(journal.Scope{Room: "room", Thread: "root"})
				if err != nil || (d.Generation > 0) != changed {
					t.Fatal("incorrect journal invalidation", err)
				}
				if !changed {
					close(release)
					synctest.Wait()
					<-f.posts
				}
				cancel()
				<-done
			})
		})
	}
}

func TestJournalBootstrapKeepsEarlierUncertainDelivery(t *testing.T) {
	c := journalTestConversation(t)
	journals := c.journals
	c.journals = nil
	f := newFake()
	c.turns = 1
	c.records = []inspection.Turn{{Number: 1, Started: time.Now(), State: "running"}}
	if err := respond(context.Background(), uncertainDelivery{f}, c, []chatto.MessageEvent{input("first", "do it", true)}, nil); err == nil {
		t.Fatal("expected uncertain delivery")
	}
	c.journals = journals
	journalTestTurn(t, c, f, "continue", false)
	d, err := journals.Read(c.journalScope())
	if err != nil || !d.Uncertain {
		t.Fatal("later success erased old uncertainty", err)
	}
}

func TestJournalInvalidationCancelsScheduledSummaryOutsideRecentHistory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := journalTestConversation(t)
		store, err := OpenStore(t.TempDir(), "server", "bot", "model")
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		m, err := scheduler.Open(t.TempDir(), "test")
		if err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		makeSchedule(t, m)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f := newFake()
		started := make(chan context.Context, 1)
		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
				return func(ctx context.Context, _ []agent.Message) (agent.Message, error) {
					started <- ctx
					<-ctx.Done()
					return nil, ctx.Err()
				}
			}, func(tui.Event) {}, Options{Store: store, Journals: c.journals, Scheduler: m, OwnerUserIDs: []string{"human"}})
		}()
		<-f.ready
		time.Sleep(time.Minute)
		modelCtx := <-started
		f.events <- chatto.MessageEvent{InvalidatedRoom: "room", InvalidatedMessage: "old-message-outside-window", InvalidationReason: chatto.MessageEdited}
		synctest.Wait()
		stopped := modelCtx.Err() != nil
		cancel()
		<-done
		if !stopped {
			t.Fatal("scheduled turn kept invalidated journal context")
		}
	})
}

func TestJournalCondensesOnlyWhenNotebookIsFull(t *testing.T) {
	c := journalTestConversation(t)
	scope := c.journalScope()
	if err := c.journals.Summarize(scope, 0, 0, strings.Repeat("old note ", 1730), nil); err != nil {
		t.Fatal(err)
	}
	requests := 0
	c.complete = func(_ context.Context, messages []agent.Message) (agent.Message, error) {
		var system, input string
		_ = json.Unmarshal(messages[0]["content"], &system)
		if !strings.HasPrefix(system, "Maintain a private") {
			return answer(), nil
		}
		_ = json.Unmarshal(messages[1]["content"], &input)
		var request struct {
			Condense bool
			Entries  []journal.Entry
		}
		if err := json.Unmarshal([]byte(input), &request); err != nil {
			t.Fatal(err)
		}
		if request.Condense != (requests == 0) {
			t.Fatal("ordinary note requested a rewrite, or full notebook did not")
		}
		requests++
		result := map[string]any{"notes": []string{"A new decision."}}
		if request.Condense {
			result["body"] = "- Keep the earlier decision and the new decision.\n"
		}
		b, _ := json.Marshal(result)
		content, _ := json.Marshal(string(b))
		return agent.Message{"role": json.RawMessage(`"assistant"`), "content": content}, nil
	}
	f := newFake()
	journalTestTurn(t, c, f, "continue", false)
	journalTestTurn(t, c, f, "another decision", false)
	d, err := c.journals.Read(scope)
	if err != nil || requests != 2 || d.Summary != "- Keep the earlier decision and the new decision.\n- A new decision.\n" {
		t.Fatal("condensation or subsequent append failed", err)
	}
}
