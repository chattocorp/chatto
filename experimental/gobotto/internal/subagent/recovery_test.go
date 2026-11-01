package subagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func providerFailure(t *testing.T, status int) error {
	t.Helper()
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	http.DefaultTransport = transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("private provider body"))}, nil
	})
	_, err := agent.OpenRouter("test", "test", "test", nil)(context.Background(), nil)
	if err == nil {
		t.Fatal("expected model failure")
	}
	return err
}

func TestRequestRecoveryDoesNotReplayTools(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		failure := providerFailure(t, 503)
		dir := t.TempDir()
		write(t, filepath.Join(dir, "code.go"), "package evidence")
		runs := map[string]*Run{}
		calls := 0
		var failedInput string
		m, err := Open(map[string]config.Subagent{"code": {Dir: dir}}, func(_, _ string, _ func(agent.Usage)) agent.Complete {
			return func(_ context.Context, msgs []agent.Message) (agent.Message, error) {
				calls++
				if calls == 1 {
					return message(t, `{"role":"assistant","tool_calls":[{"id":"read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"code.go\"}"}}]}`), nil
				}
				data, _ := json.Marshal(msgs)
				if calls == 2 {
					failedInput = string(data)
					return nil, failure
				}
				if string(data) != failedInput {
					t.Error("retry changed transcript or replayed a tool")
				}
				return message(t, `{"role":"assistant","content":"code.go:1 contains package evidence."}`), nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		out, err := m.Tools(0, nil, nil, Binding{Runs: runs})[0].Execute(context.Background(), json.RawMessage(`{"agent":"code","task":"Inspect"}`))
		if err != nil || calls != 3 || !strings.Contains(out, `"status":"complete"`) {
			t.Fatal(out, err, calls)
		}
		for _, r := range runs {
			if len(r.Evidence) != 1 || r.Recoveries != 1 || len(r.Usage) != 1 || !r.Usage[0].Usage.Unreported {
				t.Fatal("missing receipts or unknown-charge marker", r)
			}
		}
	})
}

func TestRecoveryResumesOnlyEligibleExistingRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		failure := providerFailure(t, 503)
		info := agent.ModelFailureInfo(failure)
		dir := t.TempDir()
		zero := 0
		calls := 0
		m, err := Open(map[string]config.Subagent{"code": {Dir: dir, Model: "model", RequestRetries: &zero}}, func(_, id string, _ func(agent.Usage)) agent.Complete {
			if id != "retained" {
				t.Error("created a replacement child")
			}
			return func(_ context.Context, msgs []agent.Message) (agent.Message, error) {
				calls++
				data, _ := json.Marshal(msgs)
				if !strings.Contains(string(data), "retained evidence") {
					t.Error("lost evidence")
				}
				return nil, failure
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		r := &Run{ID: "retained", Agent: "code", Model: "model", Dir: dir, Status: "partial", Reason: "http_503", LastFailure: &info, History: agent.Snapshot{Messages: []agent.Message{message(t, `{"role":"system","content":"old"}`), message(t, `{"role":"user","content":"retained evidence"}`)}}}
		for _, reason := range []string{"cancelled", "deadline", "context_limit", "checkpoint_failed"} {
			r.Reason = reason
			probe := NewBackground(context.Background(), m, map[string]*Run{r.ID: r}, nil, nil)
			if len(probe.RecoveryTools([]string{r.ID}, 0, 2, "trigger", nil)) != 0 {
				t.Fatal("automatic recovery ignored stop reason", reason)
			}
			probe.Close()
		}
		r.Reason = "http_503"
		b := NewBackground(context.Background(), m, map[string]*Run{r.ID: r}, nil, nil)
		defer b.Close()
		tools := b.RecoveryTools([]string{r.ID}, 0, 7, "trigger", nil)
		if len(tools) != 1 {
			t.Fatal("recovery tool unavailable")
		}
		for _, raw := range []string{`{"run_id":"other"}`, `{"run_id":"retained","agent":"other"}`} {
			out, _ := tools[0].Execute(context.Background(), json.RawMessage(raw))
			if !strings.Contains(out, "rejected") {
				t.Fatal(out)
			}
		}
		out, _ := tools[0].Execute(context.Background(), json.RawMessage(`{"run_id":"retained","focus":"Check the unresolved branch"}`))
		if !strings.Contains(out, `"status":"running"`) {
			t.Fatal(out)
		}
		synctest.Wait()
		runs, _, done, err := b.Drain()
		if err != nil || calls != 1 || len(done) != 1 || runs[r.ID].AutoResumes != 1 || runs[r.ID].Target != "trigger" {
			t.Fatal(err, calls, done)
		}
		if len(b.RecoveryTools(done, 0, 8, "trigger", nil)) != 0 {
			t.Fatal("automatic recovery allowance was reset")
		}
	})
}

func TestRecoveryBackoffHonorsCancellation(t *testing.T) {
	failure := providerFailure(t, 429)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	_, err := recoverRequest(ctx, 10, &Run{}, func() (agent.Message, error) { calls++; return nil, failure }, func() error { cancel(); return nil })
	if err != context.Canceled || calls != 1 {
		t.Fatal(err, calls)
	}
}
