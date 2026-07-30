package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadConfig_AuthProvidersFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_URL", "https://chat.example")
	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_ID", "hub")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_TYPE", "oidc")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_LABEL", "Chatto Hub")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_ISSUER_URL", "https://id.example")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_CLIENT_ID", "chatto")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_CLIENT_SECRET", "secret")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_SCOPES", "openid, profile, groups")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_REQUEST_EMAIL", "false")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_AUTO_PROVISION", "true")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_PROVIDER_OPTIONS_PROMPT", "select_account")
	t.Setenv("CHATTO_AUTH_PROVIDERS_1_ID", "github-main")
	t.Setenv("CHATTO_AUTH_PROVIDERS_1_TYPE", "github")
	t.Setenv("CHATTO_AUTH_PROVIDERS_1_CLIENT_ID", "github-id")
	t.Setenv("CHATTO_AUTH_PROVIDERS_1_CLIENT_SECRET", "github-secret")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if len(cfg.Auth.Providers) != 2 {
		t.Fatalf("Auth.Providers len = %d, want 2", len(cfg.Auth.Providers))
	}
	if got := cfg.Auth.Providers[0]; got.ID != "hub" || got.Type != AuthProviderTypeOpenIDConnect || got.Label != "Chatto Hub" || got.IssuerURL != "https://id.example" || got.ClientID != "chatto" || got.ClientSecret != "secret" {
		t.Fatalf("Auth.Providers[0] = %+v", got)
	}
	if got := cfg.Auth.Providers[0]; got.RequestEmail == nil || *got.RequestEmail {
		t.Fatalf("Auth.Providers[0].RequestEmail = %v, want false", got.RequestEmail)
	}
	if got := cfg.Auth.Providers[0]; got.AutoProvision == nil || !*got.AutoProvision {
		t.Fatalf("Auth.Providers[0].AutoProvision = %v, want true", got.AutoProvision)
	}
	if got := strings.Join(cfg.Auth.Providers[0].Scopes, ","); got != "openid,profile,groups" {
		t.Fatalf("Auth.Providers[0].Scopes = %q", got)
	}
	if got := cfg.Auth.Providers[0].ProviderOptions["prompt"]; got != "select_account" {
		t.Fatalf("Auth.Providers[0].ProviderOptions[prompt] = %q", got)
	}
	if got := cfg.Auth.Providers[1]; got.ID != "github-main" || got.Type != AuthProviderTypeGitHub || got.ClientID != "github-id" || got.ClientSecret != "github-secret" {
		t.Fatalf("Auth.Providers[1] = %+v", got)
	}
}

func TestAuthProviderConfig_RequestEmailDefault(t *testing.T) {
	if got := (AuthProviderConfig{}).RequestEmailOrDefault(); got {
		t.Fatalf("RequestEmailOrDefault() = %v, want false", got)
	}
}

func TestReadConfig_AuthProvidersEnvOverridesFile(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	configContent := `
[webserver]
url = "https://chat.example"
port = 4000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

[[auth.providers]]
id = "toml-github"
type = "github"
client_id = "toml-id"
client_secret = "toml-secret"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_ID", "env-discord")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_TYPE", "discord")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_CLIENT_ID", "env-id")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_CLIENT_SECRET", "env-secret")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if len(cfg.Auth.Providers) != 1 {
		t.Fatalf("Auth.Providers len = %d, want 1", len(cfg.Auth.Providers))
	}
	if got := cfg.Auth.Providers[0]; got.ID != "env-discord" || got.Type != AuthProviderTypeDiscord {
		t.Fatalf("Auth.Providers[0] = %+v", got)
	}
}

func TestReadConfig_InvalidAuthProvidersEnvField(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_URL", "https://chat.example")
	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_ID", "github")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_UNKNOWN", "value")

	_, err = ReadConfig("")
	if err == nil || !strings.Contains(err.Error(), "unknown auth provider field") {
		t.Fatalf("ReadConfig() error = %v, want unknown auth provider field error", err)
	}
}

func TestReadConfig_InvalidAuthProvidersEnvIndexGap(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_URL", "https://chat.example")
	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_AUTH_PROVIDERS_1_ID", "github")

	_, err = ReadConfig("")
	if err == nil || !strings.Contains(err.Error(), "indexes must be contiguous") {
		t.Fatalf("ReadConfig() error = %v, want contiguous index error", err)
	}
}

func TestReadConfig_LegacyOIDCEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_URL", "https://chat.example")
	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_AUTH_OIDC_ENABLED", "true")
	t.Setenv("CHATTO_AUTH_OIDC_ISSUER_URL", "https://id.example")
	t.Setenv("CHATTO_AUTH_OIDC_CLIENT_ID", "chatto")
	t.Setenv("CHATTO_AUTH_OIDC_CLIENT_SECRET", "secret")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if len(cfg.Auth.Providers) != 1 {
		t.Fatalf("Auth.Providers len = %d, want 1", len(cfg.Auth.Providers))
	}
	got := cfg.Auth.Providers[0]
	if got.ID != "oidc" || got.Type != AuthProviderTypeOpenIDConnect || got.Label != "Chatto Hub" || got.IssuerURL != "https://id.example" || got.ClientID != "chatto" || got.ClientSecret != "secret" {
		t.Fatalf("legacy OIDC provider = %+v", got)
	}
}

func TestReadConfig_LegacyOIDCEnvCannotCombineWithAuthProvidersEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_URL", "https://chat.example")
	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_ID", "github")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_TYPE", "github")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_CLIENT_ID", "id")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_CLIENT_SECRET", "secret")
	t.Setenv("CHATTO_AUTH_OIDC_ENABLED", "true")

	_, err = ReadConfig("")
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("ReadConfig() error = %v, want combined provider env error", err)
	}
}
