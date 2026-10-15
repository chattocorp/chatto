package linkpreview

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		{"shared range start", "100.64.0.0", true},
		{"shared metadata address", "100.100.100.200", true},
		{"shared range end", "100.127.255.255", true},
		{"mapped shared address", "::ffff:100.64.0.1", true},
		{"NAT64 private IPv4", "64:ff9b::a00:1", true},
		{"NAT64 public IPv4", "64:ff9b::808:808", true},
		{"local-use NAT64", "64:ff9b:1::a00:1", true},
		{"IPv4-compatible IPv6", "::a00:1", true},
		{"IPv6 dummy prefix", "100:0:0:1::1", true},
		{"6to4", "2002:a00:1::1", true},
		{"Teredo", "2001::1", true},
		{"documentation IPv4", "192.0.2.1", true},
		{"documentation IPv6", "2001:db8::1", true},
		{"benchmark IPv4", "198.18.0.1", true},
		{"multicast IPv4", "224.0.0.1", true},
		{"multicast IPv6", "ff0e::1", true},
		{"reserved IPv4", "240.0.0.1", true},

		// Public IPs - should be allowed
		{"public 8.8.8.8", "8.8.8.8", false},
		{"public 1.1.1.1", "1.1.1.1", false},
		{"public 93.184.216.34", "93.184.216.34", false},
		{"not RFC1918 172.32.0.1", "172.32.0.1", false},
		{"public IPv6", "2001:4860:4860::8888", false},
		{"before shared range", "100.63.255.255", false},
		{"after shared range", "100.128.0.0", false},
		{"mapped public IPv4", "::ffff:8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			assert.NotNil(t, ip, "failed to parse IP %s", tt.ip)
			assert.Equal(t, tt.expected, isPrivateIP(ip))
		})
	}
	assert.True(t, isPrivateIP(nil), "invalid IPs must fail closed")
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
	assert.ErrorContains(t, err, "blocked address")
}

func TestSSRFSafeDialRejectsEmptyDNSResults(t *testing.T) {
	dial := ssrfSafeDialContextWithResolver(time.Second, staticIPResolver{})

	_, err := dial(context.Background(), "tcp", "preview.example:443")
	assert.ErrorContains(t, err, "resolved to no addresses")
}

func TestWebhookLocalhostDialPolicy(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	for _, tc := range []struct {
		host    string
		ips     []net.IP
		allowed bool
	}{
		{"localhost", []net.IP{net.ParseIP("127.0.0.1")}, true},
		{"Runling.Localhost.", []net.IP{net.ParseIP("::1"), net.ParseIP("127.0.0.1")}, true},
		{"localhost", []net.IP{net.ParseIP("192.168.1.10")}, false},
		{"runling.localhost", []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("8.8.8.8")}, false},
		{"runling.localhost", []net.IP{net.ParseIP("8.8.8.8")}, false},
		{"localhost.example.com", []net.IP{net.ParseIP("127.0.0.1")}, false},
		{"notlocalhost", []net.IP{net.ParseIP("127.0.0.1")}, false},
		{"127.0.0.1", []net.IP{net.ParseIP("127.0.0.1")}, false},
		{"example.com", []net.IP{net.ParseIP("192.168.1.10")}, false},
		{"localhost", []net.IP{net.ParseIP("100.64.0.1")}, false},
		{"localhost", []net.IP{net.ParseIP("64:ff9b::7f00:1")}, false},
		{"example.com", []net.IP{net.ParseIP("100.64.0.1")}, false},
		{"localhost", nil, false},
	} {
		t.Run(tc.host+"/"+fmt.Sprint(tc.ips), func(t *testing.T) {
			dial := ssrfSafeDialContextWithPolicy(time.Second, staticIPResolver{ips: tc.ips}, true)
			conn, err := dial(context.Background(), "tcp", net.JoinHostPort(tc.host, port))
			if tc.allowed {
				require.NoError(t, err)
				conn.Close()
			} else {
				require.Error(t, err)
				require.Nil(t, conn)
			}
		})
	}
}

