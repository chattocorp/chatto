package connectapi

import "context"

type requestBaseURLContextKey struct{}
type browserSessionCreatorContextKey struct{}

type BrowserSession struct {
	Revoke func(ctx context.Context) error
}

type BrowserSessionCreator func(ctx context.Context, userID, source string) (BrowserSession, error)

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

// WithBrowserSessionCreator stores an HTTP-edge callback that can establish a
// cookie-backed browser session from transport-agnostic Connect handlers.
func WithBrowserSessionCreator(ctx context.Context, creator BrowserSessionCreator) context.Context {
	if creator == nil {
		return ctx
	}
	return context.WithValue(ctx, browserSessionCreatorContextKey{}, creator)
}

func createBrowserSessionFromContext(ctx context.Context, userID, source string) (BrowserSession, error) {
	creator, _ := ctx.Value(browserSessionCreatorContextKey{}).(BrowserSessionCreator)
	if creator == nil {
		return BrowserSession{}, nil
	}
	return creator(ctx, userID, source)
}
