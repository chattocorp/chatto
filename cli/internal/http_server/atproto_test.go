package http_server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/bluesky-social/indigo/atproto/auth"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/connectapi"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/stretchr/testify/require"
)

const testATProtoDID = syntax.DID("did:plc:testatprotouser00000000")

func TestAtprotoServiceDID(t *testing.T) {
	for _, tc := range []struct{ url, want string }{
		{"https://chat.example.com", "did:web:chat.example.com"},
		{"https://Chat.Example.com:443", "did:web:chat.example.com"},
		{"https://chat.example.com:8443", ""},
		{"http://localhost:4000", "did:web:localhost%3A4000"},
		{"http://chatto.ws1.localhost:55280", "did:web:localhost%3A55280"},
		{"http://127.0.0.1", "did:web:localhost"},
		{"http://chat.example.com", ""},
		{"", ""},
	} {
		require.Equal(t, tc.want, config.AtprotoServiceDID(tc.url), tc.url)
	}
}

// atprotoTestSetup serves the sign-in routes with an identity directory that
// knows one DID and its signing key.
type atprotoTestSetup struct {
	baseURL string
	client  *http.Client
	core    *core.ChattoCore
	key     atcrypto.PrivateKey
	aud     string
}

func newATProtoTestSetup(t *testing.T, provider config.AuthProviderConfig) *atprotoTestSetup {
	t.Helper()
	key, err := atcrypto.GeneratePrivateKeyK256()
	require.NoError(t, err)
	pub, err := key.PublicKey()
	require.NoError(t, err)
	dir := identity.NewMockDirectory()
	dir.Insert(identity.Identity{
		DID:    testATProtoDID,
		Handle: syntax.Handle("alice.test"),
		Keys:   map[string]identity.VerificationMethod{"atproto": {Type: "Multikey", PublicKeyMultibase: pub.Multibase()}},
	})
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Auth.Providers = []config.AuthProviderConfig{provider}
		s.atprotoDirectory = dir
		s.setupATProtoRoutes()
	})
	return &atprotoTestSetup{baseURL: ts.URL, client: client, core: chattoCore, key: key, aud: "did:web:localhost%3A4000#chatto"}
}

func (s *atprotoTestSetup) token(t *testing.T, aud, method string, ttl time.Duration) string {
	t.Helper()
	nsid := syntax.NSID(method)
	token, err := auth.SignServiceAuth(testATProtoDID, aud, ttl, &nsid, s.key)
	require.NoError(t, err)
	return token
}

// post sends body to path. Browser requests ask for a cookie session.
func (s *atprotoTestSetup) post(t *testing.T, path string, body map[string]string) (int, map[string]any, *http.Response) {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, s.baseURL+path, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if path == "/auth/browser/atproto" {
		req.Header.Set(connectapi.BrowserAuthenticationModeHeader, connectapi.BrowserAuthenticationModeCookie)
	}
	resp, err := s.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return resp.StatusCode, result, resp
}

