package oidcprovider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/nats-io/nats.go/jetstream"
	liboidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
	"hmans.de/authling/internal/ids"
	"hmans.de/authling/internal/issuer"
	"hmans.de/authling/internal/runtimejson"
	"hmans.de/authling/internal/storage"
)

const (
	authRequestLifetime = 10 * time.Minute
	accessTokenLifetime = 5 * time.Minute
	maxAuthRequests     = 1000
)

// ErrLoginRequired means the browser must authenticate again before approval.
var ErrLoginRequired = errors.New("fresh authentication required")

var errOIDCStateNotFound = errors.New("OIDC state not found")

type authRequestState struct {
	ID            string                      `json:"id"`
	CreatedAt     time.Time                   `json:"created_at"`
	ExpiresAt     time.Time                   `json:"expires_at"`
	ClientID      string                      `json:"client_id"`
	ClientName    string                      `json:"client_name"`
	ClientHost    string                      `json:"client_host"`
	RedirectURI   string                      `json:"redirect_uri"`
	State         string                      `json:"state,omitempty"`
	Nonce         string                      `json:"nonce,omitempty"`
	Scopes        []string                    `json:"scopes"`
	ResponseType  liboidc.ResponseType        `json:"response_type"`
	ResponseMode  liboidc.ResponseMode        `json:"response_mode,omitempty"`
	CodeChallenge string                      `json:"code_challenge"`
	CodeMethod    liboidc.CodeChallengeMethod `json:"code_challenge_method"`
	MaxAge        *uint                       `json:"max_age,omitempty"`
	ForceLogin    bool                        `json:"force_login,omitempty"`
	ForceConsent  bool                        `json:"force_consent,omitempty"`
	// Silent prohibits login, consent, and other interactive pages.
	Silent     bool      `json:"silent,omitempty"`
	Subject    string    `json:"subject,omitempty"`
	Authorized bool      `json:"authorized"`
	AuthTime   time.Time `json:"auth_time,omitempty"`
	CodeKey    string    `json:"code_key,omitempty"`
}

func (r *authRequestState) GetID() string { return r.ID }
func (*authRequestState) GetACR() string  { return "" }
func (r *authRequestState) GetAMR() []string {
	if r.Authorized {
		return []string{"pwd"}
	}
	return nil
}
func (r *authRequestState) GetAudience() []string  { return []string{r.ClientID} }
func (r *authRequestState) GetAuthTime() time.Time { return r.AuthTime }
func (r *authRequestState) GetClientID() string    { return r.ClientID }
func (r *authRequestState) GetCodeChallenge() *liboidc.CodeChallenge {
	return &liboidc.CodeChallenge{Challenge: r.CodeChallenge, Method: r.CodeMethod}
}
func (r *authRequestState) GetNonce() string                      { return r.Nonce }
func (r *authRequestState) GetRedirectURI() string                { return r.RedirectURI }
func (r *authRequestState) GetResponseType() liboidc.ResponseType { return r.ResponseType }
func (r *authRequestState) GetResponseMode() liboidc.ResponseMode { return r.ResponseMode }
func (r *authRequestState) GetScopes() []string                   { return append([]string(nil), r.Scopes...) }
func (r *authRequestState) GetState() string                      { return r.State }
func (r *authRequestState) GetSubject() string                    { return r.Subject }
func (r *authRequestState) Done() bool                            { return r.Authorized }

type codeState struct {
	RequestID string `json:"request_id"`
	Claimed   bool   `json:"claimed"`
}

type tokenState struct {
	ClientID string    `json:"client_id"`
	Subject  string    `json:"subject"`
	Scopes   []string  `json:"scopes"`
	Expires  time.Time `json:"expires"`
}

// ConsentRequest contains the non-sensitive metadata shown to an authenticated user.
type ConsentRequest struct {
	ID, ClientID, ClientName, ClientHost, RedirectOrigin string
	Scopes                                               []string
	ForceConsent                                         bool
	// Silent requires a code or protocol error without interactive pages.
	Silent bool
}

// Storage persists OIDC protocol state in Authling's encrypted runtime bucket.
type Storage struct {
	kv      jetstream.KeyValue
	js      jetstream.JetStream
	key     []byte
	clients *Resolver
	issuer  *issuer.Service
	now     func() time.Time
	profile func(context.Context, string) (string, string, error)
}

