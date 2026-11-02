package bot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestToolSearchMatchesRemoteDescriptionsWithoutExecution(t *testing.T) {
	tool := searchTool([]agent.Tool{
		{Name: "memory_brain_create", Description: "Create an app brain."},
		{Name: "mcp_discover", Catalog: []agent.ToolInfo{{Name: "mcp_example__lookup", Description: "Search repository issues", Via: "mcp_discover"}}, Execute: func(context.Context, json.RawMessage) (string, error) {
			t.Fatal("search executed another tool")
			return "", nil
		}},
	}, "owner-only")
	for _, tc := range []struct{ query, want string }{{"ISSUES", "mcp_example__lookup"}, {"brain create", "memory_brain_create"}, {"", "stay_silent"}} {
		raw, _ := json.Marshal(map[string]string{"query": tc.query})
		result, err := tool.Execute(context.Background(), raw)
		if err != nil || !strings.Contains(result, tc.want) || !strings.Contains(result, "current_turn") {
			t.Fatal("missing search result", result, err)
		}
		if tc.query == "ISSUES" && (!strings.Contains(result, `"via":"mcp_discover"`) || strings.Contains(result, "memory_brain_create")) {
			t.Fatal("incorrect indirect search result", result)
		}
	}
	result, _ := tool.Execute(context.Background(), json.RawMessage(`{"query":"no-such-capability"}`))
	if !strings.Contains(result, `"tools":[]`) || !strings.Contains(result, "not exhaustive") {
		t.Fatal("keyword miss implied capability absence", result)
	}
	for _, raw := range []string{`null`, `{}`, `{"query":null}`, `{"query":1}`, `{"query":"","extra":true}`, `{"query":""} {}`, `{"query":"` + strings.Repeat("a", 201) + `"}`} {
		result, err := tool.Execute(context.Background(), json.RawMessage(raw))
		if err != nil || !strings.Contains(result, `"status":"rejected"`) {
			t.Fatal("invalid search accepted", raw, result, err)
		}
	}
}

func TestToolSearchRefreshesAcrossRestoredTurns(t *testing.T) {
	c := &conversation{key: threadKey{"room", "root"}, ownerUserIDs: []string{"human"}}
	f := newFake()
	for turn := 0; turn < 3; turn++ {
		// Simulate replacement of host-selected capabilities between turns. The
		// retained transcript must not restore a tool that the host removed.
		c.tools = nil
		if turn == 1 {
			c.tools = []agent.Tool{{Name: "memory_brain_create", Description: "Create an app brain", Execute: func(context.Context, json.RawMessage) (string, error) {
				t.Fatal("search performed a mutation")
				return "", nil
			}}}
		}
		step := 0
		c.complete = func(_ context.Context, messages []agent.Message) (agent.Message, error) {
			var content string
			if err := json.Unmarshal(messages[len(messages)-1]["content"], &content); err != nil {
				t.Fatal(err)
			}
			if step == 0 {
				var input struct {
					Guidance string `json:"toolGuidance"`
				}
				if err := json.Unmarshal([]byte(content), &input); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(input.Guidance, "Use tool_search in this turn") {
					t.Fatal("missing turn reminder")
				}
				step++
				var call agent.Message
				if err := json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"search","type":"function","function":{"name":"tool_search","arguments":"{\"query\":\"\"}"}}]}`), &call); err != nil {
					t.Fatal(err)
				}
				return call, nil
			}
			if strings.Contains(content, `"name":"memory_brain_create"`) != (turn == 1) {
				t.Fatal("catalog retained stale permissions", content)
			}
			if turn == 0 && !strings.Contains(content, "not entirely owner-authored") {
				t.Fatal("missing authorization reason")
			}
			if turn == 1 && !strings.Contains(content, "earlier unavailability no longer applies") {
				t.Fatal("missing restored availability")
			}
			if turn == 2 && !strings.Contains(content, "not configured") {
				t.Fatal("missing configuration reason")
			}
			return answer(), nil
		}
		batch := []chatto.MessageEvent{input(string(rune('a'+turn)), "create an app", true)}
		if turn == 0 {
			guest := input("guest", "aside", false)
			guest.Message.ActorID = "guest"
			batch = append(batch, guest)
		}
		if err := respond(context.Background(), f, c, batch, nil); err != nil {
			t.Fatal(err)
		}
		c.model = agent.Restore(c.model.Snapshot())
	}
}

func TestCompletionToolSearchCannotExposeUserCapabilities(t *testing.T) {
	tools := []agent.Tool{{Name: "investigation_status", Description: "Read a report"}}
	access := toolAccess(false, false, tools)
	result, err := searchTool(tools, access).Execute(context.Background(), json.RawMessage(`{"query":""}`))
	if err != nil || !strings.Contains(result, "completion notification") || strings.Contains(result, "memory_brain_create") || strings.Contains(result, "mcp_call") {
		t.Fatal("completion discovery expanded authority", result, err)
	}
}
