package linkpreview

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticIPResolver struct {
	ips []net.IP
}

func (r staticIPResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return r.ips, nil
}

func TestIsPrivateIP(t *testing.T) {
	// Ensure allowLocalhost is false for this test (may be true under test_endpoints tag)
	orig := allowLocalhost
	allowLocalhost = false
	defer func() { allowLocalhost = orig }()

	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Private IPs - should be blocked
		{"loopback 127.0.0.1", "127.0.0.1", true},
		{"loopback 127.255.255.255", "127.255.255.255", true},
		{"RFC1918 10.0.0.1", "10.0.0.1", true},
		{"RFC1918 10.255.255.255", "10.255.255.255", true},
		{"RFC1918 172.16.0.1", "172.16.0.1", true},
		{"RFC1918 172.31.255.255", "172.31.255.255", true},
		{"RFC1918 192.168.0.1", "192.168.0.1", true},
		{"RFC1918 192.168.255.255", "192.168.255.255", true},
		{"link-local 169.254.0.1", "169.254.0.1", true},
		{"unspecified 0.0.0.0", "0.0.0.0", true},
		{"this-network 0.0.0.1", "0.0.0.1", true},
		{"this-network 0.1.2.3", "0.1.2.3", true},
		{"IPv6 loopback", "::1", true},
		{"IPv6 link-local", "fe80::1", true},
		{"IPv6 unique local", "fc00::1", true},
		{"IPv6 unspecified", "::", true},

		// Public IPs - should be allowed
		{"public 8.8.8.8", "8.8.8.8", false},
		{"public 1.1.1.1", "1.1.1.1", false},
		{"public 93.184.216.34", "93.184.216.34", false},
		{"not RFC1918 172.32.0.1", "172.32.0.1", false},
		{"public IPv6", "2001:4860:4860::8888", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			assert.NotNil(t, ip, "failed to parse IP %s", tt.ip)
			assert.Equal(t, tt.expected, isPrivateIP(ip))
		})
	}
}

func TestAllowLocalhost(t *testing.T) {
	// Save and restore original value
	orig := allowLocalhost
	defer func() { allowLocalhost = orig }()

	loopback := net.ParseIP("127.0.0.1")
	loopbackV6 := net.ParseIP("::1")
	private := net.ParseIP("10.0.0.1")

	// Default: loopback is private
	allowLocalhost = false
	assert.True(t, isPrivateIP(loopback), "loopback should be private by default")
	assert.True(t, isPrivateIP(loopbackV6), "IPv6 loopback should be private by default")

	// With allowLocalhost: loopback is allowed
	allowLocalhost = true
	assert.False(t, isPrivateIP(loopback), "loopback should be allowed when allowLocalhost is true")
	assert.False(t, isPrivateIP(loopbackV6), "IPv6 loopback should be allowed when allowLocalhost is true")

	// Other private IPs remain blocked regardless
	assert.True(t, isPrivateIP(private), "RFC1918 should still be blocked with allowLocalhost")
}

func TestSSRFSafeDialFallsBackAcrossValidatedAddresses(t *testing.T) {
	orig := allowLocalhost
	allowLocalhost = true
	defer func() { allowLocalhost = orig }()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	dial := ssrfSafeDialContextWithResolver(time.Second, staticIPResolver{ips: []net.IP{
		net.ParseIP("::1"),
		net.ParseIP("127.0.0.1"),
	}})

	conn, err := dial(context.Background(), "tcp", net.JoinHostPort("preview.example", port))
	require.NoError(t, err)
	if conn != nil {
		_ = conn.Close()
	}
}

func TestSSRFSafeDialRejectsAllResultsWhenAnyAddressIsPrivate(t *testing.T) {
	orig := allowLocalhost
	allowLocalhost = false
	defer func() { allowLocalhost = orig }()

	dial := ssrfSafeDialContextWithResolver(time.Second, staticIPResolver{ips: []net.IP{
		net.ParseIP("93.184.216.34"),
		net.ParseIP("127.0.0.1"),
	}})

	_, err := dial(context.Background(), "tcp", "preview.example:443")
	assert.ErrorContains(t, err, "resolves to private IP")
}
