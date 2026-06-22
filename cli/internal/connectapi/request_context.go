package connectapi

import "context"

type requestBaseURLContextKey struct{}

// WithRequestBaseURL stores the incoming request's scheme and host for service
// methods that need to return absolute URLs to cross-origin clients.
func WithRequestBaseURL(ctx context.Context, baseURL string) context.Context {
	if baseURL == "" {
		return ctx
	}
	return context.WithValue(ctx, requestBaseURLContextKey{}, baseURL)
}

func requestBaseURLFromContext(ctx context.Context) string {
	baseURL, _ := ctx.Value(requestBaseURLContextKey{}).(string)
	return baseURL
}
