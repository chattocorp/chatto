package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadEmbeddedConfigWithEnvironmentOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authling.toml")
	if err := os.WriteFile(path, []byte(`
[nats]
replicas = 1

[nats.embedded]
enabled = true
data_dir = "/var/lib/authling"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	wantDataDir := filepath.Join(t.TempDir(), "nats")
	t.Setenv("AUTHLING_NATS_EMBEDDED_DATA_DIR", wantDataDir)
	t.Setenv("AUTHLING_AUTHENTICATION_PASSWORD_MINIMUM_LENGTH", "12")

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !cfg.NATS.Embedded.Enabled {
		t.Fatal("embedded NATS is disabled, want enabled")
	}
	if cfg.NATS.Embedded.DataDir != wantDataDir {
		t.Fatalf("embedded NATS data directory = %q, want %q", cfg.NATS.Embedded.DataDir, wantDataDir)
	}
	if got := cfg.Authentication.PasswordMinimumLengthOrDefault(); got != 12 {
		t.Fatalf("password minimum length = %d, want 12", got)
	}
}

func TestReadRejectsUnknownTOMLFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authling.toml")
	if err := os.WriteFile(path, []byte(`
[nats.embedded]
enabled = true
data_dir = ".authling/nats"
surprise = true
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "strict mode") {
		t.Fatalf("read config error = %v, want unknown-field error", err)
	}
}

func TestReadRequiresOneNATSMode(t *testing.T) {
	t.Chdir(t.TempDir())

	if _, err := Read(""); err == nil || !strings.Contains(err.Error(), "enable nats.embedded") {
		t.Fatalf("read config error = %v, want missing NATS mode", err)
	}
}

func TestReadRequiresExplicitConfigFileToExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")

	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "read "+path) {
		t.Fatalf("read config error = %v, want missing explicit file error", err)
	}
}

func TestValidateRejectsCredentialsInNATSURL(t *testing.T) {
	cfg := Config{
		NATS: NATSConfig{
			Client: NATSClientConfig{
				URL:             "nats://user:password@nats.example:4222",
				CredentialsFile: "/run/secrets/authling.creds",
			},
		},
	}

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "without credentials") {
		t.Fatalf("validate config error = %v, want embedded-credential error", err)
	}
}

func TestValidateRejectsInvalidHTTPBindAddress(t *testing.T) {
	cfg := Config{
		HTTP: HTTPConfig{BindAddress: "not-an-address"},
		NATS: NATSConfig{
			Embedded: EmbeddedNATSConfig{
				Enabled: true,
				DataDir: t.TempDir(),
			},
		},
	}

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "http.bind_address") {
		t.Fatalf("validate config error = %v, want HTTP listener error", err)
	}
}

func TestValidateAllowsPlainHTTPPublicURLOnlyOnLoopback(t *testing.T) {
	validNATS := NATSConfig{Embedded: EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}}
	for _, bindAddress := range []string{"0.0.0.0:8080", "192.0.2.1:8080", "authling.example:8080"} {
		cfg := Config{HTTP: HTTPConfig{BindAddress: bindAddress, PublicURL: "http://authling.example:8080"}, NATS: validNATS}
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "plain HTTP") {
			t.Fatalf("Validate(%q) error = %v, want public-origin error", bindAddress, err)
		}
	}
	for _, bindAddress := range []string{"127.0.0.1:8080", "[::1]:8080", "localhost:8080"} {
		cfg := Config{HTTP: HTTPConfig{BindAddress: bindAddress, PublicURL: "http://" + bindAddress}, NATS: validNATS}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate(%q): %v", bindAddress, err)
		}
	}
	cfg := Config{HTTP: HTTPConfig{BindAddress: "127.0.0.1:8080", PublicURL: "https://authling.example"}, NATS: validNATS}
	if err := cfg.Validate(); err != nil || !cfg.HTTP.SecureCookies() {
		t.Fatalf("HTTPS proxy config validation = %v, SecureCookies = %v", err, cfg.HTTP.SecureCookies())
	}
}

func TestValidateSMTPRequiresSafeCompleteConfiguration(t *testing.T) {
	cfg := Config{NATS: NATSConfig{Embedded: EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}}, SMTP: SMTPConfig{Enabled: true, TLS: "plaintext"}}
	err := cfg.Validate()
	for _, want := range []string{"smtp.host", "smtp.port", "smtp.from", "smtp.tls"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("validation error = %v, want %q", err, want)
		}
	}
}

