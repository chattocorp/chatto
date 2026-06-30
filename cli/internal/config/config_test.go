package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadConfig_WithoutConfigFile(t *testing.T) {
	// Create a temp directory with no config file
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	// Set required env vars
	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_ENCRYPTION_SECRET", "000102030405060708090a0b0c0d0e0f")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")

	// ReadConfig should succeed even without chatto.toml
	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed without config file: %v", err)
	}

	// Verify env vars were applied
	if cfg.Webserver.Port != 4000 {
		t.Errorf("expected port 4000, got %d", cfg.Webserver.Port)
	}
	if cfg.Webserver.CookieSigningSecret != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Errorf("expected cookie secret to be set from env var")
	}
	if cfg.Webserver.CookieEncryptionSecret != "000102030405060708090a0b0c0d0e0f" {
		t.Errorf("expected cookie encryption secret to be set from env var")
	}
	if cfg.Core.SecretKey != "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" {
		t.Errorf("expected core secret to be set from env var")
	}
}

func TestReadConfig_WithConfigFile(t *testing.T) {
	// Create a temp directory with a config file
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	// Write a minimal config file
	configContent := `
[webserver]
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// ReadConfig should read from file
	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed with config file: %v", err)
	}

	// Verify file values were applied
	if cfg.Webserver.Port != 5000 {
		t.Errorf("expected port 5000 from file, got %d", cfg.Webserver.Port)
	}
}

func TestReadConfig_EnvOverridesFile(t *testing.T) {
	// Create a temp directory with a config file
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	// Write a config file with port 5000
	configContent := `
[webserver]
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set env var to override port
	t.Setenv("CHATTO_WEBSERVER_PORT", "6000")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}

	// Env var should override file
	if cfg.Webserver.Port != 6000 {
		t.Errorf("expected port 6000 from env override, got %d", cfg.Webserver.Port)
	}
}

func TestReadConfig_OperatorAPIFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_OPERATOR_API_ENABLED", "true")
	t.Setenv("CHATTO_OPERATOR_API_SOCKET_PATH", "/tmp/chatto-test/operator.sock")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if !cfg.OperatorAPI.Enabled {
		t.Fatal("OperatorAPI.Enabled = false, want true")
	}
	if got := cfg.OperatorAPI.SocketPathOrDefault(); got != "/tmp/chatto-test/operator.sock" {
		t.Fatalf("OperatorAPI.SocketPathOrDefault() = %q", got)
	}
}

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

func TestReadConfig_InvalidCookieEncryptionSecretFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_ENCRYPTION_SECRET", "not-hex")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")

	_, err = ReadConfig("")
	if err == nil || !strings.Contains(err.Error(), "webserver.cookie_encryption_secret must be hex-encoded") {
		t.Fatalf("ReadConfig() error = %v, want cookie encryption validation error", err)
	}
}

func TestReadConfig_ValidatesEnvOverrides(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		env       map[string]string
		wantError string
	}{
		{
			name: "required secret overridden by env must be valid hex",
			config: `
[webserver]
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`,
			env: map[string]string{
				"CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET": "not-hex",
			},
			wantError: "webserver.cookie_signing_secret must be hex-encoded",
		},
		{
			name: "allowed origins overridden by env must be real origins",
			config: `
[webserver]
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
allowed_origins = ["https://client.example"]

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`,
			env: map[string]string{
				"CHATTO_WEBSERVER_ALLOWED_ORIGINS": "https://client.example/path",
			},
			wantError: "webserver.allowed_origins contains invalid origin",
		},
		{
			name: "webserver URL from env must include scheme and host",
			env: map[string]string{
				"CHATTO_WEBSERVER_PORT":                  "4000",
				"CHATTO_WEBSERVER_URL":                   "chat.example",
				"CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"CHATTO_CORE_SECRET_KEY":                 "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
				"CHATTO_CORE_ASSETS_SIGNING_SECRET":      "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
			},
			wantError: "webserver.url must use http or https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			originalDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get working directory: %v", err)
			}
			if err := os.Chdir(tmpDir); err != nil {
				t.Fatalf("failed to change to temp directory: %v", err)
			}
			t.Cleanup(func() { os.Chdir(originalDir) })

			if tt.config != "" {
				if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(tt.config), 0644); err != nil {
					t.Fatalf("failed to write config file: %v", err)
				}
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			_, err = ReadConfig("")
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("ReadConfig() error = %v, want to contain %q", err, tt.wantError)
			}
		})
	}
}

func TestReadConfig_GeneralLogFormatFromTOMLAndEnv(t *testing.T) {
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
[general]
log_format = "text"

[webserver]
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if cfg.General.LogFormat != "text" {
		t.Fatalf("expected TOML log_format %q, got %q", "text", cfg.General.LogFormat)
	}

	t.Setenv("CHATTO_LOG_FORMAT", "json")
	cfg, err = ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() with env override failed: %v", err)
	}
	if cfg.General.LogFormat != "json" {
		t.Fatalf("expected env log_format %q, got %q", "json", cfg.General.LogFormat)
	}
}

func TestReadConfig_OAuthRedirectOriginsFromTOMLAndEnv(t *testing.T) {
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
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
oauth_redirect_origins = ["https://client.example"]

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if got, want := strings.Join(cfg.Webserver.OAuthRedirectOrigins, ","), "https://client.example"; got != want {
		t.Fatalf("expected TOML oauth_redirect_origins %q, got %q", want, got)
	}

	t.Setenv("CHATTO_WEBSERVER_OAUTH_REDIRECT_ORIGINS", "*")
	cfg, err = ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() with env override failed: %v", err)
	}
	if got, want := strings.Join(cfg.Webserver.OAuthRedirectOrigins, ","), "*"; got != want {
		t.Fatalf("expected env oauth_redirect_origins %q, got %q", want, got)
	}
}

func TestReadConfig_S3PathPrefixFromTOMLAndEnv(t *testing.T) {
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
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
storage_backend = "s3"

[core.assets.s3]
endpoint = "s3.amazonaws.com"
bucket = "test-bucket"
path_prefix = "/tenant-a/chatto/"
access_key_id = "test-key"
secret_access_key = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if cfg.Core.Assets.S3.PathPrefix != "tenant-a/chatto" {
		t.Fatalf("expected normalized TOML prefix, got %q", cfg.Core.Assets.S3.PathPrefix)
	}

	t.Setenv("CHATTO_CORE_ASSETS_S3_PATH_PREFIX", "/tenant-b/chatto/")
	cfg, err = ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() with env override failed: %v", err)
	}
	if cfg.Core.Assets.S3.PathPrefix != "tenant-b/chatto" {
		t.Fatalf("expected normalized env prefix, got %q", cfg.Core.Assets.S3.PathPrefix)
	}
}

func TestReadConfig_SMTPPolicyFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_SMTP_TLS", "opportunistic")
	t.Setenv("CHATTO_SMTP_TLS_SERVER_NAME", "mail.example.com")
	t.Setenv("CHATTO_SMTP_TLS_SKIP_VERIFY", "true")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}

	if got := cfg.SMTP.TLSPolicyOrDefault(); got != SMTPTLSOpportunistic {
		t.Errorf("expected SMTP TLS policy %q from env, got %q", SMTPTLSOpportunistic, got)
	}
	if got := cfg.SMTP.TLSServerName; got != "mail.example.com" {
		t.Errorf("expected SMTP TLS server name from env, got %q", got)
	}
	if !cfg.SMTP.TLSSkipVerify {
		t.Error("expected SMTP TLS skip verify from env")
	}
}

