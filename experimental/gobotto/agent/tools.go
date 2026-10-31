package agent

import (
	"context"
	"encoding/json"
)

// Tool is a host-selected capability for one turn. Execute receives untrusted
// model arguments and must enforce permissions and validate them independently.
// Results are private model context. The tool must not retry external mutations.
// A host rejection returns a JSON object with status="rejected" and an error
// message, with nil error so the model can respond to or correct the rejection.
// A non-nil error indicates execution failure or uncertain effects.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Execute     func(context.Context, json.RawMessage) (string, error)
}

type toolsKey struct{}

// WithTools binds capabilities to a turn, never to persisted model history.
func WithTools(ctx context.Context, tools []Tool) context.Context {
	return context.WithValue(ctx, toolsKey{}, append([]Tool(nil), tools...))
}

func turnTools(ctx context.Context) []Tool {
	tools, _ := ctx.Value(toolsKey{}).([]Tool)
	return tools
}