type sequenceIPResolver struct {
	results [][]net.IP
	calls   int
}

func (r *sequenceIPResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	result := r.results[r.calls]
	r.calls++
	return result, nil
}

func TestSSRFSafeDialRejectsSpecialUseBeforeConnecting(t *testing.T) {
	for _, raw := range []string{"100.64.0.1", "::ffff:100.64.0.1", "64:ff9b::a00:1", "64:ff9b:1::a00:1", "2002:a00:1::1", "224.0.0.1", "2001:db8::1"} {
		for _, host := range []string{raw, "preview.example"} {
			t.Run(host+"/"+raw, func(t *testing.T) {
				// Literal inputs use Go's resolver, which parses them without DNS.
				var resolver ipResolver = net.DefaultResolver
				if host == "preview.example" {
					// Even a public first answer must not be dialed when another
					// answer is blocked.
					resolver = staticIPResolver{ips: []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP(raw)}}
				}
				dial := ssrfSafeDialContextWithDialer(time.Second, resolver, false, func(context.Context, string, string) (net.Conn, error) {
					t.Fatal("blocked destination reached the dialer")
					return nil, errors.New("unexpected dial")
				})
				conn, err := dial(context.Background(), "tcp", net.JoinHostPort(host, "80"))
				require.Nil(t, conn)
				require.ErrorContains(t, err, "blocked address")
			})
		}
	}
}

func TestSSRFSafeDialRevalidatesDNSAndUsesValidatedIPs(t *testing.T) {
	resolver := &sequenceIPResolver{results: [][]net.IP{
		{net.ParseIP("2001:4860:4860::8888"), net.ParseIP("8.8.8.8")},
		{net.ParseIP("100.64.0.1")},
	}}
	var dialed []string
	dial := ssrfSafeDialContextWithDialer(time.Second, resolver, false, func(ctx context.Context, _, address string) (net.Conn, error) {
		dialed = append(dialed, address)
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.Positive(t, time.Until(deadline))
		require.LessOrEqual(t, time.Until(deadline), 500*time.Millisecond)
		return nil, errors.New("simulated connection failure")
	})
	_, err := dial(context.Background(), "tcp", "preview.example:443")
	require.Error(t, err)
	require.Equal(t, []string{"[2001:4860:4860::8888]:443", "8.8.8.8:443"}, dialed)
	require.Equal(t, 1, resolver.calls, "dialing must not resolve the hostname again")
	_, err = dial(context.Background(), "tcp", "preview.example:443")
	require.ErrorContains(t, err, "blocked address")
	require.Len(t, dialed, 2, "rebound address must not reach the dialer")
	require.Equal(t, 2, resolver.calls)
}

func TestSSRFSafeClientRejectsRedirectToSpecialUseAddress(t *testing.T) {
	for _, raw := range []string{"100.64.0.1", "64:ff9b::a00:1"} {
		t.Run(raw, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "http://"+net.JoinHostPort(raw, "80")+"/private", http.StatusFound)
			}))
			defer server.Close()
			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			resolver := &sequenceIPResolver{results: [][]net.IP{{net.ParseIP("8.8.8.8")}, {net.ParseIP(raw)}}}
			dialCalls := 0
			dial := ssrfSafeDialContextWithDialer(time.Second, resolver, false, func(ctx context.Context, network, address string) (net.Conn, error) {
				dialCalls++
				if address != "8.8.8.8:80" {
					return nil, fmt.Errorf("unexpected dial to %s", address)
				}
				// Substitute a local fixture only after the production policy
				// accepts the public destination. Never contact an external IP.
				return (&net.Dialer{}).DialContext(ctx, network, serverURL.Host)
			})
			client := newSSRFSafeClient(time.Second, dial)
			defer client.CloseIdleConnections()
			require.Nil(t, client.Transport.(*http.Transport).Proxy)
			resp, err := client.Get("http://preview.example/")
			if resp != nil {
				resp.Body.Close()
			}
			require.ErrorContains(t, err, "blocked address")
			require.Equal(t, 1, dialCalls)
			require.Equal(t, 2, resolver.calls)
		})
	}
}