func TestSMTPConfig_TLSPolicyOrDefault(t *testing.T) {
	tests := []struct {
		name string
		cfg  SMTPConfig
		want SMTPTLSPolicy
	}{
		{
			name: "empty policy defaults to mandatory STARTTLS",
			cfg:  SMTPConfig{Port: 587},
			want: SMTPTLSMandatory,
		},
		{
			name: "empty policy on port 465 defaults to implicit TLS",
			cfg:  SMTPConfig{Port: 465},
			want: SMTPTLSImplicit,
		},
		{
			name: "mandatory policy on port 465 uses implicit TLS",
			cfg:  SMTPConfig{Port: 465, TLS: SMTPTLSMandatory},
			want: SMTPTLSImplicit,
		},
		{
			name: "explicit implicit TLS",
			cfg:  SMTPConfig{Port: 465, TLS: SMTPTLSImplicit},
			want: SMTPTLSImplicit,
		},
		{
			name: "opportunistic policy",
			cfg:  SMTPConfig{Port: 587, TLS: SMTPTLSOpportunistic},
			want: SMTPTLSOpportunistic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.TLSPolicyOrDefault(); got != tt.want {
				t.Errorf("TLSPolicyOrDefault() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTLSConfig_CacheDirOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		cacheDir string
		want     string
	}{
		{
			name:     "empty returns default",
			cacheDir: "",
			want:     ".chatto/certs",
		},
		{
			name:     "custom value returned",
			cacheDir: "/var/cache/certs",
			want:     "/var/cache/certs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &TLSConfig{CacheDir: tt.cacheDir}
			if got := c.CacheDirOrDefault(); got != tt.want {
				t.Errorf("CacheDirOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTLSConfig_HTTPPortOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		httpPort int
		want     int
	}{
		{
			name:     "zero returns default 80",
			httpPort: 0,
			want:     80,
		},
		{
			name:     "custom value returned",
			httpPort: 8080,
			want:     8080,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &TLSConfig{HTTPPort: tt.httpPort}
			if got := c.HTTPPortOrDefault(); got != tt.want {
				t.Errorf("HTTPPortOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebserverConfig_EffectivePort(t *testing.T) {
	tests := []struct {
		name       string
		port       int
		tlsEnabled bool
		want       int
	}{
		{
			name:       "TLS enabled with port 0 returns 443",
			port:       0,
			tlsEnabled: true,
			want:       443,
		},
		{
			name:       "TLS enabled with custom port returns custom",
			port:       8443,
			tlsEnabled: true,
			want:       8443,
		},
		{
			name:       "TLS disabled returns configured port",
			port:       4000,
			tlsEnabled: false,
			want:       4000,
		},
		{
			name:       "TLS disabled with port 0 returns 0",
			port:       0,
			tlsEnabled: false,
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &WebserverConfig{
				Port: tt.port,
				TLS:  TLSConfig{Enabled: tt.tlsEnabled},
			}
			if got := c.EffectivePort(); got != tt.want {
				t.Errorf("EffectivePort() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebserverConfig_WebSocketCompressionEnabled(t *testing.T) {
	tests := []struct {
		name        string
		compression *bool
		want        bool
	}{
		{
			name:        "nil returns true (default)",
			compression: nil,
			want:        true,
		},
		{
			name:        "true returns true",
			compression: boolPtr(true),
			want:        true,
		},
		{
			name:        "false returns false",
			compression: boolPtr(false),
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &WebserverConfig{WebSocketCompression: tt.compression}
			if got := c.WebSocketCompressionEnabled(); got != tt.want {
				t.Errorf("WebSocketCompressionEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMetricsConfig_Defaults(t *testing.T) {
	cfg := MetricsConfig{}

	if got := cfg.BindAddressOrDefault(); got != "127.0.0.1" {
		t.Errorf("BindAddressOrDefault() = %q, want 127.0.0.1", got)
	}
	if got := cfg.PortOrDefault(); got != 9090 {
		t.Errorf("PortOrDefault() = %d, want 9090", got)
	}
	if got := cfg.PathOrDefault(); got != "/metrics" {
		t.Errorf("PathOrDefault() = %q, want /metrics", got)
	}
}

func TestExporterConfig_Defaults(t *testing.T) {
	cfg := ExporterConfig{}

	if got := cfg.BindAddressOrDefault(); got != "127.0.0.1" {
		t.Errorf("BindAddressOrDefault() = %q, want 127.0.0.1", got)
	}
	if got := cfg.PortOrDefault(); got != 9100 {
		t.Errorf("PortOrDefault() = %d, want 9100", got)
	}
	if got := cfg.PathOrDefault(); got != "/metrics" {
		t.Errorf("PathOrDefault() = %q, want /metrics", got)
	}
	if got := cfg.S3RefreshIntervalOrDefault(); got != 15*time.Minute {
		t.Errorf("S3RefreshIntervalOrDefault() = %s, want 15m", got)
	}
	if got := cfg.S3TimeoutOrDefault(); got != 30*time.Second {
		t.Errorf("S3TimeoutOrDefault() = %s, want 30s", got)
	}
}

func TestReadConfig_MetricsFromTOMLAndEnv(t *testing.T) {
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
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[metrics]
enabled = true
bind_address = "0.0.0.0"
port = 9100
path = "/internal/metrics"
pprof = true

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if !cfg.Metrics.Enabled {
		t.Fatal("Metrics.Enabled = false, want true")
	}
	if got := cfg.Metrics.BindAddressOrDefault(); got != "0.0.0.0" {
		t.Errorf("Metrics.BindAddress = %q, want 0.0.0.0", got)
	}
	if got := cfg.Metrics.PortOrDefault(); got != 9100 {
		t.Errorf("Metrics.Port = %d, want 9100", got)
	}
	if got := cfg.Metrics.PathOrDefault(); got != "/internal/metrics" {
		t.Errorf("Metrics.Path = %q, want /internal/metrics", got)
	}
	if !cfg.Metrics.Pprof {
		t.Fatal("Metrics.Pprof = false, want true")
	}

	t.Setenv("CHATTO_METRICS_ENABLED", "false")
	t.Setenv("CHATTO_METRICS_PORT", "9200")
	t.Setenv("CHATTO_METRICS_PATH", "/metrics")
	t.Setenv("CHATTO_METRICS_PPROF", "false")

	cfg, err = ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() with env override failed: %v", err)
	}
	if cfg.Metrics.Enabled {
		t.Fatal("Metrics.Enabled = true, want env override false")
	}
	if got := cfg.Metrics.PortOrDefault(); got != 9200 {
		t.Errorf("Metrics.Port env override = %d, want 9200", got)
	}
	if got := cfg.Metrics.PathOrDefault(); got != "/metrics" {
		t.Errorf("Metrics.Path env override = %q, want /metrics", got)
	}
	if cfg.Metrics.Pprof {
		t.Fatal("Metrics.Pprof = true, want env override false")
	}
}

func TestReadConfig_ExporterFromTOMLAndEnv(t *testing.T) {
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
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[exporter]
enabled = true
bind_address = "0.0.0.0"
port = 9200
path = "/internal/exporter"
s3_refresh_interval = "30m"
s3_timeout = "45s"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if !cfg.Exporter.Enabled {
		t.Fatal("Exporter.Enabled = false, want true")
	}
	if got := cfg.Exporter.BindAddressOrDefault(); got != "0.0.0.0" {
		t.Errorf("Exporter.BindAddress = %q, want 0.0.0.0", got)
	}
	if got := cfg.Exporter.PortOrDefault(); got != 9200 {
		t.Errorf("Exporter.Port = %d, want 9200", got)
	}
	if got := cfg.Exporter.PathOrDefault(); got != "/internal/exporter" {
		t.Errorf("Exporter.Path = %q, want /internal/exporter", got)
	}
	if got := cfg.Exporter.S3RefreshIntervalOrDefault(); got != 30*time.Minute {
		t.Errorf("Exporter.S3RefreshInterval = %s, want 30m", got)
	}
	if got := cfg.Exporter.S3TimeoutOrDefault(); got != 45*time.Second {
		t.Errorf("Exporter.S3Timeout = %s, want 45s", got)
	}

	t.Setenv("CHATTO_EXPORTER_ENABLED", "false")
	t.Setenv("CHATTO_EXPORTER_PORT", "9300")
	t.Setenv("CHATTO_EXPORTER_PATH", "/metrics")
	t.Setenv("CHATTO_EXPORTER_S3_REFRESH_INTERVAL", "5m")
	t.Setenv("CHATTO_EXPORTER_S3_TIMEOUT", "10s")

	cfg, err = ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() with env override failed: %v", err)
	}
	if cfg.Exporter.Enabled {
		t.Fatal("Exporter.Enabled = true, want env override false")
	}
	if got := cfg.Exporter.PortOrDefault(); got != 9300 {
		t.Errorf("Exporter.Port env override = %d, want 9300", got)
	}
	if got := cfg.Exporter.PathOrDefault(); got != "/metrics" {
		t.Errorf("Exporter.Path env override = %q, want /metrics", got)
	}
	if got := cfg.Exporter.S3RefreshIntervalOrDefault(); got != 5*time.Minute {
		t.Errorf("Exporter.S3RefreshInterval env override = %s, want 5m", got)
	}
	if got := cfg.Exporter.S3TimeoutOrDefault(); got != 10*time.Second {
		t.Errorf("Exporter.S3Timeout env override = %s, want 10s", got)
	}
}

func TestReadConfig_DiagnosticsFromTOMLAndEnv(t *testing.T) {
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
port = 5000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[diagnostics]
startup_cpu_profile = "startup.pprof"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if got := cfg.Diagnostics.StartupCPUProfile; got != "startup.pprof" {
		t.Fatalf("Diagnostics.StartupCPUProfile = %q, want startup.pprof", got)
	}

	t.Setenv("CHATTO_DIAGNOSTICS_STARTUP_CPU_PROFILE", "env-startup.pprof")

	cfg, err = ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() with env override failed: %v", err)
	}
	if got := cfg.Diagnostics.StartupCPUProfile; got != "env-startup.pprof" {
		t.Fatalf("Diagnostics.StartupCPUProfile env override = %q, want env-startup.pprof", got)
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}

func validTestConfig() ChattoConfig {
	return ChattoConfig{
		Webserver: WebserverConfig{
			Port:                4000,
			CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		Core: CoreConfig{
			SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			Assets: AssetsConfig{
				SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
			},
		},
	}
}

func TestChattoConfig_Validate_RequiredSecrets(t *testing.T) {
	base := validTestConfig()

	tests := []struct {
		name     string
		modify   func(*ChattoConfig)
		errorMsg string
	}{
		{
			name: "missing core secret",
			modify: func(c *ChattoConfig) {
				c.Core.SecretKey = ""
			},
			errorMsg: "core.secret_key is required",
		},
		{
			name: "missing webserver cookie secret",
			modify: func(c *ChattoConfig) {
				c.Webserver.CookieSigningSecret = ""
			},
			errorMsg: "webserver.cookie_signing_secret is required",
		},
		{
			name: "missing asset signing secret",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.SigningSecret = ""
			},
			errorMsg: "core.assets.signing_secret is required",
		},
		{
			name: "core secret must be hex",
			modify: func(c *ChattoConfig) {
				c.Core.SecretKey = "not-hex"
			},
			errorMsg: "core.secret_key must be hex-encoded",
		},
		{
			name: "webserver cookie secret must be 32 bytes",
			modify: func(c *ChattoConfig) {
				c.Webserver.CookieSigningSecret = "000102"
			},
			errorMsg: "webserver.cookie_signing_secret must decode to 32 bytes",
		},
		{
			name: "asset signing secret must be 32 bytes",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.SigningSecret = "000102"
			},
			errorMsg: "core.assets.signing_secret must decode to 32 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.modify(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
				t.Fatalf("Validate() error = %v, want to contain %q", err, tt.errorMsg)
			}
		})
	}
}

func TestChattoConfig_Validate_OperatorAPI(t *testing.T) {
	t.Run("uses socket defaults", func(t *testing.T) {
		operatorAPI := OperatorAPIConfig{}
		if got := operatorAPI.SocketPathOrDefault(); got != "/tmp/chatto/operator.sock" {
			t.Fatalf("SocketPathOrDefault() = %q", got)
		}
	})

	t.Run("enabled accepts defaults", func(t *testing.T) {
		cfg := validTestConfig()
		cfg.OperatorAPI.Enabled = true
		err := cfg.Validate()
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("rejects configured socket mode", func(t *testing.T) {
		cfg := validTestConfig()
		cfg.OperatorAPI.Enabled = true
		cfg.OperatorAPI.SocketMode = "0600"
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "operator_api.socket_mode is no longer supported") {
			t.Fatalf("Validate() error = %v, want unsupported socket mode error", err)
		}
	})
}

func TestChattoConfig_Validate_CookieEncryptionSecret(t *testing.T) {
	base := validTestConfig()

	tests := []struct {
		name      string
		secret    string
		wantError string
	}{
		{
			name: "empty is allowed",
		},
		{
			name:   "16 byte key",
			secret: "000102030405060708090a0b0c0d0e0f",
		},
		{
			name:   "24 byte key",
			secret: "000102030405060708090a0b0c0d0e0f1011121314151617",
		},
		{
			name:   "32 byte key",
			secret: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		},
		{
			name:      "not hex",
			secret:    "not-hex",
			wantError: "webserver.cookie_encryption_secret must be hex-encoded",
		},
		{
			name:      "wrong decoded length",
			secret:    "000102030405060708090a0b0c0d0e",
			wantError: "webserver.cookie_encryption_secret must decode to 16, 24, or 32 bytes (got 15)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			cfg.Webserver.CookieEncryptionSecret = tt.secret
			err := cfg.Validate()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Validate() error = %v, want to contain %q", err, tt.wantError)
			}
		})
	}
}

func TestChattoConfig_Validate_LogFormat(t *testing.T) {
	base := validTestConfig()

	for _, format := range []string{"", "auto", "text", "json", "logfmt", "JSON"} {
		t.Run("valid_"+format, func(t *testing.T) {
			cfg := base
			cfg.General.LogFormat = format
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() unexpected error = %v", err)
			}
		})
	}

	cfg := base
	cfg.General.LogFormat = "pretty"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "general.log_format must be one of: auto, text, json, logfmt") {
		t.Fatalf("Validate() error = %v, want invalid log_format error", err)
	}
}

func TestChattoConfig_Validate_URLsAndOrigins(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*ChattoConfig)
		wantError string
	}{
		{
			name: "valid webserver URL and origins",
			modify: func(c *ChattoConfig) {
				c.Webserver.URL = "https://chat.example"
				c.Webserver.AllowedOrigins = []string{"https://client.example", "http://localhost:5173", "*"}
				c.Webserver.OAuthRedirectOrigins = []string{"https://client.example", "http://localhost:5173", "*"}
			},
		},
		{
			name: "webserver URL requires http or https",
			modify: func(c *ChattoConfig) {
				c.Webserver.URL = "chat.example"
			},
			wantError: "webserver.url must use http or https",
		},
		{
			name: "allowed origin rejects paths",
			modify: func(c *ChattoConfig) {
				c.Webserver.AllowedOrigins = []string{"https://client.example/path"}
			},
			wantError: "webserver.allowed_origins contains invalid origin",
		},
		{
			name: "OAuth origin requires https outside loopback",
			modify: func(c *ChattoConfig) {
				c.Webserver.OAuthRedirectOrigins = []string{"http://client.example"}
			},
			wantError: "non-loopback OAuth redirect origins must use https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTestConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Validate() error = %v, want to contain %q", err, tt.wantError)
			}
		})
	}
}

func TestChattoConfig_Validate_EnabledIntegrationsRequireWebserverURL(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*ChattoConfig)
		wantError string
	}{
		{
			name: "SMTP",
			modify: func(c *ChattoConfig) {
				c.SMTP.Enabled = true
				c.SMTP.Host = "smtp.example.com"
				c.SMTP.Port = 587
				c.SMTP.From = "noreply@example.com"
			},
			wantError: "webserver.url is required when SMTP is enabled",
		},
		{
			name: "auth provider",
			modify: func(c *ChattoConfig) {
				c.Auth.Providers = []AuthProviderConfig{{
					ID:           "hub",
					Type:         AuthProviderTypeOpenIDConnect,
					IssuerURL:    "https://id.example",
					ClientID:     "chatto",
					ClientSecret: "secret",
				}}
			},
			wantError: "webserver.url is required when auth providers are configured",
		},
		{
			name: "push",
			modify: func(c *ChattoConfig) {
				c.Push.Enabled = true
				c.Push.VAPIDPublicKey = "public-key"
				c.Push.VAPIDPrivateKey = "private-key"
				c.Push.VAPIDSubject = "mailto:admin@example.com"
			},
			wantError: "webserver.url is required when push is enabled",
		},
		{
			name: "LiveKit",
			modify: func(c *ChattoConfig) {
				c.LiveKit.Enabled = true
				c.LiveKit.URL = "wss://livekit.example"
				c.LiveKit.APIKey = "key"
				c.LiveKit.APISecret = "secret"
			},
			wantError: "webserver.url is required when LiveKit is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTestConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Validate() error = %v, want to contain %q", err, tt.wantError)
			}
		})
	}
}

