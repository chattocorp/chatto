package http_server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

type oauthRoundTripFunc func(*http.Request) (*http.Response, error)

func (f oauthRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestOAuthClientResolverFetchesValidCIMDAndCachesIt(t *testing.T) {
	var requests atomic.Int32
	var clientID string
	var metadataOrigin string
	metadataServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_ = json.NewEncoder(w).Encode(cimdDocument{
			ClientID: clientID, ClientName: "Example Client", ClientURI: metadataOrigin,
			ApplicationType: "web", RedirectURIs: []string{metadataOrigin + "/callback"},
			TokenEndpointAuthMethod: "none", GrantTypes: []string{"authorization_code"}, ResponseTypes: []string{"code"},
		})
	}))
	defer metadataServer.Close()
	metadataOrigin = metadataServer.URL
	clientID = metadataServer.URL + "/oauth/client-metadata.json"

	resolver, err := newOAuthClientResolver("http://localhost:4000", metadataServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	first, err := resolver.Resolve(context.Background(), clientID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolver.Resolve(context.Background(), clientID)
	if err != nil {
		t.Fatal(err)
	}
	if first.ClientID != clientID || first.ClientName != "Example Client" || !first.allowsRedirectURI(metadataServer.URL+"/callback") {
		t.Fatalf("client = %#v", first)
	}
	if second.ClientID != first.ClientID || requests.Load() != 1 {
		t.Fatalf("cached client = %#v, requests = %d", second, requests.Load())
	}
}

func TestOAuthClientResolverCacheIsBoundedAndPrunesExpiredEntries(t *testing.T) {
	resolver, err := newOAuthClientResolver("https://chatto.example", &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	resolver.cacheClient("expired", OAuthClient{ClientID: "expired"}, now.Add(-time.Second), now.Add(-2*time.Second))
	for index := range maxOAuthClientCacheEntries + 20 {
		clientID := "client-" + strconv.Itoa(index)
		resolver.cacheClient(clientID, OAuthClient{ClientID: clientID}, now.Add(time.Minute+time.Duration(index)*time.Second), now)
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if len(resolver.cache) != maxOAuthClientCacheEntries {
		t.Fatalf("cache entries = %d, want %d", len(resolver.cache), maxOAuthClientCacheEntries)
	}
	if _, exists := resolver.cache["expired"]; exists {
		t.Fatal("expired entry was retained")
	}
}

func TestValidateOAuthClientMetadataRequiresExactIdentityAndRedirects(t *testing.T) {
	identifier, err := url.Parse("https://client.example/oauth/metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	valid := cimdDocument{
		ClientID: "https://client.example/oauth/metadata.json", ClientName: "Client", ClientURI: "https://client.example",
		ApplicationType: "web", RedirectURIs: []string{"https://client.example/callback?mode=popup"},
		TokenEndpointAuthMethod: "none", GrantTypes: []string{"authorization_code"}, ResponseTypes: []string{"code"},
	}
	client, err := validateOAuthClientMetadata(valid.ClientID, identifier, valid, false)
	if err != nil {
		t.Fatal(err)
	}
	if !client.allowsRedirectURI(valid.RedirectURIs[0]) || client.allowsRedirectURI("https://client.example/callback") {
		t.Fatalf("redirect matching was not exact: %#v", client.RedirectURIs)
	}

	withPrivateClientURIData := valid
	withPrivateClientURIData.ClientURI = "https://client.example/products/chatto?account=person@example.com&token=secret"
	client, err = validateOAuthClientMetadata(valid.ClientID, identifier, withPrivateClientURIData, false)
	if err != nil {
		t.Fatal(err)
	}
	if client.ClientURI != "https://client.example" {
		t.Fatalf("client URI = %q, want privacy-safe origin", client.ClientURI)
	}

	mismatch := valid
	mismatch.ClientID = "https://other.example/oauth/metadata.json"
	if _, err := validateOAuthClientMetadata(valid.ClientID, identifier, mismatch, false); err == nil {
		t.Fatal("accepted mismatched client_id")
	}
	mismatch = valid
	mismatch.RedirectURIs = []string{"http://client.example/callback"}
	if _, err := validateOAuthClientMetadata(valid.ClientID, identifier, mismatch, false); err == nil {
		t.Fatal("accepted insecure web redirect URI")
	}
}

func TestValidateOAuthClientMetadataSupportsNativeAppRedirect(t *testing.T) {
	identifier, _ := url.Parse("https://mobile.example/oauth/metadata.json")
	document := cimdDocument{
		ClientID: "https://mobile.example/oauth/metadata.json", ClientName: "Mobile App", ClientURI: "https://mobile.example",
		ApplicationType: "native", RedirectURIs: []string{"com.example.chatto:/oauth/callback"}, TokenEndpointAuthMethod: "none",
	}
	client, err := validateOAuthClientMetadata(document.ClientID, identifier, document, false)
	if err != nil {
		t.Fatal(err)
	}
	if !client.allowsRedirectURI("com.example.chatto:/oauth/callback") {
		t.Fatalf("client = %#v", client)
	}
}

func TestValidateOAuthClientMetadataSupportsNativeLoopbackRedirectsOnRemoteServer(t *testing.T) {
	identifier, _ := url.Parse("https://native.example/oauth/metadata.json")
	for _, redirectURI := range []string{
		"http://127.0.0.1:49152/oauth/callback",
		"http://[::1]:49152/oauth/callback",
		"http://localhost:49152/oauth/callback",
		"http://inspector.feature.localhost:49152/oauth/callback",
	} {
		t.Run(redirectURI, func(t *testing.T) {
			document := cimdDocument{
				ClientID: identifier.String(), ApplicationType: "native",
				RedirectURIs: []string{redirectURI}, TokenEndpointAuthMethod: "none",
			}
			client, err := validateOAuthClientMetadata(document.ClientID, identifier, document, false)
			if err != nil {
				t.Fatalf("validateOAuthClientMetadata: %v", err)
			}
			if !client.Native {
				t.Fatal("validated native client lost its application type")
			}
			if !client.allowsRedirectURI(redirectURI) {
				t.Fatalf("client does not allow registered redirect %q", redirectURI)
			}
		})
	}
}

func TestValidateOAuthClientMetadataRestrictsHTTPLoopbackRedirects(t *testing.T) {
	identifier, _ := url.Parse("https://client.example/oauth/metadata.json")
	tests := []struct {
		name            string
		applicationType string
		redirectURI     string
		allowLoopback   bool
		wantValid       bool
	}{
		{name: "remote web client", applicationType: "web", redirectURI: "http://localhost:3000/callback"},
		{name: "local web development", applicationType: "web", redirectURI: "http://app.feature.localhost:3000/callback", allowLoopback: true, wantValid: true},
		{name: "public HTTP host", applicationType: "native", redirectURI: "http://client.example/callback"},
		{name: "localhost lookalike", applicationType: "native", redirectURI: "http://localhost.example/callback"},
		{name: "wildcard localhost", applicationType: "native", redirectURI: "http://*.localhost:3000/callback"},
		{name: "empty localhost label", applicationType: "native", redirectURI: "http://tool..localhost:3000/callback"},
		{name: "invalid localhost label", applicationType: "native", redirectURI: "http://tool_name.localhost:3000/callback"},
		{name: "wildcard HTTPS", applicationType: "web", redirectURI: "https://*.example/callback"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document := cimdDocument{
				ClientID: identifier.String(), ApplicationType: tt.applicationType,
				RedirectURIs: []string{tt.redirectURI}, TokenEndpointAuthMethod: "none",
			}
			client, err := validateOAuthClientMetadata(document.ClientID, identifier, document, tt.allowLoopback)
			if (err == nil) != tt.wantValid {
				t.Fatalf("validateOAuthClientMetadata error = %v, wantValid = %v", err, tt.wantValid)
			}
			if err == nil && client.Native != (tt.applicationType == "native") {
				t.Fatalf("client.Native = %v, application_type = %q", client.Native, tt.applicationType)
			}
		})
	}
}

func TestOAuthClientNativeLoopbackIPRedirectUsesVariablePortOnly(t *testing.T) {
	client := OAuthClient{Native: true, RedirectURIs: []string{
		"http://127.0.0.1:41000/oauth/callback?source=codex",
		"http://[::1]:41000/oauth/callback",
		"http://inspector.feature.localhost:41000/oauth/callback",
	}}
	tests := []struct {
		name      string
		candidate string
		want      bool
	}{
		{name: "IPv4 different port", candidate: "http://127.0.0.1:52000/oauth/callback?source=codex", want: true},
		{name: "IPv6 different port", candidate: "http://[::1]:52000/oauth/callback", want: true},
		{name: "different IPv4 path", candidate: "http://127.0.0.1:52000/other?source=codex"},
		{name: "different IPv4 query", candidate: "http://127.0.0.1:52000/oauth/callback?source=other"},
		{name: "named localhost different port", candidate: "http://inspector.feature.localhost:52000/oauth/callback"},
		{name: "named localhost exact", candidate: "http://inspector.feature.localhost:41000/oauth/callback", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := client.allowsRedirectURI(tt.candidate); got != tt.want {
				t.Fatalf("allowsRedirectURI(%q) = %v, want %v", tt.candidate, got, tt.want)
			}
		})
	}
}

func TestOAuthClientWebLoopbackIPRedirectRequiresExactPort(t *testing.T) {
	client := OAuthClient{RedirectURIs: []string{"http://127.0.0.1:41000/oauth/callback"}}
	if client.allowsRedirectURI("http://127.0.0.1:52000/oauth/callback") {
		t.Fatal("web client received the native variable-port exception")
	}
	if !client.allowsRedirectURI("http://127.0.0.1:41000/oauth/callback") {
		t.Fatal("web client could not use its exact registered callback")
	}
}

func TestOAuthClientResolverRecognizesConcreteLocalhostDevelopmentHost(t *testing.T) {
	resolver, err := newOAuthClientResolver("https://chatto.feature.localhost:42444", &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	if !resolver.allowLoopback {
		t.Fatal("concrete .localhost server URL did not enable loopback development metadata")
	}
	if _, err := validateOAuthClientIdentifierURL("http://client.feature.localhost/oauth/metadata.json", resolver.allowLoopback); err != nil {
		t.Fatalf("local development CIMD URL was rejected: %v", err)
	}
	if _, err := validateOAuthClientIdentifierURL("http://client.feature.localhost.example/oauth/metadata.json", resolver.allowLoopback); err == nil {
		t.Fatal("localhost lookalike CIMD URL was accepted")
	}
}

func TestOAuthClientMetadataBlocksSpecialUseAddresses(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.0.0.1", "169.254.169.254", "192.0.2.1",
		"192.31.196.1", "192.52.193.1", "192.88.99.1", "192.175.48.1",
		"::1", "::ffff:0:0:1", "64:ff9b::1", "64:ff9b:1::1", "100::1", "2001::1",
		"2001:db8::1", "2002::1", "2620:4f:8000::1", "3fff::1", "5f00::1",
	}
	for _, raw := range blocked {
		address := netip.MustParseAddr(raw)
		if !blockedOAuthClientAddress(address) {
			t.Fatalf("special-use address %s was accepted", raw)
		}
	}
	if blockedOAuthClientAddress(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("public address was blocked")
	}
	if blockedOAuthClientAddress(netip.MustParseAddr("2606:4700:4700::1111")) {
		t.Fatal("public IPv6 address was blocked")
	}
}

func TestOAuthClientMetadataAllowsLoopbackResolutionOnlyForLocalHostname(t *testing.T) {
	loopback := netip.MustParseAddr("127.0.0.1")
	if !allowedOAuthClientAddress("client.feature.localhost", loopback, true) {
		t.Fatal("local development hostname could not resolve to loopback")
	}
	if allowedOAuthClientAddress("client.example", loopback, true) {
		t.Fatal("public-looking metadata hostname could resolve to loopback")
	}
	if allowedOAuthClientAddress("client.feature.localhost", loopback, false) {
		t.Fatal("remote Chatto server could resolve local metadata")
	}
	publicAddress := netip.MustParseAddr("8.8.8.8")
	if allowedOAuthClientAddress("client.feature.localhost", publicAddress, true) {
		t.Fatal("local metadata hostname could resolve to a public address")
	}
	if !allowedOAuthClientAddress("client.example", publicAddress, false) {
		t.Fatal("public metadata address was blocked")
	}
}

func TestOAuthClientResolverDeadlineIncludesDestinationValidation(t *testing.T) {
	releaseValidation := make(chan struct{})
	client := &http.Client{
		Timeout: 25 * time.Millisecond,
		Transport: oauthRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("HTTP fetch should not be reached")
		}),
	}
	resolver, err := newOAuthClientResolver("https://chatto.example", client)
	if err != nil {
		t.Fatal(err)
	}
	resolver.validateDestination = func(ctx context.Context, _ string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-releaseValidation:
			return nil
		}
	}

	result := make(chan error, 1)
	go func() {
		_, resolveErr := resolver.Resolve(context.Background(), "https://client.example/oauth/metadata.json")
		result <- resolveErr
	}()

	select {
	case resolveErr := <-result:
		if !errors.Is(resolveErr, context.DeadlineExceeded) {
			t.Fatalf("Resolve error = %v, want deadline exceeded", resolveErr)
		}
	case <-time.After(250 * time.Millisecond):
		close(releaseValidation)
		<-result
		t.Fatal("destination validation outlived the resolver timeout")
	}
}

func TestResolveOAuthClientProvidesExactDesktopRegistration(t *testing.T) {
	server := &HTTPServer{}
	client, err := server.resolveOAuthClient(context.Background(), "chatto://desktop")
	if err != nil {
		t.Fatal(err)
	}
	if !client.BuiltIn || client.ClientName != "Chatto Desktop" || !client.allowsRedirectURI("chatto://desktop/servers/callback?mode=popup") {
		t.Fatalf("client = %#v", client)
	}
}
