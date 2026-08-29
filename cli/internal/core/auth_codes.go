package core

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// ============================================================================
// Auth Code Errors
// ============================================================================

var (
	// ErrAuthCodeNotFound is returned when an authorization code doesn't exist or has expired.
	ErrAuthCodeNotFound = errors.New("authorization code not found")

	// ErrAuthCodeInvalidVerifier is returned when the PKCE code_verifier doesn't match the stored code_challenge.
	ErrAuthCodeInvalidVerifier = errors.New("invalid code verifier")

	// ErrAuthCodeRedirectMismatch is returned when the redirect_uri doesn't match the one used during authorization.
	ErrAuthCodeRedirectMismatch = errors.New("redirect URI mismatch")

	// ErrAuthCodeClientMismatch is returned when client_id doesn't match the one used during authorization.
	ErrAuthCodeClientMismatch = errors.New("client ID mismatch")

	// ErrAuthCodeInvalidMethod is returned when the code_challenge_method is not S256.
	ErrAuthCodeInvalidMethod = errors.New("unsupported code challenge method: only S256 is supported")
)

// authCodeTTL is the lifetime of an authorization code. Codes that aren't
// exchanged within this window are automatically purged by NATS KV per-key TTL.
const authCodeTTL = 5 * time.Minute

// authCodeKeyPrefix is the RUNTIME_STATE key prefix that distinguishes
// authorization codes from bearer tokens.
const authCodeKeyPrefix = "grant."

func (c *ChattoCore) authCodeKey(code string) string {
	return c.runtimeTokenKey(authCodeKeyPrefix, code)
}

// ============================================================================
// Auth Code Types
// ============================================================================

// AuthCodeData is the JSON value stored in RUNTIME_STATE for authorization codes.
type AuthCodeData struct {
	UserID              string    `json:"user_id"`
	ClientID            string    `json:"client_id,omitempty"`
	Resource            string    `json:"resource,omitempty"`
	Scopes              []string  `json:"scopes,omitempty"`
	RedirectURI         string    `json:"redirect_uri"`
	CodeChallenge       string    `json:"code_challenge"`
	CodeChallengeMethod string    `json:"code_challenge_method"`
	CreatedAt           time.Time `json:"created_at"`
	AuthGeneration      uint64    `json:"auth_generation,omitempty"`
}

// ============================================================================
// Auth Code Operations
// ============================================================================

// CreateAuthCode generates a new OAuth authorization code for the given user.
// The code is stored in RUNTIME_STATE with a "grant." key prefix
// and a 5-minute per-key TTL. The code is single-use — it must be exchanged
// via ExchangeAuthCode and is deleted on successful exchange.
func (c *ChattoCore) CreateAuthCode(ctx context.Context, userID, redirectURI, codeChallenge, codeChallengeMethod string) (string, error) {
	authGeneration, err := c.CurrentAuthGeneration(ctx, userID)
	if err != nil {
		return "", err
	}
	return c.CreateAuthCodeForGeneration(ctx, userID, redirectURI, codeChallenge, codeChallengeMethod, authGeneration)
}

// CreateAuthCodeForGeneration creates an OAuth authorization code for an
// already-authenticated session that proved authGeneration.
func (c *ChattoCore) CreateAuthCodeForGeneration(ctx context.Context, userID, redirectURI, codeChallenge, codeChallengeMethod string, authGeneration uint64) (string, error) {
	return c.CreateAuthCodeForClientGeneration(ctx, userID, "", redirectURI, codeChallenge, codeChallengeMethod, authGeneration)
}

// CreateAuthCodeForClientGeneration creates an OAuth authorization code bound
// to the validated public client and authenticated account generation.
func (c *ChattoCore) CreateAuthCodeForClientGeneration(ctx context.Context, userID, clientID, redirectURI, codeChallenge, codeChallengeMethod string, authGeneration uint64) (string, error) {
	return c.CreateAuthCodeForClientGrantGeneration(ctx, userID, clientID, "", nil, redirectURI, codeChallenge, codeChallengeMethod, authGeneration)
}