func TestChattoConfig_ApplyDefaultsAndNormalize(t *testing.T) {
	cfg := validTestConfig()
	cfg.Webserver.URL = "https://chat.example"
	cfg.NATS.Embedded = EmbeddedNATSConfig{
		Enabled:   true,
		Port:      4222,
		AuthToken: "nats-token",
	}
	cfg.LiveKit = LiveKitConfig{
		Enabled:    true,
		URL:        "wss://livekit.example",
		APIKey:     "key",
		APISecret:  "secret",
		InstanceID: "legacy-server-id",
	}
	cfg.Core.Assets.StorageBackend = StorageBackendS3
	cfg.Core.Assets.S3 = S3Config{
		Endpoint:        "s3.amazonaws.com",
		Bucket:          "assets",
		PathPrefix:      "/tenant/chatto/",
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
	}
	cfg.Bootstrap.LegacyInstance = &BootstrapServer{Name: "Legacy"}
	cfg.Bootstrap.Users = []BootstrapUser{{Login: "alice", InstanceRole: "owner"}}

	cfg.ApplyDefaults()
	cfg.Normalize()

	if cfg.NATS.Client.URL != "nats://127.0.0.1:4222" {
		t.Fatalf("derived NATS client URL = %q", cfg.NATS.Client.URL)
	}
	if cfg.NATS.Client.AuthMethod != NATSAuthToken || cfg.NATS.Client.Token != "nats-token" {
		t.Fatalf("derived NATS client auth = %q/%q", cfg.NATS.Client.AuthMethod, cfg.NATS.Client.Token)
	}
	if cfg.LiveKit.ServerID != "legacy-server-id" {
		t.Fatalf("LiveKit server ID = %q", cfg.LiveKit.ServerID)
	}
	if cfg.LiveKit.WebhookURL != "https://chat.example/webhooks/livekit" {
		t.Fatalf("LiveKit webhook URL = %q", cfg.LiveKit.WebhookURL)
	}
	if cfg.Core.Assets.S3.PathPrefix != "tenant/chatto" {
		t.Fatalf("normalized S3 prefix = %q", cfg.Core.Assets.S3.PathPrefix)
	}
	if cfg.Bootstrap.Server == nil || cfg.Bootstrap.Server.Name != "Legacy" {
		t.Fatalf("bootstrap server alias was not applied: %+v", cfg.Bootstrap.Server)
	}
	if cfg.Bootstrap.Users[0].ServerRole != "owner" {
		t.Fatalf("bootstrap server_role alias = %q", cfg.Bootstrap.Users[0].ServerRole)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() after defaults failed: %v", err)
	}
}

