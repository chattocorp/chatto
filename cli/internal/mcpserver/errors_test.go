package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/pkg/events"
)

// Exercise the real descriptors and canonical operations, not handler calls.
// Each contract runs through both the official SDK and independent raw HTTP.
func TestMCPToolErrorContracts(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	c, err := core.NewChattoCore(context.Background(), nc, config.CoreConfig{
		SecretKey: "errors-test-secret", Assets: config.AssetsConfig{SigningSecret: "errors-test-assets"},
	})
	if err != nil {
		t.Fatal(err)
	}
	startTestCore(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	user, err := c.CreateUser(ctx, core.SystemActorID, "errors-user", "Errors User", "password")
	if err != nil {
		t.Fatal(err)
	}
	group, err := c.CreateRoomGroup(ctx, core.SystemActorID, "Errors Rooms", "")
	if err != nil {
		t.Fatal(err)
	}
	createRoom := func(name string, joined bool) string {
		t.Helper()
		room, err := c.CreateRoom(ctx, core.SystemActorID, core.KindChannel, group.GetId(), name, "")
		if err != nil {
			t.Fatal(err)
		}
		if joined {
			if _, err := c.JoinRoom(ctx, user.GetId(), core.KindChannel, user.GetId(), room.GetId()); err != nil {
				t.Fatal(err)
			}
		}
		return room.GetId()
	}
	visible := createRoom("visible-unjoined", false)
	denied := createRoom("visible-denied", true)
	archived := createRoom("private-policy-detail", true)
	hidden := createRoom("private-hidden-name", false)
	for _, permission := range []core.Permission{core.PermRoomJoin, core.PermMessagePost, core.PermMessageRead, core.PermMessageReadInteractions} {
		if err := c.DenyRoomPermission(ctx, core.SystemActorID, denied, core.RoleEveryone, permission); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.DenyRoomPermission(ctx, core.SystemActorID, hidden, core.RoleEveryone, core.PermRoomList); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ArchiveRoom(ctx, core.SystemActorID, core.KindChannel, archived); err != nil {
		t.Fatal(err)
	}
	other, err := c.CreateUser(ctx, core.SystemActorID, "errors-other", "Errors Other", "password")
	if err != nil {
		t.Fatal(err)
	}
	dm, _, err := c.FindOrCreateDM(ctx, other.GetId(), []string{core.SystemActorID})
	if err != nil {
		t.Fatal(err)
	}
	generation, err := c.CurrentAuthGeneration(ctx, user.GetId())
	if err != nil {
		t.Fatal(err)
	}
	credential, err := c.CreateOAuthBearerSessionForClientGrant(ctx, user.GetId(), "https://agent.example/client.json", "https://chat.example/mcp", config.MCPOAuthScopes(), generation)
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"sdk", "raw"} {
		t.Run(mode, func(t *testing.T) {
			handler, err := NewHandler(c, config.ChattoConfig{Webserver: config.WebserverConfig{URL: "https://chat.example"}}, "test")
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			httpClient := server.Client()
			httpClient.Transport = canonicalHostTransport{base: httpClient.Transport, host: "chat.example"}
			client := mcp.NewClient(&mcp.Implementation{Name: "error-contract-test", Version: "test"}, nil)
			session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
				Endpoint: server.URL + "/mcp", DisableStandaloneSSE: true, HTTPClient: httpClient,
				OAuthHandler: staticOAuthHandler{token: credential.AccessToken},
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			catalog, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			call := func(name string, args any) *mcp.CallToolResult {
				t.Helper()
				if mode == "sdk" {
					result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
					if err != nil {
						t.Fatal(err)
					}
					return result
				}
				body := rawToolCallBody(t, name, args)
				response := performRawMCPRequest(t, handler, credential.AccessToken, "tools/call", name, body)
				var wire struct {
					Result *mcp.CallToolResult `json:"result"`
				}
				decodeMCPResponse(t, response, &wire)
				if wire.Result == nil {
					t.Fatal("no MCP tool result")
				}
				return wire.Result
			}
			check := func(name string, args any, code, action string, missing []string) *mcp.CallToolResult {
				t.Helper()
				result := call(name, args)
				f := assertToolFailure(t, result, catalog, name)
				if f.Code != code || f.NextAction != action || !reflect.DeepEqual(f.MissingPermissions, missing) || f.Retry != "after_change" || f.Outcome != "not_applied" {
					t.Fatalf("%s failure = %#v", name, f)
				}
				return result
			}
			// Includes the identity tools: malformed arguments must not echo
			// their value or leak SDK validation diagnostics through text.
			for _, tool := range catalog.Tools {
				result := check(tool.Name, []string{"private-submitted-value"}, "invalid_argument", "fix_arguments", nil)
				raw, _ := json.Marshal(result)
				if strings.Contains(string(raw), "private-submitted-value") {
					t.Fatal("input reflected in failure")
				}
			}
			check("list_rooms", map[string]any{"limit": -1}, "invalid_argument", "fix_arguments", nil)
			check("post_message", map[string]any{"room_id": visible, "body": " "}, "invalid_argument", "fix_arguments", nil)
			for _, name := range []string{"list_room_messages", "post_message"} {
				check(name, toolRoomArguments(name, visible), "not_room_member", "join_room", nil)
			}
			check("join_room", map[string]any{"room_id": denied}, "permission_denied", "contact_administrator", []string{"room.join"})
			check("post_message", map[string]any{"room_id": denied, "body": "Test"}, "permission_denied", "contact_administrator", []string{"message.post"})
			check("list_room_messages", map[string]any{"room_id": denied}, "permission_denied", "contact_administrator", []string{"message.read", "message.read-interactions"})
			policy := check("post_message", map[string]any{"room_id": archived, "body": "Test"}, "policy_denied", "contact_administrator", nil)
			for _, name := range []string{"join_room", "leave_room", "list_room_messages", "post_message"} {
				var absent *mcp.CallToolResult
				for _, roomID := range []string{"absent-room", hidden, dm.GetId()} {
					result := check(name, toolRoomArguments(name, roomID), "not_found", "check_access", nil)
					if absent == nil {
						absent = result
					} else if !reflect.DeepEqual(absent, result) {
						t.Fatalf("%s reveals hidden channel/DM existence", name)
					}
				}
			}
			raw, _ := json.Marshal(policy)
			for _, secret := range []string{"private-policy-detail", archived, "archived", "threading", credential.AccessToken} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("policy details or credential in failure")
				}
			}
			// Replaying an authorization failure after a permission is regained
			// must not invent a missing RBAC permission from the old rejection.
			if mode == "sdk" {
				requestCtx, _, err := authenticatedRequestFromToken(t, c, credential.AccessToken)
				if err != nil {
					t.Fatal(err)
				}
				f := classifyToolError(toolOperationError(requestCtx, c, user.GetId(), visible, []core.Permission{core.PermRoomJoin}, core.ErrPermissionDenied, "join_room"), "join_room")
				if f.Code != "policy_denied" || len(f.MissingPermissions) != 0 {
					t.Fatalf("stale rejection = %#v", f)
				}
			}
		})
	}
	member, err := c.RoomMembershipExists(ctx, core.KindChannel, user.GetId(), hidden)
	if err != nil || member {
		t.Fatal("hidden join changed membership")
	}
}

