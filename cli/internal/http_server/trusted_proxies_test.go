package http_server

import (
	"net/http/httptest"
	"testing"

	"github.com/charmbracelet/log"
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
