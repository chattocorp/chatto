package bot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/subagent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

type recoveryTransport func(*http.Request) (*http.Response, error)

func (f recoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCompletionResumesSameChildAndDeliversResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := http.DefaultTransport
		http.DefaultTransport = recoveryTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("private"))}, nil
		})
		_, failure := agent.OpenRouter("test", "model", "test", nil)(context.Background(), nil)
		http.DefaultTransport = original
		if failure == nil {
			t.Fatal("expected failure")
		}
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "router.go"), []byte("package retained_evidence"), 0600); err != nil {
			t.Fatal(err)
		}
		zero := 0
		attempts := 0
		childID := ""
		children, err := subagent.Open(map[string]config.Subagent{"code": {Dir: dir, Model: "child", RequestRetries: &zero}}, func(_, id string, _ func(agent.Usage)) agent.Complete {
			attempts++
			attempt := attempts
			calls := 0
			if childID == "" {
				childID = id
			} else if childID != id {
				t.Error("recovery created a replacement child")
			}
			return func(_ context.Context, msgs []agent.Message) (agent.Message, error) {
				calls++
				if attempt == 1 && calls == 1 {
					return wireMessage(`{"role":"assistant","tool_calls":[{"id":"read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"router.go\"}"}}]}`), nil
				}
				if attempt == 1 {
					return nil, failure
				}
				data, _ := json.Marshal(msgs)
				if !strings.Contains(string(data), "retained_evidence") {
					t.Error("resume lost completed file read")
				}
				return wireMessage(`{"role":"assistant","content":"Verified from retained evidence."}`), nil
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		defer children.Close()
		store, err := OpenStore(t.TempDir(), "server", "bot", "model")
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f := &backgroundClient{fakeClient: newFake(), replies: make(chan string, 8)}
		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(_ string, _ func(agent.Usage)) agent.Complete {
				calls := 0
				return func(_ context.Context, msgs []agent.Message) (agent.Message, error) {
					calls++
					switch calls {
					case 1:
						return wireMessage(`{"role":"assistant","tool_calls":[{"id":"child","type":"function","function":{"name":"delegate","arguments":"{\"agent\":\"code\",\"task\":\"Inspect router\"}"}}]}`), nil
					case 2, 4:
						return wireMessage(`{"role":"assistant","tool_calls":[{"id":"wait","type":"function","function":{"name":"defer_response","arguments":"{}"}}]}`), nil
					case 3:
						var content string
						_ = json.Unmarshal(msgs[len(msgs)-1]["content"], &content)
						var input struct {
							Completed []string `json:"completedInvestigations"`
						}
						_ = json.Unmarshal([]byte(content), &input)
						if len(input.Completed) != 1 {
							return nil, errors.New("missing completion event")
						}
						args, _ := json.Marshal(map[string]string{"run_id": input.Completed[0], "focus": "Summarize retained evidence"})
						quoted, _ := json.Marshal(string(args))
						return wireMessage(`{"role":"assistant","tool_calls":[{"id":"resume","type":"function","function":{"name":"resume_investigation","arguments":` + string(quoted) + `}}]}`), nil
					case 5:
						return wireMessage(`{"role":"assistant","content":"Investigation recovered; here are the findings."}`), nil
					default:
						return nil, errors.New("unexpected repeated recovery")
					}
				}
			}, func(tui.Event) {}, Options{Subagents: children, Store: store})
		}()
		<-f.ready
		f.events <- input("first", "Inspect router", true)
		time.Sleep(batchWindow)
		synctest.Wait()
		<-f.replies
		if body := <-f.replies; !strings.Contains(body, "recovered") {
			t.Fatal(body)
		}
		synctest.Wait()
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		loaded, err := store.load()
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded) != 1 || len(loaded[0].childRuns) != 1 || attempts != 2 {
			t.Fatal("unexpected task count", attempts)
		}
		r := loaded[0].childRuns[childID]
		if r.Status != "complete" || r.AutoResumes != 1 || r.Target != "first" || !loaded[0].usage.CostMissing {
			t.Fatal("recovery state or unknown cost lost", r.Status, r.AutoResumes, r.Target)
		}
	})
}
