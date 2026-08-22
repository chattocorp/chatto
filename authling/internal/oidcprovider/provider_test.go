package oidcprovider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	liboidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
	"hmans.de/authling/internal/config"
)

func TestAccessTokenCryptoAcceptsLegacyTokensAndUsesStableKey(t *testing.T) {
	var tokenKey, legacyKey [32]byte
	copy(tokenKey[:], bytes.Repeat([]byte{1}, 32))
	copy(legacyKey[:], bytes.Repeat([]byte{2}, 32))
	crypto := accessTokenCrypto(tokenKey, legacyKey, "sig_legacy")

	for name, legacy := range map[string]op.Crypto{
		"GCM": op.NewAES256GCMCrypto(legacyKey, "sig_legacy"),
		"AES": op.NewAESCrypto(legacyKey),
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := legacy.Encrypt("token:subject")
			if err != nil {
				t.Fatal(err)
			}
			if got, err := crypto.Decrypt(raw); err != nil || got != "token:subject" {
				t.Fatalf("decrypt legacy token = %q, %v", got, err)
			}
		})
	}

	raw, err := crypto.Encrypt("new:subject")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := op.NewAES256GCMCrypto(tokenKey, "authling-oidc-token-v1").Decrypt(raw); err != nil || got != "new:subject" {
		t.Fatalf("new token did not use stable key: %q, %v", got, err)
	}
	if _, err := op.NewAES256GCMCrypto(legacyKey, "sig_legacy").Decrypt(raw); err == nil {
		t.Fatal("new token remained encrypted with legacy signing-derived key")
	}

	migrating := accessTokenCrypto(legacyKey, legacyKey, "sig_legacy")
	raw, err = migrating.Encrypt("rolling:subject")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := op.NewAES256GCMCrypto(legacyKey, "sig_legacy").Decrypt(raw); err != nil || got != "rolling:subject" {
		t.Fatalf("legacy replica could not read seeded stable token: %q, %v", got, err)
	}
}

func TestJWKSCacheControlDependsOnResponseStatus(t *testing.T) {
	tests := []struct {
		name       string
		serve      http.HandlerFunc
		wantStatus int
		wantCache  string
	}{
		{
			name: "successful response is public",
			serve: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"keys":[]}`))
			},
			wantStatus: http.StatusOK,
			wantCache:  "public, max-age=300",
		},
		{
			name: "failed response is not stored",
			serve: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Cache-Control", "public, max-age=300")
				http.Error(w, "key vault unavailable", http.StatusInternalServerError)
			},
			wantStatus: http.StatusInternalServerError,
			wantCache:  "no-store",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := (&Service{}).wrap(test.serve)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://auth.example/oauth/jwks", nil))
			if response.Code != test.wantStatus || response.Header().Get("Cache-Control") != test.wantCache {
				t.Fatalf("JWKS status/cache = %d %q, want %d %q", response.Code, response.Header().Get("Cache-Control"), test.wantStatus, test.wantCache)
			}
		})
	}
}

func TestValidateAuthorizeRequestRequiresExactCodePKCEProfile(t *testing.T) {
	valid := "https://auth.example/oauth/authorize?client_id=client&redirect_uri=https%3A%2F%2Fclient.example%2Fcallback&response_type=code&scope=openid&code_challenge=" + strings.Repeat("a", 43) + "&code_challenge_method=S256"
	tests := []struct {
		name string
		want bool
	}{
		{name: "valid", want: true},
		{name: "account data"},
		{name: "account data first"},
		{name: "missing PKCE"},
		{name: "plain PKCE"},
		{name: "extra scope"},
		{name: "duplicate scope"},
		{name: "account data without openid"},
		{name: "prompt none"},
		{name: "prompt login"},
		{name: "form post"},
		{name: "max age"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := valid
			switch test.name {
			case "missing PKCE":
				raw = strings.ReplaceAll(raw, "&code_challenge="+strings.Repeat("a", 43), "")
			case "plain PKCE":
				raw = strings.ReplaceAll(raw, "code_challenge_method=S256", "code_challenge_method=plain")
			case "extra scope":
				raw = strings.ReplaceAll(raw, "scope=openid", "scope=openid%20email")
			case "account data":
				raw = strings.ReplaceAll(raw, "scope=openid", "scope=openid%20account_data")
			case "account data first":
				raw = strings.ReplaceAll(raw, "scope=openid", "scope=account_data%20openid")
			case "duplicate scope":
				raw = strings.ReplaceAll(raw, "scope=openid", "scope=openid%20openid")
			case "account data without openid":
				raw = strings.ReplaceAll(raw, "scope=openid", "scope=account_data")
			case "prompt none":
				raw += "&prompt=none"
			case "prompt login":
				raw += "&prompt=login"
			case "form post":
				raw += "&response_mode=form_post"
			case "max age":
				raw += "&max_age=0"
			}
			req := httptest.NewRequest(http.MethodGet, raw, nil)
			if got := validateAuthorizeRequest(req) == nil; got != test.want {
				t.Fatalf("valid = %v, want %v", got, test.want)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestCIMDResolverFetchesBoundsAndCachesValidDocuments(t *testing.T) {
	clientID := "https://client.example/metadata.json"
	document := `{"client_id":"` + clientID + `","client_name":"Client","redirect_uris":["https://client.example/callback"],"token_endpoint_auth_method":"none"}`
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}, "Cache-Control": {"max-age=60"}}, Body: io.NopCloser(strings.NewReader(document)), Request: request}, nil
	})}
	resolver, err := NewCIMDResolver("https://auth.example", client, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolver.validateDestination = func(context.Context, string) error { return nil }
	for range 2 {
		if _, err := resolver.Resolve(context.Background(), clientID); err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("CIMD fetches = %d, want one cached fetch", requests.Load())
	}

	client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(make([]byte, maxCIMDBytes+1))), Request: request}, nil
	})
	uncached, _ := NewCIMDResolver("https://auth.example", client, nil, nil)
	uncached.validateDestination = func(context.Context, string) error { return nil }
	if _, err := uncached.Resolve(context.Background(), clientID); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized CIMD error = %v", err)
	}
}

func TestCIMDResolverBoundsConcurrentFetches(t *testing.T) {
	var concurrent, maximum atomic.Int32
	release := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		current := concurrent.Add(1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		defer concurrent.Add(-1)
		select {
		case <-release:
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
		document := `{"client_id":"` + request.URL.String() + `","redirect_uris":["https://client.example/callback"],"token_endpoint_auth_method":"none"}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}, "Cache-Control": {"no-store"}}, Body: io.NopCloser(strings.NewReader(document)), Request: request}, nil
	})}
	resolver, _ := NewCIMDResolver("https://auth.example", client, nil, nil)
	resolver.validateDestination = func(context.Context, string) error { return nil }
	errors := make(chan error, 9)
	for index := range 9 {
		go func() {
			_, err := resolver.Resolve(context.Background(), fmt.Sprintf("https://client.example/metadata-%d.json", index))
			errors <- err
		}()
	}
	deadline := time.Now().Add(time.Second)
	for maximum.Load() < 8 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if maximum.Load() != 8 {
		close(release)
		t.Fatalf("maximum concurrent CIMD fetches = %d, want 8", maximum.Load())
	}
	close(release)
	for range 9 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	if maximum.Load() > 8 {
		t.Fatalf("maximum concurrent CIMD fetches = %d", maximum.Load())
	}
}

