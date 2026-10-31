package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestReconnectDoesNotReplayOrChangeTools(t *testing.T) {
	remote := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	install := func(description string) {
		remote.AddTool(&mcp.Tool{Name: "search", Description: description, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "found"}}}, nil
		})
	}
	install("original")
	var failNext atomic.Bool
	var attempts atomic.Int32
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return remote }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(data))
		var request struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(data, &request)
		if request.Method == "tools/call" {
			attempts.Add(1)
			if failNext.Swap(false) {
				http.Error(w, "sensitive transport error", 500)
				return
			}
		}
		handler.ServeHTTP(w, r)
	}))
	defer s.Close()
	m, err := Open(context.Background(), config.MCP{Servers: map[string]config.MCPServer{"test": {URL: s.URL, Tools: []string{"search"}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	tool := m.tools[0]
	failNext.Store(true)
	result, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if attempts.Load() != 1 || !strings.Contains(result, `"status":"failed"`) || strings.Contains(result, "sensitive") {
		t.Fatal("failed request retried or leaked")
	}
	result, _ = tool.Execute(context.Background(), json.RawMessage(`{}`))
	if attempts.Load() != 2 || !strings.Contains(result, `"status":"complete"`) {
		t.Fatal("next call did not reconnect", result)
	}
	install("changed instructions")
	failNext.Store(true)
	tool.Execute(context.Background(), json.RawMessage(`{}`))
	result, _ = tool.Execute(context.Background(), json.RawMessage(`{}`))
	if attempts.Load() != 3 || !strings.Contains(result, `"status":"failed"`) {
		t.Fatal("changed definitions accepted or call replayed")
	}
}

func TestCredentialRedirectIsNotFollowed(t *testing.T) {
	t.Setenv("GOBOTTO_TEST_MCP_KEY", "private-key")
	var reached atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Store(true) }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	m, err := Open(context.Background(), config.MCP{Servers: map[string]config.MCPServer{"test": {URL: source.URL, Tools: []string{"search"}, OptionalHeadersFromEnv: map[string]string{"x-api-key": "GOBOTTO_TEST_MCP_KEY"}}}})
	if err == nil {
		m.Close()
		t.Fatal("redirect accepted")
	}
	if reached.Load() || strings.Contains(err.Error(), "private-key") {
		t.Fatal("credential redirect followed or leaked")
	}
}

func TestHTTPTools(t *testing.T) {
	for _, returnJSON := range []bool{false, true} {
		t.Run(map[bool]string{false: "sse", true: "json"}[returnJSON], func(t *testing.T) {
			t.Setenv("GOBOTTO_TEST_MCP_KEY", "test-credential-value")
			remote := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
			var calls atomic.Int32
			for _, name := range []string{"search", "fetch", "not_allowed"} {
				remote.AddTool(&mcp.Tool{Name: name, Description: "Read public data", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					calls.Add(1)
					if strings.Contains(string(req.Params.Arguments), "fail") {
						return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Invalid query test-credential-value"}}}, nil
					}
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "https://example.com test-credential-value " + strings.Repeat("é", maxResultBytes)}}}, nil
				})
			}
			handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return remote }, &mcp.StreamableHTTPOptions{JSONResponse: returnJSON})
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-api-key") != "test-credential-value" {
					t.Error("missing optional header")
				}
				handler.ServeHTTP(w, r)
			}))
			defer httpServer.Close()
			manager, err := Open(context.Background(), config.MCP{Servers: map[string]config.MCPServer{"test": {URL: httpServer.URL, Tools: []string{"search", "fetch"}, OptionalHeadersFromEnv: map[string]string{"x-api-key": "GOBOTTO_TEST_MCP_KEY"}}}})
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			tools := manager.tools
			if len(tools) != 2 || tools[0].Name != "mcp_test__fetch" || tools[1].Name != "mcp_test__search" {
				t.Fatal("allowlist or ordering incorrect")
			}
			result, err := tools[1].Execute(context.Background(), json.RawMessage(`{"query":"public query"}`))
			if err != nil || !strings.Contains(result, `"truncated":true`) || !strings.Contains(result, "https://example.com") || strings.Contains(result, "test-credential-value") || len(result) > maxResultBytes+200 {
				t.Fatal("result not bounded or credential leaked")
			}
			result, _ = tools[1].Execute(context.Background(), json.RawMessage(`{"query":"fail"}`))
			if !strings.Contains(result, `"status":"failed"`) || !strings.Contains(result, "Invalid query") || strings.Contains(result, "test-credential-value") || calls.Load() != 2 {
				t.Fatal("tool error hidden, leaked, or retried")
			}
			result, _ = tools[0].Execute(context.Background(), json.RawMessage(`null`))
			if !strings.Contains(result, `"status":"rejected"`) || calls.Load() != 2 {
				t.Fatal("invalid arguments sent")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			tools[0].Execute(ctx, json.RawMessage(`{}`))
			if calls.Load() != 2 {
				t.Fatal("cancelled request sent")
			}
			modelCalls := 0
			var states []string
			_, err = agent.NewSession("public context").Turn(agent.WithTools(context.Background(), tools), func(_ context.Context, messages []agent.Message) (agent.Message, error) {
				modelCalls++
				var message agent.Message
				if modelCalls == 1 {
					_ = json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"search","type":"function","function":{"name":"mcp_test__search","arguments":"{\"query\":\"fail\"}"}}]}`), &message)
				} else {
					if !strings.Contains(string(messages[len(messages)-1]["content"]), "Invalid query") {
						t.Error("model lost actionable tool error")
					}
					_ = json.Unmarshal([]byte(`{"role":"assistant","content":"Search was unavailable."}`), &message)
				}
				return message, nil
			}, nil, "search", func(event agent.ToolEvent) { states = append(states, event.State) })
			if err != nil || strings.Join(states, ",") != "running,failed" || calls.Load() != 3 {
				t.Fatal("MCP failure was not recorded as a failed agent tool", err, states)
			}
		})
	}
}

func TestKeylessAndMissingTool(t *testing.T) {
	t.Setenv("GOBOTTO_TEST_MCP_KEY", "")
	remote := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	remote.AddTool(&mcp.Tool{Name: "search", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{StructuredContent: map[string]string{"url": "https://example.com"}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return remote }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "" {
			t.Error("keyless request sent credentials")
		}
		handler.ServeHTTP(w, r)
	}))
	defer s.Close()
	cfg := config.MCP{Servers: map[string]config.MCPServer{"test": {URL: s.URL, Tools: []string{"search"}, OptionalHeadersFromEnv: map[string]string{"x-api-key": "GOBOTTO_TEST_MCP_KEY"}}}}
	m, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := m.tools[0].Execute(context.Background(), json.RawMessage(`{}`))
	if !strings.Contains(result, "https://example.com") {
		t.Fatal("structured result lost")
	}
	m.Close()
	m.Close()
	cfg.Servers["test"] = config.MCPServer{URL: s.URL, Tools: []string{"missing"}}
	if m, err := Open(context.Background(), cfg); err == nil {
		m.Close()
		t.Fatal("missing configured tool accepted")
	}
}

// This opt-in smoke test never reads an API key or contacts the model/Chatto.
func TestExaKeylessSmoke(t *testing.T) {
	if os.Getenv("GOBOTTO_EXA_SMOKE") != "1" {
		t.Skip("set GOBOTTO_EXA_SMOKE=1 to verify the public keyless endpoint")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	m, err := Open(ctx, config.MCP{Servers: map[string]config.MCPServer{"exa": {URL: "https://mcp.exa.ai/mcp?tools=web_search_exa,web_fetch_exa", Tools: []string{"web_search_exa", "web_fetch_exa"}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	oldSize, newSize := len(toolPayload(m.tools)), len(toolPayload(m.Tools()))
	t.Logf("MCP tool-definition JSON: %d -> %d bytes (%.1f%% smaller)", oldSize, newSize, 100*(1-float64(newSize)/float64(oldSize)))
	for _, tool := range m.tools {
		discoveryArgs, _ := json.Marshal(map[string]string{"name": tool.Name})
		description, _ := m.discover(ctx, discoveryArgs)
		if !strings.Contains(description, `"parameters"`) {
			t.Fatal("schema discovery failed")
		}
		args := json.RawMessage(`{"query":"Exa official API documentation","numResults":1}`)
		if strings.HasSuffix(tool.Name, "web_fetch_exa") {
			args = json.RawMessage(`{"urls":["https://example.com"]}`)
		}
		callArgs, _ := json.Marshal(map[string]any{"name": tool.Name, "arguments": args})
		result, err := m.dispatch(ctx, callArgs)
		if err != nil || !strings.Contains(result, `"status":"complete"`) {
			t.Fatalf("%s failed: %s (%v)", tool.Name, result, err)
		}
		t.Log(tool.Name + " succeeded without credentials")
	}
}
