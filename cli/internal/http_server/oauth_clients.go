package http_server

import (
	"context"
	"encoding/json"
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

	"hmans.de/chatto/internal/config"
)

const (
	maxOAuthClientMetadataBytes = 5 << 10
	maxOAuthClientCacheAge      = 5 * time.Minute
	defaultOAuthClientCacheAge  = time.Minute
	maxOAuthClientCacheEntries  = 256
)

// OAuthClient is the validated identity and redirect contract for one public
// OAuth client. Metadata is informational; ClientID and RedirectURIs are the
// security-sensitive fields.
type OAuthClient struct {
	ClientID     string
	ClientName   string
	ClientURI    string
	RedirectURIs []string
	BuiltIn      bool
}

func (c OAuthClient) allowsRedirectURI(candidate string) bool {
	for _, redirectURI := range c.RedirectURIs {
		if redirectURI == candidate {
			return true
		}
	}
	return false
}

type cachedOAuthClient struct {
	client  OAuthClient
	expires time.Time
}

// OAuthClientResolver safely retrieves public-client metadata from CIMD
// Client Identifier URLs. Valid documents are cached briefly according to
// their response headers.
type OAuthClientResolver struct {
	client              *http.Client
	allowLoopback       bool
	timeout             time.Duration
	slots               chan struct{}
	validateDestination func(context.Context, string) error

	mu    sync.Mutex
	cache map[string]cachedOAuthClient
}

func newOAuthClientResolver(serverURL string, client *http.Client) (*OAuthClientResolver, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("parse webserver URL for OAuth client resolver: %w", err)
	}
	allowLoopback := isLoopbackOAuthRedirectHost(parsed.Hostname())
	if client == nil {
		client = &http.Client{Transport: oauthClientMetadataTransport(allowLoopback), Timeout: 5 * time.Second}
	} else {
		clone := *client
		client = &clone
		if client.Timeout == 0 || client.Timeout > 5*time.Second {
			client.Timeout = 5 * time.Second
		}
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resolver := &OAuthClientResolver{
		client:        client,
		allowLoopback: allowLoopback,
		timeout:       client.Timeout,
		slots:         make(chan struct{}, 8),
		cache:         make(map[string]cachedOAuthClient),
	}
	resolver.validateDestination = func(ctx context.Context, host string) error {
		_, err := resolveOAuthClientAddresses(ctx, host, allowLoopback)
		return err
	}
	return resolver, nil
}

