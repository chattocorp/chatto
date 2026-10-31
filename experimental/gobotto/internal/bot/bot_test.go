package bot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

type fakeClient struct {
	events chan chatto.MessageEvent
	ready  chan struct{}
	posts  chan string
	mu     sync.Mutex
	reads  int
}

func newFake() *fakeClient {
	return &fakeClient{events: make(chan chatto.MessageEvent, 128), ready: make(chan struct{}), posts: make(chan string, 32)}
}
func (f *fakeClient) ObserveMessages(ctx context.Context, _ string, ready func(), handle func(context.Context, chatto.MessageEvent) error) error {
	ready()
	close(f.ready)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-f.events:
			if err := handle(ctx, e); err != nil {
				return err
			}
		}
	}
}
func (f *fakeClient) Thread(context.Context, string, string, string) (chatto.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return chatto.Page{Messages: []chatto.Message{{ID: "first", Body: "initial-private-text", ActorID: "human", RoomID: "room", ThreadRootEventID: "root"}}}, nil
}
func (f *fakeClient) Reply(_ context.Context, _, _, target, _, _ string) error {
	f.posts <- target
	return nil
}
func (f *fakeClient) React(_ context.Context, _, target, _ string) error {
	f.posts <- target
	return nil
}
func (f *fakeClient) readCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.reads }
func input(id, body string, addressed bool) chatto.MessageEvent {
	return chatto.MessageEvent{Message: chatto.Message{ID: id, Body: body, ActorID: "human", RoomID: "room", ThreadRootEventID: "root"}, Addressed: addressed}
}
func answer() agent.Message {
	return agent.Message{"role": json.RawMessage(`"assistant"`), "content": json.RawMessage(`"reply"`)}
}

func TestBatchingAndAppendOnlyConversation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f := newFake()
		requests := make(chan []agent.Message, 8)
		done := make(chan error, 1)
		var ids []string
		titleCalls := 0
		go func() {
			done <- Run(ctx, f, chatto.Profile{ID: "bot", Login: "test-bot", DisplayName: "Test Bot"}, func(id string, _ func(agent.Usage)) agent.Complete {
				ids = append(ids, id)
				return func(_ context.Context, m []agent.Message) (agent.Message, error) {
					requests <- append([]agent.Message(nil), m...)
					return answer(), nil
				}
			}, func(tui.Event) {}, Options{SystemPrompt: "Configured operator tone", Titles: func(context.Context, agent.Snapshot) (agent.TitleResult, error) {
				titleCalls++
				if len(f.posts) == 0 {
					t.Error("title request preceded delivery")
				}
				return agent.TitleResult{Title: "A useful conversation"}, nil
			}})
		}()
		<-f.ready
		f.events <- input("ignored", "no session yet", false)
		synctest.Wait()
		time.Sleep(3 * time.Second)
		if len(ids) != 0 {
			t.Fatal("ordinary message opened a session")
		}
		f.events <- input("first", "initial-private-text", true)
		synctest.Wait()
		time.Sleep(time.Second)
		f.events <- input("second", "second-private-text", true)
		synctest.Wait()
		time.Sleep(1900 * time.Millisecond)
		select {
		case <-requests:
			t.Fatal("batch ran before two seconds of quiet")
		default:
		}
		// Context-only replies also extend an already pending turn.
		f.events <- input("during-batch", "additional context", false)
		synctest.Wait()
		time.Sleep(1900 * time.Millisecond)
		select {
		case <-requests:
			t.Fatal("unaddressed reply did not reset debounce")
		default:
		}
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		first := <-requests
		var prompt string
		json.Unmarshal(first[0]["content"], &prompt)
		if !strings.Contains(prompt, "Configured operator tone") {
			t.Fatal("operator instructions did not reach the model")
		}
		if !strings.Contains(prompt, `"login":"test-bot"`) || !strings.Contains(prompt, `"displayName":"Test Bot"`) || strings.Contains(prompt, "You are Gobotto,") {
			t.Fatal("model did not receive its authenticated Chatto identity")
		}
		if target := <-f.posts; target != "during-batch" {
			t.Fatalf("reply target=%s", target)
		}
		encoded, _ := json.Marshal(first)
		if strings.Count(string(encoded), "initial-private-text") != 1 || !strings.Contains(string(encoded), "second-private-text") {
			t.Fatal("snapshot and incoming messages not deduplicated")
		}
		f.events <- input("ordinary", "context-only-text", false)
		synctest.Wait()
		time.Sleep(time.Second)
		select {
		case <-requests:
			t.Fatal("ordinary message bypassed debounce")
		default:
		}
		f.events <- input("third", "third-private-text", false)
		synctest.Wait()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		second := <-requests
		var batch struct {
			Triggers  []string `json:"triggerMessageIds"`
			Addressed []string `json:"addressedMessageIds"`
		}
		var batchText string
		json.Unmarshal(second[len(second)-1]["content"], &batchText)
		if json.Unmarshal([]byte(batchText), &batch) != nil || len(batch.Triggers) != 2 || len(batch.Addressed) != 0 {
			t.Fatal("ordinary messages were omitted or misrepresented as mentions")
		}
		if target := <-f.posts; target != "third" {
			t.Fatal("ordinary reply was not the delivery target")
		}
		for i, m := range first {
			a, _ := json.Marshal(m)
			b, _ := json.Marshal(second[i])
			if string(a) != string(b) {
				t.Fatalf("prefix changed at message %d", i)
			}
		}
		encoded, _ = json.Marshal(second)
		if !strings.Contains(string(encoded), "context-only-text") || !strings.Contains(string(encoded), "third-private-text") {
			t.Fatal("later input missing")
		}
		if f.readCount() != 1 || len(ids) != 1 {
			t.Fatalf("reads=%d provider sessions=%d", f.readCount(), len(ids))
		}
		if titleCalls != 1 {
			t.Fatal("title was not generated exactly once")
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}

