// Package mcpclient adapts Chatto and configured external MCP tools to Gobotto.
// Tool definitions and server instructions stay fixed until restart. Remote
// prompt and resource catalogs are not imported. Each server owns one
// serialized, reconnectable HTTP session.
package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const callTimeout = 30 * time.Second
const maxResultBytes = 24000
const maxWireBytes = 4 << 20

// Manager owns sessions until Close. Tools returns the startup catalog in a
// stable order. Callers must stop agent work before closing the manager.
type Manager struct {
	servers       []*server
	tools         []agent.Tool
	instructions  map[string]string
	chatto        *server
	delivery      *server
	minimumCursor atomic.Pointer[string]
}
type server struct {
	mu           sync.Mutex
	config       config.MCPServer
	headers      http.Header
	session      *mcp.ClientSession
	definitions  map[string]*mcp.Tool
	closed       bool
	discoverAll  bool
	hostDelivery bool
	cursor       func() string
	instructions string
}

// ChattoConnection supplies the host's existing Chatto credential directly.
// It is never read from remote instructions or exposed in the tool catalog.
type ChattoConnection struct{ URL, Token string }

// Open discovers explicit external tools and, when supplied, the connected
// Chatto server’s complete catalog. Credentials remain in HTTP headers.
func Open(ctx context.Context, cfg config.MCP, chatto ...ChattoConnection) (*Manager, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	m := &Manager{instructions: map[string]string{}}
	if len(chatto) > 1 {
		return nil, errors.New("only one Chatto connection is supported")
	}
	if len(chatto) == 1 {
		if _, exists := cfg.Servers["chatto"]; exists {
			return nil, errors.New("MCP server name chatto is reserved for the connected Chatto server")
		}
		endpoint, err := url.Parse(chatto[0].URL)
		if err != nil || endpoint.Host == "" || endpoint.User != nil || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || strings.ContainsAny(chatto[0].Token, "\r\n") || chatto[0].Token == "" {
			return nil, errors.New("invalid Chatto MCP connection")
		}
		endpoint.Path = "/mcp"
		endpoint.RawPath = ""
		endpoint.RawQuery = ""
		endpoint.Fragment = ""
		m.chatto = &server{config: config.MCPServer{URL: endpoint.String()}, headers: http.Header{"Authorization": []string{"Bearer " + chatto[0].Token}}, discoverAll: true, cursor: m.chattoCursor}
	}

	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	if m.chatto != nil {
		names = append(names, "chatto")
	}
	slices.Sort(names)
	for _, name := range names {
		s := &server{config: cfg.Servers[name], headers: http.Header{}}
		if name == "chatto" && m.chatto != nil {
			s = m.chatto
		}
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
			if name == "chatto" && m.chatto != nil {
				return nil, errors.New("Chatto MCP unavailable: enable [mcp].enabled on the connected server and check the bot credential")
			}
			return nil, errors.New("MCP server " + name + " unavailable or configured tools invalid")
		}
		if s == m.chatto {
			post := s.definitions["post_message"]
			supported := false
			if post != nil {
				schema, _ := post.InputSchema.(map[string]any)
				properties, _ := schema["properties"].(map[string]any)
				_, supported = properties["thread_root_message_id"]
			}
			if !supported || s.definitions["add_reaction"] == nil {
				m.Close()
				return nil, errors.New("Chatto MCP needs thread-aware post_message and add_reaction; update the connected Chatto server")
			}
		}
		toolNames := make([]string, 0, len(s.definitions))
		for toolName := range s.definitions {
			toolNames = append(toolNames, toolName)
		}
		slices.Sort(toolNames)
		for _, toolName := range toolNames {
			remote := s.definitions[toolName]
			m.instructions["mcp_"+name+"__"+toolName] = s.instructions
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
	if m.chatto != nil {
		// Host delivery has a fixed contract and a separate session. Model tool
		// snapshots can remain pinned without blocking ordinary replies on upgrades.
		m.delivery = &server{config: config.MCPServer{URL: m.chatto.config.URL, Tools: []string{"post_message", "add_reaction"}}, headers: m.chatto.headers.Clone(), hostDelivery: true, cursor: m.chattoCursor, definitions: map[string]*mcp.Tool{"post_message": m.chatto.definitions["post_message"], "add_reaction": m.chatto.definitions["add_reaction"]}}
		m.servers = append(m.servers, m.delivery)
	}
	return m, nil
}

// SetChattoCursor records the latest durably accepted realtime boundary. It is
// sent only to Chatto, never to external MCP servers. An empty cursor clears it
// after a recovery gap; subsequent checkpoints supply a fresh boundary.
func (m *Manager) SetChattoCursor(cursor string) { m.minimumCursor.Store(&cursor) }
func (m *Manager) chattoCursor() string {
	if cursor := m.minimumCursor.Load(); cursor != nil {
		return *cursor
	}
	return ""
}

// Tools exposes a compact discovery and dispatch pair. Full remote schemas are
// retrieved on demand and retained as ordinary tool results in session history.
func (m *Manager) Tools() []agent.Tool {
	if len(m.tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(m.tools))
	catalog := make([]agent.ToolInfo, 0, len(m.tools))
	for _, tool := range m.tools {
		names = append(names, tool.Name+" ("+schemaRevision(tool, m.instructions[tool.Name])+")")
		catalog = append(catalog, agent.ToolInfo{Name: tool.Name, Description: tool.Description, Via: "mcp_discover"})
	}
	return []agent.Tool{
		{Name: "mcp_discover", Description: "Get an external tool's description and argument schema when needed. Reuse a schema from this session when its revision matches; do not discover again every turn. Available tools (schema revision): " + strings.Join(names, ", "),
			Catalog:    catalog,
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
	httpClient := &http.Client{Timeout: callTimeout, Transport: credentialTransport{headers: s.headers, cursor: s.cursor}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
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
		if !s.discoverAll && !slices.Contains(s.config.Tools, tool.Name) {
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
		if previous := s.definitions[tool.Name]; previous != nil && !s.hostDelivery {
			old, _ := json.Marshal(previous)
			if string(old) != string(data) {
				return errors.New("MCP definition changed; restart required")
			}
		}
		definitions[tool.Name] = tool
	}
	if (!s.discoverAll && len(definitions) != len(s.config.Tools)) || (s.definitions != nil && len(definitions) != len(s.definitions)) {
		return errors.New("configured MCP tool missing")
	}
	for name := range s.definitions {
		if definitions[name] == nil {
			return errors.New("MCP catalog changed; restart required")
		}
	}
	instructions := session.InitializeResult().Instructions
	if len(instructions) > 16000 {
		return errors.New("MCP instructions too large")
	}
	if s.definitions != nil && instructions != s.instructions && !s.hostDelivery {
		return errors.New("MCP instructions changed; restart required")
	}
	if s.hostDelivery {
		post := definitions["post_message"]
		schema, _ := post.InputSchema.(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		if _, ok := properties["thread_root_message_id"]; !ok {
			return errors.New("Chatto host delivery contract unavailable")
		}
	}
	s.instructions = instructions
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
type credentialTransport struct {
	headers http.Header
	cursor  func() string
}

func (t credentialTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r := request.Clone(request.Context())
	r.Header = request.Header.Clone()
	for key, values := range t.headers {
		r.Header[key] = append([]string(nil), values...)
	}
	if t.cursor != nil && r.Header.Get("Mcp-Method") == "tools/call" {
		if cursor := t.cursor(); cursor != "" {
			r.Header.Set("Chatto-Realtime-Minimum-Cursor", cursor)
		}
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

// Deliver executes one host-selected Chatto action without retries. A transport,
// tool, or malformed-result failure is uncertain; callers retain their existing
// delivery checkpoint and must not replay it automatically.
func (m *Manager) Deliver(ctx context.Context, name string, arguments any) error {
	if name != "post_message" && name != "add_reaction" {
		return errors.New("unsupported Chatto delivery operation")
	}
	if m.delivery == nil {
		return errors.New("Chatto MCP unavailable")
	}
	raw, err := json.Marshal(arguments)
	if err != nil {
		return errors.New("invalid Chatto delivery arguments")
	}
	result := m.delivery.call(ctx, name, raw)
	var envelope struct {
		Status    string
		Content   string
		Truncated bool
	}
	if json.Unmarshal([]byte(result), &envelope) != nil || envelope.Status != "complete" || envelope.Truncated {
		return errors.New("Chatto MCP delivery failed or is uncertain")
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal([]byte(envelope.Content), &payload) != nil {
		return errors.New("invalid Chatto delivery receipt")
	}
	switch name {
	case "post_message":
		var message struct{ ID string }
		if json.Unmarshal(payload["message"], &message) != nil || message.ID == "" {
			return errors.New("missing Chatto message receipt")
		}
	case "add_reaction":
		var id string
		if json.Unmarshal(payload["messageId"], &id) != nil || id == "" {
			return errors.New("missing Chatto reaction receipt")
		}
	default:
		return errors.New("unsupported Chatto delivery operation")
	}
	return nil
}