func (r *OAuthClientResolver) Resolve(ctx context.Context, clientID string) (OAuthClient, error) {
	now := time.Now()
	r.mu.Lock()
	r.pruneExpiredLocked(now)
	if cached, ok := r.cache[clientID]; ok && now.Before(cached.expires) {
		client := cloneOAuthClient(cached.client)
		r.mu.Unlock()
		return client, nil
	}
	delete(r.cache, clientID)
	r.mu.Unlock()

	identifier, err := validateOAuthClientIdentifierURL(clientID, r.allowLoopback)
	if err != nil {
		return OAuthClient{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	case <-ctx.Done():
		return OAuthClient{}, ctx.Err()
	}
	if err := r.validateDestination(ctx, identifier.Hostname()); err != nil {
		return OAuthClient{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clientID, nil)
	if err != nil {
		return OAuthClient{}, fmt.Errorf("create CIMD request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	response, err := r.client.Do(req)
	if err != nil {
		return OAuthClient{}, fmt.Errorf("fetch CIMD: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return OAuthClient{}, fmt.Errorf("fetch CIMD: unexpected HTTP status %d", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !(mediaType == "application/json" || strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json")) {
		return OAuthClient{}, fmt.Errorf("fetch CIMD: response is not JSON")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxOAuthClientMetadataBytes+1))
	if err != nil {
		return OAuthClient{}, fmt.Errorf("read CIMD: %w", err)
	}
	if len(data) > maxOAuthClientMetadataBytes {
		return OAuthClient{}, fmt.Errorf("read CIMD: document exceeds %d bytes", maxOAuthClientMetadataBytes)
	}
	var document cimdDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return OAuthClient{}, fmt.Errorf("decode CIMD: %w", err)
	}
	client, err := validateOAuthClientMetadata(clientID, identifier, document, r.allowLoopback)
	if err != nil {
		return OAuthClient{}, err
	}
	if age, cache := oauthClientCacheAge(response.Header.Get("Cache-Control")); cache {
		r.cacheClient(clientID, client, now.Add(age), now)
	}
	return cloneOAuthClient(client), nil
}

func (r *OAuthClientResolver) cacheClient(clientID string, client OAuthClient, expires, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneExpiredLocked(now)
	if _, exists := r.cache[clientID]; !exists && len(r.cache) >= maxOAuthClientCacheEntries {
		var oldestID string
		var oldestExpiry time.Time
		for id, cached := range r.cache {
			if oldestID == "" || cached.expires.Before(oldestExpiry) {
				oldestID = id
				oldestExpiry = cached.expires
			}
		}
		delete(r.cache, oldestID)
	}
	r.cache[clientID] = cachedOAuthClient{client: cloneOAuthClient(client), expires: expires}
}

func (r *OAuthClientResolver) pruneExpiredLocked(now time.Time) {
	for clientID, cached := range r.cache {
		if !now.Before(cached.expires) {
			delete(r.cache, clientID)
		}
	}
}

func cloneOAuthClient(client OAuthClient) OAuthClient {
	client.RedirectURIs = append([]string(nil), client.RedirectURIs...)
	return client
}

func validateOAuthClientIdentifierURL(raw string, allowLoopback bool) (*url.URL, error) {
	if len(raw) > 2048 {
		return nil, fmt.Errorf("invalid CIMD client identifier URL")
	}
	parsed, err := url.Parse(raw)
	validScheme := parsed != nil && (parsed.Scheme == "https" || allowLoopback && parsed.Scheme == "http" && isLoopbackOAuthRedirectHost(parsed.Hostname()))
	if err != nil || !validScheme || parsed.Host == "" || parsed.User != nil || parsed.Path == "" || parsed.Path == "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
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

func validateOAuthClientMetadata(clientID string, identifier *url.URL, document cimdDocument, allowLoopback bool) (OAuthClient, error) {
	if document.ClientID != clientID {
		return OAuthClient{}, fmt.Errorf("CIMD client_id does not match its URL")
	}
	if document.TokenEndpointAuthMethod != "none" {
		return OAuthClient{}, fmt.Errorf("CIMD client must use token_endpoint_auth_method none")
	}
	if document.ApplicationType != "" && document.ApplicationType != "web" && document.ApplicationType != "native" {
		return OAuthClient{}, fmt.Errorf("CIMD client has an unsupported application_type")
	}
	if len(document.RedirectURIs) == 0 {
		return OAuthClient{}, fmt.Errorf("CIMD redirect_uris is required")
	}
	seen := make(map[string]struct{}, len(document.RedirectURIs))
	for _, raw := range document.RedirectURIs {
		if len(raw) > 2048 {
			return OAuthClient{}, fmt.Errorf("CIMD contains an invalid redirect URI")
		}
		if _, exists := seen[raw]; exists {
			return OAuthClient{}, fmt.Errorf("CIMD redirect_uris contains a duplicate")
		}
		seen[raw] = struct{}{}
		redirect, err := url.Parse(raw)
		if err != nil || !validOAuthClientRedirectURI(redirect, document.ApplicationType, allowLoopback) {
			return OAuthClient{}, fmt.Errorf("CIMD contains an invalid redirect URI")
		}
	}
	if len(document.GrantTypes) > 0 && !(len(document.GrantTypes) == 1 && document.GrantTypes[0] == "authorization_code") {
		return OAuthClient{}, fmt.Errorf("CIMD client supports an unsupported grant type")
	}
	if len(document.ResponseTypes) > 0 && !(len(document.ResponseTypes) == 1 && document.ResponseTypes[0] == "code") {
		return OAuthClient{}, fmt.Errorf("CIMD client supports an unsupported response type")
	}
	clientURIValue := ""
	if document.ClientURI != "" {
		clientURI, err := url.Parse(document.ClientURI)
		validScheme := clientURI != nil && (clientURI.Scheme == "https" || allowLoopback && clientURI.Scheme == "http" && isLoopbackOAuthRedirectHost(clientURI.Hostname()))
		if err != nil || !validScheme || clientURI.Host == "" || clientURI.User != nil || clientURI.Fragment != "" || canonicalOrigin(clientURI) != canonicalOrigin(identifier) {
			return OAuthClient{}, fmt.Errorf("CIMD client_uri must share the client identifier origin")
		}
		clientURIValue = canonicalOrigin(clientURI)
	}
	name := strings.TrimSpace(document.ClientName)
	if name == "" {
		name = identifier.Hostname()
	}
	if len(name) > 100 {
		return OAuthClient{}, fmt.Errorf("CIMD client_name exceeds 100 characters")
	}
	return OAuthClient{ClientID: clientID, ClientName: name, ClientURI: clientURIValue, RedirectURIs: append([]string(nil), document.RedirectURIs...)}, nil
}

func validOAuthClientRedirectURI(redirect *url.URL, applicationType string, allowLoopback bool) bool {
	if redirect == nil || redirect.Scheme == "" || redirect.User != nil || redirect.Fragment != "" {
		return false
	}
	if redirect.Scheme == "https" {
		return redirect.Host != ""
	}
	if redirect.Scheme == "http" {
		return allowLoopback && redirect.Host != "" && isLoopbackOAuthRedirectHost(redirect.Hostname())
	}
	if applicationType != "native" || redirect.Opaque != "" {
		return false
	}
	scheme := strings.ToLower(redirect.Scheme)
	return strings.Contains(scheme, ".") && scheme != "file" && scheme != "data" && scheme != "javascript"
}

func oauthClientCacheAge(header string) (time.Duration, bool) {
	age := defaultOAuthClientCacheAge
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
	if age > maxOAuthClientCacheAge {
		age = maxOAuthClientCacheAge
	}
	return age, true
}

func oauthClientMetadataTransport(allowLoopback bool) *http.Transport {
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := resolveOAuthClientAddresses(ctx, host, allowLoopback)
			if err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
		},
		ForceAttemptHTTP2: true, MaxIdleConns: 16, MaxIdleConnsPerHost: 2,
		IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second,
	}
}

func resolveOAuthClientAddresses(ctx context.Context, host string, allowLoopback bool) ([]netip.Addr, error) {
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("resolve CIMD destination")
	}
	for _, address := range addresses {
		if blockedOAuthClientAddress(address) && !(allowLoopback && address.IsLoopback()) {
			return nil, fmt.Errorf("CIMD destination resolves to a special-use address")
		}
	}
	return addresses, nil
}

func blockedOAuthClientAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || address.IsUnspecified() || address.IsLoopback() || address.IsMulticast() || address.IsLinkLocalUnicast() || address.IsPrivate() {
		return true
	}
	for _, prefix := range oauthClientSpecialUsePrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

var oauthClientSpecialUsePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"), netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.175.48.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::ffff:0:0:0/96"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2620:4f:8000::/48"), netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
}

func (s *HTTPServer) resolveOAuthClient(ctx context.Context, clientID string) (OAuthClient, error) {
	if s.oauthClientResolveHook != nil {
		if client, handled, err := s.oauthClientResolveHook(ctx, clientID); handled {
			return client, err
		}
	}
	if clientID == config.ChattoDesktopOrigin {
		return OAuthClient{
			ClientID: clientID, ClientName: "Chatto Desktop", ClientURI: config.ChattoDesktopOrigin,
			RedirectURIs: []string{config.ChattoDesktopOrigin + config.ChattoDesktopOAuthCallbackPath + "?mode=popup"}, BuiltIn: true,
		}, nil
	}
	if s.oauthClientResolver == nil {
		return OAuthClient{}, fmt.Errorf("OAuth client metadata resolution is unavailable")
	}
	return s.oauthClientResolver.Resolve(ctx, clientID)
}
