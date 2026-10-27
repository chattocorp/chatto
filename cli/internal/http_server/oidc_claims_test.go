package http_server

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/markbates/goth/providers/discord"
	"golang.org/x/oauth2"
)

func TestOIDCTokenAuthenticationSelection(t *testing.T) {
	for _, tc := range []struct {
		name, override, secret, metadata, want string
	}{
		{"public", "", "", `["client_secret_basic"]`, "none"},
		{"public explicit", "none", "", `[]`, "none"},
		{"missing metadata", "", "secret", "", "client_secret_basic"},
		{"prefer basic", "", "secret", `["client_secret_post","client_secret_basic"]`, "client_secret_basic"},
		{"post only", "", "secret", `["client_secret_post"]`, "client_secret_post"},
		{"override post", "client_secret_post", "secret", `["client_secret_basic"]`, "client_secret_post"},
		{"override basic", "client_secret_basic", "secret", `[]`, "client_secret_basic"},
		{"empty", "", "secret", `[]`, ""},
		{"null", "", "secret", `null`, ""},
		{"unsupported", "", "secret", `["private_key_jwt"]`, ""},
		{"malformed", "", "secret", `123`, ""},
		{"none with secret", "none", "secret", "", ""},
		{"basic without secret", "client_secret_basic", "", "", ""},
		{"post without secret", "client_secret_post", "", "", ""},
		{"unknown override", "unknown", "secret", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			method, style, err := selectOIDCTokenAuth(tc.override, tc.secret, json.RawMessage(tc.metadata))
			if (err != nil) != (tc.want == "") || method != tc.want {
				t.Fatalf("method=%q error=%v; want %q", method, err, tc.want)
			}
			if err == nil && (style == oauth2.AuthStyleAutoDetect || (method == "client_secret_basic") != (style == oauth2.AuthStyleInHeader)) {
				t.Fatalf("incorrect auth style %v for %s", style, method)
			}
		})
	}
}

func TestOIDCUnsupportedKeysDoNotHideSigningKey(t *testing.T) {
	for _, tc := range []struct {
		name      string
		keys      []any
		omit      bool
		wantError bool
	}{
		{"mixed", []any{map[string]any{"kty": "EC", "crv": "future-curve"}, map[string]any{"kty": "future-key"}, map[string]any{"kty": "OKP", "crv": "Ed448"}}, false, false},
		{"no supported signing key", []any{map[string]any{"kty": "EC", "crv": "future-curve"}}, true, true},
		{"malformed supported key", []any{map[string]any{"kty": "RSA", "n": "invalid!", "e": "AQAB"}}, false, true},
		{"oversized key set", []any{map[string]any{"kty": "future-key", "padding": strings.Repeat("x", 1<<20)}}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issuer := newNoEmailOIDCIssuer(t, "client-id")
			defer issuer.Close()
			issuer.extraKeys, issuer.omitSigningKey = tc.keys, tc.omit
			_, err := resolveTestOIDC(t, issuer, "none", "", false)
			if (err != nil) != tc.wantError {
				t.Fatalf("verification error = %v, want error %v", err, tc.wantError)
			}
		})
	}
}

func TestOIDCSingleExchangeWithPKCE(t *testing.T) {
	for _, method := range []string{"none", "client_secret_basic", "client_secret_post"} {
		for _, fail := range []bool{false, true} {
			t.Run(method+map[bool]string{false: "/success", true: "/failure"}[fail], func(t *testing.T) {
				issuer := newNoEmailOIDCIssuer(t, "client-id")
				defer issuer.Close()
				secret := "secret"
				if method == "none" {
					secret = ""
				}
				issuer.tokenCheck = func(r *http.Request) bool {
					if err := r.ParseForm(); err != nil {
						t.Error(err)
						return false
					}
					if r.Form.Get("code_verifier") != "test-verifier" || r.Form.Get("code") != "single-use-code" {
						t.Error("missing code or PKCE")
					}
					id, password, basic := r.BasicAuth()
					if method == "client_secret_basic" {
						if !basic || id != "client-id" || password != secret || r.PostForm.Has("client_secret") {
							t.Error("incorrect Basic credentials")
						}
					} else if basic || r.PostForm.Get("client_id") != "client-id" || r.PostForm.Get("client_secret") != secret {
						t.Error("incorrect body credentials")
					}
					return !fail
				}
				override := method
				if !fail {
					// Exercise discovery through the HTTP boundary on success,
					// and explicit overrides on rejected single-use exchanges.
					issuer.methods = json.RawMessage(`["` + method + `"]`)
					override = ""
				}
				_, err := resolveTestOIDC(t, issuer, override, secret, false)
				if (err != nil) != fail {
					t.Fatalf("exchange error = %v", err)
				}
				if issuer.tokenRequests != 1 {
					t.Fatalf("code submitted %d times", issuer.tokenRequests)
				}
			})
		}
	}
}

