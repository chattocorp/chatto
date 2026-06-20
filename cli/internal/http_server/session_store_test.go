package http_server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	ginsessions "github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gorilla/securecookie"
	gsessions "github.com/gorilla/sessions"
)

func TestDebugSessionStoreSuppressesSecureCookieDecodeErrors(t *testing.T) {
	const cookieName = "chatto_session"
	authKey := []byte("test-secret-key-32-bytes-long!!")
	baseStore := cookie.NewStore(authKey)

	var logOutput bytes.Buffer
	store := newDebugSessionStore(baseStore, log.NewWithOptions(&logOutput, log.Options{
		Level:     log.DebugLevel,
		Formatter: log.JSONFormatter,
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name: cookieName,
		Value: expiredSecureCookieValue(t, authKey, cookieName, map[interface{}]interface{}{
			sessionKeyUserID: "user_123",
		}),
	})

	session, err := store.Get(req, cookieName)
	if err != nil {
		t.Fatalf("Get returned error for expired securecookie: %v", err)
	}
	if session == nil {
		t.Fatal("Get returned nil session")
	}
	if !session.IsNew {
		t.Fatal("expired securecookie session was not treated as a new session")
	}
	if len(session.Values) != 0 {
		t.Fatalf("expired securecookie session kept values: %#v", session.Values)
	}

	logged := logOutput.String()
	if !strings.Contains(logged, "Ignoring invalid session cookie") {
		t.Fatalf("debug log did not include expected message: %s", logged)
	}
	if !strings.Contains(logged, `"level":"debug"`) {
		t.Fatalf("log was not emitted at debug level: %s", logged)
	}
	if !strings.Contains(logged, "securecookie: expired timestamp") {
		t.Fatalf("debug log did not include securecookie reason: %s", logged)
	}
	if strings.Contains(logged, `"level":"error"`) || strings.Contains(logged, "[sessions] ERROR!") {
		t.Fatalf("expired securecookie was logged as an error: %s", logged)
	}
}

func TestDebugSessionStoreReturnsNonDecodeErrors(t *testing.T) {
	expectedErr := errors.New("store failed")
	baseStore := &failingSessionStore{err: expectedErr}

	var logOutput bytes.Buffer
	store := newDebugSessionStore(baseStore, log.NewWithOptions(&logOutput, log.Options{
		Level:     log.DebugLevel,
		Formatter: log.JSONFormatter,
	}))

	_, err := store.Get(httptest.NewRequest(http.MethodGet, "/", nil), "chatto_session")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("Get error = %v, want %v", err, expectedErr)
	}
	if logOutput.Len() != 0 {
		t.Fatalf("unexpected log output for non-decode error: %s", logOutput.String())
	}
}

func TestDebugSessionStoreReturnsSecureCookieUsageErrors(t *testing.T) {
	var values map[interface{}]interface{}
	expectedErr := securecookie.New(nil, nil).Decode("chatto_session", "invalid", &values)
	if expectedErr == nil {
		t.Fatal("expected securecookie usage error")
	}

	baseStore := &failingSessionStore{err: expectedErr}
	var logOutput bytes.Buffer
	store := newDebugSessionStore(baseStore, log.NewWithOptions(&logOutput, log.Options{
		Level:     log.DebugLevel,
		Formatter: log.JSONFormatter,
	}))

	_, err := store.Get(httptest.NewRequest(http.MethodGet, "/", nil), "chatto_session")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("Get error = %v, want %v", err, expectedErr)
	}
	if logOutput.Len() != 0 {
		t.Fatalf("unexpected log output for securecookie usage error: %s", logOutput.String())
	}
}

type failingSessionStore struct {
	ginsessions.Store
	err error
}

func (s *failingSessionStore) Get(_ *http.Request, name string) (*gsessions.Session, error) {
	return gsessions.NewSession(s, name), s.err
}

func expiredSecureCookieValue(t *testing.T, authKey []byte, name string, values map[interface{}]interface{}) string {
	t.Helper()

	encoded, err := securecookie.New(authKey, nil).Encode(name, values)
	if err != nil {
		t.Fatalf("encode securecookie: %v", err)
	}

	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode securecookie envelope: %v", err)
	}

	parts := bytes.SplitN(decoded, []byte("|"), 3)
	if len(parts) != 3 {
		t.Fatalf("unexpected securecookie envelope: %q", decoded)
	}

	expiredAt := time.Now().Add(-31 * 24 * time.Hour).UTC().Unix()
	payload := parts[1]
	macInput := []byte(fmt.Sprintf("%s|%d|%s", name, expiredAt, payload))
	mac := hmac.New(sha256.New, authKey)
	_, _ = mac.Write(macInput)

	envelope := append([]byte(fmt.Sprintf("%d|%s|", expiredAt, payload)), mac.Sum(nil)...)
	return base64.URLEncoding.EncodeToString(envelope)
}
