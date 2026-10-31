package mcpclient

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
)

func toolPayload(tools []agent.Tool) []byte {
	var definitions []any
	for _, tool := range tools {
		definitions = append(definitions, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": tool.Parameters}})
	}
	data, _ := json.Marshal(definitions)
	return data
}

func TestLazyDiscoverySurvivesTurnsAndRestore(t *testing.T) {
	calls := 0
	m := &Manager{tools: []agent.Tool{{Name: "mcp_test__search", Description: strings.Repeat("long external documentation ", 100), Parameters: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string", "description": "schema-marker"}}}, Execute: func(context.Context, json.RawMessage) (string, error) {
		calls++
		return `{"status":"complete","content":"found"}`, nil
	}}}}
	tools := m.Tools()
	if len(tools) != 2 || strings.Contains(string(toolPayload(tools)), "schema-marker") || len(toolPayload(tools)) >= len(toolPayload(m.tools)) {
		t.Fatal("remote schema is still in the always-present tools")
	}
	before := string(toolPayload(tools))
	session := agent.NewSession("context")
	for turn := 0; turn < 2; turn++ {
		step := 0
		_, err := session.Turn(agent.WithTools(context.Background(), m.Tools()), func(_ context.Context, messages []agent.Message) (agent.Message, error) {
			var message agent.Message
			if turn == 0 && step == 0 {
				_ = json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"discover","type":"function","function":{"name":"mcp_discover","arguments":"{\"name\":\"mcp_test__search\"}"}}]}`), &message)
			} else if (turn == 0 && step == 1) || (turn == 1 && step == 0) {
				history, _ := json.Marshal(messages)
				if !strings.Contains(string(history), "schema-marker") {
					t.Error("discovered schema missing from retained history")
				}
				_ = json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"mcp_call","arguments":"{\"name\":\"mcp_test__search\",\"arguments\":{\"query\":\"test\"}}"}}]}`), &message)
			} else {
				_ = json.Unmarshal([]byte(`{"role":"assistant","content":"Found it."}`), &message)
			}
			step++
			return message, nil
		}, nil, "look it up")
		if err != nil {
			t.Fatal(err)
		}
		session.RecordDelivery(nil)
		session = agent.Restore(session.Snapshot())
	}
	if calls != 2 || string(toolPayload(m.Tools())) != before {
		t.Fatal("discovery executed a tool or changed the exposed schemas")
	}
	oldRevision := schemaRevision(m.tools[0])
	m.tools[0].Parameters["description"] = "updated schema"
	if schemaRevision(m.tools[0]) == oldRevision || string(toolPayload(m.Tools())) == before {
		t.Fatal("changed schema was not advertised to restored sessions")
	}
	for _, request := range []string{`{"name":"not_allowed","arguments":{}}`, `{"name":"memory_brain_create","arguments":{}}`, `{"name":"mcp_test__search","arguments":{},"extra":true}`, `{} {}`} {
		result, _ := m.dispatch(context.Background(), json.RawMessage(request))
		if !strings.Contains(result, `"status":"rejected"`) || calls != 2 {
			t.Fatal("dispatcher bypassed allowlist or accepted invalid envelope")
		}
	}
	// A restored schema must not authorize a tool removed from configuration.
	m.tools = nil
	result, _ := m.dispatch(context.Background(), json.RawMessage(`{"name":"mcp_test__search","arguments":{}}`))
	if !strings.Contains(result, `"status":"rejected"`) || len(m.Tools()) != 0 {
		t.Fatal("removed tool remains available")
	}
}
