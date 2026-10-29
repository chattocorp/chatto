package http_server

import (
	"bytes"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/connectapi"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1/apiv1connect"
	authv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/auth/v1"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/auth/v1/authv1connect"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// managedProviderBrowser mounts the real account API and preserves browser cookies
// across provider redirects. The client never follows a redirect to a live provider.
func managedProviderBrowser(t *testing.T, s *HTTPServer) (*httptest.Server, *http.Client) {
	t.Helper()
	s.setupConnectAPI()
	ts := httptest.NewServer(s.router)
	t.Cleanup(ts.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := ts.Client()
	client.Jar = jar
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return ts, client
}

func TestManagedOAuthCallbackCreatesAccount(t *testing.T) {
	for _, tc := range []struct {
		providerType, profile, wantBase string
	}{
		{"google", `{"id":"google-subject","name":"DisplayName","email":"Google.User@example.com","verified_email":true}`, "google.user"},
		{"discord", `{"id":"discord-subject","username":"discord-user","global_name":"Display Name"}`, "discord-user"},
	} {
		t.Run(tc.providerType, func(t *testing.T) {
			provider := config.AuthProviderConfig{
				ID: "company", Type: tc.providerType, ClientID: "client-id", ClientSecret: "client-secret",
				ManageUsernames: true, AutoProvision: new(true), RequestEmail: new(true),
			}
			s := setupOAuthServerWithAuth(t, config.AuthConfig{Providers: []config.AuthProviderConfig{provider}})
			runtime, err := newAuthProviderRuntime(provider, "http://chat.example/auth/providers/company/callback")
			require.NoError(t, err)
			// Keep token exchange and Goth profile decoding real, but serve both
			// provider responses locally. No account fields are injected into core.
			var exchanges, profiles atomic.Int32
			runtime.httpClient.Transport = handlerRoundTripper{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					if err := r.ParseForm(); err != nil || r.Form.Get("code") != "test-code" {
						t.Error("unexpected provider token exchange")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					exchanges.Add(1)
					_, _ = w.Write([]byte(`{"access_token":"test-access","token_type":"Bearer","expires_in":3600}`))
					return
				}
				profiles.Add(1)
				_, _ = w.Write([]byte(tc.profile))
			})}
			s.router.GET("/auth/providers/company", func(c *gin.Context) { s.handleProviderStart(c, runtime) })
			s.router.GET("/auth/providers/company/callback", func(c *gin.Context) { s.handleProviderCallback(c, runtime) })
			ts, client := managedProviderBrowser(t, s)
			_, err = s.core.CreateUser(t.Context(), core.SystemActorID, tc.wantBase, "Existing", "password123")
			require.NoError(t, err)

			start, err := client.Get(ts.URL + "/auth/providers/company")
			require.NoError(t, err)
			start.Body.Close()
			require.Equal(t, http.StatusTemporaryRedirect, start.StatusCode)
			state := authStateFromLocation(t, start.Header.Get("Location"))
			location := finishNoEmailOIDCCallback(t, client, ts.URL, "company", state)
			confirmation, err := url.Parse(location)
			require.NoError(t, err)
			require.Equal(t, "/sso/confirm", confirmation.Path)
			require.EqualValues(t, 1, exchanges.Load())
			require.EqualValues(t, 1, profiles.Load())
			token := confirmation.Query().Get("token")
			require.NotEmpty(t, token)
			flow, err := s.core.GetPendingExternalIdentityCreateFlow(t.Context(), token)
			require.NoError(t, err)
			require.Equal(t, tc.wantBase, core.NormalizeProviderUsername(flow.ProviderUsername))

			api := authv1connect.NewExternalIdentityAuthServiceClient(client, ts.URL+connectapi.Prefix)
			pending, err := api.GetPendingExternalIdentity(t.Context(), connect.NewRequest(&authv1.GetPendingExternalIdentityRequest{Token: token}))
			require.NoError(t, err)
			require.Equal(t, authv1.UsernameChoiceAvailability_USERNAME_CHOICE_AVAILABILITY_PROVIDER_MANAGED, pending.Msg.Pending.GetChooseUsername())
			require.Equal(t, tc.wantBase+"-2", pending.Msg.Pending.LoginHint)
			created, err := api.CreateExternalIdentityAccount(t.Context(), connect.NewRequest(&authv1.CreateExternalIdentityAccountRequest{Token: token}))
			require.NoError(t, err)
			require.Equal(t, tc.wantBase+"-2", created.Msg.Login)
			linked, err := s.core.GetUserByExternalIdentity(t.Context(), flow.Issuer, flow.Subject)
			require.NoError(t, err)
			require.Equal(t, created.Msg.UserId, linked.Id)
			require.Equal(t, created.Msg.Login, linked.Login)
		})
	}
}

