package linkpreview

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/chattocorp/chatto/cli/internal/ssrf"
)

// allowLocalhost can be set to true to permit loopback addresses (e.g. for e2e tests
// that use a local mock HTTP server). This is set via init() in ssrf_testing.go when
// built with the test_endpoints build tag.
var allowLocalhost bool

type ipResolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}

// isPrivateIP applies the shared outbound address policy, with the loopback-only
// exception used by test builds. Invalid addresses fail closed.
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return !allowLocalhost
	}
	address, ok := netip.AddrFromSlice(ip)
	return !ok || ssrf.BlockedAddress(address)
}

// ssrfSafeDialContext returns a DialContext function that validates resolved IPs
// against the private IP blocklist before establishing a connection.
// This prevents DNS rebinding attacks by checking the IP at connection time
// (not in a separate pre-check that could be subject to TOCTOU races).
func ssrfSafeDialContext(timeout time.Duration) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return ssrfSafeDialContextWithResolver(timeout, net.DefaultResolver)
}

func ssrfSafeDialContextWithResolver(timeout time.Duration, resolver ipResolver) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return ssrfSafeDialContextWithPolicy(timeout, resolver, false)
}

// The localhost exception is host-scoped and never permits other private IPs.
func ssrfSafeDialContextWithPolicy(timeout time.Duration, resolver ipResolver, localhost bool) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	return ssrfSafeDialContextWithDialer(timeout, resolver, localhost, dialer.DialContext)
}

// ssrfSafeDialContextWithDialer validates all results before calling dial. The
// dial function receives IP literals only, with a per-attempt deadline.
func ssrfSafeDialContextWithDialer(timeout time.Duration, resolver ipResolver, localhost bool, dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("ssrf: invalid address %s: %w", addr, err)
		}

		if host == "" {
			return nil, fmt.Errorf("ssrf: empty hostname")
		}

		// Resolve hostname to IP addresses
		resolveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		ips, err := resolver.LookupIP(resolveCtx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("ssrf: failed to resolve hostname %s: %w", host, err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("ssrf: hostname %s resolved to no addresses", host)
		}

		// Check all resolved IPs against the blocklist
		for _, ip := range ips {
			blocked := isPrivateIP(ip)
			if localhost {
				if IsLocalhostHostname(host) {
					blocked = !ip.IsLoopback()
				} else {
					blocked = blocked || ip.IsLoopback()
				}
			}
			if blocked {
				return nil, fmt.Errorf("ssrf: destination resolves to a blocked address")
			}
		}

		// Connect to the already-validated addresses directly, preventing a second
		// DNS lookup while still falling back when the first family is unreachable
		// (for example, an IPv6-first result on an IPv4-only host).
		attemptTimeout := timeout
		if len(ips) > 1 {
			attemptTimeout = min(timeout/2, 2*time.Second)
		}
		var dialErrors []error
		for _, ip := range ips {
			attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
			conn, err := dial(attemptCtx, network, net.JoinHostPort(ip.String(), port))
			cancel()
			if err == nil {
				return conn, nil
			}
			dialErrors = append(dialErrors, fmt.Errorf("%s: %w", ip, err))
		}
		return nil, fmt.Errorf("ssrf: failed to connect to %s: %w", host, errors.Join(dialErrors...))
	}
}

// NewSSRFSafeClient creates an HTTP client with SSRF protection.
// IP validation happens at connection time in DialContext, preventing DNS rebinding attacks.
func NewSSRFSafeClient(timeout time.Duration) *http.Client {
	return newSSRFSafeClient(timeout, ssrfSafeDialContext(10*time.Second))
}

// IsLocalhostHostname identifies localhost and its subdomains, including a final DNS dot.
// IP literals and lookalike suffixes are not localhost hostnames.
func IsLocalhostHostname(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "localhost" || (len(host) > len(".localhost") && strings.HasSuffix(host, ".localhost"))
}

// NewSSRFSafeClientWithLocalhost permits localhost names only when every resolved
// address is loopback. All other hosts retain the private-address blocklist.
// Addresses are validated and dialed without a second lookup. Proxies are disabled.
func NewSSRFSafeClientWithLocalhost(timeout time.Duration) *http.Client {
	return newSSRFSafeClient(timeout, ssrfSafeDialContextWithPolicy(10*time.Second, net.DefaultResolver, true))
}

func newSSRFSafeClient(timeout time.Duration, dial func(context.Context, string, string) (net.Conn, error)) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:           dial,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("ssrf: too many redirects (max 5)")
			}
			return nil
		},
	}
}
