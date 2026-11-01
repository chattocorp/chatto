// Package agent implements an OpenRouter tool-calling conversation with optional work limits.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Message retains provider fields, including reasoning_details, unchanged between
// tool steps. Some models require those fields to resume a tool-calling turn.
type Message map[string]json.RawMessage

// Complete supplies the next assistant message. Implementations must honor cancellation.
type Complete func(context.Context, []Message) (Message, error)

type textOnlyKey struct{}
type workerKey struct{}

// ReadThread reads only the host-selected thread. The model cannot choose a destination.
type ReadThread func(context.Context) (string, error)

// ToolEvent exposes executed tool activity for a private inspector. It excludes
// model reasoning. Arguments and Result can contain private conversation data.
type ToolEvent struct {
	Name, Arguments, State, Result string
	Duration                       time.Duration
}

// Outcome is one host-applied action in the ping's thread. Only one field is set.
type Outcome struct {
	Body, Emoji string
	// Limited distinguishes a forced wrap-up from a normal completion.
	Limited bool
	// Waiting ends a conversation turn without a final message while child work continues.
	Waiting bool
}

// Usage counts provider-reported tokens for one request. CachedInput is included
// in Input, not additional to it. Missing usage is never inferred to be zero.
type Usage struct {
	// Unreported marks an attempted request whose usage and charge are unknown.
	Unreported                 bool
	Input, CachedInput, Output int64
	// CacheKnown distinguishes a reported zero from absent cache telemetry.
	CacheKnown bool
	// Reasoning counts a subset of Output; absence is not a reported zero.
	Reasoning      int64
	ReasoningKnown bool
	// Provider and RequestID identify the serving endpoint and generation for audits.
	Provider, RequestID string
	// Cost is the OpenRouter account charge; nil means it was not reported.
	Cost *float64
}

var emojiName = regexp.MustCompile(`^[A-Za-z0-9_+\-]{1,128}$`)

// ErrOutputLimit means the provider truncated a completion. Its partial output
// must never be delivered or executed, though reported usage still applies.
var ErrOutputLimit = errors.New("model output token limit reached; check the configured cap and provider limits")

// CompletionOptions controls the per-request generation budget.
type CompletionOptions struct {
	// MaxOutputTokens includes reasoning tokens where counted by the provider.
	// Zero omits the cap. Provider defaults and model limits still apply.
	MaxOutputTokens int
	Reasoning       *ReasoningOptions
}

// ReasoningOptions maps the optional unified OpenRouter reasoning controls.
// Nil booleans preserve provider defaults, including when false is meaningful.
type ReasoningOptions struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	Effort    string `json:"effort,omitempty"`
	MaxTokens *int   `json:"max_tokens,omitempty"`
	Exclude   *bool  `json:"exclude,omitempty"`
}

