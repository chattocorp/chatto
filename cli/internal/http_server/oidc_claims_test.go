package http_server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"hmans.de/chatto/internal/config"
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

func resolveTestOIDC(t *testing.T, issuer *noEmailOIDCIssuer, method, secret string, email bool) (resolvedProviderIdentity, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	runtime, err := newAuthProviderRuntime(config.AuthProviderConfig{ID: "test", Type: "oidc", IssuerURL: issuer.URL(), ClientID: "client-id", ClientSecret: secret, TokenEndpointAuthMethod: method, RequestEmail: &email}, "http://client.example/callback")
	if err != nil {
		t.Fatal(err)
	}
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
		identity, resolveErr = runtime.resolveOIDCIdentity(c, session)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/callback?code=single-use-code", nil))
	return identity, resolveErr
}
