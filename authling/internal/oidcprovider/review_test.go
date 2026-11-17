package oidcprovider

import (
	"net/http"
	"slices"
	"testing"

	"github.com/chattocorp/chatto/authling/internal/config"
	liboidc "github.com/zitadel/oidc/v3/pkg/oidc"
)

func TestClientRefreshGrantAdmission(t *testing.T) {
	identifier, err := validateClientIdentifierURL("https://client.example/oidc.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, grants := range [][]string{nil, {"authorization_code"}, {"authorization_code", "refresh_token"}} {
		client, err := validateCIMD(identifier.String(), identifier, cimdDocument{ClientID: identifier.String(), ClientName: "Client", TokenEndpointAuthMethod: "none", RedirectURIs: []string{"https://client.example/callback"}, GrantTypes: grants}, false)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(client.GrantTypes(), liboidc.GrantTypeRefreshToken) != slices.Contains(grants, "refresh_token") {
			t.Fatal("CIMD grant policy mismatch")
		}
	}
	disabled := false
	resolver := NewResolver(config.Config{OIDC: config.OIDCConfig{Clients: []config.OIDCClientConfig{{ID: "default"}, {ID: "disabled", AllowRefreshTokens: &disabled}}}}, nil)
	for _, id := range []string{"default", "disabled"} {
		client, err := resolver.Resolve(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(client.GrantTypes(), liboidc.GrantTypeRefreshToken) != (id == "default") {
			t.Fatal("configured grant policy mismatch")
		}
	}
}

func TestResourcesCannotResolveAsCIMDClients(t *testing.T) {
	cimd := newCIMDTestResolver(t, func(request *http.Request) (*http.Response, error) {
		t.Fatal("reserved audience triggered metadata fetch")
		return nil, nil
	})
	cfg := config.Config{HTTP: config.HTTPConfig{PublicURL: "https://auth.example/issuer"}, OIDC: config.OIDCConfig{AllowUnregisteredClients: true, Resources: []config.OIDCResourceConfig{{URI: "https://api.example/metadata.json", AllowCIMD: true}}, Clients: []config.OIDCClientConfig{{ID: "local", Resources: []string{"https://private.example/api"}}}}}
	resolver := NewResolver(cfg, cimd)
	for _, id := range []string{cfg.HTTP.PublicURL, cfg.OIDC.Resources[0].URI, cfg.OIDC.Clients[0].Resources[0]} {
		if _, err := resolver.Resolve(t.Context(), id); err == nil {
			t.Fatal("reserved resource resolved as client")
		}
	}
}
