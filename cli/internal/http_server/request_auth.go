package http_server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/core"
)

type authenticationValidationErrorKey struct{}

var errAuthenticationServiceUnavailable = errors.New("authentication service temporarily unavailable")

func authenticationValidationError(ctx context.Context) error {
	err, _ := ctx.Value(authenticationValidationErrorKey{}).(error)
	return err
}

func writeAuthenticationUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Authentication service temporarily unavailable"})
}

// injectUserIntoContext extracts the authenticated user from either a bearer token
// or the runtime credential handle in the Gin session cookie, and returns an updated http.Request with the user
// injected into its context.
// Returns the original request if no user is authenticated (allowing unauthenticated requests).
func (s *HTTPServer) injectUserIntoContext(c *gin.Context) *http.Request {
	credential, ok, err := s.presentedCredentialFromRequest(c)
	if err != nil {
		ctx := context.WithValue(c.Request.Context(), authenticationValidationErrorKey{}, err)
		return c.Request.WithContext(ctx)
	}
	if !ok {
		return c.Request
	}

	ctx := authctx.WithUser(c.Request.Context(), credential.user)
	ctx = authctx.WithCredential(ctx, credential.auth)

	return c.Request.WithContext(ctx)
}

func (s *HTTPServer) presentedCredentialFromRequest(c *gin.Context) (presentedRuntimeCredential, bool, error) {
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		if token, ok := strings.CutPrefix(authHeader, "Bearer "); ok && strings.TrimSpace(token) != "" {
			return s.bearerPresentedCredential(c.Request.Context(), strings.TrimSpace(token))
		}
		// An explicit Authorization header is authoritative. Never execute the
		// request with a different ambient cookie identity when it is malformed,
		// unsupported, expired, or revoked.
		return presentedRuntimeCredential{}, false, nil
	}

	if !s.requestIsSameOrigin(c.Request) {
		return presentedRuntimeCredential{}, false, nil
	}
	return s.cookiePresentedCredential(c)
}

// requestIsSameOrigin treats requests without an Origin header as same-origin
// or non-browser traffic. Browser requests with an Origin may use ambient
// cookie authentication only when it exactly matches the request target or
// Chatto's configured public origin. The latter covers TLS-terminating proxies;
// the former keeps direct hostname aliases usable without trusting forwarded
// headers.
func (s *HTTPServer) requestIsSameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	presented, ok := parseBrowserOrigin(origin)
	if !ok {
		return false
	}
	presentedOrigin := canonicalOrigin(presented)
	if presentedOrigin == directRequestOrigin(r) {
		return true
	}
	for _, allowed := range s.config.Webserver.AllowedOrigins {
		if allowed == "*" {
			continue
		}
		allowedOrigin, ok := parseBrowserOrigin(allowed)
		if ok && presentedOrigin == canonicalOrigin(allowedOrigin) {
			return true
		}
	}
	base, err := url.Parse(s.requestBaseURL(r))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return false
	}
	return presentedOrigin == canonicalOrigin(base)
}

func directRequestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	direct, err := url.Parse(scheme + "://" + r.Host)
	if err != nil || direct.Host == "" || direct.User != nil {
		return ""
	}
	return canonicalOrigin(direct)
}

func parseBrowserOrigin(raw string) (*url.URL, bool) {
	origin, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || origin.Scheme == "" || origin.Host == "" || origin.User != nil || origin.Opaque != "" || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return nil, false
	}
	return origin, true
}

func (s *HTTPServer) bearerPresentedCredential(ctx context.Context, token string) (presentedRuntimeCredential, bool, error) {
	if strings.HasPrefix(token, "cht_BK_") {
		user, verifier, err := s.core.ValidateBotAPIKeyCredential(ctx, token)
		if err != nil {
			if errors.Is(err, core.ErrAuthTokenNotFound) {
				return presentedRuntimeCredential{}, false, nil
			}
			return presentedRuntimeCredential{}, false, err
		}
		return presentedRuntimeCredential{
			user: user,
			// Keep the raw key out of the request context after verification. Bot
			// keys do not support freshness or per-session lifecycle operations.
			// Keep only the bot ID and non-secret verifier generation so long-lived
			// transports can observe a later durable revocation.
			auth: authctx.RuntimeCredential{
				Kind: authctx.RuntimeCredentialKindBotAPIKey, UserID: user.GetId(), Handle: user.GetId(),
				BotAPIKeyVerifier: append([]byte(nil), verifier...),
			},
		}, true, nil
	}
	credential, err := s.core.ValidatePresentedRuntimeCredential(ctx, token, core.AuthTokenPresentationBearer)
	if err != nil {
		if errors.Is(err, core.ErrAuthTokenNotFound) {
			return presentedRuntimeCredential{}, false, nil
		}
		return presentedRuntimeCredential{}, false, err
	}
	user, err := s.core.GetUser(ctx, credential.UserID)
	if err != nil {
		log.Warn("Bearer runtime credential valid but user not found", "userId", credential.UserID, "error", err)
		return presentedRuntimeCredential{}, false, nil
	}
	return presentedRuntimeCredential{
		user: user,
		auth: authctx.RuntimeCredential{
			Kind:          authctx.RuntimeCredentialKindBearerToken,
			UserID:        credential.UserID,
			Handle:        token,
			OAuthClientID: oauthClientIDForRuntimeCredential(credential),
			ExpiresAt:     credential.ExpiresAt,
		},
	}, true, nil
}

func oauthClientIDForRuntimeCredential(credential core.ValidatedRuntimeCredential) string {
	if credential.Kind != core.AuthTokenKindOAuthAccessToken {
		return ""
	}
	return credential.ClientID
}
