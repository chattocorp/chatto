// Package mcpclient adapts explicitly configured remote MCP tools to Gobotto.
// Tool definitions stay fixed until restart; remote prompts and resources are
// not imported. Each server owns one serialized, reconnectable HTTP session.
package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const callTimeout = 30 * time.Second
const maxResultBytes = 24000
const maxWireBytes = 4 << 20

// Manager owns sessions until Close. Tools returns the startup allowlist in a
// stable order. Callers must stop agent work before closing the manager.
type Manager struct {
	servers []*server
	tools   []agent.Tool
}
type server struct {
	mu          sync.Mutex
	config      config.MCPServer
	headers     http.Header
	session     *mcp.ClientSession
	definitions map[string]*mcp.Tool
	closed      bool
}

// Open connects and discovers all configured tools, failing startup if a tool
// is missing. Credentials never enter tool definitions, transcripts, or errors.
func Open(ctx context.Context, cfg config.MCP) (*Manager, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	m := &Manager{}
	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		s := &server{config: cfg.Servers[name], headers: http.Header{}}
		m.servers = append(m.servers, s)
		for header, variable := range s.config.OptionalHeadersFromEnv {
			value := os.Getenv(variable)
			if strings.ContainsAny(value, "\r\n") {
				m.Close()
				return nil, errors.New("invalid MCP credential header")
			}
			if value != "" {
				s.headers.Set(header, value)
			}
		}
		setupCtx, cancel := context.WithTimeout(ctx, callTimeout)
		err := s.connect(setupCtx)
		cancel()
		if err != nil {
			m.Close()
			return nil, errors.New("MCP server " + name + " unavailable or configured tools invalid")
		}
		toolNames := append([]string(nil), s.config.Tools...)
		slices.Sort(toolNames)
		for _, toolName := range toolNames {
			remote := s.definitions[toolName]
			m.tools = append(m.tools, agent.Tool{
				Name:        "mcp_" + name + "__" + toolName,
				Description: "External tool from " + name + ". " + remote.Description,
				Parameters:  remote.InputSchema.(map[string]any),
				Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
					return s.call(ctx, toolName, args), nil
				},
			})
		}
	}
	return m, nil
}

// Tools exposes a compact discovery and dispatch pair. Full remote schemas are
// retrieved on demand and retained as ordinary tool results in session history.
func (m *Manager) Tools() []agent.Tool {
	if len(m.tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(m.tools))
	for _, tool := range m.tools {
		names = append(names, tool.Name+" ("+schemaRevision(tool)+")")
	}
	return []agent.Tool{
		{Name: "mcp_discover", Description: "Get an external tool's description and argument schema when needed. Reuse a schema from this session when its revision matches; do not discover again every turn. Available tools (schema revision): " + strings.Join(names, ", "),
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}, "required": []string{"name"}, "additionalProperties": false},
			Execute:    m.discover},
		{Name: "mcp_call", Description: "Call a configured external tool using its discovered argument schema. Use only a name from mcp_discover's available list. Prior discovery results can be reused across turns.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}, "arguments": map[string]any{"type": "object", "additionalProperties": true}}, "required": []string{"name", "arguments"}, "additionalProperties": false},
			Execute:    m.dispatch},
	}
}

// Close releases every remote session. It is safe to call more than once.
func (m *Manager) Close() {
	for _, s := range m.servers {
		s.mu.Lock()
		s.closed = true
		if s.session != nil {
			_ = s.session.Close()
			s.session = nil
		}
		s.mu.Unlock()
	}
}