func TestATProtoSignInCreatesAccountConfirmationThenSignsIn(t *testing.T) {
	setup := newATProtoTestSetup(t, config.AuthProviderConfig{ID: "atproto", Type: config.AuthProviderTypeATProto, AutoProvision: new(true), ManageUsernames: true})
	token := setup.token(t, setup.aud, atprotoSignInMethod, time.Minute)

	status, body, _ := setup.post(t, "/auth/browser/atproto", map[string]string{"token": token})
	require.Equal(t, http.StatusOK, status, body)
	require.Equal(t, "confirm_account", body["status"])
	pending, err := setup.core.GetPendingExternalIdentityFlow(t.Context(), body["token"].(string))
	require.NoError(t, err)
	require.Equal(t, testATProtoDID.String(), pending.Subject)
	require.Equal(t, "alice.test", pending.ProviderUsername)

	// A token works once, on every replica.
	status, body, _ = setup.post(t, "/auth/browser/atproto", map[string]string{"token": token})
	require.Equal(t, http.StatusUnauthorized, status)
	require.Equal(t, "invalid_token", body["error"])

	user, err := setup.core.CreateUserForExternalIdentity(t.Context(), "alice.test", "Alice", pending)
	require.NoError(t, err)
	require.Equal(t, testATProtoDID.String(), setup.core.AtprotoDIDForUser(user.Id), "the linked DID is published")

	status, body, resp := setup.post(t, "/auth/browser/atproto", map[string]string{"token": setup.token(t, setup.aud, atprotoSignInMethod, time.Minute)})
	require.Equal(t, http.StatusOK, status, body)
	require.Equal(t, "signed_in", body["status"])
	require.NotEmpty(t, resp.Cookies(), "the browser route creates a cookie session")
	require.Nil(t, body["token"], "the browser route returns no bearer token")

	status, body, _ = setup.post(t, "/auth/atproto", map[string]string{"token": setup.token(t, setup.aud, atprotoSignInMethod, time.Minute)})
	require.Equal(t, http.StatusOK, status, body)
	require.NotEmpty(t, body["token"], "the programmatic route returns a bearer session")
	require.NotEmpty(t, body["refreshToken"])
}

func TestATProtoSignInRejectsInvalidTokens(t *testing.T) {
	setup := newATProtoTestSetup(t, config.AuthProviderConfig{ID: "atproto", Type: config.AuthProviderTypeATProto, AutoProvision: new(true)})
	for name, token := range map[string]string{
		"other server": setup.token(t, "did:web:other.example#chatto", atprotoSignInMethod, time.Minute),
		"other method": setup.token(t, setup.aud, "com.example.other", time.Minute),
		"long-lived":   setup.token(t, setup.aud, atprotoSignInMethod, time.Hour),
		"expired":      setup.token(t, setup.aud, atprotoSignInMethod, -time.Minute),
		"garbage":      "not-a-token",
	} {
		status, body, _ := setup.post(t, "/auth/browser/atproto", map[string]string{"token": token})
		require.Equal(t, http.StatusUnauthorized, status, name)
		require.Equal(t, "invalid_token", body["error"], name)
	}
}

func TestATProtoSignInWithoutAutoProvisionOrProgrammatic(t *testing.T) {
	setup := newATProtoTestSetup(t, config.AuthProviderConfig{ID: "atproto", Type: config.AuthProviderTypeATProto})
	status, body, _ := setup.post(t, "/auth/browser/atproto", map[string]string{"token": setup.token(t, setup.aud, atprotoSignInMethod, time.Minute)})
	require.Equal(t, http.StatusForbidden, status)
	require.Equal(t, "external_identity_unlinked", body["error"])
}

func TestATProtoSignInLinksWithLinkToken(t *testing.T) {
	setup := newATProtoTestSetup(t, config.AuthProviderConfig{ID: "atproto", Type: config.AuthProviderTypeATProto})
	user, err := setup.core.CreateUser(t.Context(), core.SystemActorID, "linker", "Linker", "password123")
	require.NoError(t, err)
	linkToken, err := setup.core.CreatePendingExternalIdentityLinkStart(t.Context(), "atproto", "/chat", user.Id)
	require.NoError(t, err)

	status, body, _ := setup.post(t, "/auth/atproto", map[string]string{"token": setup.token(t, setup.aud, atprotoSignInMethod, time.Minute), "link_token": linkToken})
	require.Equal(t, http.StatusOK, status, body)
	require.Equal(t, "linked", body["status"])
	require.Equal(t, testATProtoDID.String(), setup.core.AtprotoDIDForUser(user.Id))

	// The link token is single-use.
	status, _, _ = setup.post(t, "/auth/atproto", map[string]string{"token": setup.token(t, setup.aud, atprotoSignInMethod, time.Minute), "link_token": linkToken})
	require.Equal(t, http.StatusForbidden, status)
}