func TestOIDCExchangeFailureLogsOnlySafeMetadata(t *testing.T) {
	var output bytes.Buffer
	previous := log.Default()
	log.SetDefault(log.New(&output))
	t.Cleanup(func() { log.SetDefault(previous) })
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()
	issuer.tokenCheck = func(*http.Request) bool { return false }
	if _, err := resolveTestOIDC(t, issuer, "client_secret_post", "secret", false); err == nil {
		t.Fatal("expected exchange failure")
	}
	logged := output.String()
	for _, required := range []string{"provider_id=test", "auth_method=client_secret_post", "http_status=401", "oauth_error=invalid_client"} {
		if !strings.Contains(logged, required) {
			t.Errorf("missing safe field %s", required)
		}
	}
	for _, private := range []string{"private provider response", "single-use-code", "test-verifier"} {
		if strings.Contains(logged, private) {
			t.Fatal("private response or credential was logged")
		}
	}
	if safeOAuthError("private-user@example.com") != "unknown" {
		t.Fatal("provider-controlled error accepted")
	}
}

func TestOIDCUserInfoMergesMissingClaims(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		id, info              map[string]any
		login, display, email string
		fail                  bool
	}{
		{"names with existing email", map[string]any{"email": "mailbox@example.com", "email_verified": true}, map[string]any{"preferred_username": "handle", "name": "Full Name", "email": "other@example.com", "email_verified": true}, "handle", "Full Name", "mailbox@example.com", false},
		{"preserve populated name", map[string]any{"name": "Original", "email": "mailbox@example.com", "email_verified": true}, map[string]any{"preferred_username": "handle", "name": "Replacement"}, "handle", "Original", "mailbox@example.com", false},
		{"verification cannot cross emails", map[string]any{"email": "unverified@example.com"}, map[string]any{"preferred_username": "handle", "name": "Full Name", "email": "other@example.com", "email_verified": true}, "handle", "Full Name", "", false},
		{"email pair", map[string]any{"preferred_username": "handle", "name": "Full Name"}, map[string]any{"email": "mailbox@example.com", "email_verified": true}, "handle", "Full Name", "mailbox@example.com", false},
		{"no inherited verification", map[string]any{"email_verified": true}, map[string]any{"preferred_username": "handle", "name": "Full Name", "email": "unverified@example.com"}, "handle", "Full Name", "", false},
		{"malformed leaves ID token intact", map[string]any{"email": "mailbox@example.com", "email_verified": true}, map[string]any{"preferred_username": "partial", "name": 123}, "mailbox", "mailbox", "mailbox@example.com", false},
		{"mismatched subject", map[string]any{"email": "mailbox@example.com", "email_verified": true}, map[string]any{"sub": "other", "preferred_username": "attacker"}, "", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issuer := newNoEmailOIDCIssuer(t, "client-id")
			defer issuer.Close()
			issuer.tokenClaims, issuer.userInfoClaims = tc.id, tc.info
			if _, ok := tc.info["sub"]; !ok {
				tc.info["sub"] = issuer.subject
			}
			identity, err := resolveTestOIDC(t, issuer, "none", "", true)
			if (err != nil) != tc.fail {
				t.Fatalf("resolve error = %v", err)
			}
			if !tc.fail && (identity.loginHint != tc.login || identity.displayNameHint != tc.display || identity.verifiedEmail != tc.email) {
				t.Fatalf("unexpected merged claims: %+v", identity)
			}
			if issuer.UserInfoRequests() != 1 {
				t.Fatal("missing UserInfo request")
			}
		})
	}
}

