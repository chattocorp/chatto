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
	RedirectURI         string    `json:"redirect_uri"`
	CodeChallenge       string    `json:"code_challenge"`
	CodeChallengeMethod string    `json:"code_challenge_method"`
	CreatedAt           time.Time `json:"created_at"`
}

// ============================================================================
// Auth Code Operations
// ============================================================================

// CreateAuthCode generates a new OAuth authorization code for the given user.
// The code is stored in RUNTIME_STATE with a "grant." key prefix
// and a 5-minute per-key TTL. The code is single-use — it must be exchanged
// via ExchangeAuthCode and is deleted on successful exchange.
func (c *ChattoCore) CreateAuthCode(ctx context.Context, userID, redirectURI, codeChallenge, codeChallengeMethod string) (string, error) {
	if codeChallengeMethod != "S256" {
		return "", ErrAuthCodeInvalidMethod
	}

	code := NewAuthCode()

	data, err := json.Marshal(AuthCodeData{
		UserID:              userID,
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		CreatedAt:           time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth code: %w", err)
	}

	_, err = c.storage.runtimeStateKV.Create(ctx, c.authCodeKey(code), data, jetstream.KeyTTL(authCodeTTL))
	if err != nil {
		return "", fmt.Errorf("failed to store auth code: %w", err)
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
	key := c.authCodeKey(code)

	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return "", "", ErrAuthCodeNotFound
		}
		return "", "", fmt.Errorf("failed to get auth code: %w", err)
	}

	// Delete immediately (single-use). Even if subsequent validation fails,
	// the code should not be reusable.
	_ = c.storage.runtimeStateKV.Delete(ctx, key)

	var codeData AuthCodeData
	if err := json.Unmarshal(entry.Value(), &codeData); err != nil {
		return "", "", fmt.Errorf("failed to unmarshal auth code: %w", err)
	}

	// Validate redirect_uri matches
	if codeData.RedirectURI != redirectURI {
		return "", "", ErrAuthCodeRedirectMismatch
	}

	// Validate PKCE
	if !verifyCodeChallenge(codeData.CodeChallengeMethod, codeVerifier, codeData.CodeChallenge) {
		return "", "", ErrAuthCodeInvalidVerifier
	}

	// Issue a bearer token
	token, err := c.CreateAuthToken(ctx, codeData.UserID)
	if err != nil {
		return "", "", fmt.Errorf("failed to create bearer token: %w", err)
	}

	return token, codeData.UserID, nil
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
