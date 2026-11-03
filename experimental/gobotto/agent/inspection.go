package agent

import (
	"context"
	"encoding/json"
)

type requestObserverKey struct{}

// WithRequestObserver observes the provider payload synchronously on the owning
// completion goroutine. Credentials are never included; image data and provider
// reasoning are removed. The callback must not mutate live agent state.
func WithRequestObserver(ctx context.Context, observe func(string)) context.Context {
	return context.WithValue(ctx, requestObserverKey{}, observe)
}

func observeRequest(ctx context.Context, input map[string]any) {
	observe, _ := ctx.Value(requestObserverKey{}).(func(string))
	if observe != nil {
		observe(InspectionJSON(input))
	}
}

// InspectionJSON returns detached readable JSON for private inspection only.
// It excludes provider reasoning and image URLs/pixels from saved transcripts.
func InspectionJSON(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		return "[inspection encoding failed]"
	}
	var decoded any
	if json.Unmarshal(b, &decoded) != nil {
		return "[inspection decoding failed]"
	}
	var clean func(any)
	clean = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				if k == "reasoning_details" || k == "reasoning_content" {
					delete(x, k)
					continue
				}
				if k == "reasoning" {
					if _, text := child.(string); text {
						delete(x, k)
						continue
					}
				}
				if k == "image_url" {
					x[k] = "[image omitted from inspection]"
					continue
				}
				clean(child)
			}
		case []any:
			for _, child := range x {
				clean(child)
			}
		}
	}
	clean(decoded)
	b, _ = json.MarshalIndent(decoded, "", "  ")
	return string(b)
}
