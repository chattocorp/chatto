package agent

import (
	"context"
	"encoding/json"
	"sync"
)

// Tool is a host-selected capability for one turn. Execute receives untrusted
// model arguments and must enforce permissions and validate them independently.
// Results are private model context. The tool must not retry external mutations.
// A host rejection returns a JSON object with status="rejected" and an error
// message, with nil error so the model can respond to or correct the rejection.
// A non-nil error indicates execution failure or uncertain effects.
type Tool struct {
	// Catalog lists capabilities reached through this tool, such as remote MCP
	// tools. It is local discovery metadata, not part of the wire definition.
	Catalog []ToolInfo
	// ReadOnly permits sequential batching only inside isolated worker sessions.
	ReadOnly    bool
	Name        string
	Description string
	Parameters  map[string]any
	Execute     func(context.Context, json.RawMessage) (string, error)
}

// ToolInfo describes an available capability without loading its argument schema.
// Via names the discovery tool to call before using an indirect capability.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Via         string `json:"via,omitempty"`
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
	ctx = context.WithValue(ctx, discoveryKey{}, (*toolDiscovery)(nil))
	return context.WithValue(ctx, toolsKey{}, append([]Tool(nil), tools...))
}

type discoveryKey struct{}
type toolDiscovery struct {
	mu    sync.Mutex
	names map[string]bool
}

// WithToolDiscovery starts fresh schema exposure for this turn. It never grants
// execution authority: turnTools remains the host-selected capability boundary.
func WithToolDiscovery(ctx context.Context, core ...string) context.Context {
	d := &toolDiscovery{names: map[string]bool{}}
	for _, name := range core {
		d.names[name] = true
	}
	return context.WithValue(ctx, discoveryKey{}, d)
}

// ExposeTools reveals schemas only for capabilities already authorized by the host.
func ExposeTools(ctx context.Context, names ...string) {
	d, _ := ctx.Value(discoveryKey{}).(*toolDiscovery)
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, name := range names {
		for _, tool := range turnTools(ctx) {
			if tool.Name == name {
				d.names[name] = true
				break
			}
		}
	}
}

func visibleTools(ctx context.Context) []Tool {
	all := turnTools(ctx)
	d, _ := ctx.Value(discoveryKey{}).(*toolDiscovery)
	if d == nil {
		return all
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	visible := make([]Tool, 0, len(all))
	for _, tool := range all {
		if d.names[tool.Name] {
			visible = append(visible, tool)
		}
	}
	return visible
}

func turnTools(ctx context.Context) []Tool {
	tools, _ := ctx.Value(toolsKey{}).([]Tool)
	return tools
}