func NewStorage(kv jetstream.KeyValue, js jetstream.JetStream, key []byte, clients *Resolver, issuerService *issuer.Service, profile func(context.Context, string) (string, string, error)) *Storage {
	return &Storage{kv: kv, js: js, key: append([]byte(nil), key...), clients: clients, issuer: issuerService, now: time.Now, profile: profile}
}

// admitAuthRequest runs at HTTP admission before client lookup or state creation.
func (s *Storage) admitAuthRequest(ctx context.Context) error {
	return storage.AdmitRequest(ctx, s.kv, s.js, "oidc.admission.global", maxAuthRequests, authRequestLifetime)
}

func (s *Storage) CreateAuthRequest(ctx context.Context, request *liboidc.AuthRequest, _ string) (op.AuthRequest, error) {
	client, err := s.clients.Resolve(ctx, request.ClientID)
	if err != nil {
		return nil, err
	}
	id, err := ids.New("ar")
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	state := &authRequestState{
		ID: id, CreatedAt: now, ExpiresAt: now.Add(authRequestLifetime), ClientID: request.ClientID,
		ClientName: client.NameValue, ClientHost: client.DisplayHost, RedirectURI: request.RedirectURI,
		State: request.State, Nonce: request.Nonce, Scopes: append([]string(nil), request.Scopes...),
		ResponseType: request.ResponseType, ResponseMode: request.ResponseMode,
		CodeChallenge: request.CodeChallenge, CodeMethod: request.CodeChallengeMethod,
		ForceConsent: slices.Contains(request.Prompt, liboidc.PromptConsent),
		ForceLogin:   slices.Contains(request.Prompt, liboidc.PromptLogin),
		Silent:       slices.Contains(request.Prompt, liboidc.PromptNone),
		MaxAge:       request.MaxAge,
	}
	if err := s.create(ctx, s.requestKey(id), state, authRequestLifetime); err != nil {
		return nil, err
	}
	return state, nil
}

func (s *Storage) AuthRequestByID(ctx context.Context, id string) (op.AuthRequest, error) {
	_, state, err := s.readRequest(ctx, id)
	return state, err
}

func (s *Storage) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	key := s.codeKey(code)
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		return nil, errOIDCStateNotFound
	}
	var state codeState
	if err := s.open(key, entry.Value(), &state); err != nil || state.RequestID == "" || state.Claimed {
		return nil, errOIDCStateNotFound
	}
	request, err := s.AuthRequestByID(ctx, state.RequestID)
	if err != nil || request.(*authRequestState).CodeKey != key {
		return nil, errOIDCStateNotFound
	}
	return request, nil
}

func (s *Storage) SaveAuthCode(ctx context.Context, id, code string) error {
	entry, request, err := s.readRequest(ctx, id)
	if err != nil || !request.Authorized {
		return errOIDCStateNotFound
	}
	key := s.codeKey(code)
	remaining := request.ExpiresAt.Sub(s.now().UTC())
	if remaining <= 0 {
		return errOIDCStateNotFound
	}
	if err := s.create(ctx, key, codeState{RequestID: id}, remaining); err != nil {
		return err
	}
	request.CodeKey = key
	data, err := s.seal(s.requestKey(id), request)
	if err != nil {
		return err
	}
	_, err = storage.UpdateKeyWithTTL(ctx, s.js, storage.RuntimeStateBucket, s.requestKey(id), data, entry.Revision(), remaining)
	return err
}

func (s *Storage) DeleteAuthRequest(ctx context.Context, id string) error {
	entry, request, err := s.readRequest(ctx, id)
	if err != nil {
		return nil
	}
	if request.CodeKey != "" {
		_ = s.kv.Delete(ctx, request.CodeKey)
	}
	if err := s.kv.Delete(ctx, s.requestKey(id), jetstream.LastRevision(entry.Revision())); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return err
	}
	return nil
}

