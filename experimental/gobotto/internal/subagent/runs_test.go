package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
)

func TestRetentionKeepsActiveRunWithEqualTimestamps(t *testing.T) {
	b := Binding{Runs: map[string]*Run{}}
	for n := 0; n < 9; n++ {
		id := fmt.Sprint(n)
		b.Runs[id] = &Run{ID: id}
	}
	b.retain("0")
	if len(b.Runs) != 8 || b.Runs["0"] == nil {
		t.Fatal("active run was evicted")
	}
}

func TestPartialReportAndResumeAfterReload(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "code.go"), "package retained_evidence")
	runs := map[string]*Run{}
	request, resumes, checkpoints := 0, 0, 0
	var identity string
	m, err := Open(map[string]config.Subagent{"code": {Dir: dir, Model: "test"}}, func(_, id string, _ func(agent.Usage)) agent.Complete {
		if identity == "" {
			identity = id
		} else if id != identity {
			t.Fatal("resume changed provider session")
		}
		return func(_ context.Context, msgs []agent.Message) (agent.Message, error) {
			request++
			switch request {
			case 1:
				return message(t, `{"role":"assistant","content":"I will verify the package declaration.","tool_calls":[{"id":"r","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"code.go\"}"}}]}`), nil
			case 2:
				return nil, errors.New("provider unavailable")
			default:
				encoded, _ := json.Marshal(msgs)
				if !strings.Contains(string(encoded), "retained_evidence") {
					t.Fatal("resume lost file receipt")
				}
				resumes++
				return message(t, `{"role":"assistant","content":"code.go:1 declares retained_evidence."}`), nil
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	binding := Binding{Runs: runs, Checkpoint: func() error { checkpoints++; return nil }}
	out, err := m.Tools(0, nil, nil, binding)[0].Execute(context.Background(), json.RawMessage(`{"agent":"code","task":"Identify package"}`))
	if err != nil || !strings.Contains(out, `"status":"partial"`) || !strings.Contains(out, "retained_evidence") || !strings.Contains(out, "model_failure") || checkpoints < 3 {
		t.Fatal(out, err, checkpoints)
	}
	// The parent persists this private payload without putting it in model context.
	data, _ := json.Marshal(runs)
	var restored map[string]*Run
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	for id := range restored {
		raw, _ := json.Marshal(map[string]string{"agent": "code", "task": "Summarize the existing evidence", "resume_id": id})
		out, err = m.Tools(0, nil, nil, Binding{Runs: restored})[0].Execute(context.Background(), raw)
		if err != nil || !strings.Contains(out, `"status":"complete"`) || resumes != 1 {
			t.Fatal(out, err)
		}
		// Another conversation cannot resume this history.
		out, _ = m.Tools(0, nil, nil)[0].Execute(context.Background(), raw)
		if !strings.Contains(out, `"status":"rejected"`) {
			t.Fatal("cross-thread resume accepted", out)
		}
	}
}

func TestStatusDoesNotRunModelAndPreservesPartialResult(t *testing.T) {
	run := &Run{ID: "retained", Agent: "code", Status: "partial", Reason: "deadline", Report: "Provisional finding", Evidence: []Evidence{{Tool: "read_file", Result: "source evidence"}}}
	b := Binding{Runs: map[string]*Run{run.ID: run}}
	out, _ := b.statusTool().Execute(context.Background(), json.RawMessage(`{}`))
	if !strings.Contains(out, "Provisional finding") || strings.Contains(out, "source evidence") {
		t.Fatal(out)
	}
	out, _ = b.statusTool().Execute(context.Background(), json.RawMessage(`{"run_id":"retained"}`))
	if !strings.Contains(out, "source evidence") || !strings.Contains(out, "deadline") {
		t.Fatal(out)
	}
}