func TestChattoConfig_ValidateDoesNotMutate(t *testing.T) {
	cfg := validTestConfig()
	cfg.Webserver.URL = "https://chat.example"
	cfg.LiveKit = LiveKitConfig{
		Enabled:   true,
		URL:       "wss://livekit.example",
		APIKey:    "key",
		APISecret: "secret",
	}
	cfg.Core.Assets.StorageBackend = StorageBackendS3
	cfg.Core.Assets.S3 = S3Config{
		Endpoint:        "s3.amazonaws.com",
		Bucket:          "assets",
		PathPrefix:      "/tenant/chatto/",
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error = %v", err)
	}
	if cfg.LiveKit.WebhookURL != "" {
		t.Fatalf("Validate() mutated LiveKit webhook URL to %q", cfg.LiveKit.WebhookURL)
	}
	if cfg.Core.Assets.S3.PathPrefix != "/tenant/chatto/" {
		t.Fatalf("Validate() mutated S3 path prefix to %q", cfg.Core.Assets.S3.PathPrefix)
	}
}

func TestChattoConfig_Validate_NATSClientTokenMatchesEmbedded(t *testing.T) {
	cfg := validTestConfig()
	cfg.NATS.Embedded = EmbeddedNATSConfig{
		Enabled:   true,
		Port:      4222,
		AuthToken: "embedded-token",
	}
	cfg.NATS.Client = NATSClientConfig{
		AuthMethod: NATSAuthToken,
		Token:      "other-token",
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "nats.client.token must match nats.embedded.auth_token") {
		t.Fatalf("Validate() error = %v, want NATS token mismatch", err)
	}
}

func TestReadConfig_DeprecatedServerAliases(t *testing.T) {
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
port = 4000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

[livekit]
instance_id = "legacy-server-id"

[[bootstrap.users]]
login = "alice"
instance_role = "owner"

[bootstrap.instance]
name = "Legacy Bootstrap"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if cfg.LiveKit.ServerID != "legacy-server-id" {
		t.Fatalf("LiveKit legacy instance_id alias = %q", cfg.LiveKit.ServerID)
	}
	if cfg.Bootstrap.Server == nil || cfg.Bootstrap.Server.Name != "Legacy Bootstrap" {
		t.Fatalf("bootstrap legacy instance alias = %+v", cfg.Bootstrap.Server)
	}
	if got := cfg.Bootstrap.Users[0].ServerRole; got != "owner" {
		t.Fatalf("bootstrap legacy instance_role alias = %q", got)
	}
}

func TestLimitsConfig_Defaults(t *testing.T) {
	c := &LimitsConfig{}
	if got := c.MaxUsersOrDefault(); got != -1 {
		t.Errorf("MaxUsersOrDefault() with unset = %d, want -1", got)
	}

	zero := 0
	c = &LimitsConfig{MaxUsers: &zero}
	if got := c.MaxUsersOrDefault(); got != 0 {
		t.Errorf("MaxUsersOrDefault() with explicit 0 = %d, want 0", got)
	}

	c = &LimitsConfig{MaxUsers: intPtr(100)}
	if got := c.MaxUsersOrDefault(); got != 100 {
		t.Errorf("MaxUsersOrDefault() with 100 = %d, want 100", got)
	}
}

func TestEmailOTPConfig_Defaults(t *testing.T) {
	c := &EmailOTPConfig{}
	if got := c.ThrottlingEnabledOrDefault(); got != true {
		t.Errorf("ThrottlingEnabledOrDefault() with unset = %v, want true", got)
	}
	if got := c.TTLOrDefault(); got != 15*time.Minute {
		t.Errorf("TTLOrDefault() with unset = %v, want 15m", got)
	}
	if got := c.MaxDeliveredCodesOrDefault(); got != 10 {
		t.Errorf("MaxDeliveredCodesOrDefault() with unset = %d, want 10", got)
	}
	if got := c.MaxWrongAttemptsOrDefault(); got != 5 {
		t.Errorf("MaxWrongAttemptsOrDefault() with unset = %d, want 5", got)
	}

	c = &EmailOTPConfig{
		ThrottlingEnabled: boolPtr(false),
		TTL:               Duration(30 * time.Minute),
		MaxDeliveredCodes: 3,
		MaxWrongAttempts:  2,
	}
	if got := c.ThrottlingEnabledOrDefault(); got != false {
		t.Errorf("ThrottlingEnabledOrDefault() with custom value = %v, want false", got)
	}
	if got := c.TTLOrDefault(); got != 30*time.Minute {
		t.Errorf("TTLOrDefault() with custom value = %v, want 30m", got)
	}
	if got := c.MaxDeliveredCodesOrDefault(); got != 3 {
		t.Errorf("MaxDeliveredCodesOrDefault() with custom value = %d, want 3", got)
	}
	if got := c.MaxWrongAttemptsOrDefault(); got != 2 {
		t.Errorf("MaxWrongAttemptsOrDefault() with custom value = %d, want 2", got)
	}
}