func TestPKCEValuesUseTheRFC7636UnreservedAlphabet(t *testing.T) {
	for _, valid := range []string{strings.Repeat("a", 43), strings.Repeat("Z", 128), strings.Repeat("-._~", 11)} {
		if !validPKCEValue(valid) {
			t.Fatalf("valid PKCE value %q was rejected", valid)
		}
	}
	for _, invalid := range []string{strings.Repeat("a", 42), strings.Repeat("a", 129), strings.Repeat("!", 43), strings.Repeat("a", 42) + "="} {
		if validPKCEValue(invalid) {
			t.Fatalf("invalid PKCE value %q was accepted", invalid)
		}
	}
}

func TestResolverSupportsConventionalPublicAndBasicClients(t *testing.T) {
	cfg := config.Config{HTTP: config.HTTPConfig{PublicURL: "https://auth.example"}, OIDC: config.OIDCConfig{Clients: []config.OIDCClientConfig{
		{ID: "public", Name: "Public", RedirectURIs: []string{"https://public.example/callback"}},
		{ID: "confidential", Name: "Confidential", Secret: strings.Repeat("secret", 6), RedirectURIs: []string{"https://confidential.example/callback"}},
	}}}
	resolver := NewResolver(cfg, nil)
	public, err := resolver.Resolve(context.Background(), "public")
	if err != nil || public.AuthMethod() != liboidc.AuthMethodNone {
		t.Fatalf("public client = %+v, %v", public, err)
	}
	confidential, err := resolver.Resolve(context.Background(), "confidential")
	if err != nil || confidential.AuthMethod() != liboidc.AuthMethodBasic {
		t.Fatalf("confidential client = %+v, %v", confidential, err)
	}
	if err := resolver.AuthorizeSecret(context.Background(), "confidential", strings.Repeat("secret", 6)); err != nil {
		t.Fatalf("authorize secret: %v", err)
	}
	if err := resolver.AuthorizeSecret(context.Background(), "confidential", strings.Repeat("wrong!", 6)); err == nil {
		t.Fatal("wrong secret was accepted")
	}
	if err := resolver.AuthorizeSecret(context.Background(), "public", "anything"); err == nil {
		t.Fatal("public client accepted a secret")
	}
}