func TestManagedOIDCCallbackUsernameSync(t *testing.T) {
	// Serial: the failed-sync case captures the process-wide default logger.
	for _, tc := range []struct {
		name, claim, want string
		sync, collision   bool
		wantSyncError     error
	}{
		{"stable by default", "provider-new", "existing-name", false, false, nil},
		{"sync enabled", "provider-new", "provider-new", true, false, nil},
		{"sync collision", "provider-new", "provider-new-2", true, true, nil},
		{"invalid name still signs in", "invalid@name", "existing-name", true, false, core.ErrLoginInvalidCharacter},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issuer := newNoEmailOIDCIssuer(t, "client-id")
			defer issuer.Close()
			issuer.SetSubject("existing-subject")
			issuer.tokenClaims = map[string]any{"name": "Person", "preferred_username": tc.claim}
			provider := config.AuthProviderConfig{
				ID: "company", Type: config.AuthProviderTypeOpenIDConnect, IssuerURL: issuer.URL(),
				ClientID: "client-id", ClientSecret: "client-secret", ManageUsernames: true, UpdateUsernames: tc.sync,
			}
			s := setupOAuthServerWithAuth(t, config.AuthConfig{Providers: []config.AuthProviderConfig{provider}})
			s.setupOIDCRoutes()
			ts, client := managedProviderBrowser(t, s)
			user, err := s.core.CreateUser(t.Context(), core.SystemActorID, "existing-name", "Person", "")
			require.NoError(t, err)
			require.NoError(t, s.core.LinkExternalIdentity(t.Context(), provider.ID, provider.Type, issuer.URL(), "existing-subject", user.Id))
			if tc.collision {
				_, err = s.core.CreateUser(t.Context(), core.SystemActorID, tc.claim, "Other Person", "password123")
				require.NoError(t, err)
			}
			var output bytes.Buffer
			if tc.wantSyncError != nil {
				previous := log.Default()
				log.SetDefault(log.New(&output))
				t.Cleanup(func() { log.SetDefault(previous) })
			}
			require.Equal(t, "/chat", completeNoEmailOIDCLogin(t, client, ts.URL, provider.ID, "/chat"))
			if tc.wantSyncError != nil {
				require.Contains(t, output.String(), "Provider username sync failed")
				require.Contains(t, output.String(), tc.wantSyncError.Error())
				require.NotContains(t, output.String(), tc.claim)
			}
			stored, err := s.core.GetUser(t.Context(), user.Id)
			require.NoError(t, err)
			require.Equal(t, tc.want, stored.Login)
			// A successful redirect alone does not prove that sign-in produced a
			// usable session, especially when synchronization returned an error.
			viewerAPI := apiv1connect.NewViewerServiceClient(client, ts.URL+connectapi.Prefix)
			viewer, err := viewerAPI.GetViewer(t.Context(), connect.NewRequest(&apiv1.GetViewerRequest{}))
			require.NoError(t, err)
			require.Equal(t, user.Id, viewer.Msg.User.Profile.Id)
			require.Equal(t, tc.want, viewer.Msg.User.Profile.Login)
		})
	}
}
