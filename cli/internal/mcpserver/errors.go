package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/pkg/events"
)

// toolErrorOutput is the common error branch of every tool's output schema.
// Success payloads keep their existing shape. Hosts must first check IsError.
type toolErrorOutput struct {
	Error toolFailure `json:"error"`
}

// toolFailure contains only caller-safe facts. Message text is for humans;
// hosts use Code, NextAction, Retry, and Outcome without parsing that text.
type toolFailure struct {
	Code               string   `json:"code" jsonschema:"Stable failure code."`
	Message            string   `json:"message" jsonschema:"Short caller-safe explanation."`
	NextAction         string   `json:"nextAction" jsonschema:"Suggested action; it does not grant authority."`
	Retry              string   `json:"retry" jsonschema:"never, after_change, or after_delay. Never automatically retry after_change."`
	Outcome            string   `json:"outcome" jsonschema:"not_applied, applied, or unknown. Reads use not_applied."`
	MissingPermissions []string `json:"missingPermissions,omitempty" jsonschema:"Confirmed missing RBAC permissions; any one of these alternatives permits the permission gate."`
	RequiredScope      string   `json:"requiredScope,omitempty" jsonschema:"OAuth scope that requires fresh consent."`
	RetryAfterMs       int64    `json:"retryAfterMs,omitempty" jsonschema:"Minimum delay in milliseconds when retry is after_delay."`
	RetrySameRequest   bool     `json:"retrySameRequest,omitempty" jsonschema:"Retry only with the same idempotency key and exact arguments, within 30 minutes of the first attempt."`
}

// Error permits a safe failure to pass through the typed domain adapter.
func (f *toolFailure) Error() string { return f.Code + ": " + f.Message }

func failure(code, message, action, retry string) *toolFailure {
	return &toolFailure{Code: code, Message: message, NextAction: action, Retry: retry, Outcome: "not_applied"}
}

func invalidToolArgument(message string) error {
	return failure("invalid_argument", message, "fix_arguments", "after_change")
}

// This schema is immutable after construction and shared by stateless
// descriptors. Inferring it for every tool on every request is unnecessary.
var toolFailureSchema = sync.OnceValue(func() *jsonschema.Schema {
	schema, err := jsonschema.For[toolErrorOutput](nil)
	if err != nil {
		panic(fmt.Sprintf("MCP error schema: %v", err))
	}
	fields := schema.Properties["error"].Properties
	fields["code"].Enum = []any{"invalid_argument", "idempotency_conflict", "authentication_required", "insufficient_scope", "not_room_member", "permission_denied", "policy_denied", "not_found", "conflict", "rate_limited", "timeout", "temporary_failure"}
	fields["nextAction"].Enum = []any{"fix_arguments", "authorize_scope", "join_room", "contact_administrator", "check_access", "retry", "check_result"}
	fields["retry"].Enum = []any{"never", "after_change", "after_delay"}
	fields["outcome"].Enum = []any{"not_applied", "applied", "unknown"}
	return schema
})

// addTool retains SDK input decoding and output validation. The output schema
// explicitly permits both the unchanged success payload and the common error.
func addTool[In, Out any](server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	successSchema, err := jsonschema.For[Out](nil)
	if err != nil {
		panic(fmt.Sprintf("MCP success schema: %v", err))
	}
	tool.OutputSchema = &jsonschema.Schema{Type: "object", AnyOf: []*jsonschema.Schema{successSchema, toolFailureSchema()}}
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
		result, output, err := handler(ctx, request, input)
		if err != nil {
			if protocol := safeProtocolError(err); protocol != nil {
				return nil, nil, protocol
			}
			f := classifyToolError(err, tool.Name)
			return toolErrorResult(f), toolErrorOutput{Error: *f}, nil
		}
		return result, output, nil
	})
}

// structuredToolErrors handles SDK input validation failures, which occur
// before the typed handler. SDK validation text can include submitted values,
// so it must never be forwarded. Unexpected SDK errors are also sanitized.
func structuredToolErrors(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
		result, err := next(ctx, method, request)
		call, ok := request.(*mcp.CallToolRequest)
		if !ok {
			return result, err
		}
		if err != nil {
			if protocol := safeProtocolError(err); protocol != nil {
				return nil, protocol
			}
			return toolErrorResult(classifyToolError(err, call.Params.Name)), nil
		}
		if result, ok := result.(*mcp.CallToolResult); ok && result.IsError && result.StructuredContent == nil {
			return toolErrorResult(failure("invalid_argument", "Tool arguments do not match the input schema.", "fix_arguments", "after_change")), nil
		}
		return result, nil
	}
}

// safeProtocolError preserves request-level failures. SDK diagnostic text and
// data can contain submitted values, so only the standard code is forwarded.
func safeProtocolError(err error) *jsonrpc.Error {
	if wire, ok := errors.AsType[*jsonrpc.Error](err); ok {
		switch wire.Code {
		case jsonrpc.CodeInvalidParams, jsonrpc.CodeInvalidRequest, jsonrpc.CodeParseError, jsonrpc.CodeMethodNotFound:
			return &jsonrpc.Error{Code: wire.Code, Message: "MCP protocol request rejected"}
		}
	}
	return nil
}