// Consent returns an authorization request only after its client metadata and expiry have been validated.
func (s *Storage) Consent(ctx context.Context, id string) (ConsentRequest, error) {
	_, state, err := s.readRequest(ctx, id)
	if err != nil || state.Authorized {
		return ConsentRequest{}, errOIDCStateNotFound
	}
	redirect, err := url.Parse(state.RedirectURI)
	if err != nil || redirect.Scheme == "" || redirect.Host == "" {
		return ConsentRequest{}, errOIDCStateNotFound
	}
	redirectOrigin, err := canonicalOrigin(state.RedirectURI)
	if err != nil {
		return ConsentRequest{}, errOIDCStateNotFound
	}
	return ConsentRequest{
		ID: state.ID, ClientID: state.ClientID, ClientName: state.ClientName, ClientHost: state.ClientHost,
		RedirectOrigin: redirectOrigin,
		Scopes:         append([]string(nil), state.Scopes...),
		ForceConsent:   state.ForceConsent,
		Silent:         state.Silent,
	}, nil
}

// CheckAuthentication checks persisted request constraints against server-owned
// authentication evidence. It does not consume the request or change consent.
func (s *Storage) CheckAuthentication(ctx context.Context, id string, authenticatedAt time.Time) error {
	_, state, err := s.readRequest(ctx, id)
	if err != nil || state.Authorized {
		return errOIDCStateNotFound
	}
	return state.checkAuthentication(authenticatedAt, s.now().UTC())
}

// checkAuthentication compares full precision times. In particular, a session
// from earlier in the same second cannot satisfy forced authentication.
func (r *authRequestState) checkAuthentication(authenticatedAt, now time.Time) error {
	if authenticatedAt.IsZero() || authenticatedAt.After(now) {
		return ErrLoginRequired
	}
	if r.ForceLogin || r.MaxAge != nil && *r.MaxAge == 0 {
		if !authenticatedAt.After(r.CreatedAt) {
			return ErrLoginRequired
		}
	} else if r.MaxAge != nil && now.Sub(authenticatedAt).Seconds() > float64(*r.MaxAge) {
		return ErrLoginRequired
	}
	return nil
}

// Authorize binds the current account to a pending request using OCC.
func (s *Storage) Authorize(ctx context.Context, id, accountID string, authenticatedAt time.Time) error {
	entry, state, err := s.readRequest(ctx, id)
	if err != nil || state.Authorized || accountID == "" {
		return errOIDCStateNotFound
	}
	if err := state.checkAuthentication(authenticatedAt, s.now().UTC()); err != nil {
		return err
	}
	state.Subject, state.Authorized, state.AuthTime = accountID, true, authenticatedAt.UTC()
	remaining := state.ExpiresAt.Sub(s.now().UTC())
	data, err := s.seal(s.requestKey(id), state)
	if err != nil {
		return err
	}
	_, err = storage.UpdateKeyWithTTL(ctx, s.js, storage.RuntimeStateBucket, s.requestKey(id), data, entry.Revision(), remaining)
	return err
}

// Deny consumes a pending request and returns its already-validated client redirect.
func (s *Storage) Deny(ctx context.Context, id string) (string, error) {
	return s.reject(ctx, id, "access_denied")
}

// reject consumes a pending request and returns an error to its validated URI.
func (s *Storage) reject(ctx context.Context, id, code string) (string, error) {
	_, state, err := s.readRequest(ctx, id)
	if err != nil || state.Authorized {
		return "", errOIDCStateNotFound
	}
	redirect, err := url.Parse(state.RedirectURI)
	if err != nil {
		return "", errOIDCStateNotFound
	}
	query := redirect.Query()
	query.Set("error", code)
	if state.State != "" {
		query.Set("state", state.State)
	}
	redirect.RawQuery = query.Encode()
	if err := s.DeleteAuthRequest(ctx, id); err != nil {
		return "", err
	}
	return redirect.String(), nil
}