func TestValidateAuthorizeRequestRejectsDuplicateSecurityParameters(t *testing.T) {
	raw := "https://auth.example/oauth/authorize?client_id=one&client_id=two&redirect_uri=https%3A%2F%2Fclient.example%2Fcallback&response_type=code&scope=openid&code_challenge=" + strings.Repeat("a", 43) + "&code_challenge_method=S256"
	if err := validateAuthorizeRequest(httptest.NewRequest(http.MethodGet, raw, nil)); err == nil {
		t.Fatal("duplicate client_id was accepted")
	}
}

func TestAuthorizeValidationErrorsRedirectOnlyToValidatedClients(t *testing.T) {
	valid := "https://auth.example/oauth/authorize?client_id=client&redirect_uri=https%3A%2F%2Fclient.example%2Fcallback&response_type=code&scope=openid&state=opaque-state&code_challenge=" + strings.Repeat("a", 43) + "&code_challenge_method=S256"
	invalidScope := strings.ReplaceAll(valid, "scope=openid", "scope=openid%20email")
	service := &Service{storage: &Storage{clients: &Resolver{configured: map[string]*Client{
		"client": {IDValue: "client", Redirects: []string{"https://client.example/callback"}},
	}}}}
	tests := []struct {
		name       string
		raw        string
		wantStatus int
		wantError  string
	}{
		{name: "unsupported scope", raw: invalidScope, wantStatus: http.StatusFound, wantError: "invalid_scope"},
		{name: "missing PKCE", raw: strings.ReplaceAll(valid, "&code_challenge="+strings.Repeat("a", 43), ""), wantStatus: http.StatusFound, wantError: "invalid_request"},
		{name: "request object", raw: valid + "&request=opaque", wantStatus: http.StatusFound, wantError: "request_not_supported"},
		{name: "unsupported response type", raw: strings.ReplaceAll(valid, "response_type=code", "response_type=token"), wantStatus: http.StatusFound, wantError: "unauthorized_client"},
		{name: "unknown client", raw: strings.ReplaceAll(invalidScope, "client_id=client", "client_id=unknown"), wantStatus: http.StatusBadRequest},
		{name: "unregistered redirect", raw: strings.ReplaceAll(invalidScope, "client.example%2Fcallback", "attacker.example%2Fcallback"), wantStatus: http.StatusBadRequest},
		{name: "unsupported response mode", raw: valid + "&response_mode=form_post", wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nextCalled := false
			handler := service.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true }))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.raw, nil))
			if nextCalled {
				t.Fatal("invalid authorization request reached the provider")
			}
			if response.Code != test.wantStatus || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status/cache = %d %q, want %d no-store", response.Code, response.Header().Get("Cache-Control"), test.wantStatus)
			}
			location, err := response.Result().Location()
			if test.wantStatus != http.StatusFound {
				if err == nil {
					t.Fatalf("unsafe request redirected to %q", location)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse error redirect: %v", err)
			}
			if got := location.Scheme + "://" + location.Host + location.Path; got != "https://client.example/callback" {
				t.Fatalf("error redirect target = %q", got)
			}
			if got := location.Query().Get("error"); got != test.wantError {
				t.Fatalf("error = %q, want %q", got, test.wantError)
			}
			if got := location.Query().Get("state"); got != "opaque-state" {
				t.Fatalf("state = %q, want opaque-state", got)
			}
		})
	}
}

