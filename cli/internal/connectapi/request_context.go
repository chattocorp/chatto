package connectapi

import "context"

const (
	// BrowserAuthenticationModeHeader lets the bundled same-origin browser ask
	// an authentication flow to establish only its HttpOnly cookie session.
	// Programmatic clients omit it and keep the bearer response contract.
	BrowserAuthenticationModeHeader = "X-Chatto-Authentication-Mode"
	BrowserAuthenticationModeCookie = "cookie"
)

type requestBaseURLContextKey struct{}
type browserSessionCreatorContextKey struct{}
type registrationAdmissionCompleterContextKey struct{}

// WithRegistrationAdmissionCompleter lets the HTTP edge clear remembered
// admission after account creation. The callback never grants admission.
func WithRegistrationAdmissionCompleter(ctx context.Context, complete func(string)) context.Context {
	return context.WithValue(ctx, registrationAdmissionCompleterContextKey{}, complete)
}

func completeRegistrationAdmission(ctx context.Context, invitationID string) {
	if complete, ok := ctx.Value(registrationAdmissionCompleterContextKey{}).(func(string)); ok && invitationID != "" {
		complete(invitationID)
	}
}

type BrowserSession struct {
	Revoke func(ctx context.Context) error
}

type BrowserSessionCreator func(ctx context.Context, userID, source string) (BrowserSession, error)

// WithRequestBaseURL stores the public origin of the incoming request for
// service methods that return absolute URLs. The HTTP edge must pass a trusted
// origin: a configured server origin, or the direct request origin when no
// public URL is configured. Service methods use it without further checks.
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
