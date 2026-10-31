package bot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestOutputLimitIsVisibleInInspection(t *testing.T) {
	c := &conversation{key: threadKey{"room", "root"}, records: []inspection.Turn{{Number: 1, State: "running"}}}
	c.complete = func(context.Context, []agent.Message) (agent.Message, error) { return nil, agent.ErrOutputLimit }
	var logged string
	err := respond(context.Background(), newFake(), c, []chatto.MessageEvent{input("message", "create a brain", true)}, func(s string) { logged = s })
	if !errors.Is(err, agent.ErrOutputLimit) || c.records[0].Failure != agent.ErrOutputLimit.Error() || logged != agent.ErrOutputLimit.Error() {
		t.Fatal("output limit hidden by generic failure")
	}
	last := c.records[0].Steps[len(c.records[0].Steps)-1]
	if last.Kind != "model" || last.State != "failed" || last.Detail != agent.ErrOutputLimit.Error() {
		t.Fatal("inspector lost model stop reason")
	}
}

func TestTurnInspectionCapturesToolsUsageAndDelivery(t *testing.T) {
	s, err := OpenStore(t.TempDir(), "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f := newFake()
	c := &conversation{id: 1, key: threadKey{"room", "root"}, provider: "stable", store: s, records: []inspection.Turn{{Number: 1, Started: time.Now(), State: "running", Inputs: []inspection.Input{{Text: "private input"}}}}}
	var updates []inspection.Turn
	c.emitInspection = func(record inspection.Turn) { updates = append(updates, record) }
	calls := 0
	c.complete = func(context.Context, []agent.Message) (agent.Message, error) {
		calls++
		cost := 0.001
		c.recordUsage(agent.Usage{Input: 100, CachedInput: 80, Output: 10, Cost: &cost})
		if calls == 1 {
			var msg agent.Message
			json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"read","type":"function","function":{"name":"read_thread","arguments":"{}"}}]}`), &msg)
			return msg, nil
		}
		return answer(), nil
	}
	err = respond(context.Background(), f, c, []chatto.MessageEvent{input("first", "private input", true)}, func(text string) { c.send(func(tui.Event) {}, tui.Event{Text: text}) })
	if err != nil {
		t.Fatal(err)
	}
	r := c.records[0]
	if r.State != "delivered" || r.Response != "reply" || r.Finished.IsZero() || len(r.Steps) != 5 {
		t.Fatalf("incomplete inspection: state=%s steps=%d", r.State, len(r.Steps))
	}
	if r.Steps[1].Usage.Cost != 0.001 || r.Steps[2].Kind != "tool" || r.Steps[2].State != "complete" || r.Steps[4].Kind != "delivery" {
		t.Fatal("missing billing, tool, or delivery evidence")
	}
	if updates[0].Steps[0].State != "running" {
		t.Fatal("UI snapshot mutated after emission")
	}
	for _, log := range c.storedLogs() {
		if strings.Contains(log.Text, "private input") {
			t.Fatal("private data entered operational logs")
		}
	}
	loaded, err := s.load()
	if err != nil || len(loaded) != 1 {
		t.Fatal("reload failed")
	}
	if len(loaded[0].records) != 1 || loaded[0].records[0].Response != "reply" || len(loaded[0].logs) == 0 {
		t.Fatal("inspection or log lost at restart")
	}
	if c.humanID != loaded[0].humanID {
		t.Fatal("inspection changed identity")
	}
}

type uncertainDelivery struct{ *fakeClient }

func (uncertainDelivery) Reply(context.Context, string, string, string, string, string) error {
	return errors.New("private upstream error")
}

func TestInspectorDistinguishesUncertainDelivery(t *testing.T) {
	c := &conversation{key: threadKey{"room", "root"}, records: []inspection.Turn{{Number: 1, Started: time.Now(), State: "running"}}}
	c.complete = func(context.Context, []agent.Message) (agent.Message, error) { return answer(), nil }
	err := respond(context.Background(), uncertainDelivery{newFake()}, c, []chatto.MessageEvent{input("first", "input", true)}, nil)
	if err == nil || c.records[0].State != "uncertain" {
		t.Fatal("uncertain delivery marked successful")
	}
	encoded, _ := json.Marshal(c.records)
	if strings.Contains(string(encoded), "private upstream error") {
		t.Fatal("raw error leaked into inspector")
	}
}