func TestReadConfig_EmailOTPFromTOML(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	configContent := `
[webserver]
port = 4000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

[auth.email_otp]
throttling_enabled = false
ttl = "30m"
max_delivered_codes = 4
max_wrong_attempts = 2
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if got := cfg.Auth.EmailOTP.TTLOrDefault(); got != 30*time.Minute {
		t.Errorf("auth.email_otp.ttl from TOML = %v, want 30m", got)
	}
	if got := cfg.Auth.EmailOTP.ThrottlingEnabledOrDefault(); got != false {
		t.Errorf("auth.email_otp.throttling_enabled from TOML = %v, want false", got)
	}
	if got := cfg.Auth.EmailOTP.MaxDeliveredCodesOrDefault(); got != 4 {
		t.Errorf("auth.email_otp.max_delivered_codes from TOML = %d, want 4", got)
	}
	if got := cfg.Auth.EmailOTP.MaxWrongAttemptsOrDefault(); got != 2 {
		t.Errorf("auth.email_otp.max_wrong_attempts from TOML = %d, want 2", got)
	}
}

func TestReadConfig_EmailOTPFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_AUTH_EMAIL_OTP_THROTTLING_ENABLED", "false")
	t.Setenv("CHATTO_AUTH_EMAIL_OTP_TTL", "45m")
	t.Setenv("CHATTO_AUTH_EMAIL_OTP_MAX_DELIVERED_CODES", "6")
	t.Setenv("CHATTO_AUTH_EMAIL_OTP_MAX_WRONG_ATTEMPTS", "3")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if got := cfg.Auth.EmailOTP.TTLOrDefault(); got != 45*time.Minute {
		t.Errorf("CHATTO_AUTH_EMAIL_OTP_TTL = %v, want 45m", got)
	}
	if got := cfg.Auth.EmailOTP.ThrottlingEnabledOrDefault(); got != false {
		t.Errorf("CHATTO_AUTH_EMAIL_OTP_THROTTLING_ENABLED = %v, want false", got)
	}
	if got := cfg.Auth.EmailOTP.MaxDeliveredCodesOrDefault(); got != 6 {
		t.Errorf("CHATTO_AUTH_EMAIL_OTP_MAX_DELIVERED_CODES = %d, want 6", got)
	}
	if got := cfg.Auth.EmailOTP.MaxWrongAttemptsOrDefault(); got != 3 {
		t.Errorf("CHATTO_AUTH_EMAIL_OTP_MAX_WRONG_ATTEMPTS = %d, want 3", got)
	}
}

func TestReadConfig_LimitsFromTOML(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	configContent := `
[webserver]
port = 4000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

[limits]
max_users = -1
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if got := cfg.Limits.MaxUsersOrDefault(); got != -1 {
		t.Errorf("MaxUsers from TOML = %d, want -1", got)
	}
}

func TestReadConfig_LimitsFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_LIMITS_MAX_USERS", "0")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if got := cfg.Limits.MaxUsersOrDefault(); got != 0 {
		t.Errorf("MaxUsers from env (explicit 0) = %d, want 0", got)
	}
}

func TestChattoConfig_Validate_Limits(t *testing.T) {
	base := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{Port: 4000, CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			Core:      CoreConfig{SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", Assets: AssetsConfig{SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"}},
		}
	}

	t.Run("rejects max_users below -1", func(t *testing.T) {
		c := base()
		c.Limits.MaxUsers = intPtr(-5)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "limits.max_users") {
			t.Errorf("expected limits.max_users validation error, got %v", err)
		}
	})

	t.Run("accepts -1, 0, positive", func(t *testing.T) {
		for _, v := range []int{-1, 0, 1, 100} {
			c := base()
			c.Limits.MaxUsers = intPtr(v)
			if err := c.Validate(); err != nil {
				t.Errorf("validate failed for %d: %v", v, err)
			}
		}
	})
}

func TestChattoConfig_Validate_EmailOTP(t *testing.T) {
	base := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{Port: 4000, CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			Core:      CoreConfig{SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", Assets: AssetsConfig{SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"}},
		}
	}

	tests := []struct {
		name      string
		mutate    func(*ChattoConfig)
		wantError string
	}{
		{
			name: "rejects negative ttl",
			mutate: func(c *ChattoConfig) {
				c.Auth.EmailOTP.TTL = Duration(-time.Minute)
			},
			wantError: "auth.email_otp.ttl",
		},
		{
			name: "rejects negative delivered-code limit",
			mutate: func(c *ChattoConfig) {
				c.Auth.EmailOTP.MaxDeliveredCodes = -1
			},
			wantError: "auth.email_otp.max_delivered_codes",
		},
		{
			name: "rejects negative wrong-attempt limit",
			mutate: func(c *ChattoConfig) {
				c.Auth.EmailOTP.MaxWrongAttempts = -1
			},
			wantError: "auth.email_otp.max_wrong_attempts",
		},
		{
			name: "accepts zero defaults and positive values",
			mutate: func(c *ChattoConfig) {
				c.Auth.EmailOTP = EmailOTPConfig{
					TTL:               Duration(10 * time.Minute),
					MaxDeliveredCodes: 1,
					MaxWrongAttempts:  1,
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Validate() error = %v, want to contain %q", err, tt.wantError)
			}
		})
	}
}

func TestChattoConfig_Validate_Metrics(t *testing.T) {
	base := validTestConfig()

	tests := []struct {
		name     string
		modify   func(*ChattoConfig)
		errorMsg string
	}{
		{
			name: "accepts enabled metrics with defaults",
			modify: func(c *ChattoConfig) {
				c.Metrics.Enabled = true
			},
		},
		{
			name: "rejects invalid port",
			modify: func(c *ChattoConfig) {
				c.Metrics.Enabled = true
				c.Metrics.Port = 70000
			},
			errorMsg: "metrics.port must be between 0 and 65535",
		},
		{
			name: "rejects relative path",
			modify: func(c *ChattoConfig) {
				c.Metrics.Enabled = true
				c.Metrics.Path = "metrics"
			},
			errorMsg: "metrics.path must start with /",
		},
		{
			name: "rejects query string in path",
			modify: func(c *ChattoConfig) {
				c.Metrics.Enabled = true
				c.Metrics.Path = "/metrics?token=secret"
			},
			errorMsg: "metrics.path must not contain query strings or fragments",
		},
		{
			name: "accepts enabled exporter with defaults",
			modify: func(c *ChattoConfig) {
				c.Exporter.Enabled = true
			},
		},
		{
			name: "rejects exporter invalid port",
			modify: func(c *ChattoConfig) {
				c.Exporter.Enabled = true
				c.Exporter.Port = 70000
			},
			errorMsg: "exporter.port must be between 0 and 65535",
		},
		{
			name: "rejects exporter relative path",
			modify: func(c *ChattoConfig) {
				c.Exporter.Enabled = true
				c.Exporter.Path = "metrics"
			},
			errorMsg: "exporter.path must start with /",
		},
		{
			name: "rejects exporter query string in path",
			modify: func(c *ChattoConfig) {
				c.Exporter.Enabled = true
				c.Exporter.Path = "/metrics?token=secret"
			},
			errorMsg: "exporter.path must not contain query strings or fragments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.modify(&cfg)
			err := cfg.Validate()
			if tt.errorMsg == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.errorMsg) {
				t.Fatalf("Validate() error = %v, want to contain %q", err, tt.errorMsg)
			}
		})
	}
}

