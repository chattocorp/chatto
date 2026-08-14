package oidcprovider

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/rs/cors"
	liboidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/issuer"
)

// Service owns Authling's OIDC protocol handler and user-consent operations.
type Service struct {
	issuer  *issuer.Service
	storage *Storage
	cfg     config.Config

	mu       sync.RWMutex
	provider *op.Provider
	handler  http.Handler
}

// New constructs the provider boundary. Initialize must run after issuer initialization.
func New(cfg config.Config, issuerService *issuer.Service, storage *Storage) *Service {
	return &Service{cfg: cfg, issuer: issuerService, storage: storage}
}

// Initialize constructs the protocol engine using the durable issuer identity.
func (s *Service) Initialize() error {
	state, ok := s.issuer.State()
	if !ok {
		return fmt.Errorf("OIDC issuer is not initialized")
	}
	key, ok := s.issuer.SigningKey()
	if !ok {
		return fmt.Errorf("OIDC signing key is not initialized")
	}
	var cryptoKey [32]byte
	digest := sha256.New()
	_, _ = digest.Write([]byte("authling:oidc-provider-crypto:v1\x00"))
	_, _ = digest.Write(key.Private.D.Bytes())
	copy(cryptoKey[:], digest.Sum(nil))
	options := []op.Option{
		op.WithCustomEndpoints(
			op.NewEndpoint("oauth/authorize"), op.NewEndpoint("oauth/token"),
			op.NewEndpoint("oauth/userinfo"), op.NewEndpoint("oauth/revoke"),
			op.NewEndpoint("oauth/end-session"), op.NewEndpoint("oauth/jwks"),
		),
		op.WithCORSOptions(&cors.Options{}),
	}
	parsed, _ := url.Parse(state.Issuer)
	if parsed != nil && parsed.Scheme == "http" {
		options = append(options, op.WithAllowInsecure())
	}
	provider, err := op.NewProvider(&op.Config{
		CryptoKey: cryptoKey, CryptoKeyId: key.ID, CodeMethodS256: true,
		SupportedClaims: []string{"sub"}, SupportedScopes: []string{liboidc.ScopeOpenID},
	}, s.storage, op.StaticIssuer(state.Issuer), options...)
	if err != nil {
		return fmt.Errorf("construct OIDC provider: %w", err)
	}
	s.mu.Lock()
	s.provider = provider
	s.handler = s.wrap(provider)
	s.mu.Unlock()
	return nil
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	handler := s.handler
	s.mu.RUnlock()
	if handler == nil {
		http.Error(w, "OIDC provider unavailable", http.StatusServiceUnavailable)
		return
	}
	handler.ServeHTTP(w, r)
}

// Consent returns the client metadata for a pending authorization request.
func (s *Service) Consent(ctx context.Context, id string) (ConsentRequest, error) {
	return s.storage.Consent(ctx, id)
}

// Authorize approves a pending request for the authenticated account and returns the provider callback.
func (s *Service) Authorize(ctx context.Context, id, accountID string) (string, error) {
	if err := s.storage.Authorize(ctx, id, accountID); err != nil {
		return "", err
	}
	s.mu.RLock()
	provider := s.provider
	s.mu.RUnlock()
	if provider == nil {
		return "", fmt.Errorf("OIDC provider unavailable")
	}
	return op.AuthCallbackURL(provider)(ctx, id), nil
}

// Deny rejects and consumes a pending request.
func (s *Service) Deny(ctx context.Context, id string) (string, error) {
	return s.storage.Deny(ctx, id)
}

func (s *Service) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			s.serveDiscovery(w, r)
			return
		}
		switch r.URL.Path {
		case "/healthz", "/ready", "/oauth/introspect", "/oauth/revoke", "/oauth/end-session", "/device_authorization":
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/oauth/authorize" {
			if err := validateAuthorizeRequest(r); err != nil {
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, "invalid authorization request", http.StatusBadRequest)
				return
			}
		}
		if r.URL.Path == "/oauth/token" {
			if err := validateTokenRequest(w, r); err != nil {
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, "invalid token request", http.StatusBadRequest)
				return
			}
		}
		if browserEndpoint(r.URL.Path) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		if r.URL.Path == "/oauth/token" || r.URL.Path == "/oauth/userinfo" {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Service) serveDiscovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state, ok := s.issuer.State()
	if !ok {
		http.Error(w, "OIDC provider unavailable", http.StatusServiceUnavailable)
		return
	}
	issuer := strings.TrimSuffix(state.Issuer, "/")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth/authorize",
		"token_endpoint":                        issuer + "/oauth/token",
		"userinfo_endpoint":                     issuer + "/oauth/userinfo",
		"jwks_uri":                              issuer + "/oauth/jwks",
		"scopes_supported":                      []string{"openid"},
		"response_types_supported":              []string{"code"},
		"response_modes_supported":              []string{"query"},
		"grant_types_supported":                 []string{"authorization_code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_basic"},
		"claims_supported":                      []string{"sub"},
		"code_challenge_methods_supported":      []string{"S256"},
		"request_parameter_supported":           false,
		"client_id_metadata_document_supported": true,
	})
}

