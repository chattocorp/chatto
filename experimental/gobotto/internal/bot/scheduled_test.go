package bot

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/scheduler"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func makeSchedule(t *testing.T, m *scheduler.Manager, destination ...string) {
	t.Helper()
	kind := "thread"
	if len(destination) > 0 {
		kind = destination[0]
	}
	args, _ := json.Marshal(map[string]string{"title": "Test", "task": "Report current findings", "schedule": "@every 1m", "destination": kind})
	for _, tool := range m.Tools("room", "root", "human", true) {
		if tool.Name == "schedule_create" {
			out, err := tool.Execute(context.Background(), args)
			if err != nil || !strings.Contains(out, "complete") {
				t.Fatal(out, err)
			}
			return
		}
	}
	t.Fatal("schedule_create missing")
}

func TestScheduledRunsUseFreshContextAndPersistSeparateHistory(t *testing.T) {
	for _, kind := range []string{"room", "thread"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				root := t.TempDir()
				m, err := scheduler.Open(filepath.Join(root, "schedules"), "test")
				if err != nil {
					t.Fatal(err)
				}
				defer m.Close()
				store, err := OpenStore(filepath.Join(root, "sessions"), "server", "bot", "model")
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				makeSchedule(t, m, kind)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				f := newFake()
				done := make(chan error, 1)
				var completions atomic.Int32
				providers := map[string]bool{}
				go func() {
					done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(id string, _ func(agent.Usage)) agent.Complete {
						providers[id] = true
						step := 0
						return func(ctx context.Context, messages []agent.Message) (agent.Message, error) {
							if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 5*time.Minute {
								t.Error("scheduled timeout not bounded")
							}
							for _, message := range messages {
								if string(message["role"]) == `"assistant"` && strings.Contains(string(message["content"]), "previous-run-output") {
									t.Error("scheduled transcript inherited previous assistant message")
								}
							}
							if step == 0 {
								all, _ := json.Marshal(messages)
								if strings.Contains(string(all), "initial-private-text") {
									t.Error("destination history injected without a read")
								}
								step++
								return wireMessage(`{"role":"assistant","tool_calls":[{"id":"search","type":"function","function":{"name":"tool_search","arguments":"{\"query\":\"\"}"}}]}`), nil
							}
							var result string
							_ = json.Unmarshal(messages[len(messages)-1]["content"], &result)
							for _, name := range []string{"schedule_create", "memory_brain_create", "delegate", "react_to_ping", "read_thread"} {
								if strings.Contains(result, `"name":"`+name+`"`) {
									t.Error("scheduled tool gained authority", name)
								}
							}
							if step == 1 {
								step++
								return wireMessage(`{"role":"assistant","tool_calls":[{"id":"read","type":"function","function":{"name":"read_destination","arguments":"{}"}}]}`), nil
							}
							if !strings.Contains(result, "initial-private-text") {
								t.Error("on-demand destination read omitted history")
							}
							completions.Add(1)
							return wireMessage(`{"role":"assistant","content":"previous-run-output"}`), nil
						}
					}, func(tui.Event) {}, Options{Store: store, Scheduler: m, OwnerUserIDs: []string{"human"}})
				}()
				<-f.ready
				time.Sleep(59 * time.Second)
				synctest.Wait()
				if completions.Load() != 0 {
					t.Fatal("schedule fired early")
				}
				time.Sleep(2 * time.Second)
				synctest.Wait()
				if completions.Load() != 1 {
					t.Fatal("first occurrence missing", completions.Load())
				}
				wantTarget := "root"
				if kind == "room" {
					wantTarget = ""
				}
				if target := <-f.posts; target != wantTarget {
					t.Fatal("wrong destination", target)
				}
				time.Sleep(time.Minute)
				synctest.Wait()
				if completions.Load() != 2 {
					t.Fatal("fresh second run missing", completions.Load())
				}
				<-f.posts
				cancel()
				<-done
				if len(providers) != 2 {
					t.Fatal("provider session reused")
				}
				loaded, err := store.loadIndex(context.Background())
				if err != nil || len(loaded) != 1 || !loaded[0].scheduledRun || loaded[0].turns != 2 {
					t.Fatal("scheduled history incorrect", len(loaded), err)
				}
				// Ordinary conversation history in the same thread remains independent.
				ordinary := &conversation{id: 2, provider: "ordinary", key: threadKey{"room", "root"}, started: time.Now()}
				if err := store.save(ordinary, false); err != nil {
					t.Fatal(err)
				}
				if loaded, err := store.loadIndex(context.Background()); err != nil || len(loaded) != 2 {
					t.Fatal("same-thread scheduled history collided", err)
				}
			})
		})
	}
}

type deniedScheduleClient struct{ *fakeClient }

type routedScheduleClient struct {
	*fakeClient
	room, root, replyTo string
	denyDelivery        bool
	reads               []string
}