// CreateAuthCodeForClientGrantGeneration creates an authorization code bound
// to the validated client, resource, scopes, and account generation.
func (c *ChattoCore) CreateAuthCodeForClientGrantGeneration(ctx context.Context, userID, clientID, resource string, scopes []string, redirectURI, codeChallenge, codeChallengeMethod string, authGeneration uint64) (string, error) {
	if userID == "" {
		return "", ErrAuthCodeNotFound
	}
	if codeChallengeMethod != "S256" {
		return "", ErrAuthCodeInvalidMethod
	}
	if err := c.RequireOAuthClientAllowed(ctx, clientID); err != nil {
		return "", err
	}

	code := NewAuthCode()
	createdAt := time.Now()
	key := c.authCodeKey(code)
	if err := c.RequireAuthenticationAllowed(ctx, userID, authGeneration); err != nil {
		if errors.Is(err, ErrAuthenticationRevoked) {
			return "", ErrAuthCodeNotFound
		}
		return "", err
	}

	data, err := json.Marshal(AuthCodeData{
		UserID:              userID,
		ClientID:            clientID,
		Resource:            resource,
		Scopes:              append([]string(nil), scopes...),
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		CreatedAt:           createdAt,
		AuthGeneration:      authGeneration,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth code: %w", err)
	}

	_, err = c.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(authCodeTTL))
	if err != nil {
		return "", fmt.Errorf("failed to store auth code: %w", err)
	}
	if err := c.recordAuthCodeIssued(ctx, userID, redirectURI, createdAt); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return "", err
	}
	if err := c.RequireOAuthClientAllowed(ctx, clientID); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return "", err
	}

	return code, nil
}

// ExchangeAuthCode validates an authorization code and PKCE code_verifier,
// deletes the code (single-use enforcement), and returns a new bearer token.
//
// Validation checks:
//  1. Code exists and hasn't expired (NATS TTL)
//  2. redirect_uri matches the one used during authorization
//  3. SHA256(code_verifier) == code_challenge (PKCE S256)
func (c *ChattoCore) ExchangeAuthCode(ctx context.Context, code, codeVerifier, redirectURI string) (string, string, error) {
	return c.ExchangeAuthCodeForClient(ctx, code, codeVerifier, redirectURI, "")
}

// ExchangeAuthCodeForClient exchanges a single-use authorization code and
// requires both its exact redirect URI and client identifier to match.
func (c *ChattoCore) ExchangeAuthCodeForClient(ctx context.Context, code, codeVerifier, redirectURI, clientID string) (string, string, error) {
	credentials, userID, err := c.ExchangeAuthCodeForClientSession(ctx, code, codeVerifier, redirectURI, clientID)
	return credentials.AccessToken, userID, err
}

// ExchangeAuthCodeForClientSession exchanges a single-use authorization code
// for a renewable bearer session and requires its exact redirect URI, client
// identifier, and PKCE proof to match.
func (c *ChattoCore) ExchangeAuthCodeForClientSession(ctx context.Context, code, codeVerifier, redirectURI, clientID string) (BearerSessionCredentials, string, error) {
	return c.ExchangeAuthCodeForClientResourceSession(ctx, code, codeVerifier, redirectURI, clientID, "")
}

