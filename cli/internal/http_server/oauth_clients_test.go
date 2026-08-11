package http_server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync/atomic"
	"testing"
)

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

func TestOAuthClientMetadataBlocksSpecialUseAddresses(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "192.0.2.1", "::1", "2001:db8::1"}
	for _, raw := range blocked {
		address := netip.MustParseAddr(raw)
		if !blockedOAuthClientAddress(address) {
			t.Fatalf("special-use address %s was accepted", raw)
		}
	}
	if blockedOAuthClientAddress(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("public address was blocked")
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