func (s *Storage) CreateAccessToken(ctx context.Context, request op.TokenRequest) (string, time.Time, error) {
	if authRequest, ok := request.(*authRequestState); ok {
		if err := s.claimCode(ctx, authRequest); err != nil {
			return "", time.Time{}, liboidc.ErrInvalidGrant().WithDescription("invalid authorization code").WithParent(err)
		}
	}
	id, err := ids.New("at")
	if err != nil {
		return "", time.Time{}, err
	}
	expires := s.now().UTC().Add(accessTokenLifetime)
	clientID := ""
	if auth, ok := request.(op.AuthRequest); ok {
		clientID = auth.GetClientID()
	}
	state := tokenState{ClientID: clientID, Subject: request.GetSubject(), Scopes: request.GetScopes(), Expires: expires}
	if err := s.create(ctx, s.tokenKey(id), state, accessTokenLifetime); err != nil {
		return "", time.Time{}, err
	}
	return id, expires, nil
}

func canonicalOrigin(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid origin")
	}
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	port := parsed.Port()
	if port == "" || scheme == "https" && port == "443" || scheme == "http" && port == "80" {
		if strings.Contains(hostname, ":") {
			hostname = "[" + hostname + "]"
		}
		return scheme + "://" + hostname, nil
	}
	return scheme + "://" + net.JoinHostPort(hostname, port), nil
}

func (s *Storage) claimCode(ctx context.Context, request *authRequestState) error {
	if request.CodeKey == "" {
		return errOIDCStateNotFound
	}
	entry, err := s.kv.Get(ctx, request.CodeKey)
	if err != nil {
		return errOIDCStateNotFound
	}
	var state codeState
	if err := s.open(request.CodeKey, entry.Value(), &state); err != nil || state.RequestID != request.ID || state.Claimed {
		return errOIDCStateNotFound
	}
	remaining := request.ExpiresAt.Sub(s.now().UTC())
	if remaining <= 0 {
		return errOIDCStateNotFound
	}
	state.Claimed = true
	data, err := s.seal(request.CodeKey, state)
	if err != nil {
		return err
	}
	if _, err := storage.UpdateKeyWithTTL(ctx, s.js, storage.RuntimeStateBucket, request.CodeKey, data, entry.Revision(), remaining); err != nil {
		return errOIDCStateNotFound
	}
	return nil
}

func (*Storage) CreateAccessAndRefreshTokens(context.Context, op.TokenRequest, string) (string, string, time.Time, error) {
	return "", "", time.Time{}, liboidc.ErrUnsupportedGrantType()
}
func (*Storage) TokenRequestByRefreshToken(context.Context, string) (op.RefreshTokenRequest, error) {
	return nil, op.ErrInvalidRefreshToken
}
func (*Storage) TerminateSession(context.Context, string, string) error { return nil }
func (*Storage) RevokeToken(context.Context, string, string, string) *liboidc.Error {
	return liboidc.ErrUnsupportedGrantType()
}
func (*Storage) GetRefreshTokenInfo(context.Context, string, string) (string, string, error) {
	return "", "", op.ErrInvalidRefreshToken
}

func (s *Storage) GetClientByClientID(ctx context.Context, id string) (op.Client, error) {
	return s.clients.Resolve(ctx, id)
}
func (s *Storage) AuthorizeClientIDSecret(ctx context.Context, id, secret string) error {
	return s.clients.AuthorizeSecret(ctx, id, secret)
}
func (s *Storage) SetUserinfoFromScopes(ctx context.Context, info *liboidc.UserInfo, subject, _ string, _ []string) error {
	info.Subject = subject
	if s.profile == nil {
		return nil
	}
	username, name, err := s.profile(ctx, subject)
	if err != nil {
		return err
	}
	info.PreferredUsername = username
	info.Name = name
	return nil
}
func (s *Storage) SetUserinfoFromToken(ctx context.Context, info *liboidc.UserInfo, tokenID, subject, _ string) error {
	var state tokenState
	if err := s.read(s.tokenKey(tokenID), ctx, &state); err != nil || state.Subject != subject || !state.Expires.After(s.now().UTC()) {
		return errOIDCStateNotFound
	}
	info.Subject = subject
	if s.profile != nil {
		username, name, err := s.profile(ctx, subject)
		if err != nil {
			return err
		}
		info.PreferredUsername = username
		info.Name = name
	}
	return nil
}
func (*Storage) SetIntrospectionFromToken(context.Context, *liboidc.IntrospectionResponse, string, string, string) error {
	return errOIDCStateNotFound
}
func (s *Storage) GetPrivateClaimsFromScopes(ctx context.Context, subject, _ string, _ []string) (map[string]any, error) {
	if s.profile == nil {
		return map[string]any{}, nil
	}
	username, name, err := s.profile(ctx, subject)
	if err != nil {
		return nil, err
	}
	claims := map[string]any{}
	if username != "" {
		claims["preferred_username"] = username
	}
	if name != "" {
		claims["name"] = name
	}
	if len(claims) == 0 {
		return map[string]any{}, nil
	}
	return claims, nil
}
func (*Storage) GetKeyByIDAndClientID(context.Context, string, string) (*jose.JSONWebKey, error) {
	return nil, errOIDCStateNotFound
}
func (*Storage) ValidateJWTProfileScopes(context.Context, string, []string) ([]string, error) {
	return nil, liboidc.ErrUnsupportedGrantType()
}
func (*Storage) Health(context.Context) error { return nil }