func TestOIDCManagedUsernameClaim(t *testing.T) {
	for _, tc := range []struct {
		name     string
		id, info map[string]any
		want     string
	}{
		{"token", map[string]any{"handle": "TokenName"}, map[string]any{"handle": "Other"}, "TokenName"},
		{"userinfo", map[string]any{}, map[string]any{"handle": "InfoName"}, "InfoName"},
		{"no email fallback", map[string]any{"email": "person@example.com", "name": "Person"}, map[string]any{}, ""},
		{"invalid type", map[string]any{"handle": 12}, map[string]any{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issuer := newNoEmailOIDCIssuer(t, "client-id")
			defer issuer.Close()
			issuer.tokenClaims = tc.id
			tc.info["sub"] = issuer.subject
			issuer.userInfoClaims = tc.info
			runtime := newTestOIDCRuntime(t, issuer, "none", "", false)
			runtime.config.ManageUsernames = true
			runtime.config.UsernameClaim = "handle"
			identity, err := resolveTestOIDCRuntime(t, runtime, "test-nonce")
			if err != nil {
				t.Fatal(err)
			}
			if identity.providerUsername != tc.want {
				t.Fatal("wrong managed username claim")
			}
		})
	}
}

func TestOIDCNonceBindsIDTokenToAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, sessionNonce, tokenNonce string
		wantErr                        bool
	}{
		{"matching", "test-nonce", "test-nonce", false},
		{"different", "test-nonce", "other-nonce", true},
		{"absent from ID token", "test-nonce", "", true},
		{"absent from session", "", "test-nonce", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issuer := newNoEmailOIDCIssuer(t, "client-id")
			defer issuer.Close()
			issuer.nonce = tc.tokenNonce
			_, err := resolveTestOIDCRuntime(t, newTestOIDCRuntime(t, issuer, "none", "", false), tc.sessionNonce)
			if (err != nil) != tc.wantErr {
				t.Fatalf("resolve error = %v, want error %v", err, tc.wantErr)
			}
			// Without a stored nonce, the single-use code stays unused.
			if tc.sessionNonce == "" && issuer.tokenRequests != 0 {
				t.Fatal("code exchanged without a stored nonce")
			}
		})
	}
}

func TestOIDCProviderStartSendsFreshNonce(t *testing.T) {
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()
	ts, client, _ := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://chat.example"
		s.config.Auth.Providers = []config.AuthProviderConfig{{ID: "test", Type: config.AuthProviderTypeOpenIDConnect, IssuerURL: issuer.URL(), ClientID: "client-id"}}
		s.setupOIDCRoutes()
	})
	nonces := map[string]bool{}
	for range 2 {
		location := startNoEmailOIDC(t, client, ts.URL, "test", nil)
		authURL, _ := url.Parse(location)
		nonces[authURL.Query().Get("nonce")] = true
	}
	if len(nonces) != 2 {
		t.Fatal("provider starts reused a nonce")
	}
}

// Each provider endpoint in turn accepts the request and never answers.
// Only an unavailable UserInfo endpoint still permits sign-in.
func TestOIDCProviderRequestsTimeOut(t *testing.T) {
	for _, tc := range []struct {
		path    string
		wantErr bool
	}{
		{"/token", true},
		{"/keys", true},
		{"/userinfo", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			issuer := newNoEmailOIDCIssuer(t, "client-id")
			defer issuer.Close()
			issuer.blockPath = tc.path
			runtime := newTestOIDCRuntime(t, issuer, "none", "", false)
			// Long enough for the other requests on a loaded test machine.
			runtime.httpClient.Timeout = 500 * time.Millisecond
			// The ID token lacks names, so Chatto asks UserInfo.
			issuer.tokenClaims = map[string]any{}
			started := time.Now()
			_, err := resolveTestOIDCRuntime(t, runtime, "test-nonce")
			if (err != nil) != tc.wantErr {
				t.Fatalf("resolve error = %v, want error %v", err, tc.wantErr)
			}
			if issuer.blockedRequests.Load() != 1 {
				t.Fatalf("blocked endpoint requests = %d, want 1", issuer.blockedRequests.Load())
			}
			if elapsed := time.Since(started); elapsed > 5*time.Second {
				t.Fatalf("resolve took %s", elapsed)
			}
		})
	}
}