func (s *server) connect(ctx context.Context) error {
	client := mcp.NewClient(&mcp.Implementation{Name: "gobotto", Version: "experimental"}, &mcp.ClientOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	httpClient := &http.Client{Timeout: callTimeout, Transport: credentialTransport{s.headers}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: s.config.URL, HTTPClient: httpClient, DisableStandaloneSSE: true, MaxRetries: -1, MaxEventSize: maxWireBytes}, nil)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			_ = session.Close()
		}
	}()
	definitions := map[string]*mcp.Tool{}
	count := 0
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return err
		}
		count++
		if count > 256 {
			return errors.New("too many MCP tools")
		}
		if !slices.Contains(s.config.Tools, tool.Name) {
			continue
		}
		if definitions[tool.Name] != nil {
			return errors.New("duplicate MCP tool")
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok || schema["type"] != "object" {
			return errors.New("unsupported MCP input schema")
		}
		data, err := json.Marshal(tool)
		if err != nil || len(data) > 32000 {
			return errors.New("MCP tool definition too large")
		}
		if previous := s.definitions[tool.Name]; previous != nil {
			old, _ := json.Marshal(previous)
			if string(old) != string(data) {
				return errors.New("MCP definition changed; restart required")
			}
		}
		definitions[tool.Name] = tool
	}
	if len(definitions) != len(s.config.Tools) {
		return errors.New("configured MCP tool missing")
	}
	s.definitions = definitions
	s.session = session
	keep = true
	return nil
}

func feedback(status, message string) string {
	b, _ := json.Marshal(map[string]string{"status": status, "error": message})
	return string(b)
}

func (s *server) call(ctx context.Context, name string, raw json.RawMessage) string {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	// Keep numbers and nested values exact when forwarding model arguments.
	var args map[string]json.RawMessage
	if len(raw) > 64000 || json.Unmarshal(raw, &args) != nil || args == nil {
		return feedback("rejected", "Tool arguments must be a JSON object under 64000 bytes")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || ctx.Err() != nil {
		return feedback("failed", "MCP tool unavailable or cancelled")
	}
	if s.definitions[name] == nil {
		return feedback("rejected", "MCP tool is not allowed")
	}
	if s.session == nil {
		if err := s.connect(ctx); err != nil {
			return feedback("failed", "MCP reconnect failed or tool definitions changed; check server and restart Gobotto")
		}
	}
	result, err := s.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		_ = s.session.Close()
		s.session = nil
		return feedback("failed", "MCP request failed, was rate limited, or timed out. No retry was made; do not repeat a possible write automatically.")
	}
	if result.NeedsInput() {
		return feedback("failed", "Remote MCP tool requires unsupported interaction. No retry was made.")
	}
	var text strings.Builder
	omitted := 0
	for _, content := range result.Content {
		switch c := content.(type) {
		case *mcp.TextContent:
			text.WriteString(c.Text)
			text.WriteByte('\n')
		case *mcp.ResourceLink:
			text.WriteString(c.Name + " " + c.URI + "\n" + c.Description + "\n")
		case *mcp.EmbeddedResource:
			if c.Resource != nil && c.Resource.Text != "" {
				text.WriteString(c.Resource.URI + "\n" + c.Resource.Text + "\n")
			} else {
				omitted++
			}
		default:
			omitted++
		}
	}
	if result.StructuredContent != nil && text.Len() == 0 {
		data, err := json.Marshal(result.StructuredContent)
		if err == nil {
			text.Write(data)
		}
	}
	value := text.String()
	for _, values := range s.headers {
		for _, secret := range values {
			value = strings.ReplaceAll(value, secret, "[redacted]")
			if strings.HasPrefix(secret, "Bearer ") && len(secret) > 7 {
				value = strings.ReplaceAll(value, strings.TrimPrefix(secret, "Bearer "), "[redacted]")
			}
		}
	}
	truncated := len(value) > maxResultBytes
	if truncated {
		value = value[:maxResultBytes]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	status := "complete"
	if result.IsError {
		status = "failed"
	}
	data, _ := json.Marshal(map[string]any{"status": status, "content": value, "truncated": truncated, "untrusted": true, "omitted_blocks": omitted})
	return string(data)
}

// credentialTransport keeps credentials on the configured origin (redirects
// are disabled) and bounds both JSON and SSE response bodies before decoding.
type credentialTransport struct{ headers http.Header }

func (t credentialTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r := request.Clone(request.Context())
	r.Header = request.Header.Clone()
	for key, values := range t.headers {
		r.Header[key] = append([]string(nil), values...)
	}
	response, err := http.DefaultTransport.RoundTrip(r)
	if err == nil {
		response.Body = &boundedBody{Reader: io.LimitReader(response.Body, maxWireBytes), Closer: response.Body}
	}
	return response, err
}

type boundedBody struct {
	io.Reader
	io.Closer
}
