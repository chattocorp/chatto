package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	renewableSessionKeyPrefix = "renewable_session."
	refreshTokenPrefix        = "cht_RT_"
	accessTokenPrefix         = "cht_AT"
	maxRefreshRequestIDLength = 128
)

var (
	// ErrRefreshTokenNotFound is returned for malformed, expired, revoked, or
	// otherwise unusable refresh credentials.
	ErrRefreshTokenNotFound = errors.New("refresh token not found")
	// ErrRefreshTokenReused is returned after a stale refresh credential is
	// presented under a different rotation request and the session is revoked.
	ErrRefreshTokenReused = errors.New("refresh token reuse detected")
	// ErrRefreshRequestIDInvalid is returned when rotation has no bounded opaque
	// idempotency key.
	ErrRefreshRequestIDInvalid = errors.New("refresh request ID is invalid")
	// ErrRefreshTokenClientMismatch prevents a public client from redeeming a
	// refresh credential issued to another client.
	ErrRefreshTokenClientMismatch = errors.New("refresh token client mismatch")
)

// BearerSessionCredentials is the show-once credential pair returned to a
// human client. Raw values are never persisted.
type BearerSessionCredentials struct {
	AccessToken          string
	RefreshToken         string
	AccessTokenExpiresAt time.Time
	SessionExpiresAt     time.Time
}

// RenewableSession is the latest-value authority stored in RUNTIME_STATE for
// one human bearer login. CurrentGeneration and the KV revision jointly fence
// refresh-token rotation across replicas.
type RenewableSession struct {
	UserID               string                       `json:"user_id"`
	ClientID             string                       `json:"client_id,omitempty"`
	Kind                 AuthTokenKind                `json:"kind"`
	Source               string                       `json:"source,omitempty"`
	Request              *corev1.AuditRequestMetadata `json:"request,omitempty"`
	CreatedAt            time.Time                    `json:"created_at"`
	ExpiresAt            time.Time                    `json:"expires_at"`
	AuthGeneration       uint64                       `json:"auth_generation"`
	CurrentGeneration    uint64                       `json:"current_generation"`
	LastRefreshRequestID string                       `json:"last_refresh_request_id,omitempty"`
	LastRotatedAt        time.Time                    `json:"last_rotated_at,omitempty"`
	FreshAuthAt          time.Time                    `json:"fresh_auth_at,omitempty"`
	FreshAuthMethod      string                       `json:"fresh_auth_method,omitempty"`
	FreshAuthSource      string                       `json:"fresh_auth_source,omitempty"`
}

func (c *ChattoCore) bearerAccessTokenTTL() time.Duration {
	if c.config.AuthAccessTokenTTL > 0 {
		return c.config.AuthAccessTokenTTL
	}
	return 15 * time.Minute
}

func (c *ChattoCore) renewableSessionTTL() time.Duration {
	return c.authTokenTTL()
}

func (c *ChattoCore) renewableSessionKey(sessionID string) string {
	return c.runtimeTokenKey(renewableSessionKeyPrefix, sessionID)
}

func (c *ChattoCore) credentialMAC(purpose, sessionID string, generation uint64) []byte {
	mac := hmac.New(sha256.New, []byte(c.config.SecretKey))
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(sessionID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strconv.FormatUint(generation, 10)))
	return mac.Sum(nil)
}

func (c *ChattoCore) accessTokenForGeneration(sessionID string, generation uint64) string {
	return accessTokenPrefix + base64.RawURLEncoding.EncodeToString(c.credentialMAC("bearer-access-v1", sessionID, generation))
}

func (c *ChattoCore) refreshTokenForGeneration(sessionID string, generation uint64) string {
	signature := base64.RawURLEncoding.EncodeToString(c.credentialMAC("bearer-refresh-v1", sessionID, generation))
	return refreshTokenPrefix + sessionID + "." + strconv.FormatUint(generation, 10) + "." + signature
}

func (c *ChattoCore) parseRefreshToken(token string) (string, uint64, bool) {
	raw, ok := strings.CutPrefix(token, refreshTokenPrefix)
	if !ok {
		return "", 0, false
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", 0, false
	}
	generation, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return "", 0, false
	}
	expected := c.credentialMAC("bearer-refresh-v1", parts[0], generation)
	presented, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || subtle.ConstantTimeCompare(presented, expected) != 1 {
		return "", 0, false
	}
	return parts[0], generation, true
}