func validateAuthorizeRequest(r *http.Request) error {
	if r.Method != http.MethodGet {
		return fmt.Errorf("authorization requires GET")
	}
	if len(r.URL.RawQuery) > 8<<10 {
		return fmt.Errorf("authorization query is too large")
	}
	query := r.URL.Query()
	for _, name := range []string{"client_id", "redirect_uri", "response_type", "scope", "code_challenge", "code_challenge_method", "state", "nonce"} {
		if len(query[name]) > 1 {
			return fmt.Errorf("duplicate parameter")
		}
	}
	if query.Get("client_id") == "" || query.Get("redirect_uri") == "" || query.Get("response_type") != string(liboidc.ResponseTypeCode) {
		return fmt.Errorf("invalid request shape")
	}
	if len(query.Get("client_id")) > 2048 || len(query.Get("redirect_uri")) > 2048 || len(query.Get("state")) > 1024 || len(query.Get("nonce")) > 1024 {
		return fmt.Errorf("authorization parameter is too large")
	}
	if !validAuthorizeScopes(query.Get("scope")) {
		return fmt.Errorf("unsupported scope")
	}
	challenge := query.Get("code_challenge")
	if !validPKCEValue(challenge) || query.Get("code_challenge_method") != string(liboidc.CodeChallengeMethodS256) {
		return fmt.Errorf("S256 PKCE required")
	}
	prompts := strings.Fields(query.Get("prompt"))
	if len(prompts) > 0 && !(len(prompts) == 1 && prompts[0] == liboidc.PromptConsent) {
		return fmt.Errorf("unsupported prompt")
	}
	if query.Get("request") != "" || query.Has("max_age") || (query.Get("response_mode") != "" && query.Get("response_mode") != string(liboidc.ResponseModeQuery)) {
		return fmt.Errorf("unsupported request mode")
	}
	return nil
}

func validAuthorizeScopes(raw string) bool {
	scopes := strings.Fields(raw)
	if len(scopes) != 1 {
		return false
	}
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		if scope != liboidc.ScopeOpenID {
			return false
		}
		if _, duplicate := seen[scope]; duplicate {
			return false
		}
		seen[scope] = struct{}{}
	}
	_, hasOpenID := seen[liboidc.ScopeOpenID]
	return hasOpenID
}

func validateTokenRequest(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return fmt.Errorf("token exchange requires POST")
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/x-www-form-urlencoded") {
		return fmt.Errorf("token exchange requires form encoding")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		return err
	}
	for _, name := range []string{"grant_type", "client_id", "client_secret", "redirect_uri", "code", "code_verifier"} {
		if len(r.PostForm[name]) > 1 {
			return fmt.Errorf("duplicate token parameter")
		}
	}
	if r.PostForm.Get("grant_type") != string(liboidc.GrantTypeCode) || r.PostForm.Get("code") == "" || !validPKCEValue(r.PostForm.Get("code_verifier")) {
		return fmt.Errorf("unsupported token request")
	}
	if len(r.PostForm.Get("client_id")) > 2048 || len(r.PostForm.Get("redirect_uri")) > 2048 || len(r.PostForm.Get("code")) > 1024 || len(r.PostForm.Get("client_secret")) > 4096 {
		return fmt.Errorf("token parameter is too large")
	}
	return nil
}

func validPKCEValue(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("-._~", char) {
			continue
		}
		return false
	}
	return true
}

func browserEndpoint(path string) bool {
	return path == "/.well-known/openid-configuration" || path == "/oauth/jwks" || path == "/oauth/token" || path == "/oauth/userinfo"
}

// EqualSecret exists to keep secret comparisons at this boundary constant-time in tests and integrations.
func EqualSecret(left, right []byte) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare(left, right) == 1
}
