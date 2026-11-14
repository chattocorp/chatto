package oidcprovider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/chattocorp/chatto/authling/internal/accounts"
	"github.com/chattocorp/chatto/authling/internal/ids"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
	jose "github.com/go-jose/go-jose/v4"
	liboidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

const userinfoClockSkew = 60 * time.Second

// resourceContextKey carries the validated authorize resource into CreateAuthRequest.
type resourceContextKey struct{}

// resolvedClientKey carries the client that authorize admission resolved.
type resolvedClientKey struct{}

// checkResource checks current operator policy without fetching the resource.
func (s *Storage) checkResource(client *Client, resource string) error {
	if resource == "" {
		return nil
	}
	if client.Source == ClientSourceConfigured && slices.Contains(client.Resources, resource) {
		return nil
	}
	if client.Source == ClientSourceCIMD {
		for _, registered := range s.cfg.Resources {
			if registered.URI == resource && registered.AllowCIMD {
				return nil
			}
		}
	}
	return &liboidc.Error{ErrorType: "invalid_target"}
}

// checkAuthorizationPolicy applies current client policy to requested scopes
// and resource. Authorization admission, auth-request creation and every token
// request use it, so the rules cannot drift apart.
func (s *Storage) checkAuthorizationPolicy(client *Client, scopes []string, resource string) error {
	if slices.Contains(scopes, liboidc.ScopeOfflineAccess) && !slices.Contains(client.GrantTypes(), liboidc.GrantTypeRefreshToken) {
		return liboidc.ErrInvalidScope()
	}
	return s.checkResource(client, resource)
}

