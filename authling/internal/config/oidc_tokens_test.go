package config

import (
	"testing"
	"time"
)

func TestOIDCRefreshDefaults(t *testing.T) {
	cfg := OIDCConfig{}
	if cfg.RefreshLifetime() != 30*24*time.Hour || cfg.RefreshIdleLifetime() != 7*24*time.Hour {
		t.Fatal("refresh lifetime defaults")
	}
	cfg.RefreshTokenLifetimeDays, cfg.RefreshTokenIdleDays = 60, 14
	if cfg.RefreshLifetime() != 60*24*time.Hour || cfg.RefreshIdleLifetime() != 14*24*time.Hour {
		t.Fatal("refresh lifetime overrides")
	}
}

func TestResourceIdentifiers(t *testing.T) {
	for _, raw := range []string{"https://api.example", "https://api.example/v1"} {
		if !ValidResource(raw, false) {
			t.Fatal("valid resource rejected")
		}
	}
	for _, raw := range []string{"", "relative", "http://api.example", "https://user:pass@api.example", "https://api.example#fragment", "https://api.example?token=secret", "https://api.example#", "https://api.example?", "https://:443"} {
		if ValidResource(raw, false) {
			t.Fatal("unsafe resource accepted")
		}
	}
	if !ValidResource("http://localhost:9999/api", true) || ValidResource("http://localhost:9999/api", false) {
		t.Fatal("loopback resource policy")
	}
}

func TestInvalidResourceAndRefreshConfiguration(t *testing.T) {
	for _, test := range []struct {
		name string
		oidc OIDCConfig
	}{
		{"issuer collision", OIDCConfig{Resources: []OIDCResourceConfig{{URI: "http://localhost:8080"}}}},
		{"client collision", OIDCConfig{Clients: []OIDCClientConfig{{ID: "https://api.example", Resources: []string{"https://api.example"}}}}},
		{"negative lifetime", OIDCConfig{RefreshTokenLifetimeDays: -1}},
		{"excessive lifetime", OIDCConfig{RefreshTokenLifetimeDays: 366}},
		{"negative idle", OIDCConfig{RefreshTokenIdleDays: -1}},
		{"idle exceeds total", OIDCConfig{RefreshTokenLifetimeDays: 3}},
		{"duplicate resource", OIDCConfig{Resources: []OIDCResourceConfig{{URI: "https://api.example"}, {URI: "https://api.example"}}}},
		{"invalid resource", OIDCConfig{Resources: []OIDCResourceConfig{{URI: "https://api.example#fragment"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := Config{HTTP: HTTPConfig{PublicURL: "http://localhost:8080"}, OIDC: test.oidc, NATS: NATSConfig{Embedded: EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}}}
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