func validRefreshRequestID(requestID string) bool {
	if len(requestID) < 16 || len(requestID) > maxRefreshRequestIDLength {
		return false
	}
	for _, char := range requestID {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

// CreateBearerSessionWithSource creates a renewable first-party bearer session.
func (c *ChattoCore) CreateBearerSessionWithSource(ctx context.Context, userID, source string) (BearerSessionCredentials, error) {
	authGeneration, err := c.CurrentAuthGeneration(ctx, userID)
	if err != nil {
		return BearerSessionCredentials{}, err
	}
	return c.CreateBearerSessionWithSourceGeneration(ctx, userID, source, authGeneration)
}

// CreateBearerSessionWithSourceGeneration creates a renewable first-party
// bearer session for credentials proven against authGeneration.
func (c *ChattoCore) CreateBearerSessionWithSourceGeneration(ctx context.Context, userID, source string, authGeneration uint64) (BearerSessionCredentials, error) {
	return c.createBearerSession(ctx, userID, "", source, authGeneration)
}

// CreateOAuthBearerSessionForClient creates a renewable delegated bearer
// session bound to the public OAuth client that completed authorization.
func (c *ChattoCore) CreateOAuthBearerSessionForClient(ctx context.Context, userID, clientID string, authGeneration uint64) (BearerSessionCredentials, error) {
	if err := c.RequireOAuthClientAllowed(ctx, clientID); err != nil {
		return BearerSessionCredentials{}, err
	}
	credentials, err := c.createBearerSession(ctx, userID, clientID, "oauth_code_exchange", authGeneration)
	if err != nil {
		return BearerSessionCredentials{}, err
	}
	if err := c.RequireOAuthClientAllowed(ctx, clientID); err != nil {
		_ = c.RevokeRefreshTokenWithReason(ctx, credentials.RefreshToken, "oauth_client_blocked_during_issuance")
		return BearerSessionCredentials{}, err
	}
	return credentials, nil
}

func (c *ChattoCore) createBearerSession(ctx context.Context, userID, clientID, source string, authGeneration uint64) (BearerSessionCredentials, error) {
	if userID == "" {
		return BearerSessionCredentials{}, ErrAuthTokenNotFound
	}
	if err := c.requireHumanUser(ctx, userID); err != nil {
		if errors.Is(err, ErrHumanAccountRequired) || errors.Is(err, ErrNotFound) {
			return BearerSessionCredentials{}, ErrAuthTokenNotFound
		}
		return BearerSessionCredentials{}, err
	}
	if err := c.RequireAuthenticationAllowed(ctx, userID, authGeneration); err != nil {
		if errors.Is(err, ErrAuthenticationRevoked) {
			return BearerSessionCredentials{}, ErrAuthTokenNotFound
		}
		return BearerSessionCredentials{}, err
	}

	now := time.Now()
	sessionID := newRenewableSessionID()
	session := RenewableSession{
		UserID:            userID,
		ClientID:          clientID,
		Kind:              authTokenKindForSource(source),
		Source:            source,
		Request:           auditRequestMetadata(ctx),
		CreatedAt:         now,
		ExpiresAt:         now.Add(c.renewableSessionTTL()),
		AuthGeneration:    authGeneration,
		CurrentGeneration: 0,
	}
	if sourceGrantsInitialFreshAuth(source) {
		session.FreshAuthAt = now
		session.FreshAuthMethod = freshAuthMethodForSource(source)
		session.FreshAuthSource = source
	}
	value, err := json.Marshal(session)
	if err != nil {
		return BearerSessionCredentials{}, fmt.Errorf("marshal renewable session: %w", err)
	}
	if _, err := c.storage.runtimeStateKV.Create(ctx, c.renewableSessionKey(sessionID), value, jetstream.KeyTTL(c.renewableSessionTTL())); err != nil {
		return BearerSessionCredentials{}, fmt.Errorf("store renewable session: %w", err)
	}
	credentials := c.credentialsForGeneration(sessionID, session, now)
	if err := c.createAccessTokenRecord(ctx, sessionID, session, now); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, c.renewableSessionKey(sessionID))
		return BearerSessionCredentials{}, err
	}
	if err := c.recordBearerTokenIssued(ctx, userID, credentials.AccessTokenExpiresAt, source); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, c.authTokenKey(credentials.AccessToken))
		_ = c.storage.runtimeStateKV.Delete(ctx, c.renewableSessionKey(sessionID))
		return BearerSessionCredentials{}, err
	}
	return credentials, nil
}

