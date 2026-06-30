package core

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestChattoCore_CreateAuthCode(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)

	code, err := core.CreateAuthCode(ctx, user.Id, "https://example.com/callback", challenge, "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode failed: %v", err)
	}

	if !strings.HasPrefix(code, "cht_AC") {
		t.Errorf("Code %q does not start with 'cht_AC'", code)
	}

	// cht_ (4) + AC (2) + nanoid (14) = 20 chars
	if len(code) != 20 {
		t.Errorf("Code length is %d, want 20", len(code))
	}

	key := core.authCodeKey(code)
	if _, err := core.storage.runtimeStateKV.Get(ctx, key); err != nil {
		t.Fatalf("expected auth code in RUNTIME_STATE: %v", err)
	}
	assertRuntimeKVHasTTL(t, core, key)
	assertRawRuntimeTokenKeyAbsent(t, core, authCodeKeyPrefix+code)
}

func TestChattoCore_ExchangeAuthCode_HappyPath(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)
	redirectURI := "https://example.com/callback"

	code, err := core.CreateAuthCode(ctx, user.Id, redirectURI, challenge, "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode failed: %v", err)
	}

	token, userID, err := core.ExchangeAuthCode(ctx, code, verifier, redirectURI)
	if err != nil {
		t.Fatalf("ExchangeAuthCode failed: %v", err)
	}

	if userID != user.Id {
		t.Errorf("ExchangeAuthCode returned userID %q, want %q", userID, user.Id)
	}

	if !strings.HasPrefix(token, "cht_AT") {
		t.Errorf("Token %q does not start with 'cht_AT'", token)
	}

	// Verify the bearer token is valid
	validatedUserID, err := core.ValidateAuthToken(ctx, token)
	if err != nil {
		t.Fatalf("ValidateAuthToken on exchanged token failed: %v", err)
	}
	if validatedUserID != user.Id {
		t.Errorf("ValidateAuthToken returned userID %q, want %q", validatedUserID, user.Id)
	}
	if data := readAuthTokenData(t, core, token); data.Kind != AuthTokenKindOAuthAccessToken {
		t.Fatalf("exchanged auth token kind = %q, want %q", data.Kind, AuthTokenKindOAuthAccessToken)
	}
	if err := core.RequireFreshAuthForBearerToken(ctx, token); !errors.Is(err, ErrFreshAuthRequired) {
		t.Fatalf("exchanged token fresh auth err = %v, want ErrFreshAuthRequired", err)
	}
}

func TestChattoCore_ExchangeAuthCodeRejectsStaleAuthGeneration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "auth-code-generation-user", "Auth Code Generation User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)
	redirectURI := "https://example.com/callback"
	code, err := core.CreateAuthCode(ctx, user.Id, redirectURI, challenge, "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode: %v", err)
	}
	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	token, userID, err := core.ExchangeAuthCode(ctx, code, verifier, redirectURI)
	if !errors.Is(err, ErrAuthCodeNotFound) {
		t.Fatalf("ExchangeAuthCode err = %v, want ErrAuthCodeNotFound", err)
	}
	if token != "" || userID != "" {
		t.Fatalf("ExchangeAuthCode returned token=%q userID=%q, want empty", token, userID)
	}
}

func TestChattoCore_ExchangeAuthCodeGrandfathersLegacyGeneration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "auth-code-legacy-user", "Auth Code Legacy User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)
	redirectURI := "https://example.com/callback"
	code := NewAuthCode()
	data, err := json.Marshal(AuthCodeData{
		UserID:              user.Id,
		RedirectURI:         redirectURI,
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
		CreatedAt:           time.Now(),
	})
	if err != nil {
		t.Fatalf("marshal legacy auth code: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, core.authCodeKey(code), data, jetstream.KeyTTL(authCodeTTL)); err != nil {
		t.Fatalf("store legacy auth code: %v", err)
	}

	token, userID, err := core.ExchangeAuthCode(ctx, code, verifier, redirectURI)
	if err != nil {
		t.Fatalf("ExchangeAuthCode: %v", err)
	}
	if userID != user.Id {
		t.Fatalf("ExchangeAuthCode user ID = %q, want %q", userID, user.Id)
	}
	if gotUserID, err := core.ValidateAuthToken(ctx, token); err != nil {
		t.Fatalf("exchanged token should validate: %v", err)
	} else if gotUserID != user.Id {
		t.Fatalf("exchanged token user ID = %q, want %q", gotUserID, user.Id)
	}
}

