package oidcprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	liboidc "github.com/zitadel/oidc/v3/pkg/oidc"
)

const (
	maxCIMDBytes        = 5 << 10
	maxCIMDCacheAge     = 5 * time.Minute
	defaultCIMDAge      = time.Minute
	maxCIMDCacheEntries = 256
	maxCIMDLookups      = 8
	cimdLookupTimeout   = 5 * time.Second
)

// errCIMDBusy rejects a cache miss when all lookup slots are in use.
var errCIMDBusy = errors.New("CIMD resolver busy")

// cimdLookupContextKey carries the lookup context through net/http, which
// detaches dial cancellation but preserves context values for connection reuse.
type cimdLookupContextKey struct{}

type cimdDocument struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

type cachedClient struct {
	client  *Client
	expires time.Time
}

// CIMDResolver safely retrieves public-client metadata from Client Identifier
// URLs. The injected client exists for TLS-root and transport testing; normal
// operation uses the rebinding-resistant transport constructed here.
type CIMDResolver struct {
	client              *http.Client
	allowLoopback       bool
	slots               chan struct{}
	validateDestination func(context.Context, string) error

	mu    sync.Mutex
	cache map[string]cachedClient
}

// NewCIMDResolver constructs a resolver for one issuer. Loopback destinations
// are allowed when the issuer is loopback development or for explicitly
// trusted development hostnames.
func NewCIMDResolver(issuer string, client *http.Client, trustedPrivateHosts, trustedLoopbackHosts []string) (*CIMDResolver, error) {
	parsed, err := url.Parse(issuer)
	if err != nil {
		return nil, err
	}
	allowLoopback := isLoopbackHost(parsed.Hostname())
	trustedHosts := make(map[string]struct{}, len(trustedPrivateHosts))
	for _, host := range trustedPrivateHosts {
		trustedHosts[normalizeCIMDHost(host)] = struct{}{}
	}
	trustedLoopback := make(map[string]struct{}, len(trustedLoopbackHosts))
	for _, host := range trustedLoopbackHosts {
		trustedLoopback[normalizeCIMDHost(host)] = struct{}{}
	}
	if client == nil {
		client = &http.Client{Transport: cimdTransport(allowLoopback, trustedHosts, trustedLoopback), Timeout: cimdLookupTimeout}
	} else {
		clone := *client
		client = &clone
		if client.Timeout == 0 || client.Timeout > cimdLookupTimeout {
			client.Timeout = cimdLookupTimeout
		}
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resolver := &CIMDResolver{client: client, allowLoopback: allowLoopback, slots: make(chan struct{}, maxCIMDLookups), cache: make(map[string]cachedClient)}
	resolver.validateDestination = func(ctx context.Context, host string) error {
		return validateCIMDDestination(ctx, host, allowLoopback, trustedHosts, trustedLoopback)
	}
	return resolver, nil
}

// Resolve fetches and validates one CIMD document, caching only valid results.
// A cache miss must acquire a slot without waiting before it can start DNS or
// HTTP work. One deadline covers the complete lookup, including the body read.
// Cache access removes expired entries; the entry cap also bounds idle retention.
func (r *CIMDResolver) Resolve(ctx context.Context, clientID string) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, cimdLookupTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parsed, err := validateClientIdentifierURL(clientID)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	if err := ctx.Err(); err != nil {
		r.mu.Unlock()
		return nil, err
	}
	r.removeExpiredLocked(time.Now())
	if cached, ok := r.cache[clientID]; ok {
		copy := *cached.client
		copy.Redirects = append([]string(nil), cached.client.Redirects...)
		r.mu.Unlock()
		return &copy, nil
	}
	r.mu.Unlock()

	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	default:
		return nil, errCIMDBusy
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.validateDestination(ctx, parsed.Hostname()); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(context.WithValue(ctx, cimdLookupContextKey{}, ctx), http.MethodGet, clientID, nil)
	if err != nil {
		return nil, fmt.Errorf("create CIMD request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	response, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch CIMD: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch CIMD: unexpected HTTP status %d", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !(mediaType == "application/json" || strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json")) {
		return nil, fmt.Errorf("fetch CIMD: response is not JSON")
	}
	limited := io.LimitReader(response.Body, maxCIMDBytes+1)
	data, err := io.ReadAll(limited)
	// A canceled peer can finish a partial body with EOF before the transport
	// reports cancellation. Preserve the caller's reason before decoding it.
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		return nil, fmt.Errorf("read CIMD: %w", err)
	}
	if len(data) > maxCIMDBytes {
		return nil, fmt.Errorf("read CIMD: document exceeds %d bytes", maxCIMDBytes)
	}
	var document cimdDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode CIMD: %w", err)
	}
	client, err := validateCIMD(clientID, parsed, document, r.allowLoopback)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if age, cache := cimdCacheAge(response.Header.Get("Cache-Control")); cache {
		r.mu.Lock()
		if err := ctx.Err(); err != nil {
			r.mu.Unlock()
			return nil, err
		}
		now := time.Now()
		r.removeExpiredLocked(now)
		if _, exists := r.cache[clientID]; !exists && len(r.cache) >= maxCIMDCacheEntries {
			var earliestID string
			var earliest time.Time
			for id, entry := range r.cache {
				if earliest.IsZero() || entry.expires.Before(earliest) {
					earliestID, earliest = id, entry.expires
				}
			}
			delete(r.cache, earliestID)
		}
		r.cache[clientID] = cachedClient{client: client, expires: now.Add(age)}
		r.mu.Unlock()
	}
	copy := *client
	copy.Redirects = append([]string(nil), client.Redirects...)
	return &copy, nil
}