func (c *ChattoCore) credentialsForGeneration(sessionID string, session RenewableSession, issuedAt time.Time) BearerSessionCredentials {
	return BearerSessionCredentials{
		AccessToken:          c.accessTokenForGeneration(sessionID, session.CurrentGeneration),
		RefreshToken:         c.refreshTokenForGeneration(sessionID, session.CurrentGeneration),
		AccessTokenExpiresAt: c.bearerAccessExpiresAt(session, issuedAt),
		SessionExpiresAt:     session.ExpiresAt,
	}
}

func (c *ChattoCore) bearerAccessExpiresAt(session RenewableSession, issuedAt time.Time) time.Time {
	expiresAt := issuedAt.Add(c.bearerAccessTokenTTL())
	if session.ExpiresAt.Before(expiresAt) {
		return session.ExpiresAt
	}
	return expiresAt
}

func (c *ChattoCore) createAccessTokenRecord(ctx context.Context, sessionID string, session RenewableSession, issuedAt time.Time) error {
	token := c.accessTokenForGeneration(sessionID, session.CurrentGeneration)
	expiresAt := c.bearerAccessExpiresAt(session, issuedAt)
	ttl := expiresAt.Sub(issuedAt)
	if ttl <= 0 {
		return ErrRefreshTokenNotFound
	}
	data := AuthTokenData{
		UserID:             session.UserID,
		ClientID:           session.ClientID,
		Kind:               session.Kind,
		Presentation:       AuthTokenPresentationBearer,
		Source:             session.Source,
		Request:            session.Request,
		CreatedAt:          issuedAt,
		ExpiresAt:          expiresAt,
		AuthGeneration:     session.AuthGeneration,
		RenewableSessionID: sessionID,
		AccessGeneration:   session.CurrentGeneration,
		FreshAuthAt:        session.FreshAuthAt,
		FreshAuthMethod:    session.FreshAuthMethod,
		FreshAuthSource:    session.FreshAuthSource,
	}
	value, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal bearer access token: %w", err)
	}
	if _, err := c.storage.runtimeStateKV.Create(ctx, c.authTokenKey(token), value, jetstream.KeyTTL(ttl)); err != nil {
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("store bearer access token: %w", err)
		}
		entry, getErr := c.storage.runtimeStateKV.Get(ctx, c.authTokenKey(token))
		if getErr != nil || subtle.ConstantTimeCompare(entry.Value(), value) != 1 {
			return fmt.Errorf("store bearer access token: deterministic token collision")
		}
	}
	return nil
}

func (c *ChattoCore) loadRenewableSession(ctx context.Context, sessionID string) (RenewableSession, jetstream.KeyValueEntry, error) {
	entry, err := c.storage.runtimeStateKV.Get(ctx, c.renewableSessionKey(sessionID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return RenewableSession{}, nil, ErrRefreshTokenNotFound
		}
		return RenewableSession{}, nil, fmt.Errorf("get renewable session: %w", err)
	}
	var session RenewableSession
	if err := json.Unmarshal(entry.Value(), &session); err != nil || session.UserID == "" || session.ExpiresAt.IsZero() {
		_ = c.storage.runtimeStateKV.Delete(ctx, c.renewableSessionKey(sessionID))
		return RenewableSession{}, nil, ErrRefreshTokenNotFound
	}
	return session, entry, nil
}