func TestMessagesArrivingDuringTurnDoNotOverlapOrWaitAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f := newFake()
		started := make(chan int, 4)
		release := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
				calls := 0
				return func(ctx context.Context, _ []agent.Message) (agent.Message, error) {
					calls++
					started <- calls
					if calls == 1 {
						select {
						case <-release:
						case <-ctx.Done():
							return nil, ctx.Err()
						}
					}
					return answer(), nil
				}
			}, func(tui.Event) {})
		}()
		<-f.ready
		f.events <- input("first", "first", true)
		synctest.Wait()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if <-started != 1 {
			t.Fatal("missing first turn")
		}
		f.events <- input("later", "later", false)
		synctest.Wait()
		time.Sleep(3 * time.Second)
		select {
		case <-started:
			t.Fatal("concurrent turn in same thread")
		default:
		}
		close(release)
		synctest.Wait()
		select {
		case n := <-started:
			if n != 2 {
				t.Fatal(n)
			}
		default:
			t.Fatal("elapsed window added another debounce delay")
		}
		cancel()
		<-done
	})
}

func TestHistoryInvalidationCancelsOldTurnAndReloads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f := newFake()
		started := make(chan struct{}, 2)
		done := make(chan error, 1)
		sessions := 0
		go func() {
			done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
				sessions++
				generation := sessions
				return func(ctx context.Context, _ []agent.Message) (agent.Message, error) {
					started <- struct{}{}
					if generation == 1 {
						<-ctx.Done()
						return nil, ctx.Err()
					}
					return answer(), nil
				}
			}, func(tui.Event) {})
		}()
		<-f.ready
		f.events <- input("first", "first", true)
		synctest.Wait()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		<-started
		f.events <- chatto.MessageEvent{InvalidatedRoom: "room"}
		synctest.Wait()
		select {
		case <-f.posts:
			t.Fatal("invalidated turn posted")
		default:
		}
		f.events <- input("fresh", "fresh", true)
		synctest.Wait()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		<-started
		<-f.posts
		if f.readCount() != 2 || sessions != 2 {
			t.Fatalf("reads=%d sessions=%d", f.readCount(), sessions)
		}
		cancel()
		<-done
	})
}

func TestThreadIsolationAndSelfMessages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f := newFake()
		requests := make(chan string, 4)
		done := make(chan error, 1)
		ids := make(chan string, 4)
		go func() {
			done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(id string, _ func(agent.Usage)) agent.Complete {
				ids <- id
				return func(_ context.Context, m []agent.Message) (agent.Message, error) {
					data, _ := json.Marshal(m)
					requests <- string(data)
					return answer(), nil
				}
			}, func(tui.Event) {})
		}()
		<-f.ready
		a := input("a", "thread-A-only", true)
		b := input("b", "thread-B-only", true)
		b.Message.ThreadRootEventID = "other-root"
		self := input("own", "own-output", true)
		self.Message.ActorID = "bot"
		f.events <- a
		f.events <- b
		f.events <- self
		synctest.Wait()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		for range 2 {
			payload := <-requests
			if strings.Contains(payload, "thread-A-only") == strings.Contains(payload, "thread-B-only") || strings.Contains(payload, "own-output") {
				t.Fatal("thread context crossed session boundary")
			}
			<-f.posts
		}
		if first, second := <-ids, <-ids; first == second {
			t.Fatal("threads share provider session ID")
		}
		select {
		case <-requests:
			t.Fatal("self-message started a turn")
		default:
		}
		cancel()
		<-done
	})
}
