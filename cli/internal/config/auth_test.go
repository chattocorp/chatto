package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

	t.Run("accepts public oidc client without secret", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Auth.Providers = []AuthProviderConfig{{
			ID: "authling", Type: AuthProviderTypeOpenIDConnect,
			ClientID: "https://chat.example/oauth/client-metadata.json", IssuerURL: "https://auth.example",
		}}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() unexpected error = %v", err)
		}
	})

	t.Run("still requires secret for oauth-only provider", func(t *testing.T) {
		cfg := baseConfig()
		cfg.Auth.Providers = []AuthProviderConfig{{ID: "github", Type: AuthProviderTypeGitHub, ClientID: "id"}}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "client_secret is required") {
			t.Fatalf("Validate() error = %v, want client secret error", err)
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