func (c *ChattoCore) validateRenewableSession(ctx context.Context, sessionID string, now time.Time) (RenewableSession, jetstream.KeyValueEntry, error) {
	session, entry, err := c.loadRenewableSession(ctx, sessionID)
	if err != nil {
		return RenewableSession{}, nil, err
	}
	if !now.Before(session.ExpiresAt) {
		_ = c.storage.runtimeStateKV.Delete(ctx, c.renewableSessionKey(sessionID), jetstream.LastRevision(entry.Revision()))
		return RenewableSession{}, nil, ErrRefreshTokenNotFound
	}
	if session.Kind == AuthTokenKindOAuthAccessToken {
		if err := c.RequireOAuthClientAllowed(ctx, session.ClientID); err != nil {
			if errors.Is(err, ErrOAuthClientBlocked) {
				_ = c.storage.runtimeStateKV.Delete(ctx, c.renewableSessionKey(sessionID), jetstream.LastRevision(entry.Revision()))
				return RenewableSession{}, nil, ErrRefreshTokenNotFound
			}
			return RenewableSession{}, nil, err
		}
	}
	if _, err := c.ValidateRuntimeCredential(ctx, RuntimeCredential{
		UserID: session.UserID, CreatedAt: session.CreatedAt, AuthGeneration: session.AuthGeneration,
	}); err != nil {
		if errors.Is(err, ErrAuthenticationRevoked) {
			_ = c.storage.runtimeStateKV.Delete(ctx, c.renewableSessionKey(sessionID), jetstream.LastRevision(entry.Revision()))
			return RenewableSession{}, nil, ErrRefreshTokenNotFound
		}
		return RenewableSession{}, nil, err
	}
	return session, entry, nil
}

// RefreshBearerSession rotates a refresh credential using a client-persisted
// idempotency key. The immediately preceding committed rotation can be replayed
// only with the same request ID and only while its access token remains useful.
func (c *ChattoCore) RefreshBearerSession(ctx context.Context, refreshToken, requestID, clientID string) (BearerSessionCredentials, error) {
	return c.refreshBearerSessionAt(ctx, refreshToken, requestID, clientID, time.Now())
}

func (c *ChattoCore) refreshBearerSessionAt(ctx context.Context, refreshToken, requestID, clientID string, now time.Time) (BearerSessionCredentials, error) {
	if !validRefreshRequestID(requestID) {
		return BearerSessionCredentials{}, ErrRefreshRequestIDInvalid
	}
	sessionID, presentedGeneration, ok := c.parseRefreshToken(refreshToken)
	if !ok {
		return BearerSessionCredentials{}, ErrRefreshTokenNotFound
	}

	for attempt := 0; attempt < 8; attempt++ {
		session, entry, err := c.validateRenewableSession(ctx, sessionID, now)
		if err != nil {
			return BearerSessionCredentials{}, err
		}
		if session.ClientID != clientID {
			return BearerSessionCredentials{}, ErrRefreshTokenClientMismatch
		}

		if presentedGeneration != session.CurrentGeneration {
			if presentedGeneration+1 == session.CurrentGeneration &&
				session.LastRefreshRequestID == requestID &&
				!session.LastRotatedAt.IsZero() &&
				now.Sub(session.LastRotatedAt) >= 0 &&
				now.Sub(session.LastRotatedAt) < c.bearerAccessTokenTTL() {
				if err := c.createAccessTokenRecord(ctx, sessionID, session, session.LastRotatedAt); err != nil {
					return BearerSessionCredentials{}, err
				}
				return c.credentialsForGeneration(sessionID, session, session.LastRotatedAt), nil
			}
			if err := c.revokeRenewableSession(ctx, sessionID, "refresh_token_reuse"); err != nil {
				return BearerSessionCredentials{}, err
			}
			return BearerSessionCredentials{}, ErrRefreshTokenReused
		}

		next := session
		next.CurrentGeneration++
		next.LastRefreshRequestID = requestID
		next.LastRotatedAt = now
		value, err := json.Marshal(next)
		if err != nil {
			return BearerSessionCredentials{}, fmt.Errorf("marshal rotated renewable session: %w", err)
		}
		remaining := next.ExpiresAt.Sub(now)
		if remaining <= 0 {
			return BearerSessionCredentials{}, ErrRefreshTokenNotFound
		}
		if _, err := c.updateRuntimeStateTokenTTL(ctx, c.renewableSessionKey(sessionID), value, entry.Revision(), remaining); err != nil {
			if isRuntimeStateRevisionConflict(err) {
				continue
			}
			return BearerSessionCredentials{}, fmt.Errorf("rotate renewable session: %w", err)
		}
		// Commit the rotation before publishing its access credential. If the
		// process fails between these operations, retrying the same persisted
		// request ID recreates the exact deterministic credential above.
		if err := c.createAccessTokenRecord(ctx, sessionID, next, now); err != nil {
			return BearerSessionCredentials{}, err
		}
		confirmed, _, err := c.validateRenewableSession(ctx, sessionID, now)
		if err != nil {
			return BearerSessionCredentials{}, err
		}
		if confirmed.CurrentGeneration != next.CurrentGeneration ||
			confirmed.LastRefreshRequestID != requestID ||
			!confirmed.LastRotatedAt.Equal(now) {
			return BearerSessionCredentials{}, ErrRefreshTokenNotFound
		}
		return c.credentialsForGeneration(sessionID, next, now), nil
	}
	return BearerSessionCredentials{}, fmt.Errorf("rotate renewable session: too much contention")
}