// ExchangeAuthCodeForClientResourceSession exchanges a code and requires its
// exact OAuth resource identifier to match the token request.
func (c *ChattoCore) ExchangeAuthCodeForClientResourceSession(ctx context.Context, code, codeVerifier, redirectURI, clientID, resource string) (BearerSessionCredentials, string, error) {
	key := c.authCodeKey(code)

	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return BearerSessionCredentials{}, "", ErrAuthCodeNotFound
		}
		return BearerSessionCredentials{}, "", fmt.Errorf("failed to get auth code: %w", err)
	}

	// Atomically claim the code before validation and token issuance. A
	// concurrent exchange that read the same revision must not also succeed.
	if err := c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) || isRuntimeStateRevisionConflict(err) {
			return BearerSessionCredentials{}, "", ErrAuthCodeNotFound
		}
		return BearerSessionCredentials{}, "", fmt.Errorf("failed to consume auth code: %w", err)
	}

	var codeData AuthCodeData
	if err := json.Unmarshal(entry.Value(), &codeData); err != nil {
		return BearerSessionCredentials{}, "", fmt.Errorf("failed to unmarshal auth code: %w", err)
	}
	if codeData.UserID == "" {
		return BearerSessionCredentials{}, "", ErrAuthCodeNotFound
	}

	// Validate redirect_uri matches
	if codeData.RedirectURI != redirectURI {
		if err := c.recordAuthCodeExchangeFailed(ctx, codeData.UserID, codeData.RedirectURI, "redirect_mismatch"); err != nil {
			return BearerSessionCredentials{}, "", err
		}
		return BearerSessionCredentials{}, "", ErrAuthCodeRedirectMismatch
	}
	if codeData.ClientID != clientID {
		if err := c.recordAuthCodeExchangeFailed(ctx, codeData.UserID, codeData.RedirectURI, "client_mismatch"); err != nil {
			return BearerSessionCredentials{}, "", err
		}
		return BearerSessionCredentials{}, "", ErrAuthCodeClientMismatch
	}
	if codeData.Resource != resource {
		if err := c.recordAuthCodeExchangeFailed(ctx, codeData.UserID, codeData.RedirectURI, "resource_mismatch"); err != nil {
			return BearerSessionCredentials{}, "", err
		}
		return BearerSessionCredentials{}, "", ErrAuthCodeClientMismatch
	}

	// Validate PKCE
	if !verifyCodeChallenge(codeData.CodeChallengeMethod, codeVerifier, codeData.CodeChallenge) {
		if err := c.recordAuthCodeExchangeFailed(ctx, codeData.UserID, codeData.RedirectURI, "invalid_verifier"); err != nil {
			return BearerSessionCredentials{}, "", err
		}
		return BearerSessionCredentials{}, "", ErrAuthCodeInvalidVerifier
	}

	validation, err := c.ValidateRuntimeCredential(ctx, RuntimeCredential{
		UserID:         codeData.UserID,
		CreatedAt:      codeData.CreatedAt,
		AuthGeneration: codeData.AuthGeneration,
	})
	if err != nil {
		if !errors.Is(err, ErrAuthenticationRevoked) {
			return BearerSessionCredentials{}, "", err
		}
		if err := c.recordAuthCodeExchangeFailed(ctx, codeData.UserID, codeData.RedirectURI, "auth_revoked"); err != nil {
			return BearerSessionCredentials{}, "", err
		}
		return BearerSessionCredentials{}, "", ErrAuthCodeNotFound
	}
	codeData.AuthGeneration = validation.AuthGeneration

	// Issue a renewable bearer session.
	credentials, err := c.CreateOAuthBearerSessionForClientGrant(ctx, validation.UserID, codeData.ClientID, codeData.Resource, codeData.Scopes, validation.AuthGeneration)
	if err != nil {
		return BearerSessionCredentials{}, "", fmt.Errorf("failed to create bearer session: %w", err)
	}

	if err := c.recordAuthCodeExchangeSucceeded(ctx, codeData.UserID, codeData.RedirectURI); err != nil {
		if revokeErr := c.RevokeRefreshTokenWithReason(ctx, credentials.RefreshToken, "oauth_exchange_audit_failed"); revokeErr != nil {
			return BearerSessionCredentials{}, "", fmt.Errorf("%w; failed to revoke issued bearer session: %v", err, revokeErr)
		}
		return BearerSessionCredentials{}, "", err
	}

	return credentials, validation.UserID, nil
}

// ============================================================================
// PKCE Helpers
// ============================================================================

// verifyCodeChallenge validates that SHA256(codeVerifier) matches the stored codeChallenge.
// Only the S256 method is supported (plain is insecure and discouraged by RFC 7636).
func verifyCodeChallenge(method, codeVerifier, codeChallenge string) bool {
	if method != "S256" {
		return false
	}

	hash := sha256.Sum256([]byte(codeVerifier))
	computed := base64.RawURLEncoding.EncodeToString(hash[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(codeChallenge)) == 1
}

// GenerateCodeChallenge computes the S256 code challenge for a given verifier.
// This is a convenience for tests; clients compute this themselves.
func GenerateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}
