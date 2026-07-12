package http_server

import (
	"net/http/httptest"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
)

func TestCheckRealtimeWebSocketOriginTrustsForwardedHostOnlyFromProxy(t *testing.T) {
	tests := []struct {
		name           string
		trustedProxies []string
		remoteAddr     string
		host           string
		forwardedHost  string
		want           bool
	}{
		{
			name:          "direct peer cannot spoof forwarded host",
			remoteAddr:    "192.0.2.10:1234",
			host:          "internal:4000",
			forwardedHost: "chat.example",
			want:          false,
		},
		{
			name:           "trusted proxy supplies public host",
			trustedProxies: []string{"192.0.2.0/24"},
			remoteAddr:     "192.0.2.10:1234",
			host:           "internal:4000",
			forwardedHost:  "chat.example",
			want:           true,
		},
		{
			name:           "last forwarded host wins",
			trustedProxies: []string{"192.0.2.10"},
			remoteAddr:     "192.0.2.10:1234",
			host:           "internal:4000",
			forwardedHost:  "attacker.example, chat.example",
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxies, err := newTrustedProxySet(tt.trustedProxies)
			if err != nil {
				t.Fatal(err)
			}
			s := &HTTPServer{trustedProxies: proxies, logger: log.WithPrefix("test")}
			req := httptest.NewRequest("GET", realtimePath, nil)
			req.Header.Set("Origin", "https://chat.example")
			req.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			req.RemoteAddr = tt.remoteAddr
			req.Host = tt.host
			if got := s.checkRealtimeWebSocketOrigin(req, []string{"https://other.example"}); got != tt.want {
				t.Fatalf("checkRealtimeWebSocketOrigin = %v, want %v", got, tt.want)
			}
		})
	}
}

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
