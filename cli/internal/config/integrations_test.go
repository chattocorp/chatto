package config

import (
	"os"
	"strings"
	"testing"
)

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