func (f *routedScheduleClient) Timeline(_ context.Context, room, root, before string) (chatto.TimelinePage, error) {
	f.reads = append(f.reads, "room:"+room+":"+root)
	return chatto.TimelinePage{}, nil
}
func (f *routedScheduleClient) Thread(ctx context.Context, room, root, before string) (chatto.Page, error) {
	f.reads = append(f.reads, "thread:"+room+":"+root)
	return f.fakeClient.Thread(ctx, room, root, before)
}

func (f *routedScheduleClient) Reply(_ context.Context, room, root, replyTo, body, key string) error {
	f.room, f.root, f.replyTo = room, root, replyTo
	if f.denyDelivery {
		return errors.New("posting denied")
	}
	return nil
}

func TestScheduledDestinationRouting(t *testing.T) {
	for _, destination := range []string{"room", "thread"} {
		for _, denied := range []bool{false, true} {
			t.Run(destination+map[bool]string{true: " denied", false: " delivered"}[denied], func(t *testing.T) {
				m, err := scheduler.Open(t.TempDir(), "test")
				if err != nil {
					t.Fatal(err)
				}
				defer m.Close()
				makeSchedule(t, m, destination)
				d, err := m.Claim(time.Now().Add(time.Minute), []string{"human"})
				if err != nil || d == nil {
					t.Fatal("missing claim", err)
				}
				c := &conversation{scheduled: d, scheduler: m, ownerUserIDs: []string{"human"}, records: []inspection.Turn{{Number: 1, State: "running"}}, complete: func(_ context.Context, messages []agent.Message) (agent.Message, error) {
					all, _ := json.Marshal(messages)
					if !strings.Contains(string(all), `\"kind\":\"`+destination+`\"`) {
						t.Error("destination missing from model input")
					}
					return wireMessage(`{"role":"assistant","content":"Scheduled update"}`), nil
				}}
				f := &routedScheduleClient{fakeClient: newFake(), denyDelivery: denied}
				err = respondScheduled(context.Background(), f, c)
				if (err != nil) != denied {
					t.Fatal("unexpected delivery result", err)
				}
				wantRoot := "root"
				if destination == "room" {
					wantRoot = ""
				}
				if f.room != "room" || f.root != wantRoot || f.replyTo != wantRoot {
					t.Fatalf("incorrect routing: %q %q %q", f.room, f.root, f.replyTo)
				}
				wantRead := destination + ":room:" + wantRoot
				if len(f.reads) != 2 || f.reads[0] != wantRead || f.reads[1] != wantRead {
					t.Fatal("access/context did not use destination", f.reads)
				}
				wantState := "delivered"
				if denied {
					wantState = "uncertain"
				}
				if c.records[0].State != wantState {
					t.Fatal("incorrect receipt", c.records[0].State)
				}
			})
		}
	}
}

func (f deniedScheduleClient) Thread(context.Context, string, string, string) (chatto.Page, error) {
	return chatto.Page{}, errors.New("access denied")
}

func TestScheduledAccessFailurePreventsModelWork(t *testing.T) {
	m, err := scheduler.Open(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	makeSchedule(t, m)
	d, err := m.Claim(time.Now().Add(time.Minute), []string{"human"})
	if err != nil || d == nil {
		t.Fatal(err)
	}
	c := &conversation{scheduled: d, scheduler: m, ownerUserIDs: []string{"human"}, records: []inspection.Turn{{Number: 1, State: "running"}}, complete: func(context.Context, []agent.Message) (agent.Message, error) {
		t.Fatal("model ran without destination access")
		return nil, nil
	}}
	if err := respondScheduled(context.Background(), deniedScheduleClient{newFake()}, c); err == nil {
		t.Fatal("ignored lost access")
	}
}

func TestScheduledRunCancelsOnRealtimeAccessLoss(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		root := t.TempDir()
		m, err := scheduler.Open(filepath.Join(root, "schedules"), "test")
		if err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		store, err := OpenStore(filepath.Join(root, "sessions"), "server", "bot", "model")
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		makeSchedule(t, m)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f := newFake()
		started := make(chan struct{})
		stopped := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
				return func(ctx context.Context, _ []agent.Message) (agent.Message, error) {
					close(started)
					<-ctx.Done()
					if errors.Is(ctx.Err(), context.DeadlineExceeded) {
						t.Error("realtime access loss did not cancel the run before its timeout")
					}
					close(stopped)
					return nil, ctx.Err()
				}
			}, func(tui.Event) {}, Options{Store: store, Scheduler: m, OwnerUserIDs: []string{"human"}})
		}()
		<-f.ready
		time.Sleep(time.Minute)
		<-started
		f.events <- chatto.MessageEvent{ID: "access-loss", InvalidatedRoom: "room", InvalidationReason: chatto.ViewerLeftRoom}
		<-stopped
		cancel()
		<-done
		if len(f.posts) != 0 {
			t.Fatal("scheduled output after access loss")
		}
	})
}
