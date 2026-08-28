package pushendpoint

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{name: "https endpoint", endpoint: "https://push.example.com/send/device-token"},
		{name: "https endpoint with port and query", endpoint: "https://push.example.com:8443/send?id=123"},
		{name: "http endpoint", endpoint: "http://push.example.com/send", wantErr: true},
		{name: "relative endpoint", endpoint: "/send/device-token", wantErr: true},
		{name: "missing host", endpoint: "https:///send/device-token", wantErr: true},
		{name: "userinfo", endpoint: "https://user:password@push.example.com/send", wantErr: true},
		{name: "fragment", endpoint: "https://push.example.com/send#fragment", wantErr: true},
		{name: "opaque URL", endpoint: "https:push.example.com/send", wantErr: true},
		{name: "loopback IP literal", endpoint: "https://127.0.0.1/send", wantErr: true},
		{name: "private IPv6 literal", endpoint: "https://[fd00::1]/send", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.endpoint)
			if tt.wantErr && err == nil {
				t.Fatal("Validate returned nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate returned error: %v", err)
			}
		})
	}
}

func TestBlockedAddress(t *testing.T) {
	blocked := []string{
		"0.0.0.0", "127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.169.254",
		"172.16.0.1", "192.168.0.1", "192.0.2.1", "198.18.0.1", "224.0.0.1",
		"::", "::1", "::ffff:127.0.0.1", "64:ff9b::7f00:1", "fe80::1", "fc00::1",
		"2001:db8::1",
	}
	for _, raw := range blocked {
		t.Run(raw, func(t *testing.T) {
			if !blockedAddress(netip.MustParseAddr(raw)) {
				t.Fatalf("blockedAddress(%s) = false, want true", raw)
			}
		})
	}

	for _, raw := range []string{"8.8.8.8", "93.184.216.34", "2606:4700:4700::1111"} {
		t.Run(raw, func(t *testing.T) {
			if blockedAddress(netip.MustParseAddr(raw)) {
				t.Fatalf("blockedAddress(%s) = true, want false", raw)
			}
		})
	}
}

type sequenceResolver struct {
	results [][]netip.Addr
	calls   int
}

func (r *sequenceResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	result := r.results[r.calls]
	r.calls++
	return result, nil
}

func TestSafeDialContextValidatesEveryResolutionBeforeConnecting(t *testing.T) {
	resolver := &sequenceResolver{results: [][]netip.Addr{
		{netip.MustParseAddr("93.184.216.34")},
		{netip.MustParseAddr("127.0.0.1")},
	}}
	var dialed []string
	dial := safeDialContext(resolver, func(_ context.Context, _, address string) (net.Conn, error) {
		dialed = append(dialed, address)
		return nil, errors.New("test dial stopped")
	})

	if _, err := dial(context.Background(), "tcp", "push.example.com:443"); err == nil {
		t.Fatal("first dial returned nil error")
	}
	if len(dialed) != 1 || dialed[0] != "93.184.216.34:443" {
		t.Fatalf("first dial targets = %v, want public resolved address", dialed)
	}

	if _, err := dial(context.Background(), "tcp", "push.example.com:443"); err == nil {
		t.Fatal("second dial returned nil error")
	}
	if len(dialed) != 1 {
		t.Fatalf("private rebound address reached dialer; targets = %v", dialed)
	}
}

func TestSafeDialContextRejectsMixedPublicAndPrivateResults(t *testing.T) {
	resolver := &sequenceResolver{results: [][]netip.Addr{{
		netip.MustParseAddr("93.184.216.34"),
		netip.MustParseAddr("10.0.0.1"),
	}}}
	dialCalled := false
	dial := safeDialContext(resolver, func(context.Context, string, string) (net.Conn, error) {
		dialCalled = true
		return nil, errors.New("unexpected dial")
	})

	if _, err := dial(context.Background(), "tcp", "push.example.com:443"); err == nil {
		t.Fatal("dial returned nil error")
	}
	if dialCalled {
		t.Fatal("dialer was called for mixed public/private DNS results")
	}
}

func TestNewHTTPClientDisablesRedirectsAndProxies(t *testing.T) {
	client := NewHTTPClient(time.Second)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("push transport must not use environment proxies")
	}

	req, err := http.NewRequest(http.MethodGet, "https://redirect.example.com/next", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if err := client.CheckRedirect(req, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect error = %v, want http.ErrUseLastResponse", err)
	}
}