func TestValidatePasswordMinimumLength(t *testing.T) {
	validNATS := NATSConfig{Embedded: EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}}
	for _, minimum := range []int{7, 129} {
		cfg := Config{Authentication: AuthenticationConfig{PasswordMinimumLength: minimum}, NATS: validNATS}
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "authentication.password_minimum_length") {
			t.Fatalf("Validate minimum %d error = %v, want password policy error", minimum, err)
		}
	}
	if cfg := (Config{NATS: validNATS}); cfg.Authentication.PasswordMinimumLengthOrDefault() != DefaultPasswordMinimumLength {
		t.Fatalf("default password minimum = %d, want %d", cfg.Authentication.PasswordMinimumLengthOrDefault(), DefaultPasswordMinimumLength)
	}
}

func TestDevelopmentConfigIsValid(t *testing.T) {
	cfg, err := Read(filepath.Join("..", "..", "authling.toml"))
	if err != nil {
		t.Fatalf("read development config: %v", err)
	}
	if !cfg.NATS.Embedded.Enabled {
		t.Fatal("development config does not enable embedded NATS")
	}
	if got, want := cfg.HTTP.BindAddressOrDefault(), "127.0.0.1:8080"; got != want {
		t.Fatalf("development HTTP bind address = %q, want %q", got, want)
	}
	if got, want := cfg.HTTP.PublicURLOrDefault(), "http://localhost:8080"; got != want || cfg.HTTP.SecureCookies() {
		t.Fatalf("development public URL = %q, secure cookies = %v, want %q over plain loopback HTTP", got, cfg.HTTP.SecureCookies(), want)
	}
	if got := cfg.Authentication.PasswordMinimumLengthOrDefault(); got != 10 {
		t.Fatalf("development password minimum length = %d, want 10", got)
	}
	if !cfg.SMTP.Enabled || cfg.SMTP.Host != "127.0.0.1" || cfg.SMTP.Port != 1025 || cfg.SMTP.TLSPolicyOrDefault() != SMTPTLSOpportunistic {
		t.Fatalf("development SMTP config = %+v, want local Mailpit", cfg.SMTP)
	}
}

func TestValidateOIDCConventionalClients(t *testing.T) {
	validNATS := NATSConfig{Embedded: EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}}
	base := Config{HTTP: HTTPConfig{PublicURL: "https://auth.example"}, NATS: validNATS, OIDC: OIDCConfig{Clients: []OIDCClientConfig{{ID: "client", Name: "Client", Secret: strings.Repeat("s", 32), RedirectURIs: []string{"https://client.example/callback"}}}}}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid client: %v", err)
	}

	tests := []struct {
		name   string
		client OIDCClientConfig
	}{
		{"URL identifier reserved for CIMD", OIDCClientConfig{ID: "https://client.example/meta.json", Name: "Client", RedirectURIs: []string{"https://client.example/callback"}}},
		{"short secret", OIDCClientConfig{ID: "client", Name: "Client", Secret: "short", RedirectURIs: []string{"https://client.example/callback"}}},
		{"redirect fragment", OIDCClientConfig{ID: "client", Name: "Client", RedirectURIs: []string{"https://client.example/callback#fragment"}}},
		{"production HTTP redirect", OIDCClientConfig{ID: "client", Name: "Client", RedirectURIs: []string{"http://localhost/callback"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := base
			cfg.OIDC.Clients = []OIDCClientConfig{test.client}
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid client was accepted")
			}
		})
	}
}

func TestValidateCIMDTrustedPrivateHosts(t *testing.T) {
	validNATS := NATSConfig{Embedded: EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()}}
	cfg := Config{NATS: validNATS, OIDC: OIDCConfig{CIMDTrustedPrivateHosts: []string{"Chatto.Dev.Orb.Local."}}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid trusted private host: %v", err)
	}
	if got := cfg.OIDC.TrustedPrivateCIMDHosts(); len(got) != 1 || got[0] != "chatto.dev.orb.local" {
		t.Fatalf("normalized trusted hosts = %#v", got)
	}

	for _, host := range []string{"https://chatto.example", "chatto.example:443", "chatto.example/path", ""} {
		cfg.OIDC.CIMDTrustedPrivateHosts = []string{host}
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "cimd_trusted_private_hosts") {
			t.Fatalf("trusted host %q validation error = %v", host, err)
		}
	}
}