func TestOIDCDiscoveryTimeoutIsSharedByConcurrentSignIns(t *testing.T) {
	issuer := newNoEmailOIDCIssuer(t, "client-id")
	defer issuer.Close()
	issuer.blockPath = "/.well-known/openid-configuration"
	runtime := newTestOIDCRuntime(t, issuer, "none", "", false)
	// Long enough for all callers to join the first attempt.
	runtime.httpClient.Timeout = time.Second
	started := time.Now()
	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for range 5 {
		wg.Go(func() {
			_, err := runtime.oidc.init(issuer.URL(), "client-id", "", runtime.callbackURL, []string{"openid"}, "")
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err == nil {
			t.Fatal("discovery succeeded against an unanswered issuer")
		}
	}
	// Serial attempts would take at least one timeout per waiting sign-in.
	if elapsed := time.Since(started); elapsed > 3*runtime.httpClient.Timeout {
		t.Fatalf("concurrent discovery took %s", elapsed)
	}
	if issuer.blockedRequests.Load() != 1 {
		t.Fatalf("discovery requests = %d, want one shared attempt", issuer.blockedRequests.Load())
	}
	if runtime.oidc.discovery.Load() != nil {
		t.Fatal("failed discovery marked the provider ready")
	}
}

func TestOAuthProvidersUseBoundedHTTPClient(t *testing.T) {
	for _, providerType := range []string{config.AuthProviderTypeGitHub, config.AuthProviderTypeGitLab, config.AuthProviderTypeGoogle, config.AuthProviderTypeDiscord} {
		runtime, err := newAuthProviderRuntime(config.AuthProviderConfig{ID: providerType, Type: providerType, ClientID: "id", ClientSecret: "secret"}, "http://chat.example/callback")
		if err != nil {
			t.Fatal(err)
		}
		client := runtime.goth.(interface{ Client() *http.Client }).Client()
		if client != runtime.httpClient || client.Timeout != providerHTTPTimeout {
			t.Fatalf("%s provider uses an unbounded HTTP client", providerType)
		}
	}
}

func TestDiscordCodeExchangeTimesOut(t *testing.T) {
	release := make(chan struct{})
	tokenServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer tokenServer.Close()
	defer close(release)
	runtime, err := newAuthProviderRuntime(config.AuthProviderConfig{ID: "discord", Type: config.AuthProviderTypeDiscord, ClientID: "id", ClientSecret: "secret"}, "http://chat.example/callback")
	if err != nil {
		t.Fatal(err)
	}
	runtime.discordOAuth2.Endpoint.TokenURL = tokenServer.URL
	runtime.httpClient.Timeout = 100 * time.Millisecond
	session, err := runtime.goth.UnmarshalSession(`{"AuthURL":"https://discord.com/api/oauth2/authorize"}`)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := runtime.authorizeGothSession(t.Context(), session, url.Values{"code": {"code"}}); err == nil {
		t.Fatal("exchange succeeded against an unanswered token endpoint")
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("exchange took %s", elapsed)
	}
}

func TestDiscordCodeExchangeAuthorizesSession(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("code") != "code" || r.Form.Get("redirect_uri") != "http://chat.example/callback" {
			t.Error("unexpected token request")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()
	runtime, err := newAuthProviderRuntime(config.AuthProviderConfig{ID: "discord", Type: config.AuthProviderTypeDiscord, ClientID: "id", ClientSecret: "secret"}, "http://chat.example/callback")
	if err != nil {
		t.Fatal(err)
	}
	runtime.discordOAuth2.Endpoint.TokenURL = tokenServer.URL
	session, err := runtime.goth.UnmarshalSession(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.authorizeGothSession(t.Context(), session, url.Values{"code": {"code"}}); err != nil {
		t.Fatal(err)
	}
	if discordSession := session.(*discord.Session); discordSession.AccessToken != "access" || discordSession.RefreshToken != "refresh" || discordSession.ExpiresAt.IsZero() {
		t.Fatalf("session not authorized: %+v", discordSession)
	}
}

func TestOIDCGroupsClaim(t *testing.T) {
	names := map[string]any{"name": "Full Name", "preferred_username": "handle"}
	withNames := func(extra map[string]any) map[string]any {
		claims := maps.Clone(names)
		maps.Copy(claims, extra)
		return claims
	}
	for _, tc := range []struct {
		name         string
		claim        string
		id, info     map[string]any
		want         []string
		userInfoUsed bool
	}{
		{"list in ID token", "", withNames(map[string]any{"groups": []any{"chat-users", 7, "admins"}}), nil, []string{"chat-users", "admins"}, false},
		{"string in ID token", "", withNames(map[string]any{"groups": "chat-users"}), nil, []string{"chat-users"}, false},
		{"custom claim", "roles", withNames(map[string]any{"groups": []any{"ignored"}, "roles": []any{"chat-users"}}), nil, []string{"chat-users"}, false},
		{"ID token wins", "", withNames(map[string]any{"groups": []any{"from-token"}}), map[string]any{"groups": []any{"from-userinfo"}}, []string{"from-token"}, false},
		{"UserInfo fallback", "", withNames(nil), map[string]any{"groups": []any{"chat-users"}}, []string{"chat-users"}, true},
		{"other shape", "", withNames(map[string]any{"groups": 7}), map[string]any{"groups": map[string]any{"a": 1}}, nil, true},
		{"absent", "", withNames(nil), map[string]any{}, nil, true},
	} {
		// Both group features read the claim, also on their own.
		for _, feature := range []string{"allowed_groups", "group_roles"} {
			t.Run(tc.name+"/"+feature, func(t *testing.T) {
				issuer := newNoEmailOIDCIssuer(t, "client-id")
				defer issuer.Close()
				issuer.tokenClaims = tc.id
				if tc.info != nil {
					info := maps.Clone(tc.info)
					info["sub"] = issuer.subject
					issuer.userInfoClaims = info
				}
				runtime := newTestOIDCRuntime(t, issuer, "none", "", false)
				runtime.config.GroupsClaim = tc.claim
				if feature == "allowed_groups" {
					runtime.config.AllowedGroups = []string{"chat-users"}
				} else {
					runtime.config.GroupRoles = "chat-users:moderator"
				}
				identity, err := resolveTestOIDCRuntime(t, runtime, "test-nonce")
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(identity.groups.Groups, tc.want) || identity.groups.Known != (tc.want != nil) {
					t.Fatalf("groups = %+v, want %v", identity.groups, tc.want)
				}
				if (issuer.UserInfoRequests() == 1) != tc.userInfoUsed {
					t.Fatalf("UserInfo requests = %d", issuer.UserInfoRequests())
				}
			})
		}
	}
}

func resolveTestOIDC(t *testing.T, issuer *noEmailOIDCIssuer, method, secret string, email bool) (resolvedProviderIdentity, error) {
	t.Helper()
	return resolveTestOIDCRuntime(t, newTestOIDCRuntime(t, issuer, method, secret, email), "test-nonce")
}

func newTestOIDCRuntime(t *testing.T, issuer *noEmailOIDCIssuer, method, secret string, email bool) *authProviderRuntime {
	t.Helper()
	runtime, err := newAuthProviderRuntime(config.AuthProviderConfig{ID: "test", Type: "oidc", IssuerURL: issuer.URL(), ClientID: "client-id", ClientSecret: secret, TokenEndpointAuthMethod: method, RequestEmail: &email}, "http://client.example/callback")
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

// resolveTestOIDCRuntime resolves a callback whose browser session stores
// sessionNonce. An empty value stores no nonce.
func resolveTestOIDCRuntime(t *testing.T, runtime *authProviderRuntime, sessionNonce string) (resolvedProviderIdentity, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var identity resolvedProviderIdentity
	var resolveErr error
	router := gin.New()
	router.Use(sessions.Sessions("test", cookie.NewStore([]byte("test-session-key"))))
	router.GET("/callback", func(c *gin.Context) {
		if !runtime.ensureOIDC(c) {
			t.Error("provider initialization failed")
			return
		}
		session := sessions.Default(c)
		session.Set(providerSessionKey("test", "code_verifier"), "test-verifier")
		if sessionNonce != "" {
			session.Set(providerSessionKey("test", "nonce"), sessionNonce)
		}
		identity, resolveErr = runtime.resolveOIDCIdentity(c, session)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/callback?code=single-use-code", nil))
	return identity, resolveErr
}