// OpenRouter returns a completion function using a fixed OpenRouter endpoint.
// The model is explicit because price, availability, and tool support vary.
func OpenRouter(key, model, sessionID string, report func(Usage), options ...CompletionOptions) Complete {
	maxOutputTokens := 0
	if len(options) > 0 && options[0].MaxOutputTokens > 0 {
		maxOutputTokens = options[0].MaxOutputTokens
	}
	return func(ctx context.Context, messages []Message) (Message, error) {
		var out struct {
			ID       string `json:"id"`
			Provider string `json:"provider"`
			Choices  []struct {
				Message Message `json:"message"`
				Finish  string  `json:"finish_reason"`
			} `json:"choices"`
			Usage *wireUsage `json:"usage"`
			Error *struct {
				Code json.RawMessage `json:"code"`
			} `json:"error"`
		}
		input := map[string]any{
			"model": model, "messages": messages,
			"session_id": sessionID,
			"tools": []any{map[string]any{"type": "function", "function": map[string]any{
				"name": "read_thread", "description": "Read the most recent messages in this conversation. Older messages can be omitted.",
				"parameters": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
			}}, map[string]any{"type": "function", "function": map[string]any{
				"name": "react_to_ping", "description": "Finish by reacting to the latest triggering message in the batch instead of posting text. This can be an ordinary thread reply without a mention. Use an emoji name such as thumbsup, heart, eyes, or white_check_mark, without colons.",
				"parameters": map[string]any{"type": "object", "properties": map[string]any{"emoji": map[string]any{"type": "string"}}, "required": []string{"emoji"}, "additionalProperties": false},
			}}},
		}
		if maxOutputTokens > 0 {
			input["max_tokens"] = maxOutputTokens
		}
		if worker, _ := ctx.Value(workerKey{}).(bool); worker {
			input["tools"] = []any{}
		}
		for _, tool := range turnTools(ctx) {
			input["tools"] = append(input["tools"].([]any), map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": tool.Parameters}})
		}
		if textOnly, _ := ctx.Value(textOnlyKey{}).(bool); textOnly {
			input["tool_choice"] = "none"
		}
		if len(options) > 0 && options[0].Reasoning != nil {
			input["reasoning"] = options[0].Reasoning
		}
		// Claude requires an explicit cache hint. Other model families keep their
		// provider's implicit caching behavior; session_id helps preserve routing.
		if strings.HasPrefix(strings.TrimPrefix(model, "~"), "anthropic/") {
			input["cache_control"] = map[string]string{"type": "ephemeral"}
		}
		if err := postJSON(ctx, "https://openrouter.ai/api/v1/chat/completions", key, input, &out); err != nil {
			return nil, err
		}
		if out.Usage != nil && report != nil {
			report(out.Usage.report(out.Provider, out.ID))
		}
		if len(out.Choices) != 1 {
			if out.Error != nil {
				var code int
				if json.Unmarshal(out.Error.Code, &code) == nil && code >= 100 && code <= 599 {
					return nil, modelFailure(fmt.Sprintf("Model API error %d", code))
				}
				return nil, modelFailure("Model API returned an error")
			}
			return nil, modelFailure("Expected one model completion")
		}
		choice := out.Choices[0]
		if choice.Finish == "length" {
			return nil, ErrOutputLimit
		}
		if choice.Finish != "stop" && choice.Finish != "tool_calls" {
			if choice.Finish == "content_filter" {
				return nil, modelFailure("Model completion stopped by content filter")
			}
			return nil, modelFailure("Model completion returned an unexpected finish reason")
		}
		return choice.Message, nil
	}
}

// Run executes one standalone turn. Reuse Session for multi-turn conversations.
func Run(ctx context.Context, complete Complete, read ReadThread, input string) (Outcome, error) {
	return (&Session{}).Turn(ctx, complete, read, input)
}

// Session retains the exact model transcript across turns. A single owner must
// serialize Turn and RecordDelivery calls. It contains private conversation data
// and must not be logged. A zero value is ready for use.
type Session struct {
	messages    []Message
	pendingTool string
	worker      bool
	toolLimit   int
}

// NewWorker creates an isolated, text-returning session without Chatto tools or
// conversation instructions. Its caller supplies the entire allowed tool set.
// A zero toolLimit is unlimited; model-step limits come from WithLimits.
func NewWorker(prompt string, toolLimit int) *Session {
	return &Session{messages: []Message{textMessage("system", prompt)}, worker: true, toolLimit: toolLimit}
}

// RestoreWorker resumes read-only history with current host instructions and
// permissions. Pending terminal actions are not valid in worker snapshots.
func RestoreWorker(state Snapshot, prompt string, toolLimit int) *Session {
	s := NewWorker(prompt, toolLimit)
	if len(state.Messages) > 1 {
		s.messages = append(s.messages, state.Messages[1:]...)
	}
	return s
}

// Identity describes the authenticated Chatto account, fetched by the host.
// Profile text is data only; never use it as additional model instructions.
type Identity struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
}

// SetIdentity replaces the host identity instruction, including in restored
// sessions. Repeating the same identity keeps the prompt prefix stable.
func (s *Session) SetIdentity(identity Identity) {
	s.SetInstructions(identity, "")
}

// SetInstructions installs current host rules, operator preferences, and identity
// without changing conversation history. Owner IDs replace historical authority;
// omission clears ownership. Use it before new or resumed turns.
func (s *Session) SetInstructions(identity Identity, operatorPrompt string, ownerUserIDs ...string) {
	if len(s.messages) == 0 {
		return
	}
	s.messages[0] = systemMessage(identity, operatorPrompt, ownerUserIDs...)
}

// Snapshot is the versioned store's private conversation payload. It includes
// opaque provider fields and must only be written to owner-readable storage.
type Snapshot struct {
	Messages    []Message `json:"messages"`
	PendingTool string    `json:"pendingTool,omitempty"`
}

// Snapshot returns state for synchronous encoding by the session's owner.
func (s *Session) Snapshot() Snapshot { return Snapshot{s.messages, s.pendingTool} }

