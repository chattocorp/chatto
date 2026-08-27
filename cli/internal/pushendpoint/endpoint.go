// Package pushendpoint validates Web Push endpoint URLs and provides the
// restricted HTTP client used to deliver push notifications.
package pushendpoint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

// ErrInvalid is returned when a Web Push endpoint is not a safe absolute HTTPS
// URL. Network destinations are checked again when the connection is opened.
var ErrInvalid = errors.New("invalid Web Push endpoint")

// MaxSubscriptionsPerUser bounds stored endpoints and outbound fan-out for one
// account. A generous device allowance keeps the limit out of normal use.
const MaxSubscriptionsPerUser = 16

// Validate checks the stable URL properties of a Web Push endpoint. It does
// not resolve hostnames; NewHTTPClient validates every DNS result at dial time.
func Validate(raw string) error {
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" || endpoint.Opaque != "" {
		return ErrInvalid
	}
	if address, err := netip.ParseAddr(endpoint.Hostname()); err == nil && blockedAddress(address) {
		return ErrInvalid
	}
	return nil
}

type ipResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type dialContextFunc func(context.Context, string, string) (net.Conn, error)

// NewHTTPClient creates a Web Push client that bypasses environment proxies,
// refuses redirects, and validates resolved addresses immediately before each
// connection. Connecting to the validated IP directly prevents a second DNS
// lookup from changing the destination.
func NewHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           safeDialContext(net.DefaultResolver, dialer.DialContext),
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   timeout,
			ResponseHeaderTimeout: timeout,
			MaxIdleConns:          16,
			MaxIdleConnsPerHost:   4,
			IdleConnTimeout:       30 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func safeDialContext(resolver ipResolver, dial dialContextFunc) dialContextFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || host == "" || port == "" {
			return nil, errors.New("push endpoint has an invalid network address")
		}

		resolveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		addresses, err := resolver.LookupNetIP(resolveCtx, "ip", host)
		if err != nil || len(addresses) == 0 {
			return nil, errors.New("push endpoint could not be resolved")
		}
		for _, resolved := range addresses {
			if blockedAddress(resolved) {
				return nil, errors.New("push endpoint resolves to a blocked network address")
			}
		}

		var dialErrors []error
		for _, resolved := range addresses {
			conn, err := dial(ctx, network, net.JoinHostPort(resolved.String(), port))
			if err == nil {
				return conn, nil
			}
			dialErrors = append(dialErrors, err)
		}
		return nil, fmt.Errorf("push endpoint connection failed: %w", errors.Join(dialErrors...))
	}
}

func blockedAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsUnspecified() || address.IsLoopback() || address.IsMulticast() || address.IsLinkLocalUnicast() || address.IsPrivate() {
		return true
	}
	for _, prefix := range specialUsePrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// specialUsePrefixes supplements netip's semantic checks with IANA special-use
// ranges that must never be Web Push delivery destinations.
var specialUsePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"), netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.175.48.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2620:4f:8000::/48"), netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
}
