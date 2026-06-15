package http_server

import (
	"slices"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	"hmans.de/chatto/internal/config"
)

func TestServerInfoAuthMethodsUsesCompatibilityValues(t *testing.T) {
	methods := serverInfoAuthMethods(config.AuthConfig{
		Providers: []config.AuthProviderConfig{
			{ID: "hub", Type: config.AuthProviderTypeOpenIDConnect},
			{ID: "hub-backup", Type: config.AuthProviderTypeOpenIDConnect},
			{ID: "github-main", Type: config.AuthProviderTypeGitHub},
		},
	})

	want := []string{"oidc", "github"}
	if !slices.Equal(methods, want) {
		t.Fatalf("serverInfoAuthMethods() = %v, want %v", methods, want)
	}
}

func TestProviderScopesForOIDC(t *testing.T) {
	t.Run("default requests openid profile email", func(t *testing.T) {
		scopes := providerScopes(config.AuthProviderConfig{Type: config.AuthProviderTypeOpenIDConnect})
		want := []string{oidc.ScopeOpenID, "profile", "email"}
		if !slices.Equal(scopes, want) {
			t.Fatalf("providerScopes() = %v, want %v", scopes, want)
		}
	})

	t.Run("request_email false keeps openid profile", func(t *testing.T) {
		requestEmail := false
		scopes := providerScopes(config.AuthProviderConfig{
			Type:         config.AuthProviderTypeOpenIDConnect,
			RequestEmail: &requestEmail,
		})
		want := []string{oidc.ScopeOpenID, "profile"}
		if !slices.Equal(scopes, want) {
			t.Fatalf("providerScopes() = %v, want %v", scopes, want)
		}
	})

	t.Run("custom scopes are honored with openid required", func(t *testing.T) {
		scopes := providerScopes(config.AuthProviderConfig{
			Type:   config.AuthProviderTypeOpenIDConnect,
			Scopes: []string{"groups", "profile"},
		})
		want := []string{oidc.ScopeOpenID, "groups", "profile"}
		if !slices.Equal(scopes, want) {
			t.Fatalf("providerScopes() = %v, want %v", scopes, want)
		}
	})
}
