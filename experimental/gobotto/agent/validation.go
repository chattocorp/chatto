package agent

import (
	"encoding/json"
	"strings"
)

// ValidationError identifies a rejected model response with a fixed, safe reason.
// It never includes provider text or tool arguments.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

// validateResponse checks the complete envelope before any tool can run.
func validateResponse(msg Message) error {
	var role string
	if json.Unmarshal(msg["role"], &role) != nil || role != "assistant" {
		return ValidationError("invalid assistant role")
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
		return ValidationError("invalid tool calls")
	}
	seen := map[string]bool{}
	for _, call := range calls {
		if call.ID == "" || seen[call.ID] || call.Type != "function" {
			return ValidationError("invalid tool call identity")
		}
		seen[call.ID] = true
	}
	var content string
	if len(calls) == 0 && (json.Unmarshal(msg["content"], &content) != nil || strings.TrimSpace(content) == "") {
		return ValidationError("empty model answer")
	}
	if json.Unmarshal(msg["content"], &content) == nil && len([]rune(content)) > 10000 {
		return ValidationError("model answer exceeds Chatto message limit")
	}
	return nil
}