func TestValidateCIMDEnforcesPublicCodeClientProfile(t *testing.T) {
	identifier, err := validateClientIdentifierURL("https://client.example/oidc.json")
	if err != nil {
		t.Fatal(err)
	}
	valid := cimdDocument{ClientID: identifier.String(), ClientName: "Example", RedirectURIs: []string{"https://client.example/callback"}, TokenEndpointAuthMethod: string(liboidc.AuthMethodNone), GrantTypes: []string{"authorization_code"}, ResponseTypes: []string{"code"}}
	if _, err := validateCIMD(identifier.String(), identifier, valid, false); err != nil {
		t.Fatalf("valid CIMD: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*cimdDocument)
	}{
		{"mismatched identity", func(d *cimdDocument) { d.ClientID = "https://attacker.example/client.json" }},
		{"client secret", func(d *cimdDocument) { d.TokenEndpointAuthMethod = "client_secret_basic" }},
		{"redirect fragment", func(d *cimdDocument) { d.RedirectURIs = []string{"https://client.example/callback#stolen"} }},
		{"redirect downgrade", func(d *cimdDocument) { d.RedirectURIs = []string{"http://client.example/callback"} }},
		{"implicit grant", func(d *cimdDocument) { d.GrantTypes = []string{"implicit"} }},
		{"foreign client URI", func(d *cimdDocument) { d.ClientURI = "https://attacker.example/" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := valid
			document.RedirectURIs = append([]string(nil), valid.RedirectURIs...)
			test.mutate(&document)
			if _, err := validateCIMD(identifier.String(), identifier, document, false); err == nil {
				t.Fatal("invalid CIMD was accepted")
			}
		})
	}
}

func TestCIMDRejectsSpecialUseNetworksAndUnsafeIdentifiers(t *testing.T) {
	for _, raw := range []string{"http://client.example/client.json", "https://client.example/", "https://client.example/../metadata", "https://client.example/%2e./metadata", "https://client.example/meta%2fdata", "https://user@client.example/metadata", "https://client.example/metadata?variant=1"} {
		if _, err := validateClientIdentifierURL(raw); err == nil {
			t.Fatalf("unsafe client identifier %q was accepted", raw)
		}
	}
	for _, raw := range []string{
		"127.0.0.1", "10.0.0.1", "100.64.0.1", "192.0.2.1", "192.31.196.1", "192.52.193.1", "192.88.99.1", "192.175.48.1", "198.51.100.1", "203.0.113.1", "224.0.0.1",
		"::ffff:8.8.8.8", "64:ff9b::1", "64:ff9b:1::1", "100::1", "100:0:0:1::1", "2001::1", "2001:db8::1", "2002::1", "2620:4f:8000::1", "3fff::1", "5f00::1",
	} {
		if address := netip.MustParseAddr(raw); !blockedCIMDAddress(address) {
			t.Fatalf("special-use address %s was accepted", address)
		}
	}
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if address := netip.MustParseAddr(raw); blockedCIMDAddress(address) {
			t.Fatalf("ordinary public address %s was rejected", address)
		}
	}
}

func TestCIMDPrivateHostTrustAllowsOnlyPrivateAddresses(t *testing.T) {
	private := netip.MustParseAddr("192.168.1.20")
	if cimdAddressAllowed(private, false, false, false) {
		t.Fatal("private address was allowed without explicit host trust")
	}
	if !cimdAddressAllowed(private, false, true, false) {
		t.Fatal("private address was rejected for an explicitly trusted host")
	}
	for _, raw := range []string{"127.0.0.1", "169.254.169.254", "224.0.0.1", "100.64.0.1"} {
		address := netip.MustParseAddr(raw)
		if cimdAddressAllowed(address, false, true, false) {
			t.Fatalf("non-private special-use address %s was allowed by private-host trust", address)
		}
	}
}

func TestCIMDLoopbackHostTrustAllowsOnlyLoopbackAddresses(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "::1"} {
		address := netip.MustParseAddr(raw)
		if !cimdAddressAllowed(address, false, false, true) {
			t.Fatalf("loopback address %s was rejected for an explicitly trusted host", address)
		}
	}
	for _, raw := range []string{"10.0.0.1", "169.254.169.254", "224.0.0.1", "100.64.0.1"} {
		address := netip.MustParseAddr(raw)
		if cimdAddressAllowed(address, false, false, true) {
			t.Fatalf("non-loopback special-use address %s was allowed by loopback-host trust", address)
		}
	}
}

func TestCIMDDialFallsBackAcrossValidatedAddresses(t *testing.T) {
	addresses := []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")}
	var attempts []string
	client, server := net.Pipe()
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})

	connection, err := dialCIMDAddresses(t.Context(), "tcp", "42443", addresses,
		func(_ context.Context, _, address string) (net.Conn, error) {
			attempts = append(attempts, address)
			if len(attempts) == 1 {
				return nil, fmt.Errorf("IPv6 listener unavailable")
			}
			return client, nil
		})
	if err != nil {
		t.Fatalf("dial CIMD addresses: %v", err)
	}
	if connection != client {
		t.Fatal("dial returned an unexpected connection")
	}
	if got, want := strings.Join(attempts, ","), "[::1]:42443,127.0.0.1:42443"; got != want {
		t.Fatalf("dial attempts = %q, want %q", got, want)
	}
}

func TestCIMDCachePolicyIsBounded(t *testing.T) {
	if _, cache := cimdCacheAge("public, no-store, max-age=999999"); cache {
		t.Fatal("no-store response was cacheable")
	}
	if _, cache := cimdCacheAge("no-cache"); cache {
		t.Fatal("no-cache response was cacheable")
	}
	if age, cache := cimdCacheAge("max-age=999999"); !cache || age != maxCIMDCacheAge {
		t.Fatalf("cache age = %v, %v", age, cache)
	}
}
