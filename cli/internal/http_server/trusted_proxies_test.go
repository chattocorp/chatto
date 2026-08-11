package http_server

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNewTrustedProxySetRejectsInvalidEntry(t *testing.T) {
	if _, err := newTrustedProxySet([]string{"proxy.internal"}); err == nil {
		t.Fatal("newTrustedProxySet accepted hostname")
	}
}

func TestTrustedProxySetNormalizesIPv4MappedEntries(t *testing.T) {
	proxies, err := newTrustedProxySet([]string{"::ffff:192.0.2.0/120", "::ffff:198.51.100.10"})
	if err != nil {
		t.Fatal(err)
	}
	for _, remoteAddr := range []string{
		"192.0.2.25:1234",
		"[::ffff:192.0.2.25]:1234",
		"198.51.100.10:1234",
		"[::ffff:198.51.100.10]:1234",
	} {
		if !proxies.containsRemoteAddr(remoteAddr) {
			t.Errorf("trusted proxy set did not match %q", remoteAddr)
		}
	}
	if proxies.containsRemoteAddr("203.0.113.10:1234") {
		t.Fatal("trusted proxy set matched address outside mapped entries")
	}

	router := gin.New()
	if err := router.SetTrustedProxies([]string{"::ffff:192.0.2.0/120"}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[::ffff:192.0.2.25]:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.20")
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), router)
	c.Request = req
	if got := c.ClientIP(); got != "203.0.113.20" {
		t.Fatalf("Gin ClientIP = %q, want forwarded client through same mapped proxy", got)
	}
}