// removeExpiredLocked removes stale entries even when a different client is
// requested. The caller must hold mu; at most maxCIMDCacheEntries are examined.
func (r *CIMDResolver) removeExpiredLocked(now time.Time) {
	for id, entry := range r.cache {
		if !now.Before(entry.expires) {
			delete(r.cache, id)
		}
	}
}

func validateClientIdentifierURL(raw string) (*url.URL, error) {
	if len(raw) > 2048 {
		return nil, fmt.Errorf("invalid CIMD client identifier URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path == "" || parsed.Path == "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid CIMD client identifier URL")
	}
	for _, segment := range strings.Split(strings.TrimPrefix(parsed.EscapedPath(), "/"), "/") {
		decoded, decodeErr := url.PathUnescape(segment)
		if decodeErr != nil || segment == "" || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, `/\`) {
			return nil, fmt.Errorf("invalid CIMD client identifier path")
		}
	}
	return parsed, nil
}

func validateCIMD(clientID string, identifier *url.URL, document cimdDocument, allowLoopback bool) (*Client, error) {
	if document.ClientID != clientID {
		return nil, fmt.Errorf("CIMD client_id does not match its URL")
	}
	if document.TokenEndpointAuthMethod != string(liboidc.AuthMethodNone) {
		return nil, fmt.Errorf("CIMD client must use token_endpoint_auth_method none")
	}
	if len(document.RedirectURIs) == 0 {
		return nil, fmt.Errorf("CIMD redirect_uris is required")
	}
	seen := make(map[string]struct{}, len(document.RedirectURIs))
	for _, raw := range document.RedirectURIs {
		if len(raw) > 2048 {
			return nil, fmt.Errorf("CIMD contains an invalid redirect URI")
		}
		if _, exists := seen[raw]; exists {
			return nil, fmt.Errorf("CIMD redirect_uris contains a duplicate")
		}
		seen[raw] = struct{}{}
		redirect, err := url.Parse(raw)
		if err != nil || redirect.Host == "" || redirect.User != nil || redirect.Fragment != "" || (redirect.Scheme != "https" && !(allowLoopback && redirect.Scheme == "http" && isLoopbackHost(redirect.Hostname()))) {
			return nil, fmt.Errorf("CIMD contains an invalid redirect URI")
		}
	}
	if len(document.GrantTypes) > 0 && !(len(document.GrantTypes) == 1 && document.GrantTypes[0] == string(liboidc.GrantTypeCode)) {
		return nil, fmt.Errorf("CIMD client supports an unsupported grant type")
	}
	if len(document.ResponseTypes) > 0 && !(len(document.ResponseTypes) == 1 && document.ResponseTypes[0] == string(liboidc.ResponseTypeCode)) {
		return nil, fmt.Errorf("CIMD client supports an unsupported response type")
	}
	if document.ClientURI != "" {
		clientURI, err := url.Parse(document.ClientURI)
		if err != nil || clientURI.Scheme != "https" || clientURI.Host == "" || !strings.EqualFold(clientURI.Host, identifier.Host) {
			return nil, fmt.Errorf("CIMD client_uri must share the client identifier origin")
		}
	}
	name := strings.TrimSpace(document.ClientName)
	if name == "" {
		name = identifier.Hostname()
	}
	if len(name) > 100 {
		return nil, fmt.Errorf("CIMD client_name exceeds 100 characters")
	}
	return &Client{
		IDValue: clientID, NameValue: name, DisplayHost: identifier.Hostname(),
		Redirects: append([]string(nil), document.RedirectURIs...), Method: liboidc.AuthMethodNone,
		Source: ClientSourceCIMD, Development: allowLoopback,
	}, nil
}

func cimdCacheAge(header string) (time.Duration, bool) {
	age := defaultCIMDAge
	for _, directive := range strings.Split(header, ",") {
		directive = strings.TrimSpace(directive)
		if strings.EqualFold(directive, "no-store") || strings.EqualFold(directive, "no-cache") {
			return 0, false
		}
		name, value, ok := strings.Cut(directive, "=")
		if ok && strings.EqualFold(strings.TrimSpace(name), "max-age") {
			seconds, err := strconv.ParseInt(strings.Trim(strings.TrimSpace(value), `"`), 10, 64)
			if err == nil {
				age = time.Duration(seconds) * time.Second
			}
		}
	}
	if age <= 0 {
		return 0, false
	}
	if age > maxCIMDCacheAge {
		age = maxCIMDCacheAge
	}
	return age, true
}