type signingKey struct {
	key any
	id  string
}

func (k signingKey) SignatureAlgorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (k signingKey) Key() any                                    { return k.key }
func (k signingKey) ID() string                                  { return k.id }

type publicKey struct {
	key any
	id  string
}

func (k publicKey) ID() string                       { return k.id }
func (publicKey) Algorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (publicKey) Use() string                        { return "sig" }
func (k publicKey) Key() any                         { return k.key }

func (s *Storage) SigningKey(ctx context.Context) (op.SigningKey, error) {
	key, err := s.issuer.SigningKey(ctx)
	if err != nil {
		return nil, err
	}
	return signingKey{key: key.Private, id: key.ID}, nil
}
func (*Storage) SignatureAlgorithms(context.Context) ([]jose.SignatureAlgorithm, error) {
	return []jose.SignatureAlgorithm{jose.RS256}, nil
}
func (s *Storage) KeySet(ctx context.Context) ([]op.Key, error) {
	keys, err := s.issuer.VerificationKeys(ctx)
	if err != nil {
		return nil, err
	}
	public := make([]op.Key, 0, len(keys))
	for _, key := range keys {
		public = append(public, publicKey{key: &key.Private.PublicKey, id: key.ID})
	}
	return public, nil
}

func (s *Storage) readRequest(ctx context.Context, id string) (jetstream.KeyValueEntry, *authRequestState, error) {
	key := s.requestKey(id)
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		return nil, nil, errOIDCStateNotFound
	}
	var state authRequestState
	if err := s.open(key, entry.Value(), &state); err != nil || state.ID != id || !state.ExpiresAt.After(s.now().UTC()) {
		return nil, nil, errOIDCStateNotFound
	}
	return entry, &state, nil
}

func (s *Storage) create(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := s.seal(key, value)
	if err != nil {
		return err
	}
	_, err = s.js.Publish(ctx, "$KV."+storage.RuntimeStateBucket+"."+key, data, jetstream.WithExpectLastSequencePerSubject(0), jetstream.WithMsgTTL(ttl))
	return err
}
func (s *Storage) read(key string, ctx context.Context, value any) error {
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		return errOIDCStateNotFound
	}
	return s.open(key, entry.Value(), value)
}
func (s *Storage) seal(key string, value any) ([]byte, error) {
	return runtimejson.Seal(s.key, []byte("authling:oidc-runtime:v1\x00"+key), value, runtimejson.LowercaseFields)
}
func (s *Storage) open(key string, data []byte, value any) error {
	err := runtimejson.Open(s.key, []byte("authling:oidc-runtime:v1\x00"+key), data, value)
	if errors.Is(err, runtimejson.ErrInvalidEnvelope) {
		return fmt.Errorf("invalid OIDC state envelope")
	}
	return err
}
func (s *Storage) derivedKey(kind, secret string) string {
	digest := hmac.New(sha256.New, s.key)
	_, _ = digest.Write([]byte("oidc:" + kind + "\x00" + secret))
	return "oidc." + kind + "." + base64.RawURLEncoding.EncodeToString(digest.Sum(nil))
}
func (s *Storage) requestKey(id string) string { return s.derivedKey("request", id) }
func (s *Storage) codeKey(code string) string  { return s.derivedKey("code", code) }
func (s *Storage) tokenKey(id string) string   { return s.derivedKey("token", id) }
