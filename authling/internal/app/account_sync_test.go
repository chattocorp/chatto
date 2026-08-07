package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/tinybasesync"
	"hmans.de/authling/internal/web"
)

func TestAccountSyncAcceptsScopedOIDCAccessTokenFromBoundOrigin(t *testing.T) {
	cfg := embeddedTestConfig(t)
	cfg.HTTP = config.HTTPConfig{BindAddress: "127.0.0.1:8080", PublicURL: "http://localhost:8080"}
	cfg.OIDC.Clients = []config.OIDCClientConfig{{
		ID: "test-client", Name: "Test Client", RedirectURIs: []string{"http://localhost:9999/callback"},
	}}
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	if _, err := runtime.Accounts.CreateLocal(testContext(t), "oidc@example.com", "a deliberately uncommon password"); err != nil {
		t.Fatal(err)
	}
	oidcHandler := web.Handler(web.Dependencies{
		Accounts: runtime.Accounts, Authentication: runtime.Authentication, Registration: runtime.Registration,
		Sessions: runtime.Sessions, OIDC: runtime.OIDC, PublicURL: cfg.HTTP.PublicURLOrDefault(),
	})
	accessToken := issueAccountDataToken(t, oidcHandler, "openid account_data")
	identityOnlyToken := issueAccountDataToken(t, oidcHandler, "openid")
	syncServer := httptest.NewServer(accountSyncHandler(runtime))
	defer syncServer.Close()

	connection := dialTokenAccountSync(t, syncServer.URL, accessToken, "http://localhost:9999")
	defer connection.CloseNow()
	if err := connection.Write(t.Context(), websocket.MessageText, []byte(`["hashes",1,""]`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := connection.Read(t.Context()); err != nil {
		t.Fatalf("read authenticated sync response: %v", err)
	}

	for _, rejected := range []struct {
		name, token, origin string
	}{
		{name: "wrong origin", token: accessToken, origin: "http://attacker.localhost:9999"},
		{name: "missing scope", token: identityOnlyToken, origin: "http://localhost:9999"},
		{name: "invalid token", token: "not-a-token", origin: "http://localhost:9999"},
	} {
		t.Run(rejected.name, func(t *testing.T) {
			candidate := dialTokenAccountSyncUnchecked(t, syncServer.URL, rejected.token, rejected.origin)
			defer candidate.CloseNow()
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			if _, _, err := candidate.Read(ctx); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
				t.Fatalf("rejected token close error/status = %v/%v", err, websocket.CloseStatus(err))
			}
		})
	}
}

func TestAccountDataAccessTokenSurvivesAuthlingRestart(t *testing.T) {
	cfg := embeddedTestConfig(t)
	cfg.HTTP = config.HTTPConfig{BindAddress: "127.0.0.1:8080", PublicURL: "http://localhost:8080"}
	cfg.OIDC.Clients = []config.OIDCClientConfig{{
		ID: "test-client", Name: "Test Client", RedirectURIs: []string{"http://localhost:9999/callback"},
	}}
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	if _, err := runtime.Accounts.CreateLocal(testContext(t), "oidc@example.com", "a deliberately uncommon password"); err != nil {
		t.Fatal(err)
	}
	handler := web.Handler(web.Dependencies{
		Accounts: runtime.Accounts, Authentication: runtime.Authentication, Registration: runtime.Registration,
		Sessions: runtime.Sessions, OIDC: runtime.OIDC, PublicURL: cfg.HTTP.PublicURLOrDefault(),
	})
	accessToken := issueAccountDataToken(t, handler, "openid account_data")
	stopTestRuntime(t, runtime, cancel, runErrors)

	restarted, restartCancel, restartErrors := startTestRuntime(t, cfg)
	defer stopTestRuntime(t, restarted, restartCancel, restartErrors)
	syncServer := httptest.NewServer(accountSyncHandler(restarted))
	defer syncServer.Close()
	connection := dialTokenAccountSync(t, syncServer.URL, accessToken, "http://localhost:9999")
	connection.CloseNow()
}

func issueAccountDataToken(t *testing.T, handler http.Handler, scopes string) string {
	t.Helper()
	verifier := strings.Repeat("v", 43)
	code := completeAuthorizationForScopes(t, handler, verifier, nil, scopes)
	response := redeemCode(t, handler, code, verifier)
	if response.Code != http.StatusOK {
		t.Fatalf("token status/body = %d %s", response.Code, response.Body.String())
	}
	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &tokens); err != nil || tokens.AccessToken == "" {
		t.Fatalf("decode access token: %v", err)
	}
	return tokens.AccessToken
}

