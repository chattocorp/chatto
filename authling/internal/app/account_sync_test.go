package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"hmans.de/authling/internal/tinybasesync"
	"hmans.de/authling/internal/web"
)

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
		AccountSync: runtime.AccountSync,
	})
}

func dialAccountSync(ctx context.Context, serverURL, token, origin string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{"Origin": {origin}}
	if token != "" {
		header.Set("Cookie", "authling_session="+token)
	}
	return websocket.Dial(ctx, strings.Replace(serverURL, "http", "ws", 1)+"/data/sync", &websocket.DialOptions{HTTPHeader: header})
}