// toolErrorResult also supplies JSON as text for hosts that discard structured
// content. Its first text block remains a short human-readable explanation.
func toolErrorResult(f *toolFailure) *mcp.CallToolResult {
	output := toolErrorOutput{Error: *f}
	raw, _ := json.Marshal(output) // Contains only strings, string slices, and integers.
	return &mcp.CallToolResult{
		IsError: true, StructuredContent: output,
		Content: []mcp.Content{&mcp.TextContent{Text: f.Error()}, &mcp.TextContent{Text: string(raw)}},
	}
}

// writeHTTPFailure preserves HTTP/OAuth status semantics before MCP dispatch.
// The body uses the same error envelope, not a JSON-RPC response without an ID.
func writeHTTPFailure(w http.ResponseWriter, status int, f *toolFailure) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(toolErrorOutput{Error: *f})
}

func scopeFailure(scope string) *toolFailure {
	f := failure("insufficient_scope", "Authorize the required MCP scope through fresh consent.", "authorize_scope", "after_change")
	f.RequiredScope = scope
	return f
}

// postedMessageResultError describes failure to assemble a response after the
// canonical post succeeded. This is not another authorization or input failure.
func postedMessageResultError(err error) *toolFailure {
	f := classifyToolError(err, "post_message")
	if f.Code != "timeout" {
		f.Code = "temporary_failure"
	}
	f.Outcome, f.NextAction, f.Retry = "applied", "check_result", "never"
	f.MissingPermissions, f.RequiredScope, f.RetryAfterMs = nil, "", 0
	f.Message = "The message was posted, but Chatto could not return its details. Check the room before taking another action."
	return f
}

// keyedPostFailure exposes retry safety only for uncertain transient failures.
// Policy failures still need a caller decision. An omitted key keeps the
// ordinary uncertain-write contract.
func keyedPostFailure(err error, key string) error {
	f, ok := errors.AsType[*toolFailure](err)
	if !ok || key == "" || (f.Code != "timeout" && f.Code != "temporary_failure") {
		return err
	}
	f.NextAction, f.Retry, f.RetrySameRequest = "retry", "after_delay", true
	f.Message = "The message result is unavailable. Retry only the same key and exact arguments within 30 minutes of the first attempt."
	return f
}

// classifyToolError never forwards backend error text. Known rejections happen
// before a write; an unrecognized write failure can include a lost commit ACK.
func classifyToolError(err error, name string) *toolFailure {
	if applied, ok := errors.AsType[*core.MessagePostAppliedError](err); ok {
		f := classifyToolError(applied.Cause, name)
		f.Outcome, f.NextAction, f.Retry = "applied", "check_result", "never"
		return f
	}
	if safe, ok := errors.AsType[*toolFailure](err); ok {
		return safe
	}
	switch {
	case errors.Is(err, core.ErrMessageIdempotencyConflict):
		return failure("idempotency_conflict", "This send key already belongs to different arguments. Check the original send before starting another operation.", "check_result", "never")
	case errors.Is(err, auth.ErrInvalidToken), errors.Is(err, core.ErrNotAuthenticated):
		return failure("authentication_required", "Authorize this MCP connection again.", "authorize_scope", "after_change")
	case errors.Is(err, core.ErrInvalidArgument), errors.Is(err, core.ErrMessageTooLong):
		return failure("invalid_argument", "The arguments are not valid for this operation.", "fix_arguments", "after_change")
	case errors.Is(err, core.ErrNotRoomMember):
		return failure("not_room_member", "This operation requires current room membership.", "join_room", "after_change")
	case errors.Is(err, core.ErrPermissionDenied), errors.Is(err, core.ErrRoomArchived), errors.Is(err, core.ErrRoomThreadingPolicy), errors.Is(err, core.ErrCannotLeaveDMConversation), errors.Is(err, core.ErrCannotLeaveUniversalRoom):
		return failure("policy_denied", "Chatto authorization or policy does not permit this operation. Ask an administrator to check access.", "contact_administrator", "after_change")
	case errors.Is(err, core.ErrNotFound), errors.Is(err, core.ErrMessageNotFound):
		f := failure("not_found", "The requested resource does not exist or is not visible. Check access with your administrator.", "check_access", "after_change")
		// Membership commands can fail when reading state after a commit.
		if name == "join_room" || name == "leave_room" {
			f.Outcome = "unknown"
		}
		return f
	case errors.Is(err, core.ErrSlowModeActive):
		f := failure("rate_limited", "Wait before trying this operation again.", "retry", "after_delay")
		if slow, ok := errors.AsType[*core.SlowModeActiveError](err); ok {
			f.RetryAfterMs = max(1, int64(math.Ceil(time.Until(slow.NextPostAt).Seconds()*1000)))
		}
		return f
	case errors.Is(err, events.ErrConflict):
		return failure("conflict", "Concurrent state changed. Try the operation again.", "retry", "after_delay")
	}
	f := failure("temporary_failure", "Chatto could not complete the request. Try again later.", "retry", "after_delay")
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		f.Code, f.Message = "timeout", "The request timed out or was canceled."
	}
	if name == "post_message" || name == "join_room" || name == "leave_room" {
		f.Outcome = "unknown"
	}
	if name == "post_message" {
		f.NextAction, f.Retry = "check_result", "never"
		f.Message += " The message may have been posted. Check the room before taking another action."
	}
	return f
}

