// Package agent implements a bounded OpenRouter tool-calling conversation.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

// Message retains provider fields, including reasoning_details, unchanged between
// tool steps. Some models require those fields to resume a tool-calling turn.
type Message map[string]json.RawMessage

// Complete supplies the next assistant message. Implementations must honor cancellation.
type Complete func(context.Context, []Message) (Message, error)

// ReadThread reads only the host-selected thread. The model cannot choose a destination.
type ReadThread func(context.Context) (string, error)

// ToolEvent exposes executed tool activity for a private inspector. It excludes
// model reasoning. Arguments and Result can contain private conversation data.
type ToolEvent struct {
	Name, Arguments, State, Result string
	Duration                       time.Duration
}

// Outcome is one host-applied action in the ping's thread. Only one field is set.
type Outcome struct{ Body, Emoji string }

// Usage counts provider-reported tokens for one request. CachedInput is included
// in Input, not additional to it. Missing usage is never inferred to be zero.
type Usage struct {
	Input, CachedInput, Output int64
	// CacheKnown distinguishes a reported zero from absent cache telemetry.
	CacheKnown bool
	// Provider and RequestID identify the serving endpoint and generation for audits.
	Provider, RequestID string
	// Cost is the OpenRouter account charge; nil means it was not reported.
	Cost *float64
}

var emojiName = regexp.MustCompile(`^[A-Za-z0-9_+\-]{1,128}$`)

// OpenRouter returns a completion function using a fixed OpenRouter endpoint.
// The model is explicit because price, availability, and tool support vary.
func OpenRouter(key, model, sessionID string, report func(Usage)) Complete {
	return func(ctx context.Context, messages []Message) (Message, error) {
		var out struct {
			ID       string `json:"id"`
			Provider string `json:"provider"`
			Choices  []struct {
				Message Message `json:"message"`
				Finish  string  `json:"finish_reason"`
			} `json:"choices"`
			Usage *wireUsage `json:"usage"`
		}
		input := map[string]any{
			"model": model, "messages": messages, "max_tokens": 2048,
			"session_id": sessionID,
			"tools": []any{map[string]any{"type": "function", "function": map[string]any{
				"name": "read_thread", "description": "Read the most recent messages in this conversation. Older messages can be omitted.",
				"parameters": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
			}}, map[string]any{"type": "function", "function": map[string]any{
				"name": "react_to_ping", "description": "Finish by reacting to the latest triggering message in the batch instead of posting text. This can be an ordinary thread reply without a mention. Use an emoji name such as thumbsup, heart, eyes, or white_check_mark, without colons.",
				"parameters": map[string]any{"type": "object", "properties": map[string]any{"emoji": map[string]any{"type": "string"}}, "required": []string{"emoji"}, "additionalProperties": false},
			}}},
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
			return nil, errors.New("expected one model completion")
		}
		choice := out.Choices[0]
		if choice.Finish != "stop" && choice.Finish != "tool_calls" {
			return nil, errors.New("model did not finish normally")
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
	return &Session{messages: state.Messages, pendingTool: state.PendingTool}
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
// without incomplete tool exchanges. Each turn is bounded to eight model calls.
func (s *Session) Turn(ctx context.Context, complete Complete, read ReadThread, input string, observers ...func(ToolEvent)) (outcome Outcome, turnErr error) {
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
			s.messages = append(base, textMessage("user", "The previous model turn failed. No output was delivered by that turn."))
		}
	}()
	toolsUsed := 0
	for range 8 {
		if err := ctx.Err(); err != nil {
			return Outcome{}, err
		}
		msg, err := complete(ctx, messages)
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
		if len(calls) == 0 {
			var content string
			if json.Unmarshal(msg["content"], &content) != nil || strings.TrimSpace(content) == "" {
				return Outcome{}, errors.New("empty model answer")
			}
			if len([]rune(content)) > 10000 {
				return Outcome{}, errors.New("model answer exceeds Chatto message limit")
			}
			s.messages = append(messages, msg)
			return Outcome{Body: content}, nil
		}
		toolsUsed += len(calls)
		if toolsUsed > 8 {
			return Outcome{}, errors.New("tool call limit reached")
		}
		messages = append(messages, msg)
		seen := map[string]bool{}
		for _, call := range calls {
			if call.ID == "" || seen[call.ID] || call.Type != "function" {
				return Outcome{}, errors.New("invalid tool call identity")
			}
			seen[call.ID] = true
			started := time.Now()
			observe(ToolEvent{Name: call.Function.Name, Arguments: call.Function.Arguments, State: "running"})
			finish := func(state, result string) {
				observe(ToolEvent{Name: call.Function.Name, Arguments: call.Function.Arguments, State: state, Result: result, Duration: time.Since(started)})
			}
			result := `{"error":"Unknown tool or invalid arguments. Use read_thread with {}, or call react_to_ping alone with an emoji name."}`
			var args map[string]json.RawMessage
			if call.Function.Name == "react_to_ping" && len(calls) == 1 && json.Unmarshal([]byte(call.Function.Arguments), &args) == nil && len(args) == 1 {
				var emoji string
				if json.Unmarshal(args["emoji"], &emoji) == nil && emojiName.MatchString(emoji) {
					s.messages = messages
					s.pendingTool = call.ID
					finish("ready", "Reaction selected; delivery is a separate step")
					return Outcome{Emoji: emoji}, nil
				}
			}
			if call.Function.Name == "read_thread" && json.Unmarshal([]byte(call.Function.Arguments), &args) == nil && args != nil && len(args) == 0 {
				result, err = read(ctx)
				if err != nil {
					finish("failed", "Thread read failed")
					return Outcome{}, err
				}
				finish("complete", result)
			} else {
				finish("rejected", "Unknown tool or invalid arguments")
			}
			tool := textMessage("tool", result)
			tool["tool_call_id"], _ = json.Marshal(call.ID)
			messages = append(messages, tool)
		}
	}
	return Outcome{}, errors.New("model step limit reached")
}

// RecordDelivery closes a terminal reaction tool call, or appends a reply receipt.
// On errors the delivery is uncertain and must not be retried automatically.
func (s *Session) RecordDelivery(err error) {
	text := "Output delivered to Chatto."
	if err != nil {
		text = "Delivery failed or is uncertain. Do not assume the output reached Chatto and do not retry it automatically."
	}
	if s.pendingTool != "" {
		msg := textMessage("tool", text)
		msg["tool_call_id"], _ = json.Marshal(s.pendingTool)
		s.messages = append(s.messages, msg)
		s.pendingTool = ""
	} else {
		s.messages = append(s.messages, textMessage("user", text))
	}
}

func textMessage(role, content string) Message {
	r, _ := json.Marshal(role)
	c, _ := json.Marshal(content)
	return Message{"role": r, "content": c}
}
