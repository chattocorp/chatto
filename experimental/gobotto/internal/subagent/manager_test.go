package subagent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
)

func message(t *testing.T, text string) agent.Message {
	t.Helper()
	var m agent.Message
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDelegationIsolatedAndBounded(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "example.go"), "package example")
	ids := map[string]bool{}
	reported := int64(0)
	var events []Event
	m, err := Open(map[string]config.Subagent{"code": {Dir: dir, Model: "test/child", Description: "Code", TimeoutDuration: time.Second, MaxToolCalls: 4}}, func(model, id string, usage func(agent.Usage)) agent.Complete {
		if model != "test/child" || ids[id] {
			t.Fatal("model or child identity wrong")
		}
		ids[id] = true
		calls := 0
		return func(ctx context.Context, messages []agent.Message) (agent.Message, error) {
			calls++
			usage(agent.Usage{Input: 5})
			if calls == 1 {
				if len(messages) != 2 {
					t.Fatal("child inherited parent history")
				}
				return message(t, `{"role":"assistant","tool_calls":[{"id":"read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"example.go\"}"}}]}`), nil
			}
			last, _ := json.Marshal(messages[len(messages)-1])
			if !strings.Contains(string(last), "package example") {
				t.Fatal("missing file result")
			}
			return message(t, `{"role":"assistant","content":"example.go:1 declares package example."}`), nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	tool := m.Tools(2, func(u agent.Usage) { reported += u.Input }, func(e Event) { events = append(events, e) })[0]
	for range 2 {
		out, err := tool.Execute(context.Background(), json.RawMessage(`{"agent":"code","task":"Find the package"}`))
		if err != nil || !strings.Contains(out, "example.go:1") {
			t.Fatal(out, err)
		}
	}
	out, _ := tool.Execute(context.Background(), json.RawMessage(`{"agent":"code","task":"Again"}`))
	if !strings.Contains(out, `"status":"rejected"`) || len(ids) != 2 || reported != 20 {
		t.Fatal("limit or usage wrong", out, reported)
	}
	if len(events) != 12 || events[0].Kind != "model" || events[2].Name != "code / read_file" {
		t.Fatal("missing child activity", events)
	}
}

func TestDelegationCancellation(t *testing.T) {
	for _, parentCancel := range []bool{false, true} {
		m, err := Open(map[string]config.Subagent{"code": {Dir: t.TempDir(), TimeoutDuration: 10 * time.Millisecond, MaxToolCalls: 1}}, func(_, _ string, _ func(agent.Usage)) agent.Complete {
			return func(ctx context.Context, _ []agent.Message) (agent.Message, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		if parentCancel {
			cancel()
		}
		out, err := m.Tools(0, nil, nil)[0].Execute(ctx, json.RawMessage(`{"agent":"code","task":"Investigate"}`))
		cancel()
		m.Close()
		if err != nil || !strings.Contains(out, `"status":"partial"`) || !strings.Contains(out, `"report":`) {
			t.Fatal(out, err)
		}
	}
}

func TestDelegationHasNoImplicitTimeoutOrRunLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, err := Open(map[string]config.Subagent{"code": {Dir: t.TempDir()}}, func(_, _ string, _ func(agent.Usage)) agent.Complete {
			return func(ctx context.Context, _ []agent.Message) (agent.Message, error) {
				if _, ok := ctx.Deadline(); ok {
					t.Fatal("implicit child deadline")
				}
				time.Sleep(3 * time.Minute)
				return message(t, `{"role":"assistant","content":"done"}`), nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		tool := m.Tools(0, nil, nil)[0]
		for range 4 {
			out, err := tool.Execute(context.Background(), json.RawMessage(`{"agent":"code","task":"Investigate"}`))
			if err != nil || !strings.Contains(out, `"report":"done"`) {
				t.Fatal(out, err)
			}
		}
	})
}