// Restore resumes a stored transcript without executing any stored actions.
func Restore(state Snapshot) *Session {
	messages := append([]Message(nil), state.Messages...)
	// Older versions used an ambiguous receipt for reactions, even when their
	// assistant message also contained text that was never sent. Clarify it on load.
	reactions := map[string]bool{}
	for n, m := range messages {
		var role, content, id string
		_ = json.Unmarshal(m["role"], &role)
		if role == "assistant" {
			var calls []struct {
				ID       string `json:"id"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			_ = json.Unmarshal(m["tool_calls"], &calls)
			for _, call := range calls {
				reactions[call.ID] = call.Function.Name == "react_to_ping"
			}
		}
		if role == "tool" {
			_ = json.Unmarshal(m["tool_call_id"], &id)
			_ = json.Unmarshal(m["content"], &content)
			if reactions[id] && content == "Output delivered to Chatto." {
				copy := Message{}
				for k, v := range m {
					copy[k] = v
				}
				copy["content"], _ = json.Marshal("Reaction delivered to Chatto. No accompanying text was posted.")
				messages[n] = copy
			}
			delete(reactions, id)
		}
	}
	return &Session{messages: messages, pendingTool: state.PendingTool}
}

// Refresh appends current authorized context after a gap in live delivery.
func (s *Session) Refresh(context string) {
	s.messages = append(s.messages, textMessage("user", "Current thread context after reconnect; this supersedes earlier thread content:\n"+context))
}

// NewSession seeds a stable prefix from one authorized thread snapshot. Callers
// deduplicate the snapshot against realtime input before each appended batch.
func NewSession(thread string) *Session {
	return &Session{messages: []Message{
		systemMessage(Identity{}, ""),
		textMessage("user", "Initial thread context:\n"+thread),
	}}
}

// Size is the encoded transcript size used for a conservative memory budget.
func (s *Session) Size() int { data, _ := json.Marshal(s.messages); return len(data) }

// Turn bootstraps history once, then appends input and complete assistant/tool
// messages. Failed model turns retain the input with a fixed failure notice,
// without incomplete tool exchanges. Tool work is bounded separately from model
// attempts; one final text-only completion reports partial progress at the limit.
func (s *Session) Turn(ctx context.Context, complete Complete, read ReadThread, input string, observers ...func(ToolEvent)) (outcome Outcome, turnErr error) {
	limits, _ := ctx.Value(limitsKey{}).(Limits)
	toolLimit, stepLimit := limits.MaxToolCalls, limits.MaxModelSteps
	if s.worker {
		ctx = context.WithValue(ctx, workerKey{}, true)
		toolLimit = s.toolLimit
	}
	observe := func(e ToolEvent) {
		for _, fn := range observers {
			fn(e)
		}
	}
	if err := ctx.Err(); err != nil {
		return Outcome{}, err
	}
	if s.pendingTool != "" {
		return Outcome{}, errors.New("previous delivery has not been recorded")
	}
	if len(s.messages) == 0 {
		thread, err := read(ctx)
		if err != nil {
			return Outcome{}, err
		}
		s.messages = NewSession(thread).messages
	}
	base := append(append([]Message(nil), s.messages...), textMessage("user", input))
	messages := append([]Message(nil), base...)
	defer func() {
		if turnErr != nil {
			s.messages = append(base, textMessage("user", "The previous model turn failed. No final Chatto answer was delivered. Completed tool receipts, including progress messages and memory operations, still apply; do not repeat them."))
		}
	}()
	toolsUsed := 0
	for step := 0; ; step++ {
		if err := ctx.Err(); err != nil {
			return Outcome{}, err
		}
		textOnly := (toolLimit > 0 && toolsUsed >= toolLimit) || (stepLimit > 0 && step >= stepLimit)
		completionCtx := ctx
		if textOnly {
			messages = append(messages, textMessage("user", "Host work limit reached for this turn. Do not call tools. Give a concise progress report based only on the receipts: what completed, what remains unfinished, and any uncertainty. Completed actions remain in effect. Do not claim the whole task finished unless the receipts support that."))
			completionCtx = context.WithValue(ctx, textOnlyKey{}, true)
		}
		msg, err := complete(completionCtx, messages)
		if err != nil {
			return Outcome{}, err
		}
		var role string
		if json.Unmarshal(msg["role"], &role) != nil || role != "assistant" {
			return Outcome{}, errors.New("invalid assistant role")
		}
		var calls []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		}
		if raw := msg["tool_calls"]; len(raw) > 0 && json.Unmarshal(raw, &calls) != nil {
			return Outcome{}, errors.New("invalid tool calls")
		}
		// A model may attach an answer to a terminal reaction call. Preserve the
		// answer and explicitly decline the reaction instead of silently losing text.
		if !s.worker && len(calls) == 1 && calls[0].Function.Name == "react_to_ping" {
			var body string
			if json.Unmarshal(msg["content"], &body) == nil && strings.TrimSpace(body) != "" {
				if len([]rune(body)) > 10000 {
					return Outcome{}, errors.New("model answer exceeds Chatto message limit")
				}
				if calls[0].ID == "" || calls[0].Type != "function" {
					return Outcome{}, errors.New("invalid tool call identity")
				}
				receipt := textMessage("tool", "Reaction skipped because this response includes a written answer. The host will attempt to post that text; delivery is not yet confirmed.")
				receipt["tool_call_id"], _ = json.Marshal(calls[0].ID)
				s.messages = append(messages, msg, receipt, textMessage("assistant", body))
				return Outcome{Body: body}, nil
			}
		}
		if textOnly && len(calls) > 0 {
			// Providers can ignore tool_choice. Never execute work after the cap
			// or retain an assistant tool call without a matching result.
			body := "I reached this turn's work limit. Any completed actions remain in effect, but I haven't finished the task."
			s.messages = append(messages, textMessage("assistant", body))
			return Outcome{Body: body, Limited: true}, nil
		}
		if len(calls) == 0 {
			var content string
			if json.Unmarshal(msg["content"], &content) != nil || strings.TrimSpace(content) == "" {
				return Outcome{}, errors.New("empty model answer")
			}
			if len([]rune(content)) > 10000 {
				return Outcome{}, errors.New("model answer exceeds Chatto message limit")
			}
			s.messages = append(messages, msg)
			return Outcome{Body: content, Limited: textOnly}, nil
		}
		messages = append(messages, msg)
		seen := map[string]bool{}
		batchAllowed := s.worker
		// Validate the whole batch before executing anything. Only explicitly
		// read-only assigned tools may share a worker model response.
		for _, call := range calls {
			if call.ID == "" || seen[call.ID] || call.Type != "function" {
				return Outcome{}, errors.New("invalid tool call identity")
			}
			seen[call.ID] = true
			readOnly := false
			for _, tool := range turnTools(ctx) {
				if tool.Name == call.Function.Name {
					readOnly = tool.ReadOnly
				}
			}
			batchAllowed = batchAllowed && readOnly
		}
		for _, call := range calls {
			started := time.Now()
			observe(ToolEvent{Name: call.Function.Name, Arguments: call.Function.Arguments, State: "running"})
			finish := func(state, result string) {
				observe(ToolEvent{Name: call.Function.Name, Arguments: call.Function.Arguments, State: state, Result: result, Duration: time.Since(started)})
			}
			if len(calls) > 1 && !batchAllowed {
				result := `{"status":"rejected","mutated":false,"error":"Multiple tool calls in one response are not supported. None of these calls ran. Call one tool at a time."}`
				tool := textMessage("tool", result)
				tool["tool_call_id"], _ = json.Marshal(call.ID)
				messages = append(messages, tool)
				finish("rejected", result)
				continue
			}
			result := `{"status":"rejected","retryable":false,"error":"Unknown tool or invalid arguments. Only tools exposed for this turn are available. No action ran; this is not a transient service error."}`
			var extra *Tool
			for _, tool := range turnTools(ctx) {
				if tool.Name == call.Function.Name {
					extra = &tool
					break
				}
			}
			if extra != nil && (len(calls) == 1 || batchAllowed) {
				if ctx.Err() != nil || (toolLimit > 0 && toolsUsed >= toolLimit) {
					result := `{"status":"rejected","error":"Work stopped; this call did not run."}`
					tool := textMessage("tool", result)
					tool["tool_call_id"], _ = json.Marshal(call.ID)
					messages = append(messages, tool)
					finish("rejected", result)
					continue
				}
				toolsUsed++
				result, toolErr := extra.Execute(ctx, json.RawMessage(call.Function.Arguments))
				if toolErr != nil {
					// Safe fixed feedback; subprocess errors may include private data.
					result = `{"error":"Tool operation failed or its result is uncertain. Do not repeat a write automatically; verify its state first."}`
				}
				tool := textMessage("tool", result)
				tool["tool_call_id"], _ = json.Marshal(call.ID)
				messages = append(messages, tool)
				// Preserve completed capability receipts even if a later model call
				// fails. Rolling back to the original input would hide durable writes.
				if len(calls) == 1 {
					base = append([]Message(nil), messages...)
					s.messages = messages
				}
				if toolErr != nil {
					finish("failed", result)
				} else {
					var envelope struct {
						Status string `json:"status"`
					}
					state := "complete"
					if json.Unmarshal([]byte(result), &envelope) == nil && (envelope.Status == "rejected" || envelope.Status == "failed" || envelope.Status == "partial") {
						state = envelope.Status
					}
					finish(state, result)
				}
				if call.Function.Name == "defer_response" && !s.worker && toolErr == nil {
					var receipt struct {
						Deferred bool `json:"deferred"`
					}
					if json.Unmarshal([]byte(result), &receipt) == nil && receipt.Deferred {
						return Outcome{Waiting: true}, nil
					}
				}
				continue
			}
			var args map[string]json.RawMessage
			if !s.worker && call.Function.Name == "react_to_ping" && len(calls) == 1 && json.Unmarshal([]byte(call.Function.Arguments), &args) == nil && len(args) == 1 {
				var emoji string
				if json.Unmarshal(args["emoji"], &emoji) == nil && emojiName.MatchString(emoji) {
					s.messages = messages
					s.pendingTool = call.ID
					finish("ready", "Reaction selected; delivery is a separate step")
					return Outcome{Emoji: emoji}, nil
				}
			}
			if !s.worker && call.Function.Name == "read_thread" && json.Unmarshal([]byte(call.Function.Arguments), &args) == nil && args != nil && len(args) == 0 {
				toolsUsed++
				result, err = read(ctx)
				if err != nil {
					finish("failed", "Thread read failed")
					return Outcome{}, err
				}
				finish("complete", result)
			} else {
				finish("rejected", result)
			}
			tool := textMessage("tool", result)
			tool["tool_call_id"], _ = json.Marshal(call.ID)
			messages = append(messages, tool)
		}
		// A batched assistant message must have every tool receipt before it
		// becomes resumable history, including calls skipped at the work limit.
		base = append([]Message(nil), messages...)
		s.messages = messages
	}
}

// RecordDelivery closes a terminal reaction tool call, or appends a reply receipt.
// On errors the delivery is uncertain and must not be retried automatically.
func (s *Session) RecordDelivery(err error) {
	text := "Output delivered to Chatto."
	if err != nil {
		text = "Delivery failed or is uncertain. Do not assume the output reached Chatto and do not retry it automatically."
	}
	if s.pendingTool != "" {
		if err == nil {
			text = "Reaction delivered to Chatto. No accompanying text was posted."
		} else {
			text = "Reaction delivery failed or is uncertain. No accompanying text was posted. Do not retry automatically."
		}
		msg := textMessage("tool", text)
		msg["tool_call_id"], _ = json.Marshal(s.pendingTool)
		s.messages = append(s.messages, msg)
		s.pendingTool = ""
	} else {
		s.messages = append(s.messages, textMessage("user", text))
	}
}

// LastDeliveredReply recovers only a terminal text reply with an immediately
// following successful host receipt. It never infers delivery from model claims.
func (s *Session) LastDeliveredReply() string {
	if len(s.messages) < 2 || s.pendingTool != "" {
		return ""
	}
	receipt, reply := s.messages[len(s.messages)-1], s.messages[len(s.messages)-2]
	var role, content, replyRole, body string
	_ = json.Unmarshal(receipt["role"], &role)
	_ = json.Unmarshal(receipt["content"], &content)
	_ = json.Unmarshal(reply["role"], &replyRole)
	_ = json.Unmarshal(reply["content"], &body)
	var calls []json.RawMessage
	if raw := reply["tool_calls"]; len(raw) > 0 && json.Unmarshal(raw, &calls) != nil {
		return ""
	}
	if role == "user" && content == "Output delivered to Chatto." && replyRole == "assistant" && len(calls) == 0 {
		return body
	}
	return ""
}

func textMessage(role, content string) Message {
	r, _ := json.Marshal(role)
	c, _ := json.Marshal(content)
	return Message{"role": r, "content": c}
}
