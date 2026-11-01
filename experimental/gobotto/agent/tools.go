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
	// ReadOnly permits sequential batching only inside isolated worker sessions.
	ReadOnly    bool
	Name        string
	Description string
	Parameters  map[string]any
	Execute     func(context.Context, json.RawMessage) (string, error)
}

type toolsKey struct{}

// Limits controls work per turn. Zero means unlimited; configuration rejects
// negative values. Model steps include rejected calls, but exclude final wrap-up.
type Limits struct{ MaxToolCalls, MaxModelSteps int }
type limitsKey struct{}

// WithLimits replaces inherited work limits, including for an isolated child.
func WithLimits(ctx context.Context, limits Limits) context.Context {
	return context.WithValue(ctx, limitsKey{}, limits)
}

// WithTools binds capabilities to a turn, never to persisted model history.
func WithTools(ctx context.Context, tools []Tool) context.Context {
	return context.WithValue(ctx, toolsKey{}, append([]Tool(nil), tools...))
}

func turnTools(ctx context.Context) []Tool {
	tools, _ := ctx.Value(toolsKey{}).([]Tool)
	return tools
}
