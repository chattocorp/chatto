package mcpclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
)

// schemaRevision lets restored sessions recognize changed definitions without
// putting those definitions back into every model request.
func schemaRevision(tool agent.Tool) string {
	data, _ := json.Marshal([]any{tool.Name, tool.Description, tool.Parameters})
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:8])
}

// decodeRequest rejects unknown envelope fields and trailing JSON. Remote tool
// arguments remain raw JSON so numeric precision and nested structures survive.
func decodeRequest(raw json.RawMessage, target any) bool {
	if len(raw) > 66000 {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return false
	}
	return decoder.Decode(new(any)) == io.EOF
}

func (m *Manager) discover(ctx context.Context, raw json.RawMessage) (string, error) {
	var request struct {
		Name string `json:"name"`
	}
	if ctx.Err() != nil {
		return feedback("failed", "Tool discovery cancelled"), nil
	}
	if !decodeRequest(raw, &request) {
		return feedback("rejected", "Supply only the configured tool name"), nil
	}
	for _, tool := range m.tools {
		if tool.Name != request.Name {
			continue
		}
		data, _ := json.Marshal(map[string]any{"status": "complete", "name": tool.Name, "revision": schemaRevision(tool), "description": tool.Description, "parameters": tool.Parameters, "untrusted": true})
		return string(data), nil
	}
	return feedback("rejected", "Tool is not available in the current configured allowlist"), nil
}

func (m *Manager) dispatch(ctx context.Context, raw json.RawMessage) (string, error) {
	var request struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if !decodeRequest(raw, &request) {
		return feedback("rejected", "Supply a configured tool name and its arguments object"), nil
	}
	for _, tool := range m.tools {
		if tool.Name == request.Name {
			return tool.Execute(ctx, request.Arguments)
		}
	}
	return feedback("rejected", "Tool is not available in the current configured allowlist"), nil
}