// This uses the production verifier and credential attachment, including the
// human privileged-mode state, for a post-operation explanation test.
func authenticatedRequestFromToken(t *testing.T, c *core.ChattoCore, token string) (context.Context, string, error) {
	t.Helper()
	var ctx context.Context
	protected := auth.RequireBearerToken(tokenVerifier(c, "https://chat.example/mcp"), &auth.RequireBearerTokenOptions{AllowMissingExpiration: true})(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) { ctx = request.Context() }))
	request := httptest.NewRequest(http.MethodPost, "https://chat.example/mcp", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	protected.ServeHTTP(httptest.NewRecorder(), request)
	if ctx == nil {
		t.Fatal("token context unavailable")
	}
	return authenticatedRequest(ctx)
}

func rawToolCallBody(t *testing.T, name string, args any) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{
		"name": name, "arguments": args, "_meta": map[string]any{
			"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
			"io.modelcontextprotocol/clientInfo":         map[string]string{"name": "raw-error-contract-test", "version": "test"},
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func toolRoomArguments(name, roomID string) map[string]any {
	args := map[string]any{"room_id": roomID}
	if name == "post_message" {
		args["body"] = "Test"
	}
	return args
}

func assertToolFailure(t *testing.T, result *mcp.CallToolResult, catalog *mcp.ListToolsResult, name string) toolFailure {
	t.Helper()
	if result == nil || !result.IsError || result.StructuredContent == nil || len(result.Content) != 2 {
		t.Fatalf("missing error contract: %#v", result)
	}
	var output toolErrorOutput
	decodeStructuredContent(t, result.StructuredContent, &output)
	if output.Error.Code == "" || output.Error.Message == "" || output.Error.NextAction == "" || output.Error.Retry == "" || output.Error.Outcome == "" {
		t.Fatalf("incomplete error: %#v", output)
	}
	text, ok := result.Content[1].(*mcp.TextContent)
	if !ok {
		t.Fatal("no JSON text fallback")
	}
	var fallback toolErrorOutput
	if err := json.Unmarshal([]byte(text.Text), &fallback); err != nil || !reflect.DeepEqual(output, fallback) {
		t.Fatal("JSON fallback differs from structured error")
	}
	tool := mcpToolNamed(catalog.Tools, name)
	var schema jsonschema.Schema
	decodeStructuredContent(t, tool.OutputSchema, &schema)
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	var instance any
	decodeStructuredContent(t, result.StructuredContent, &instance)
	if err := resolved.Validate(instance); err != nil {
		t.Fatalf("%s error violates advertised output schema: %v", name, err)
	}
	return output.Error
}

func TestMCPFailureRetryContract(t *testing.T) {
	for _, test := range []struct {
		name                 string
		err                  error
		code, retry, outcome string
	}{
		{"list_rooms", errors.New("private-internal-diagnostic"), "temporary_failure", "after_delay", "not_applied"},
		{"post_message", errors.New("private-internal-diagnostic"), "temporary_failure", "never", "unknown"},
		{"post_message", context.DeadlineExceeded, "timeout", "never", "unknown"},
		{"post_message", context.Canceled, "timeout", "never", "unknown"},
		{"join_room", context.DeadlineExceeded, "timeout", "after_delay", "unknown"},
		{"leave_room", context.Canceled, "timeout", "after_delay", "unknown"},
		{"post_message", events.ErrConflict, "conflict", "after_delay", "not_applied"},
		{"post_message", &core.SlowModeActiveError{NextPostAt: time.Now().Add(time.Minute)}, "rate_limited", "after_delay", "not_applied"},
		{"post_message", core.ErrRoomThreadingPolicy, "policy_denied", "after_change", "not_applied"},
	} {
		t.Run(test.name+"/"+test.code+"/"+test.err.Error(), func(t *testing.T) {
			f := classifyToolError(fmt.Errorf("private-wrapper: %w", test.err), test.name)
			if f.Code != test.code || f.Retry != test.retry || f.Outcome != test.outcome {
				t.Fatalf("failure = %#v", f)
			}
			if f.Code == "rate_limited" && (f.RetryAfterMs < 1 || f.RetryAfterMs > 60000) {
				t.Fatalf("delay = %d", f.RetryAfterMs)
			}
			if strings.Contains(f.Error(), "private") {
				t.Fatal("backend diagnostic leaked")
			}
		})
	}
	for _, err := range []error{errors.New("private-hydration"), core.ErrNotFound, context.DeadlineExceeded} {
		f := postedMessageResultError(err)
		if f.Outcome != "applied" || f.Retry != "never" || f.NextAction != "check_result" || (f.Code != "temporary_failure" && f.Code != "timeout") {
			t.Fatalf("committed result failure = %#v", f)
		}
	}
}

func TestMCPKeyedPostFailureRetryContract(t *testing.T) {
	for _, err := range []error{context.DeadlineExceeded, errors.New("private-backend"), postedMessageResultError(context.DeadlineExceeded)} {
		failure := classifyToolError(err, "post_message")
		keyed, ok := errors.AsType[*toolFailure](keyedPostFailure(failure, "caller-key"))
		if !ok || !keyed.RetrySameRequest || keyed.Retry != "after_delay" || keyed.NextAction != "retry" {
			t.Fatalf("keyed retry = %#v", keyed)
		}
	}
	for _, err := range []error{core.ErrMessageIdempotencyConflict, &core.MessagePostAppliedError{Cause: core.ErrPermissionDenied}} {
		keyed, ok := errors.AsType[*toolFailure](keyedPostFailure(classifyToolError(err, "post_message"), "caller-key"))
		if !ok || keyed.RetrySameRequest || keyed.Retry != "never" {
			t.Fatalf("unsafe retry advertised: %#v", keyed)
		}
	}
	failure := keyedPostFailure(classifyToolError(context.DeadlineExceeded, "post_message"), "")
	plain, _ := errors.AsType[*toolFailure](failure)
	if plain.RetrySameRequest || plain.Retry != "never" {
		t.Fatalf("unkeyed timeout = %#v", plain)
	}
}

func TestMCPAdmissionFailureContract(t *testing.T) {
	called := false
	handler := withAdmissionLimit(rate.NewLimiter(0, 0), http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	var output toolErrorOutput
	decodeMCPResponse(t, response, &output)
	if called || response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "1" || response.Header().Get("Cache-Control") != "no-store" || output.Error.Code != "rate_limited" || output.Error.Retry != "after_delay" || output.Error.RetryAfterMs != 1000 || output.Error.Outcome != "not_applied" {
		t.Fatalf("admission failure = %d/%#v", response.Code, output)
	}
}

func TestMCPProtocolErrorsRemainProtocolErrors(t *testing.T) {
	for _, code := range []int64{jsonrpc.CodeInvalidParams, jsonrpc.CodeInvalidRequest, jsonrpc.CodeParseError, jsonrpc.CodeMethodNotFound} {
		handler := structuredToolErrors(func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return nil, &jsonrpc.Error{Code: code, Message: "private-protocol-diagnostic", Data: json.RawMessage(`{"private":"submitted-value"}`)}
		})
		result, err := handler(context.Background(), "tools/call", &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "post_message"}})
		wire, ok := errors.AsType[*jsonrpc.Error](err)
		if result != nil || !ok || wire.Code != code || wire.Data != nil || strings.Contains(wire.Message, "private") {
			t.Fatalf("protocol rejection changed or leaked details: result=%#v err=%v", result, err)
		}
	}
}