func dialTokenAccountSync(t *testing.T, serverURL, token, origin string) *websocket.Conn {
	t.Helper()
	connection := dialTokenAccountSyncUnchecked(t, serverURL, token, origin)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, data, err := connection.Read(ctx)
	if err != nil || string(data) != `{"type":"ready"}` {
		connection.CloseNow()
		t.Fatalf("account sync ready message/error = %s/%v", data, err)
	}
	return connection
}

func dialTokenAccountSyncUnchecked(t *testing.T, serverURL, token, origin string) *websocket.Conn {
	t.Helper()
	connection, response, err := websocket.Dial(t.Context(), strings.Replace(serverURL, "http", "ws", 1)+"/data/sync", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {origin}}, Subprotocols: []string{"authling.account-data.v1"},
	})
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("dial token account sync status/error = %d/%v", status, err)
	}
	payload, err := json.Marshal(map[string]string{"type": "authenticate", "access_token": token})
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Write(t.Context(), websocket.MessageText, payload); err != nil {
		connection.CloseNow()
		t.Fatal(err)
	}
	return connection
}

func TestAccountSyncRequiresSameOriginSessionAndRevalidatesIt(t *testing.T) {
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t))
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(testContext(t), "sync-security@example.com", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := runtime.Sessions.Create(testContext(t), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(accountSyncHandler(runtime))
	defer server.Close()

	if connection, response, err := dialAccountSync(t.Context(), server.URL, "", server.URL); err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		if connection != nil {
			connection.CloseNow()
		}
		t.Fatalf("unauthenticated dial response/error = %v/%v", response, err)
	}
	if connection, response, err := dialAccountSync(t.Context(), server.URL, token, "https://attacker.invalid"); err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		if connection != nil {
			connection.CloseNow()
		}
		t.Fatalf("cross-origin dial response/error = %v/%v", response, err)
	}

	connection, _, err := dialAccountSync(t.Context(), server.URL, token, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	if err := connection.Write(t.Context(), websocket.MessageText, []byte(`["first",1,""]`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := connection.Read(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Sessions.Revoke(testContext(t), token); err != nil {
		t.Fatal(err)
	}
	if err := connection.Write(t.Context(), websocket.MessageText, []byte(`["second",1,""]`)); err != nil {
		t.Fatal(err)
	}
	ctx, readCancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer readCancel()
	if _, _, err := connection.Read(ctx); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("revoked session close error/status = %v/%v", err, websocket.CloseStatus(err))
	}
}

func TestAccountSyncRejectsMalformedBinaryAndExcessConnections(t *testing.T) {
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t))
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(testContext(t), "sync-limits@example.com", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := runtime.Sessions.Create(testContext(t), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(accountSyncHandler(runtime))
	defer server.Close()

	malformed, _, err := dialAccountSync(t.Context(), server.URL, token, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := malformed.Write(t.Context(), websocket.MessageText, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := malformed.Read(t.Context()); websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("malformed close error/status = %v/%v", err, websocket.CloseStatus(err))
	}
	malformed.CloseNow()

	binary, _, err := dialAccountSync(t.Context(), server.URL, token, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(t.Context(), websocket.MessageBinary, []byte{1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := binary.Read(t.Context()); websocket.CloseStatus(err) != websocket.StatusUnsupportedData {
		t.Fatalf("binary close error/status = %v/%v", err, websocket.CloseStatus(err))
	}
	binary.CloseNow()

	oversize, _, err := dialAccountSync(t.Context(), server.URL, token, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := oversize.Write(t.Context(), websocket.MessageText, make([]byte, tinybasesync.MaxWireMessageSize+1)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := oversize.Read(t.Context()); websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
		t.Fatalf("oversize close error/status = %v/%v", err, websocket.CloseStatus(err))
	}
	oversize.CloseNow()

	rateLimited, _, err := dialAccountSync(t.Context(), server.URL, token, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	rateLimitClosed := false
	for message := 0; message < 100; message++ {
		request := `[` + fmt.Sprintf("%q", fmt.Sprintf("rate-%d", message)) + `,4,{}]`
		if err := rateLimited.Write(t.Context(), websocket.MessageText, []byte(request)); err != nil {
			if websocket.CloseStatus(err) == websocket.StatusPolicyViolation {
				rateLimitClosed = true
				break
			}
			t.Fatalf("rate-limit write %d: %v", message, err)
		}
		if _, _, err := rateLimited.Read(t.Context()); err != nil {
			if websocket.CloseStatus(err) == websocket.StatusPolicyViolation {
				rateLimitClosed = true
				break
			}
			t.Fatalf("rate-limit response %d: %v", message, err)
		}
	}
	if !rateLimitClosed {
		t.Fatal("account-wide message rate did not close the connection")
	}
	rateLimited.CloseNow()

	connections := make([]*websocket.Conn, 0, 8)
	for range 8 {
		connection, _, err := dialAccountSync(t.Context(), server.URL, token, server.URL)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, connection)
	}
	defer func() {
		for _, connection := range connections {
			connection.CloseNow()
		}
	}()
	extra, response, err := dialAccountSync(t.Context(), server.URL, token, server.URL)
	if extra != nil {
		extra.CloseNow()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("excess connection response/error = %v/%v", response, err)
	}
}

func TestAccountSyncSelectsDataOnlyFromAuthenticatedAccount(t *testing.T) {
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t))
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	first, err := runtime.Accounts.CreateLocal(testContext(t), "sync-first@example.com", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.Accounts.CreateLocal(testContext(t), "sync-second@example.com", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	firstToken, _, _ := runtime.Sessions.Create(testContext(t), first.ID)
	secondToken, _, _ := runtime.Sessions.Create(testContext(t), second.ID)
	server := httptest.NewServer(accountSyncHandler(runtime))
	defer server.Close()
	firstConnection, _, err := dialAccountSync(t.Context(), server.URL, firstToken, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer firstConnection.CloseNow()
	secondConnection, _, err := dialAccountSync(t.Context(), server.URL, secondToken, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer secondConnection.CloseNow()

	change := `[[{"servers":[{"one":[{"name":["Private","0000000000000001"]}]}]}],[{}],1]`
	if err := firstConnection.Write(t.Context(), websocket.MessageText, []byte(`[null,3,`+change+`]`)); err != nil {
		t.Fatal(err)
	}
	if err := secondConnection.Write(t.Context(), websocket.MessageText, []byte(`["hashes",1,""]`)); err != nil {
		t.Fatal(err)
	}
	_, response, err := secondConnection.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var wire []json.RawMessage
	if err := json.Unmarshal(response, &wire); err != nil || len(wire) != 3 || string(wire[2]) != `[0,0]` {
		t.Fatalf("second account content hashes = %s", response)
	}
}

func accountSyncHandler(runtime *Runtime) http.Handler {
	return web.Handler(web.Dependencies{
		Accounts: runtime.Accounts, Sessions: runtime.Sessions,
		OIDC: runtime.OIDC, AccountSync: runtime.AccountSync,
	})
}

func dialAccountSync(ctx context.Context, serverURL, token, origin string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{"Origin": {origin}}
	if token != "" {
		header.Set("Cookie", "authling_session="+token)
	}
	return websocket.Dial(ctx, strings.Replace(serverURL, "http", "ws", 1)+"/data/sync", &websocket.DialOptions{HTTPHeader: header})
}