func TestChattoConfig_Validate_TLS(t *testing.T) {
	baseConfig := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{
				Port:                4000,
				CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Core: CoreConfig{
				SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
				Assets: AssetsConfig{
					SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
				},
			},
		}
	}

	tests := []struct {
		name      string
		modify    func(*ChattoConfig)
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid config without TLS",
			modify:    func(c *ChattoConfig) {},
			wantError: false,
		},
		{
			name: "valid config with TLS",
			modify: func(c *ChattoConfig) {
				c.Webserver.TLS.Enabled = true
				c.Webserver.TLS.Domain = "example.com"
				c.Webserver.TLS.Email = "admin@example.com"
			},
			wantError: false,
		},
		{
			name: "TLS enabled without domain fails",
			modify: func(c *ChattoConfig) {
				c.Webserver.TLS.Enabled = true
				c.Webserver.TLS.Email = "admin@example.com"
			},
			wantError: true,
			errorMsg:  "webserver.tls.domain is required when TLS is enabled",
		},
		{
			name: "TLS enabled without email fails",
			modify: func(c *ChattoConfig) {
				c.Webserver.TLS.Enabled = true
				c.Webserver.TLS.Domain = "example.com"
			},
			wantError: true,
			errorMsg:  "webserver.tls.email is required when TLS is enabled",
		},
		{
			name: "port 0 allowed when TLS enabled",
			modify: func(c *ChattoConfig) {
				c.Webserver.Port = 0
				c.Webserver.TLS.Enabled = true
				c.Webserver.TLS.Domain = "example.com"
				c.Webserver.TLS.Email = "admin@example.com"
			},
			wantError: false,
		},
		{
			name: "port 0 not allowed when TLS disabled",
			modify: func(c *ChattoConfig) {
				c.Webserver.Port = 0
			},
			wantError: true,
			errorMsg:  "webserver.port is required when TLS is disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.modify(&cfg)

			err := cfg.Validate()
			if tt.wantError {
				if err == nil {
					t.Error("Validate() expected error, got nil")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Validate() error = %v, want to contain %v", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestEmbeddedNATSConfig_BindAddressOrDefault(t *testing.T) {
	tests := []struct {
		name        string
		bindAddress string
		want        string
	}{
		{
			name:        "empty returns localhost",
			bindAddress: "",
			want:        "127.0.0.1",
		},
		{
			name:        "custom value returned",
			bindAddress: "0.0.0.0",
			want:        "0.0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &EmbeddedNATSConfig{BindAddress: tt.bindAddress}
			if got := c.BindAddressOrDefault(); got != tt.want {
				t.Errorf("BindAddressOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChattoConfig_Validate_EmbeddedNATS(t *testing.T) {
	baseConfig := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{
				Port:                4000,
				CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Core: CoreConfig{
				SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
				Assets: AssetsConfig{
					SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
				},
			},
			NATS: NATSConfig{
				Embedded: EmbeddedNATSConfig{
					Enabled:   true,
					Port:      4222,
					HTTPPort:  8222,
					DataDir:   "./data",
					AuthToken: "test-token",
				},
			},
		}
	}

	tests := []struct {
		name      string
		modify    func(*ChattoConfig)
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid config with TCP port and token",
			modify:    func(c *ChattoConfig) {},
			wantError: false,
		},
		{
			name: "port 0 allowed (disables TCP listener)",
			modify: func(c *ChattoConfig) {
				c.NATS.Embedded.Port = 0
				c.NATS.Embedded.AuthToken = "" // Token not required when TCP disabled
			},
			wantError: false,
		},
		{
			name: "http_port 0 allowed (disables monitoring)",
			modify: func(c *ChattoConfig) {
				c.NATS.Embedded.HTTPPort = 0
			},
			wantError: false,
		},
		{
			name: "TCP port enabled without token fails",
			modify: func(c *ChattoConfig) {
				c.NATS.Embedded.Port = 4222
				c.NATS.Embedded.AuthToken = ""
			},
			wantError: true,
			errorMsg:  "nats.embedded.auth_token is required when TCP port is enabled",
		},
		{
			name: "invalid port fails",
			modify: func(c *ChattoConfig) {
				c.NATS.Embedded.Port = -1
			},
			wantError: true,
			errorMsg:  "nats.embedded.port must be between 0 and 65535",
		},
		{
			name: "invalid http_port fails",
			modify: func(c *ChattoConfig) {
				c.NATS.Embedded.HTTPPort = 70000
			},
			wantError: true,
			errorMsg:  "nats.embedded.http_port must be between 0 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.modify(&cfg)

			err := cfg.Validate()
			if tt.wantError {
				if err == nil {
					t.Error("Validate() expected error, got nil")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Validate() error = %v, want to contain %v", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestAuthConfig_EnabledProviders(t *testing.T) {
	tests := []struct {
		name string
		auth AuthConfig
		want []string
	}{
		{
			name: "empty config returns empty slice",
			auth: AuthConfig{},
			want: nil,
		},
		{
			name: "returns configured provider ids",
			auth: AuthConfig{Providers: []AuthProviderConfig{
				{ID: "hub", Type: AuthProviderTypeOpenIDConnect},
				{ID: "github-main", Type: AuthProviderTypeGitHub},
			}},
			want: []string{"hub", "github-main"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.auth.EnabledProviders()
			if len(got) != len(tt.want) {
				t.Errorf("EnabledProviders() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("EnabledProviders()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAuthConfig_PublicProviders(t *testing.T) {
	auth := AuthConfig{Providers: []AuthProviderConfig{
		{ID: "hub", Type: AuthProviderTypeOpenIDConnect, Label: "Chatto Hub", ClientID: "id", ClientSecret: "secret", IssuerURL: "https://issuer.example"},
		{ID: "github-main", Type: AuthProviderTypeGitHub, ClientID: "id", ClientSecret: "secret"},
	}}

	got := auth.PublicProviders()
	if len(got) != 2 {
		t.Fatalf("PublicProviders() len = %d, want 2", len(got))
	}
	if got[0].ID != "hub" || got[0].Type != AuthProviderTypeOpenIDConnect || got[0].Label != "Chatto Hub" {
		t.Fatalf("PublicProviders()[0] = %+v", got[0])
	}
	if got[1].ID != "github-main" || got[1].Type != AuthProviderTypeGitHub || got[1].Label != "GitHub" {
		t.Fatalf("PublicProviders()[1] = %+v", got[1])
	}
	if got[0].ClientID != "" || got[0].ClientSecret != "" || got[0].IssuerURL != "" {
		t.Fatalf("PublicProviders leaked provider secrets/options: %+v", got[0])
	}
}

func TestChattoConfig_Validate_AuthProviders(t *testing.T) {
	baseConfig := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{
				URL:                 "https://chat.example",
				Port:                4000,
				CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Core: CoreConfig{
				SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
				Assets:    AssetsConfig{SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"},
			},
		}
	}

	t.Run("accepts curated providers", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Auth.Providers = []AuthProviderConfig{
			{ID: "hub", Type: AuthProviderTypeOpenIDConnect, ClientID: "id", ClientSecret: "secret", IssuerURL: "https://issuer.example"},
			{ID: "github-main", Type: AuthProviderTypeGitHub, ClientID: "id", ClientSecret: "secret"},
			{ID: "gitlab-main", Type: AuthProviderTypeGitLab, ClientID: "id", ClientSecret: "secret"},
			{ID: "google-main", Type: AuthProviderTypeGoogle, ClientID: "id", ClientSecret: "secret"},
			{ID: "discord-main", Type: AuthProviderTypeDiscord, ClientID: "id", ClientSecret: "secret"},
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() unexpected error = %v", err)
		}
	})

	t.Run("rejects unknown provider", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Auth.Providers = []AuthProviderConfig{{ID: "apple", Type: "apple", ClientID: "id", ClientSecret: "secret"}}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "auth.providers[0].type") {
			t.Fatalf("Validate() error = %v, want provider type error", err)
		}
	})

	t.Run("rejects microsoft provider for now", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Auth.Providers = []AuthProviderConfig{{ID: "azure", Type: "microsoftonline", ClientID: "id", ClientSecret: "secret"}}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "auth.providers[0].type") {
			t.Fatalf("Validate() error = %v, want provider type error", err)
		}
	})

	t.Run("rejects duplicate provider ids", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Auth.Providers = []AuthProviderConfig{
			{ID: "github", Type: AuthProviderTypeGitHub, ClientID: "id", ClientSecret: "secret"},
			{ID: "github", Type: AuthProviderTypeGitLab, ClientID: "id", ClientSecret: "secret"},
		}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "configured more than once") {
			t.Fatalf("Validate() error = %v, want duplicate id error", err)
		}
	})

	t.Run("rejects oidc without issuer", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Auth.Providers = []AuthProviderConfig{{ID: "hub", Type: AuthProviderTypeOpenIDConnect, ClientID: "id", ClientSecret: "secret"}}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "issuer_url is required") {
			t.Fatalf("Validate() error = %v, want issuer_url error", err)
		}
	})

	t.Run("rejects oidc with relative issuer", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Auth.Providers = []AuthProviderConfig{{ID: "hub", Type: AuthProviderTypeOpenIDConnect, ClientID: "id", ClientSecret: "secret", IssuerURL: "chatto-id"}}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "auth.providers[0].issuer_url must use http or https") {
			t.Fatalf("Validate() error = %v, want issuer_url absolute URL error", err)
		}
	})
}

func TestChattoConfig_Validate_SMTP(t *testing.T) {
	baseConfig := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{
				Port:                4000,
				CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Core: CoreConfig{
				SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
				Assets: AssetsConfig{
					SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
				},
			},
		}
	}

	tests := []struct {
		name      string
		modify    func(*ChattoConfig)
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid config without SMTP",
			modify:    func(c *ChattoConfig) {},
			wantError: false,
		},
		{
			name: "valid config with SMTP",
			modify: func(c *ChattoConfig) {
				c.Webserver.URL = "https://chat.example"
				c.SMTP.Enabled = true
				c.SMTP.Host = "smtp.example.com"
				c.SMTP.Port = 587
				c.SMTP.From = "noreply@example.com"
			},
			wantError: false,
		},
		{
			name: "valid config with explicit mandatory SMTP TLS",
			modify: func(c *ChattoConfig) {
				c.Webserver.URL = "https://chat.example"
				c.SMTP.Enabled = true
				c.SMTP.Host = "smtp.example.com"
				c.SMTP.Port = 587
				c.SMTP.TLS = SMTPTLSMandatory
				c.SMTP.From = "noreply@example.com"
			},
			wantError: false,
		},
		{
			name: "valid config with explicit opportunistic SMTP TLS",
			modify: func(c *ChattoConfig) {
				c.Webserver.URL = "https://chat.example"
				c.SMTP.Enabled = true
				c.SMTP.Host = "smtp.example.com"
				c.SMTP.Port = 587
				c.SMTP.TLS = SMTPTLSOpportunistic
				c.SMTP.From = "noreply@example.com"
			},
			wantError: false,
		},
		{
			name: "valid config with explicit implicit SMTP TLS",
			modify: func(c *ChattoConfig) {
				c.Webserver.URL = "https://chat.example"
				c.SMTP.Enabled = true
				c.SMTP.Host = "smtp.example.com"
				c.SMTP.Port = 465
				c.SMTP.TLS = SMTPTLSImplicit
				c.SMTP.From = "noreply@example.com"
			},
			wantError: false,
		},
		{
			name: "invalid SMTP TLS policy fails",
			modify: func(c *ChattoConfig) {
				c.SMTP.TLS = "plaintext"
			},
			wantError: true,
			errorMsg:  "smtp.tls must be one of: mandatory, opportunistic, implicit",
		},
		{
			name: "SMTP enabled without host fails",
			modify: func(c *ChattoConfig) {
				c.SMTP.Enabled = true
				c.SMTP.Port = 587
				c.SMTP.From = "noreply@example.com"
			},
			wantError: true,
			errorMsg:  "smtp.host is required when SMTP is enabled",
		},
		{
			name: "SMTP enabled without port fails",
			modify: func(c *ChattoConfig) {
				c.SMTP.Enabled = true
				c.SMTP.Host = "smtp.example.com"
				c.SMTP.From = "noreply@example.com"
			},
			wantError: true,
			errorMsg:  "smtp.port must be between 1 and 65535 when SMTP is enabled",
		},
		{
			name: "SMTP enabled without from fails",
			modify: func(c *ChattoConfig) {
				c.SMTP.Enabled = true
				c.SMTP.Host = "smtp.example.com"
				c.SMTP.Port = 587
			},
			wantError: true,
			errorMsg:  "smtp.from is required when SMTP is enabled",
		},
		{
			name: "SMTP enabled with invalid port fails",
			modify: func(c *ChattoConfig) {
				c.SMTP.Enabled = true
				c.SMTP.Host = "smtp.example.com"
				c.SMTP.Port = 70000
				c.SMTP.From = "noreply@example.com"
			},
			wantError: true,
			errorMsg:  "smtp.port must be between 1 and 65535 when SMTP is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.modify(&cfg)

			err := cfg.Validate()
			if tt.wantError {
				if err == nil {
					t.Error("Validate() expected error, got nil")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Validate() error = %v, want to contain %v", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestChattoConfig_Validate_S3(t *testing.T) {
	baseConfig := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{
				Port:                4000,
				CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Core: CoreConfig{
				SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
				Assets: AssetsConfig{
					SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
				},
			},
		}
	}

	tests := []struct {
		name      string
		modify    func(*ChattoConfig)
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid config without S3 (default NATS storage)",
			modify:    func(c *ChattoConfig) {},
			wantError: false,
		},
		{
			name: "valid config with S3 backend",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.StorageBackend = StorageBackendS3
				c.Core.Assets.S3 = S3Config{
					Endpoint:        "s3.amazonaws.com",
					Bucket:          "test-bucket",
					Region:          "us-east-1",
					AccessKeyID:     "test-key",
					SecretAccessKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				}
			},
			wantError: false,
		},
		{
			name: "valid S3 backend with empty path prefix",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.StorageBackend = StorageBackendS3
				c.Core.Assets.S3 = S3Config{
					Endpoint:        "s3.amazonaws.com",
					Bucket:          "test-bucket",
					PathPrefix:      "/",
					AccessKeyID:     "test-key",
					SecretAccessKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				}
			},
			wantError: false,
		},
		{
			name: "valid S3 backend normalizes path prefix",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.StorageBackend = StorageBackendS3
				c.Core.Assets.S3 = S3Config{
					Endpoint:        "s3.amazonaws.com",
					Bucket:          "test-bucket",
					PathPrefix:      "/tenant-a/chatto/",
					AccessKeyID:     "test-key",
					SecretAccessKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				}
			},
			wantError: false,
		},
		{
			name: "S3 backend with empty path segment fails",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.StorageBackend = StorageBackendS3
				c.Core.Assets.S3 = S3Config{
					Endpoint:        "s3.amazonaws.com",
					Bucket:          "test-bucket",
					PathPrefix:      "tenant//chatto",
					AccessKeyID:     "test-key",
					SecretAccessKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				}
			},
			wantError: true,
			errorMsg:  "core.assets.s3.path_prefix must not contain empty path segments",
		},
		{
			name: "S3 backend with control character path prefix fails",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.StorageBackend = StorageBackendS3
				c.Core.Assets.S3 = S3Config{
					Endpoint:        "s3.amazonaws.com",
					Bucket:          "test-bucket",
					PathPrefix:      "tenant\nchatto",
					AccessKeyID:     "test-key",
					SecretAccessKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				}
			},
			wantError: true,
			errorMsg:  "core.assets.s3.path_prefix must not contain control characters",
		},
		{
			name: "S3 backend without endpoint fails",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.StorageBackend = StorageBackendS3
				c.Core.Assets.S3 = S3Config{
					Bucket:          "test-bucket",
					AccessKeyID:     "test-key",
					SecretAccessKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				}
			},
			wantError: true,
			errorMsg:  "core.assets.s3.endpoint is required when storage_backend = 's3'",
		},
		{
			name: "S3 backend without bucket fails",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.StorageBackend = StorageBackendS3
				c.Core.Assets.S3 = S3Config{
					Endpoint:        "s3.amazonaws.com",
					AccessKeyID:     "test-key",
					SecretAccessKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				}
			},
			wantError: true,
			errorMsg:  "core.assets.s3.bucket is required when storage_backend = 's3'",
		},
		{
			name: "S3 backend without access_key_id fails",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.StorageBackend = StorageBackendS3
				c.Core.Assets.S3 = S3Config{
					Endpoint:        "s3.amazonaws.com",
					Bucket:          "test-bucket",
					SecretAccessKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				}
			},
			wantError: true,
			errorMsg:  "core.assets.s3.access_key_id is required when storage_backend = 's3'",
		},
		{
			name: "S3 backend without secret_access_key fails",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.StorageBackend = StorageBackendS3
				c.Core.Assets.S3 = S3Config{
					Endpoint:    "s3.amazonaws.com",
					Bucket:      "test-bucket",
					AccessKeyID: "test-key",
				}
			},
			wantError: true,
			errorMsg:  "core.assets.s3.secret_access_key is required when storage_backend = 's3'",
		},
		{
			name: "invalid storage backend fails",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.StorageBackend = "invalid"
			},
			wantError: true,
			errorMsg:  "core.assets.storage_backend must be 'nats' or 's3'",
		},
		{
			name: "explicit NATS backend is valid",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.StorageBackend = StorageBackendNATS
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.modify(&cfg)

			err := cfg.Validate()
			if tt.wantError {
				if err == nil {
					t.Error("Validate() expected error, got nil")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Validate() error = %v, want to contain %v", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestS3Config_Defaults(t *testing.T) {
	// Test UseSSLOrDefault
	t.Run("UseSSLOrDefault defaults to true", func(t *testing.T) {
		cfg := S3Config{}
		if !cfg.UseSSLOrDefault() {
			t.Error("UseSSLOrDefault() should return true when UseSSL is nil")
		}
	})

	t.Run("UseSSLOrDefault returns configured value", func(t *testing.T) {
		useSsl := false
		cfg := S3Config{UseSSL: &useSsl}
		if cfg.UseSSLOrDefault() {
			t.Error("UseSSLOrDefault() should return false when UseSSL is false")
		}
	})

	// Test PathStyleOrDefault
	t.Run("PathStyleOrDefault defaults to false", func(t *testing.T) {
		cfg := S3Config{}
		if cfg.PathStyleOrDefault() {
			t.Error("PathStyleOrDefault() should return false when PathStyle is nil")
		}
	})

	t.Run("PathStyleOrDefault returns configured value", func(t *testing.T) {
		pathStyle := true
		cfg := S3Config{PathStyle: &pathStyle}
		if !cfg.PathStyleOrDefault() {
			t.Error("PathStyleOrDefault() should return true when PathStyle is true")
		}
	})
}

func TestS3Config_UsePathStyleForEndpoint(t *testing.T) {
	boolPtr := func(v bool) *bool { return &v }

	tests := []struct {
		name string
		cfg  S3Config
		want bool
	}{
		{
			name: "defaults to path-style for localhost endpoint",
			cfg:  S3Config{Endpoint: "localhost:9000"},
			want: true,
		},
		{
			name: "defaults to path-style for IP endpoint",
			cfg:  S3Config{Endpoint: "http://127.0.0.1:9000"},
			want: true,
		},
		{
			name: "defaults to path-style for custom domain endpoint",
			cfg:  S3Config{Endpoint: "minio.example.com"},
			want: true,
		},
		{
			name: "defaults to virtual-hosted style for global AWS endpoint",
			cfg:  S3Config{Endpoint: "s3.amazonaws.com"},
			want: false,
		},
		{
			name: "defaults to virtual-hosted style for regional AWS endpoint",
			cfg:  S3Config{Endpoint: "https://s3.us-east-1.amazonaws.com"},
			want: false,
		},
		{
			name: "explicit false overrides custom endpoint default",
			cfg:  S3Config{Endpoint: "localhost:9000", PathStyle: boolPtr(false)},
			want: false,
		},
		{
			name: "explicit true overrides AWS endpoint default",
			cfg:  S3Config{Endpoint: "s3.amazonaws.com", PathStyle: boolPtr(true)},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.UsePathStyleForEndpoint(); got != tt.want {
				t.Errorf("UsePathStyleForEndpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPushConfig_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  PushConfig
		want bool
	}{
		{
			name: "empty config returns false",
			cfg:  PushConfig{},
			want: false,
		},
		{
			name: "enabled but missing all keys returns false",
			cfg: PushConfig{
				Enabled: true,
			},
			want: false,
		},
		{
			name: "enabled but missing public key returns false",
			cfg: PushConfig{
				Enabled:         true,
				VAPIDPrivateKey: "private-key",
				VAPIDSubject:    "mailto:admin@example.com",
			},
			want: false,
		},
		{
			name: "enabled but missing private key returns false",
			cfg: PushConfig{
				Enabled:        true,
				VAPIDPublicKey: "public-key",
				VAPIDSubject:   "mailto:admin@example.com",
			},
			want: false,
		},
		{
			name: "enabled but missing subject returns false",
			cfg: PushConfig{
				Enabled:         true,
				VAPIDPublicKey:  "public-key",
				VAPIDPrivateKey: "private-key",
			},
			want: false,
		},
		{
			name: "all fields set but not enabled returns false",
			cfg: PushConfig{
				Enabled:         false,
				VAPIDPublicKey:  "public-key",
				VAPIDPrivateKey: "private-key",
				VAPIDSubject:    "mailto:admin@example.com",
			},
			want: false,
		},
		{
			name: "fully configured returns true",
			cfg: PushConfig{
				Enabled:         true,
				VAPIDPublicKey:  "public-key",
				VAPIDPrivateKey: "private-key",
				VAPIDSubject:    "mailto:admin@example.com",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChattoConfig_Validate_Push(t *testing.T) {
	baseConfig := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{
				Port:                4000,
				CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Core: CoreConfig{
				SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
				Assets: AssetsConfig{
					SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
				},
			},
		}
	}

	tests := []struct {
		name      string
		modify    func(*ChattoConfig)
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid config without push",
			modify:    func(c *ChattoConfig) {},
			wantError: false,
		},
		{
			name: "valid config with push",
			modify: func(c *ChattoConfig) {
				c.Webserver.URL = "https://chat.example"
				c.Push.Enabled = true
				c.Push.VAPIDPublicKey = "public-key"
				c.Push.VAPIDPrivateKey = "private-key"
				c.Push.VAPIDSubject = "mailto:admin@example.com"
			},
			wantError: false,
		},
		{
			name: "push enabled without public key fails",
			modify: func(c *ChattoConfig) {
				c.Push.Enabled = true
				c.Push.VAPIDPrivateKey = "private-key"
				c.Push.VAPIDSubject = "mailto:admin@example.com"
			},
			wantError: true,
			errorMsg:  "push.vapid_public_key is required when push is enabled",
		},
		{
			name: "push enabled without private key fails",
			modify: func(c *ChattoConfig) {
				c.Push.Enabled = true
				c.Push.VAPIDPublicKey = "public-key"
				c.Push.VAPIDSubject = "mailto:admin@example.com"
			},
			wantError: true,
			errorMsg:  "push.vapid_private_key is required when push is enabled",
		},
		{
			name: "push enabled without subject fails",
			modify: func(c *ChattoConfig) {
				c.Push.Enabled = true
				c.Push.VAPIDPublicKey = "public-key"
				c.Push.VAPIDPrivateKey = "private-key"
			},
			wantError: true,
			errorMsg:  "push.vapid_subject is required when push is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.modify(&cfg)

			err := cfg.Validate()
			if tt.wantError {
				if err == nil {
					t.Error("Validate() expected error, got nil")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Validate() error = %v, want to contain %v", err, tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestDuration_UnmarshalText(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		// Extended format with days
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"0d", 0, false},

		// Extended format with weeks
		{"1w", 7 * 24 * time.Hour, false},
		{"2w", 14 * 24 * time.Hour, false},

		// Standard Go duration format
		{"168h", 168 * time.Hour, false},
		{"24h30m", 24*time.Hour + 30*time.Minute, false},
		{"1h", time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"1h30m45s", time.Hour + 30*time.Minute + 45*time.Second, false},

		// Combined formats (go-str2duration supports these)
		{"1d2h", 26 * time.Hour, false},
		{"1w1d", 8 * 24 * time.Hour, false},

		// Invalid formats
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var d Duration
			err := d.UnmarshalText([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalText(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && d.Duration() != tt.want {
				t.Errorf("UnmarshalText(%q) = %v, want %v", tt.input, d.Duration(), tt.want)
			}
		})
	}
}

func TestOwnersConfig_IsServerOwnerEmail(t *testing.T) {
	cfg := &OwnersConfig{Emails: []string{"Owner@Example.com", "  ops@example.com  "}}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"exact match", "Owner@Example.com", true},
		{"different case in user input", "owner@example.com", true},
		{"different case in config", "OWNER@EXAMPLE.COM", true},
		{"surrounding whitespace tolerated on input", "  owner@example.com  ", true},
		{"surrounding whitespace tolerated in config", "ops@example.com", true},
		{"non-owner email", "other@example.com", false},
		{"empty string", "", false},
		{"substring is not enough", "owner@example.co", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.IsServerOwnerEmail(tt.input); got != tt.want {
				t.Errorf("IsServerOwnerEmail(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