// Inject failures at the operation boundary to test wire retry semantics even
// when a backend cannot provide a commit acknowledgement. The parent request
// stays live so the server can deliver the timeout result.
func TestMCPFailureWireOutcomes(t *testing.T) {
	for _, test := range []struct {
		name                 string
		err                  error
		code, retry, outcome string
	}{
		{"list_rooms", errors.New("private-backend-error"), "temporary_failure", "after_delay", "not_applied"},
		{"post_message", errors.New("private-backend-error"), "temporary_failure", "never", "unknown"},
		{"post_message", context.DeadlineExceeded, "timeout", "never", "unknown"},
		{"post_message", postedMessageResultError(errors.New("private-hydration-error")), "temporary_failure", "never", "applied"},
		{"post_message", events.ErrConflict, "conflict", "after_delay", "not_applied"},
		{"post_message", &core.SlowModeActiveError{NextPostAt: time.Now().Add(time.Minute)}, "rate_limited", "after_delay", "not_applied"},
		{"join_room", context.DeadlineExceeded, "timeout", "after_delay", "unknown"},
		{"leave_room", context.Canceled, "timeout", "after_delay", "unknown"},
	} {
		t.Run(test.name+"/"+test.code+"/"+test.outcome, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "failure-fixture", Version: "test"}, nil)
			server.AddReceivingMiddleware(structuredToolErrors)
			addTool(server, &mcp.Tool{Name: test.name}, func(context.Context, *mcp.CallToolRequest, getServerInfoInput) (*mcp.CallToolResult, getServerInfoOutput, error) {
				return nil, getServerInfoOutput{}, test.err
			})
			handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true})
			httpServer := httptest.NewServer(handler)
			defer httpServer.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			client := mcp.NewClient(&mcp.Implementation{Name: "failure-client", Version: "test"}, nil)
			session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL, DisableStandaloneSSE: true}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			catalog, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			sdk, err := session.CallTool(ctx, &mcp.CallToolParams{Name: test.name})
			if err != nil {
				t.Fatal(err)
			}
			response := performRawMCPRequest(t, handler, "", "tools/call", test.name, rawToolCallBody(t, test.name, map[string]any{}))
			var raw struct {
				Result *mcp.CallToolResult `json:"result"`
			}
			decodeMCPResponse(t, response, &raw)
			for _, result := range []*mcp.CallToolResult{sdk, raw.Result} {
				f := assertToolFailure(t, result, catalog, test.name)
				if f.Code != test.code || f.Retry != test.retry || f.Outcome != test.outcome {
					t.Fatalf("wire failure = %#v", f)
				}
				wire, _ := json.Marshal(result)
				if strings.Contains(string(wire), "private-") {
					t.Fatal("private backend error in wire response")
				}
			}
		})
	}
}
