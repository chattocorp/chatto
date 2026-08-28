package http_server

import (
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"hmans.de/chatto/internal/config"
)

func TestAuditRequestMetadataUsesForwardedIPAndCapsUserAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/auth/forgot-password", nil)
	req.Header.Set("User-Agent", strings.Repeat("a", maxAuditUserAgentBytes+40)+"é")
	req.Header.Set("X-Forwarded-For", "203.0.113.4, 10.0.0.7")
	req.Header.Set("X-Real-IP", "198.51.100.9")
	req.RemoteAddr = "192.0.2.10:1234"
	router := gin.New()
	if err := router.SetTrustedProxies([]string{"192.0.2.10", "10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), router)
	c.Request = req

	s := &HTTPServer{config: config.ChattoConfig{
		Webserver: config.WebserverConfig{CookieSigningSecret: "test-cookie-secret"},
	}}
	metadata := s.auditRequestMetadata(c)

	if len(metadata.GetUserAgent()) > maxAuditUserAgentBytes {
		t.Fatalf("user agent length = %d, want <= %d", len(metadata.GetUserAgent()), maxAuditUserAgentBytes)
	}
	if !utf8.ValidString(metadata.GetUserAgent()) {
		t.Fatalf("user agent was truncated to invalid UTF-8")
	}
	wantHash := hmacSHA256Hex("test-cookie-secret", "203.0.113.4")
	if metadata.GetIpHash() != wantHash {
		t.Fatalf("ip hash = %q, want %q", metadata.GetIpHash(), wantHash)
	}
	if metadata.GetIpHash() == "203.0.113.4" || strings.Contains(metadata.GetIpHash(), "203.0.113.4") {
		t.Fatalf("raw IP leaked into metadata: %q", metadata.GetIpHash())
	}
}

func TestAuditRequestMetadataRemovesInvalidShortUserAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.Header.Set("User-Agent", string([]byte{'o', 'k', 0xff}))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	s := &HTTPServer{}
	metadata := s.auditRequestMetadata(c)

	if !utf8.ValidString(metadata.GetUserAgent()) {
		t.Fatalf("user agent contains invalid UTF-8: %q", metadata.GetUserAgent())
	}
	if metadata.GetUserAgent() != "ok" {
		t.Fatalf("user agent = %q, want %q", metadata.GetUserAgent(), "ok")
	}
}

func TestAuditSourceIPFallbacks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("trusted proxy real ip", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Real-IP", "198.51.100.9")
		req.RemoteAddr = "192.0.2.10:1234"
		router := gin.New()
		if err := router.SetTrustedProxies([]string{"192.0.2.10"}); err != nil {
			t.Fatal(err)
		}
		c := gin.CreateTestContextOnly(httptest.NewRecorder(), router)
		c.Request = req
		if got := auditSourceIP(c); got != "198.51.100.9" {
			t.Fatalf("auditSourceIP = %q", got)
		}
	})

	t.Run("untrusted peer cannot spoof forwarded ip", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.4")
		req.Header.Set("X-Real-IP", "198.51.100.9")
		req.RemoteAddr = "192.0.2.10:1234"
		router := gin.New()
		if err := router.SetTrustedProxies(nil); err != nil {
			t.Fatal(err)
		}
		c := gin.CreateTestContextOnly(httptest.NewRecorder(), router)
		c.Request = req
		if got := auditSourceIP(c); got != "192.0.2.10" {
			t.Fatalf("auditSourceIP = %q, want direct peer", got)
		}
	})

	t.Run("remote addr", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		router := gin.New()
		if err := router.SetTrustedProxies(nil); err != nil {
			t.Fatal(err)
		}
		c := gin.CreateTestContextOnly(httptest.NewRecorder(), router)
		c.Request = req
		if got := auditSourceIP(c); got != "192.0.2.10" {
			t.Fatalf("auditSourceIP = %q", got)
		}
	})
}