// requireVisibleToolRoom prevents both success and error responses from being
// existence probes. It adds no authority: the canonical operation still checks
// membership, permissions, policy, and the verified credential before a write.
func requireVisibleToolRoom(ctx context.Context, chattoCore *core.ChattoCore, userID, roomID string) error {
	visible, err := toolRoomVisible(ctx, chattoCore, userID, roomID)
	if err != nil {
		// No domain command has started, so a transient preflight failure is
		// safe to retry, also for message posting.
		return classifyToolError(err, "")
	}
	if !visible {
		return classifyToolError(core.ErrNotFound, "")
	}
	return nil
}

func toolRoomVisible(ctx context.Context, chattoCore *core.ChattoCore, userID, roomID string) (bool, error) {
	var visible bool
	err := chattoCore.ReadServerContentView(ctx, func(readCtx context.Context, _ uint64) error {
		var err error
		visible, err = toolRoomVisibleInView(readCtx, chattoCore, userID, roomID)
		return err
	})
	return visible, err
}

func toolRoomVisibleInView(ctx context.Context, chattoCore *core.ChattoCore, userID, roomID string) (bool, error) {
	room, err := chattoCore.FindRoomByID(ctx, roomID)
	if errors.Is(err, core.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if core.KindOfRoom(room) == core.KindDM {
		return chattoCore.RoomMembershipExists(ctx, core.KindDM, userID, roomID)
	}
	return chattoCore.CanSeeRoom(ctx, userID, core.KindOfRoom(room), roomID)
}

// toolOperationError gates detail on current visibility. Room existence, kind,
// membership, policy, and permissions must not distinguish a hidden target
// from an absent one. Failure to establish visibility is not proof of absence.
func toolOperationError(ctx context.Context, chattoCore *core.ChattoCore, userID, roomID string, permissions []core.Permission, err error, name string) error {
	f := classifyToolError(err, name)
	_, applied := errors.AsType[*core.MessagePostAppliedError](err)
	if applied {
		permissions = []core.Permission{core.PermMessageRead}
	}
	if roomID == "" || f.Code == "temporary_failure" || f.Code == "timeout" || f.Code == "conflict" {
		return f
	}
	// Visibility and all alternative permission checks share one in-memory
	// generation. A concurrent revocation cannot mix hidden state with an
	// explanation from another projection generation.
	viewErr := chattoCore.ReadServerContentView(ctx, func(readCtx context.Context, _ uint64) error {
		visible, visibilityErr := toolRoomVisibleInView(readCtx, chattoCore, userID, roomID)
		if visibilityErr != nil {
			return visibilityErr
		}
		if !visible {
			f = classifyToolError(core.ErrNotFound, name)
			return nil
		}
		if errors.Is(err, core.ErrPermissionDenied) {
			missing := missingRoomPermissions(readCtx, chattoCore, userID, roomID, permissions)
			if len(missing) > 0 {
				f = failure("permission_denied", "Chatto RBAC requires "+strings.Join(missing, " or ")+". Ask an administrator to grant access.", "contact_administrator", "after_change")
				f.MissingPermissions = missing
			}
		}
		return nil
	})
	if viewErr != nil {
		f = classifyToolError(viewErr, name)
	}
	if applied {
		// Visibility sanitization can replace the explanation, but it must
		// retain the confirmed write outcome and prevent a new send.
		f.Outcome, f.NextAction, f.Retry = "applied", "check_result", "never"
	}
	return f
}

// missingRoomPermissions returns identifiers only when every alternative is
// confirmed missing. A failed check or a newly allowed permission stays generic.
func missingRoomPermissions(ctx context.Context, chattoCore *core.ChattoCore, userID, roomID string, permissions []core.Permission) []string {
	if roomID == "" || len(permissions) == 0 {
		return nil
	}
	room, err := chattoCore.FindRoomByID(ctx, roomID)
	if err != nil || room == nil {
		return nil
	}
	missing := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		explanation, err := chattoCore.PermResolver().ExplainRoomPermission(ctx, userID, core.KindOfRoom(room), roomID, permission)
		if err != nil || explanation.State == core.DecisionAllow {
			return nil
		}
		missing = append(missing, string(permission))
	}
	return missing
}