func (c *ChattoCore) revokeRenewableSession(ctx context.Context, sessionID, reason string) error {
	session, _, err := c.loadRenewableSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ErrRefreshTokenNotFound) {
			return nil
		}
		return err
	}
	// Durable audit append remains a prerequisite for revocation. Deleting the
	// stable key without a revision then fences any rotation that raced the
	// append and ensures the latest generation is revoked too.
	if err := c.recordBearerTokenRevoked(ctx, session.UserID, reason); err != nil {
		return err
	}
	if err := c.storage.runtimeStateKV.Delete(ctx, c.renewableSessionKey(sessionID)); err != nil &&
		!errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		return fmt.Errorf("revoke renewable session: %w", err)
	}
	return nil
}

// RevokeRefreshTokenWithReason revokes the whole renewable session identified
// by an authentic refresh token, regardless of whether that token is current.
func (c *ChattoCore) RevokeRefreshTokenWithReason(ctx context.Context, refreshToken, reason string) error {
	_, _, err := c.RevokeRefreshTokenWithReasonResult(ctx, refreshToken, reason)
	return err
}

// RevokeRefreshTokenWithReasonResult revokes a renewable session and returns
// the owning user when the presented refresh credential was authentic and the
// session still existed.
func (c *ChattoCore) RevokeRefreshTokenWithReasonResult(ctx context.Context, refreshToken, reason string) (string, bool, error) {
	sessionID, _, ok := c.parseRefreshToken(refreshToken)
	if !ok {
		return "", false, nil
	}
	session, _, err := c.loadRenewableSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ErrRefreshTokenNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	if err := c.revokeRenewableSession(ctx, sessionID, reason); err != nil {
		return "", false, err
	}
	return session.UserID, true, nil
}

func (c *ChattoCore) markRenewableSessionFresh(ctx context.Context, sessionID, method, source string, now time.Time) error {
	if sessionID == "" {
		return ErrAuthTokenNotFound
	}
	for attempt := 0; attempt < 8; attempt++ {
		session, entry, err := c.validateRenewableSession(ctx, sessionID, now)
		if err != nil {
			if errors.Is(err, ErrRefreshTokenNotFound) {
				return ErrAuthTokenNotFound
			}
			return err
		}
		if session.Kind != AuthTokenKindFirstPartySession {
			return ErrFreshAuthRequired
		}
		session.FreshAuthAt = now
		session.FreshAuthMethod = method
		session.FreshAuthSource = source
		value, err := json.Marshal(session)
		if err != nil {
			return fmt.Errorf("marshal fresh renewable session: %w", err)
		}
		remaining := session.ExpiresAt.Sub(now)
		if remaining <= 0 {
			return ErrAuthTokenNotFound
		}
		if _, err := c.updateRuntimeStateTokenTTL(ctx, c.renewableSessionKey(sessionID), value, entry.Revision(), remaining); err != nil {
			if isRuntimeStateRevisionConflict(err) {
				continue
			}
			return fmt.Errorf("mark renewable session fresh: %w", err)
		}
		return nil
	}
	return fmt.Errorf("mark renewable session fresh: too much contention")
}