func TestChattoCore_ExchangeAuthCodeRejectsLegacyGenerationBeforePasswordChange(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "auth-code-legacy-stale-user", "Auth Code Legacy Stale User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)
	redirectURI := "https://example.com/callback"
	legacyCreatedAt := time.Now()
	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	code := NewAuthCode()
	data, err := json.Marshal(AuthCodeData{
		UserID:              user.Id,
		RedirectURI:         redirectURI,
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
		CreatedAt:           legacyCreatedAt,
	})
	if err != nil {
		t.Fatalf("marshal legacy auth code: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, core.authCodeKey(code), data, jetstream.KeyTTL(authCodeTTL)); err != nil {
		t.Fatalf("store legacy auth code: %v", err)
	}

	token, userID, err := core.ExchangeAuthCode(ctx, code, verifier, redirectURI)
	if !errors.Is(err, ErrAuthCodeNotFound) {
		t.Fatalf("ExchangeAuthCode err = %v, want ErrAuthCodeNotFound", err)
	}
	if token != "" || userID != "" {
		t.Fatalf("ExchangeAuthCode returned token=%q userID=%q, want empty", token, userID)
	}
}

func TestChattoCore_CreateAuthCodeForGenerationRejectsStaleAuthGeneration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "auth-code-issue-gen-user", "Auth Code Issue User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	authGeneration, err := core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}

	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)
	redirectURI := "https://example.com/callback"
	if code, err := core.CreateAuthCodeForGeneration(ctx, user.Id, redirectURI, challenge, "S256", authGeneration); !errors.Is(err, ErrAuthCodeNotFound) {
		t.Fatalf("CreateAuthCodeForGeneration err = %v, want ErrAuthCodeNotFound", err)
	} else if code != "" {
		t.Fatalf("CreateAuthCodeForGeneration returned code=%q, want empty", code)
	}
}

func TestChattoCore_ExchangeAuthCode_SingleUse(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)
	redirectURI := "https://example.com/callback"

	code, err := core.CreateAuthCode(ctx, user.Id, redirectURI, challenge, "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode failed: %v", err)
	}

	// First exchange succeeds
	_, _, err = core.ExchangeAuthCode(ctx, code, verifier, redirectURI)
	if err != nil {
		t.Fatalf("First ExchangeAuthCode failed: %v", err)
	}

	// Second exchange fails — code was deleted
	_, _, err = core.ExchangeAuthCode(ctx, code, verifier, redirectURI)
	if err != ErrAuthCodeNotFound {
		t.Errorf("Second ExchangeAuthCode returned error %v, want ErrAuthCodeNotFound", err)
	}
}

func TestChattoCore_ExchangeAuthCode_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, _, err := core.ExchangeAuthCode(ctx, "cht_ACnonexistent1234", "verifier", "https://example.com/callback")
	if err != ErrAuthCodeNotFound {
		t.Errorf("ExchangeAuthCode returned error %v, want ErrAuthCodeNotFound", err)
	}
}

func TestChattoCore_ExchangeAuthCode_InvalidVerifier(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)
	redirectURI := "https://example.com/callback"

	code, err := core.CreateAuthCode(ctx, user.Id, redirectURI, challenge, "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode failed: %v", err)
	}

	// Exchange with wrong verifier
	_, _, err = core.ExchangeAuthCode(ctx, code, "wrong-verifier", redirectURI)
	if err != ErrAuthCodeInvalidVerifier {
		t.Errorf("ExchangeAuthCode with wrong verifier returned error %v, want ErrAuthCodeInvalidVerifier", err)
	}

	// Code should be consumed even though verification failed (prevents brute-force)
	_, _, err = core.ExchangeAuthCode(ctx, code, verifier, redirectURI)
	if err != ErrAuthCodeNotFound {
		t.Errorf("ExchangeAuthCode after failed attempt returned error %v, want ErrAuthCodeNotFound", err)
	}
}

func TestChattoCore_ExchangeAuthCode_RedirectMismatch(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)

	code, err := core.CreateAuthCode(ctx, user.Id, "https://example.com/callback", challenge, "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode failed: %v", err)
	}

	// Exchange with different redirect URI
	_, _, err = core.ExchangeAuthCode(ctx, code, verifier, "https://evil.com/callback")
	if err != ErrAuthCodeRedirectMismatch {
		t.Errorf("ExchangeAuthCode with wrong redirect returned error %v, want ErrAuthCodeRedirectMismatch", err)
	}
}

func TestChattoCore_CreateAuthCode_RejectsPlainMethod(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	_, err = core.CreateAuthCode(ctx, user.Id, "https://example.com/callback", "challenge", "plain")
	if err != ErrAuthCodeInvalidMethod {
		t.Errorf("CreateAuthCode with plain method returned error %v, want ErrAuthCodeInvalidMethod", err)
	}
}

func TestGenerateCodeChallenge(t *testing.T) {
	// Test vector from RFC 7636 Appendix B
	// verifier: dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk
	// expected S256 challenge: E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	expected := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	challenge := GenerateCodeChallenge(verifier)
	if challenge != expected {
		t.Errorf("GenerateCodeChallenge(%q) = %q, want %q", verifier, challenge, expected)
	}
}

func TestVerifyCodeChallenge(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GenerateCodeChallenge(verifier)

	tests := []struct {
		name     string
		method   string
		verifier string
		want     bool
	}{
		{"valid S256", "S256", verifier, true},
		{"wrong verifier", "S256", "wrong-verifier", false},
		{"unsupported method", "plain", verifier, false},
		{"empty method", "", verifier, false},
		{"empty verifier", "S256", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyCodeChallenge(tt.method, tt.verifier, challenge)
			if got != tt.want {
				t.Errorf("verifyCodeChallenge(%q, %q, challenge) = %v, want %v", tt.method, tt.verifier, got, tt.want)
			}
		})
	}
}
