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
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/subagent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestDelegationUsageAndInspectionSurviveReload(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store, err := OpenStore(t.TempDir(), "server", "bot", "parent")
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		children, err := subagent.Open(map[string]config.Subagent{"code": {Dir: t.TempDir(), Model: "child", TimeoutDuration: time.Minute, MaxToolCalls: 2}}, func(model, id string, usage func(agent.Usage)) agent.Complete {
			return func(context.Context, []agent.Message) (agent.Message, error) {
				cost := 0.02
				usage(agent.Usage{Input: 100, Output: 20, Cost: &cost})
				return answer(), nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		defer children.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f := newFake()
		done := make(chan error, 1)
		var mu sync.Mutex
		var last inspection.Turn
		var first inspection.Turn
		go func() {
			done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(_ string, usage func(agent.Usage)) agent.Complete {
				calls := 0
				return func(_ context.Context, messages []agent.Message) (agent.Message, error) {
					calls++
					cost := 0.01
					usage(agent.Usage{Input: 10, Output: 5, Cost: &cost})
					if calls == 1 {
						var msg agent.Message
						_ = json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"child","type":"function","function":{"name":"delegate","arguments":"{\"agent\":\"code\",\"task\":\"Investigate\"}"}}]}`), &msg)
						return msg, nil
					}
					if calls == 2 {
						var msg agent.Message
						_ = json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"wait","type":"function","function":{"name":"defer_response","arguments":"{}"}}]}`), &msg)
						return msg, nil
					}
					return answer(), nil
				}
			}, func(e tui.Event) {
				if e.Inspection != nil {
					mu.Lock()
					last = e.Inspection.Clone()
					if e.Inspection.Number == 1 {
						first = e.Inspection.Clone()
					}
					mu.Unlock()
				}
			}, Options{Store: store, Subagents: children})
		}()
		<-f.ready
		f.events <- input("first", "Investigate code", true)
		synctest.Wait()
		time.Sleep(batchWindow)
		synctest.Wait()
		<-f.posts
		<-f.posts
		synctest.Wait()
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		mu.Lock()
		record := last
		mu.Unlock()
		found := false
		for _, step := range first.Steps {
			if strings.HasPrefix(step.Name, "code / ") {
				found = true
				if step.Kind != "model" || step.State != "complete" || step.Usage.Input != 100 || step.Usage.Cost != 0.02 {
					t.Fatal("child usage missing", step)
				}
			}
		}
		if !found || record.State != "delivered" {
			t.Fatal("child inspection missing", record.State)
		}
		loaded, err := store.load()
		if err != nil || len(loaded) != 1 {
			t.Fatal("reload failed", err)
		}
		if loaded[0].usage.Input != 130 || loaded[0].usage.Output != 35 || loaded[0].usage.Cost < 0.049999 || loaded[0].usage.Cost > 0.050001 {
			t.Fatal("child accounting lost", loaded[0].usage)
		}
		if len(loaded[0].childRuns) != 1 {
			t.Fatal("child history not persisted")
		}
		for _, child := range loaded[0].childRuns {
			if child.Status != "complete" || len(child.History.Messages) < 3 || child.Report == "" {
				t.Fatal("child result lost", child.Status)
			}
			child.Status = "running"
		}
		if err := store.save(loaded[0], true); err != nil {
			t.Fatal(err)
		}
		if err := store.saveChildren(loaded[0].humanID, loaded[0].childRuns); err != nil {
			t.Fatal(err)
		}
		restarted, err := store.load()
		if err != nil {
			t.Fatal(err)
		}
		for _, child := range restarted[0].childRuns {
			if child.Status != "partial" || child.Reason != "process_interrupted" {
				t.Fatal("interrupted child not marked partial")
			}
		}
	})
}