func cimdTransport(allowLoopback bool, trustedPrivateHosts, trustedLoopbackHosts map[string]struct{}) *http.Transport {
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			// Reuse must not let dial-time DNS continue after the admitted lookup.
			if lookup, ok := ctx.Value(cimdLookupContextKey{}).(context.Context); ok {
				ctx = lookup
			}
			ctx, cancel := context.WithTimeout(ctx, cimdLookupTimeout)
			defer cancel()
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := resolveCIMDAddresses(ctx, host, allowLoopback, trustedPrivateHosts, trustedLoopbackHosts)
			if err != nil {
				return nil, err
			}
			return dialCIMDAddresses(ctx, network, port, addresses, dialer.DialContext)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 3 * time.Second,
	}
}

func dialCIMDAddresses(
	ctx context.Context,
	network, port string,
	addresses []netip.Addr,
	dial func(context.Context, string, string) (net.Conn, error),
) (net.Conn, error) {
	var dialErrors []error
	for _, address := range addresses {
		connection, err := dial(ctx, network, net.JoinHostPort(address.String(), port))
		if err == nil {
			return connection, nil
		}
		dialErrors = append(dialErrors, err)
	}
	return nil, errors.Join(dialErrors...)
}

func validateCIMDDestination(ctx context.Context, host string, allowLoopback bool, trustedPrivateHosts, trustedLoopbackHosts map[string]struct{}) error {
	_, err := resolveCIMDAddresses(ctx, host, allowLoopback, trustedPrivateHosts, trustedLoopbackHosts)
	return err
}

func resolveCIMDAddresses(ctx context.Context, host string, allowLoopback bool, trustedPrivateHosts, trustedLoopbackHosts map[string]struct{}) ([]netip.Addr, error) {
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("resolve CIMD destination")
	}
	_, trustPrivate := trustedPrivateHosts[normalizeCIMDHost(host)]
	_, trustLoopback := trustedLoopbackHosts[normalizeCIMDHost(host)]
	for _, address := range addresses {
		if !cimdAddressAllowed(address, allowLoopback, trustPrivate, trustLoopback) {
			return nil, fmt.Errorf("CIMD destination resolves to a special-use address")
		}
	}
	return addresses, nil
}

func cimdAddressAllowed(address netip.Addr, allowLoopback, trustPrivate, trustLoopback bool) bool {
	return !blockedCIMDAddress(address) || (allowLoopback || trustLoopback) && address.IsLoopback() || trustPrivate && address.IsPrivate()
}

func normalizeCIMDHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

func blockedCIMDAddress(address netip.Addr) bool {
	if !address.IsValid() || address.IsUnspecified() || address.IsLoopback() || address.IsMulticast() || address.IsLinkLocalUnicast() || address.IsPrivate() {
		return true
	}
	for _, prefix := range specialUsePrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// specialUsePrefixes complements netip's address classifications with the
// IANA IPv4 and IPv6 special-purpose registries. CIMD fetches are an SSRF
// boundary, so even globally reachable special-purpose assignments are denied.
// Keep this list aligned with:
// https://www.iana.org/assignments/iana-ipv4-special-registry
// https://www.iana.org/assignments/iana-ipv6-special-registry
var specialUsePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"), netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.175.48.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::ffff:0:0/96"), netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("100:0:0:1::/64"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2620:4f:8000::/48"), netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback()
}