// tokenError writes an RFC 6749 section 5.2 error. A failed client
// authentication uses 401 with a Basic challenge.
func tokenError(w http.ResponseWriter, code string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	status := http.StatusBadRequest
	if code == string(liboidc.InvalidClient) {
		w.Header().Set("WWW-Authenticate", `Basic realm="authling"`)
		status = http.StatusUnauthorized
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

// protocolErrorCode returns the OAuth error code of err, or invalid_grant.
func protocolErrorCode(err error) string {
	var protocolError *liboidc.Error
	if errors.As(err, &protocolError) {
		return string(protocolError.ErrorType)
	}
	return "invalid_grant"
}

// serveToken issues tokens for both grant types. Each grant parser returns the
// authorized request; this function applies the shared policy and issues the
// response. Access-token signing is separate from ID-token signing because
// their audiences and JOSE types have different contracts.
func (s *Service) serveToken(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	provider := s.provider
	s.mu.RUnlock()
	if provider == nil {
		tokenError(w, "temporarily_unavailable")
		return
	}
	issuerState, ok := s.issuer.State()
	if !ok {
		tokenError(w, "temporarily_unavailable")
		return
	}
	ctx := op.ContextWithIssuer(r.Context(), issuerState.Issuer)
	r = r.WithContext(ctx)
	var request *authRequestState
	var client *Client
	var code, refresh string
	var err error
	switch r.PostForm.Get("grant_type") {
	case string(liboidc.GrantTypeCode):
		request, client, code, err = s.codeGrantRequest(ctx, r, provider)
	case string(liboidc.GrantTypeRefreshToken):
		request, client, refresh, err = s.refreshGrantRequest(ctx, r, provider)
	default:
		tokenError(w, "unsupported_grant_type")
		return
	}
	if err != nil {
		tokenError(w, protocolErrorCode(err))
		return
	}
	resources := r.PostForm["resource"]
	if len(resources) > 1 || len(resources) == 1 && (resources[0] == "" || resources[0] != request.Resource) {
		tokenError(w, "invalid_target")
		return
	}
	if err := s.storage.checkAuthorizationPolicy(client, request.Scopes, request.Resource); err != nil {
		tokenError(w, protocolErrorCode(err))
		return
	}
	offline := slices.Contains(request.Scopes, liboidc.ScopeOfflineAccess)
	// A new family needs the grant now. rotateRefresh checks it inside its OCC loop.
	if refresh == "" && offline {
		if err := s.storage.checkGrant(ctx, request); err != nil {
			tokenError(w, "invalid_grant")
			return
		}
	}
	// Prepare the access record before consuming credentials. A failed record write
	// leaves the old credential retryable; orphan records expire without a bearer.
	access, tokenID, expires, err := s.storage.signAccessToken(ctx, issuerState.Issuer, request)
	if err != nil {
		tokenError(w, issuanceError(err))
		return
	}
	var idToken string
	if slices.Contains(request.Scopes, liboidc.ScopeOpenID) {
		idToken, err = op.CreateIDToken(ctx, issuerState.Issuer, request, client.IDTokenLifetime(), access, code, s.storage, client)
		if err != nil {
			tokenError(w, issuanceError(err))
			return
		}
	}
	state := tokenState{ClientID: request.ClientID, Subject: request.Subject, Scopes: request.Scopes, Expires: expires}
	if err := s.storage.create(ctx, s.storage.tokenKey(tokenID), state, expires.Sub(s.storage.now())); err != nil {
		tokenError(w, "temporarily_unavailable")
		return
	}

	if refresh != "" {
		refresh, err = s.storage.rotateRefresh(ctx, refresh, request)
	} else {
		if offline {
			refresh, err = s.storage.newRefresh(ctx, request)
			if err != nil {
				tokenError(w, "temporarily_unavailable")
				return
			}
		}
		err = s.storage.claimCode(ctx, request)
	}
	if err != nil {
		tokenError(w, "invalid_grant")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	response := map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": int64(accessTokenLifetime / time.Second), "scope": strings.Join(request.Scopes, " ")}
	if idToken != "" {
		response["id_token"] = idToken
	}
	if refresh != "" {
		response["refresh_token"] = refresh
	}
	_ = json.NewEncoder(w).Encode(response)
}

// codeGrantRequest retains the library's code, PKCE and client authentication checks.
func (s *Service) codeGrantRequest(ctx context.Context, r *http.Request, provider *op.Provider) (*authRequestState, *Client, string, error) {
	parsed, err := op.ParseAccessTokenRequest(r, provider.Decoder())
	if err != nil {
		return nil, nil, "", err
	}
	auth, client, err := op.ValidateAccessTokenRequest(ctx, parsed, provider)
	if err != nil {
		return nil, nil, "", err
	}
	request, ok := auth.(*authRequestState)
	resolved, resolvedOK := client.(*Client)
	if !ok || !resolvedOK {
		return nil, nil, "", errOIDCStateNotFound
	}
	return request, resolved, parsed.Code, nil
}

// refreshGrantRequest authenticates the client and returns the family's request.
// An explicit scope subset narrows only this response.
func (s *Service) refreshGrantRequest(ctx context.Context, r *http.Request, provider *op.Provider) (*authRequestState, *Client, string, error) {
	parsed, err := op.ParseRefreshTokenRequest(r, provider.Decoder())
	if err != nil {
		return nil, nil, "", err
	}
	client, err := s.storage.clients.ResolveForRefresh(ctx, parsed.ClientID)
	if err != nil {
		return nil, nil, "", liboidc.ErrInvalidClient()
	}
	if !slices.Contains(client.GrantTypes(), liboidc.GrantTypeRefreshToken) {
		return nil, nil, "", liboidc.ErrUnauthorizedClient()
	}
	if client.AuthMethod() != liboidc.AuthMethodNone && s.storage.AuthorizeClientIDSecret(ctx, parsed.ClientID, parsed.ClientSecret) != nil {
		return nil, nil, "", liboidc.ErrInvalidClient()
	}
	request, err := s.storage.refreshRequest(ctx, parsed.RefreshToken, client.GetID())
	if err != nil {
		return nil, nil, "", err
	}
	if len(parsed.Scopes) > 0 {
		seen := map[string]bool{}
		for _, scope := range parsed.Scopes {
			if seen[scope] || !slices.Contains(request.Scopes, scope) {
				return nil, nil, "", liboidc.ErrInvalidScope()
			}
			seen[scope] = true
		}
		request.Scopes = append([]string(nil), parsed.Scopes...)
	}
	return request, client, parsed.RefreshToken, nil
}

func (s *Storage) signAccessToken(ctx context.Context, issuerURL string, request *authRequestState) (string, string, time.Time, error) {
	claims, err := s.accountClaims(ctx, request.Subject, request.Scopes)
	if err != nil {
		return "", "", time.Time{}, err
	}
	id, err := ids.New("at")
	if err != nil {
		return "", "", time.Time{}, err
	}
	now := s.now().UTC()
	expires := now.Add(accessTokenLifetime)
	audience := []string{strings.TrimSuffix(issuerURL, "/") + "/oauth/userinfo"}
	if request.Resource != "" && request.Resource != audience[0] {
		audience = append(audience, request.Resource)
	}
	claims["iss"], claims["sub"], claims["aud"] = issuerURL, request.Subject, audience
	claims["client_id"], claims["iat"], claims["exp"], claims["jti"] = request.ClientID, now.Unix(), expires.Unix(), id
	claims["scope"] = strings.Join(request.Scopes, " ")
	key, err := s.SigningKey(ctx)
	if err != nil {
		return "", "", time.Time{}, err
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: &jose.JSONWebKey{Key: key.Key(), KeyID: key.ID()}}, (&jose.SignerOptions{}).WithType("at+jwt"))
	if err != nil {
		return "", "", time.Time{}, err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", "", time.Time{}, err
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		return "", "", time.Time{}, err
	}
	token, err := signed.CompactSerialize()
	return token, id, expires, err
}

// serveJWTUserinfo requires an access-token type, trusted signing key, issuer,
// UserInfo audience and live encrypted token record. ID tokens cannot be used.
func (s *Service) serveJWTUserinfo(w http.ResponseWriter, r *http.Request, token string) {
	fail := func() {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		http.Error(w, "invalid token", http.StatusUnauthorized)
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		fail()
		return
	}
	if len(token) > 16<<10 {
		fail()
		return
	}
	signed, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil || len(signed.Signatures) != 1 || signed.Signatures[0].Header.ExtraHeaders["typ"] != "at+jwt" {
		fail()
		return
	}
	keys, err := s.storage.KeySet(r.Context())
	if err != nil {
		fail()
		return
	}
	var payload []byte
	for _, key := range keys {
		if key.ID() == signed.Signatures[0].Header.KeyID {
			payload, err = signed.Verify(key.Key())
			break
		}
	}
	if err != nil || payload == nil {
		fail()
		return
	}
	var claims struct {
		Issuer    string   `json:"iss"`
		Subject   string   `json:"sub"`
		Audience  []string `json:"aud"`
		Expires   int64    `json:"exp"`
		Issued    int64    `json:"iat"`
		NotBefore int64    `json:"nbf"`
		ID        string   `json:"jti"`
	}
	issuerState, ok := s.issuer.State()
	now := s.storage.now().Unix()
	if json.Unmarshal(payload, &claims) != nil || !ok || claims.Issuer != issuerState.Issuer || claims.Expires <= now || claims.Issued > now+int64(userinfoClockSkew/time.Second) || claims.NotBefore > now+int64(userinfoClockSkew/time.Second) || !slices.Contains(claims.Audience, strings.TrimSuffix(issuerState.Issuer, "/")+"/oauth/userinfo") {
		fail()
		return
	}
	info := new(liboidc.UserInfo)
	if s.storage.SetUserinfoFromToken(r.Context(), info, claims.ID, claims.Subject, "") != nil {
		fail()
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

// refreshFamily is one encrypted OCC record for a whole rotating family. Tokens
// carry a keyed authenticator over the opaque family ID and generation. This
// proves old-token reuse without retaining an unbounded list of used tokens.
type refreshFamily struct {
	Request     authRequestState `json:"request"`
	Generation  uint64           `json:"generation"`
	Expires     time.Time        `json:"expires"`
	IdleExpires time.Time        `json:"idle_expires"`
	// RotatedAt is the time of the last successful rotation. It starts the
	// reuse grace window of the previous generation. It is zero for generation 0.
	RotatedAt time.Time `json:"rotated_at,omitzero"`
	// Repeated records that the previous generation used its one repeat.
	// Each rotation resets it.
	Repeated bool `json:"repeated,omitempty"`
	Revoked  bool `json:"revoked"`
}

// The reuse grace period (config.OIDCConfig.RefreshReuseGrace) is the time
// after a rotation in which the previous credential can repeat that rotation.
// It covers a lost token response and
// concurrent refreshes by one client. A repeat does not rotate the family: it
// returns the current credential again, so both callers converge on one token.
// Each rotation permits one repeat. A second repeat, or use of the previous
// credential after the window, is reuse and revokes the family. This limits a
// stolen previous credential to one attempt, and that attempt or the client's
// own retry then exposes the theft. A zero grace period disables repeats.

// refreshUse classifies a submitted generation against the stored family.
type refreshUse int

const (
	// refreshInvalid rejects the credential without a change to the family.
	refreshInvalid refreshUse = iota
	// refreshCurrent permits a rotation to the next generation.
	refreshCurrent
	// refreshRepeat returns the current generation again and marks the repeat as used.
	refreshRepeat
	// refreshReuse rejects the credential and revokes the family.
	refreshReuse
)

func (f refreshFamily) use(generation uint64, now time.Time, grace time.Duration) refreshUse {
	switch {
	case f.Revoked || !f.Expires.After(now) || !f.IdleExpires.After(now):
		return refreshInvalid
	case generation == f.Generation:
		return refreshCurrent
	case generation+1 == f.Generation && !f.Repeated && now.Before(f.RotatedAt.Add(grace)):
		return refreshRepeat
	case generation < f.Generation:
		return refreshReuse
	default:
		return refreshInvalid
	}
}

func (s *Storage) refreshToken(id string, generation uint64) string {
	payload := id + "." + strconv.FormatUint(generation, 10)
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte("authling:refresh-token:v1\x00" + payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Storage) parseRefresh(token string) (string, uint64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(token) > 256 {
		return "", 0, errOIDCStateNotFound
	}
	generation, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || !hmac.Equal([]byte(token), []byte(s.refreshToken(parts[0], generation))) {
		return "", 0, errOIDCStateNotFound
	}
	return parts[0], generation, nil
}

func (s *Storage) checkGrant(ctx context.Context, request *authRequestState) error {
	if s.grants == nil || request.GrantID == "" {
		return errOIDCStateNotFound
	}
	grant, err := s.grants.Current(ctx, request.Subject, request.ClientID)
	if err != nil || grant.ID != request.GrantID || request.PasswordVersion == "" || grant.PasswordVersion != request.PasswordVersion || !grant.Permits(request.Resource, append(append([]string(nil), request.Scopes...), liboidc.ScopeOfflineAccess)) {
		return errOIDCStateNotFound
	}

	return nil
}

func (s *Storage) newRefresh(ctx context.Context, request *authRequestState) (string, error) {
	id, err := ids.New("rf")
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	// Retain only renewal authority, not browser state, redirects or code material.
	family := refreshFamily{Request: authRequestState{
		ClientID: request.ClientID, Subject: request.Subject, GrantID: request.GrantID, PasswordVersion: request.PasswordVersion,
		Resource: request.Resource, Scopes: append([]string(nil), request.Scopes...),
		AuthTime: request.AuthTime, Authorized: true,
	}, Expires: now.Add(s.cfg.RefreshLifetime()), IdleExpires: now.Add(s.cfg.RefreshIdleLifetime())}
	if err := s.create(ctx, s.derivedKey("refresh-family", id), family, s.cfg.RefreshLifetime()); err != nil {
		return "", err
	}
	return s.refreshToken(id, 0), nil
}

func (s *Storage) refreshRequest(ctx context.Context, token, clientID string) (*authRequestState, error) {
	id, generation, err := s.parseRefresh(token)
	if err != nil {
		return nil, err
	}
	var family refreshFamily
	if err := s.read(s.derivedKey("refresh-family", id), ctx, &family); err != nil {
		return nil, err
	}
	if family.Request.ClientID != clientID {
		return nil, errOIDCStateNotFound
	}
	switch family.use(generation, s.now(), s.cfg.RefreshReuseGrace()) {
	case refreshCurrent, refreshRepeat:
		return &family.Request, nil
	case refreshReuse:
		_ = s.revokeRefreshFamily(ctx, id)
	}
	return nil, errOIDCStateNotFound
}

// rotateRefresh advances the family by one generation with one OCC write. A
// repeat inside the reuse grace window records the repeat with one OCC write
// and returns the current credential. After an OCC conflict, it reads the
// family again, so a concurrent caller that lost to a rotation of the same
// credential gets the winner's credential. A third conflict revokes the family.
func (s *Storage) rotateRefresh(ctx context.Context, token string, request *authRequestState) (string, error) {
	id, generation, err := s.parseRefresh(token)
	if err != nil {
		return "", err
	}
	key := s.derivedKey("refresh-family", id)
	for attempt := 0; ; attempt++ {
		entry, err := s.kv.Get(ctx, key)
		if err != nil {
			return "", errOIDCStateNotFound
		}
		var family refreshFamily
		if s.open(key, entry.Value(), &family) != nil || family.Request.ClientID != request.ClientID {
			return "", errOIDCStateNotFound
		}
		now := s.now()
		switch family.use(generation, now, s.cfg.RefreshReuseGrace()) {
		case refreshCurrent:
		case refreshRepeat:
			if err := s.checkGrant(ctx, &family.Request); err != nil {
				return "", err
			}
			family.Repeated = true
			data, err := s.seal(key, family)
			if err != nil {
				return "", err
			}
			_, err = s.kv.UpdateWithTTL(ctx, key, data, entry.Revision(), family.Expires.Sub(now))
			if jetstreamutil.IsSequenceConflict(err) {
				if attempt < 2 {
					continue
				}
				_ = s.revokeRefreshFamily(ctx, id)
				return "", errOIDCStateNotFound
			}
			if err != nil {
				return "", err
			}
			return s.refreshToken(id, family.Generation), nil
		case refreshReuse:
			_ = s.revokeRefreshFamily(ctx, id)
			return "", errOIDCStateNotFound
		default:
			return "", errOIDCStateNotFound
		}
		if err := s.checkGrant(ctx, &family.Request); err != nil {
			return "", err
		}
		family.Generation++
		family.RotatedAt, family.Repeated = now, false
		// The replacement retains the original scope. A narrowed scope affects
		// only this response (RFC 6749 section 6, RFC 8707 section 2.2).
		family.IdleExpires = now.Add(s.cfg.RefreshIdleLifetime())
		if family.IdleExpires.After(family.Expires) {
			family.IdleExpires = family.Expires
		}
		data, err := s.seal(key, family)
		if err != nil {
			return "", err
		}
		_, err = s.kv.UpdateWithTTL(ctx, key, data, entry.Revision(), family.Expires.Sub(now))
		if jetstreamutil.IsSequenceConflict(err) {
			if attempt < 2 {
				continue
			}
			_ = s.revokeRefreshFamily(ctx, id)
			return "", errOIDCStateNotFound
		}
		if err != nil {
			return "", err
		}
		return s.refreshToken(id, family.Generation), nil
	}
}

// revokeRefreshFamily retries only definite OCC conflicts. An unavailable store
// fails closed; no replacement token is returned on an uncertain write outcome.
func (s *Storage) revokeRefreshFamily(ctx context.Context, id string) error {
	key := s.derivedKey("refresh-family", id)
	for range 8 {
		entry, err := s.kv.Get(ctx, key)
		if err != nil {
			return err
		}
		var family refreshFamily
		if err := s.open(key, entry.Value(), &family); err != nil {
			return err
		}
		if family.Revoked {
			return nil
		}
		remaining := family.Expires.Sub(s.now())
		if remaining <= 0 {
			return nil
		}
		family.Revoked = true
		data, err := s.seal(key, family)
		if err != nil {
			return err
		}
		_, err = s.kv.UpdateWithTTL(ctx, key, data, entry.Revision(), remaining)
		if jetstreamutil.IsSequenceConflict(err) {
			continue
		}
		return err
	}
	return errors.New("refresh family conflict")
}

// issuanceError distinguishes a lost identity from a temporary account-store failure.
func issuanceError(err error) string {
	if errors.Is(err, accounts.ErrInvalidCredentials) {
		return "invalid_grant"
	}
	return "temporarily_unavailable"
}
