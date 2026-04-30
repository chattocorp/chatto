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
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "test-cookie-secret")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "test-assets-secret")

	// ReadConfig should succeed even without chatto.toml
	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed without config file: %v", err)
	}

	// Verify env vars were applied
	if cfg.Webserver.Port != 4000 {
		t.Errorf("expected port 4000, got %d", cfg.Webserver.Port)
	}
	if cfg.Webserver.CookieSigningSecret != "test-cookie-secret" {
		t.Errorf("expected cookie secret to be set from env var")
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
cookie_signing_secret = "file-cookie-secret"

[core.assets]
signing_secret = "file-assets-secret"
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
cookie_signing_secret = "file-cookie-secret"

[core.assets]
signing_secret = "file-assets-secret"
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

func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}

func TestLimitsConfig_Defaults(t *testing.T) {
	c := &LimitsConfig{}
	if got := c.MaxSpacesOrDefault(); got != -1 {
		t.Errorf("MaxSpacesOrDefault() with unset = %d, want -1", got)
	}
	if got := c.MaxUsersOrDefault(); got != -1 {
		t.Errorf("MaxUsersOrDefault() with unset = %d, want -1", got)
	}

	zero := 0
	c = &LimitsConfig{MaxSpaces: &zero, MaxUsers: &zero}
	if got := c.MaxSpacesOrDefault(); got != 0 {
		t.Errorf("MaxSpacesOrDefault() with explicit 0 = %d, want 0", got)
	}
	if got := c.MaxUsersOrDefault(); got != 0 {
		t.Errorf("MaxUsersOrDefault() with explicit 0 = %d, want 0", got)
	}

	c = &LimitsConfig{MaxSpaces: intPtr(42), MaxUsers: intPtr(100)}
	if got := c.MaxSpacesOrDefault(); got != 42 {
		t.Errorf("MaxSpacesOrDefault() with 42 = %d, want 42", got)
	}
	if got := c.MaxUsersOrDefault(); got != 100 {
		t.Errorf("MaxUsersOrDefault() with 100 = %d, want 100", got)
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
cookie_signing_secret = "x"

[core.assets]
signing_secret = "y"

[limits]
max_spaces = 5
max_users = -1
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if got := cfg.Limits.MaxSpacesOrDefault(); got != 5 {
		t.Errorf("MaxSpaces from TOML = %d, want 5", got)
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
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "x")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "y")
	t.Setenv("CHATTO_LIMITS_MAX_SPACES", "7")
	t.Setenv("CHATTO_LIMITS_MAX_USERS", "0")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if got := cfg.Limits.MaxSpacesOrDefault(); got != 7 {
		t.Errorf("MaxSpaces from env = %d, want 7", got)
	}
	if got := cfg.Limits.MaxUsersOrDefault(); got != 0 {
		t.Errorf("MaxUsers from env (explicit 0) = %d, want 0", got)
	}
}

func TestChattoConfig_Validate_Limits(t *testing.T) {
	base := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{Port: 4000, CookieSigningSecret: "x"},
			Core:      CoreConfig{Assets: AssetsConfig{SigningSecret: "y"}},
		}
	}

	t.Run("rejects max_spaces below -1", func(t *testing.T) {
		c := base()
		c.Limits.MaxSpaces = intPtr(-2)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "limits.max_spaces") {
			t.Errorf("expected limits.max_spaces validation error, got %v", err)
		}
	})

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
			c.Limits.MaxSpaces = intPtr(v)
			c.Limits.MaxUsers = intPtr(v)
			if err := c.Validate(); err != nil {
				t.Errorf("validate failed for %d: %v", v, err)
			}
		}
	})
}

func TestChattoConfig_Validate_TLS(t *testing.T) {
	baseConfig := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{
				Port:                4000,
				CookieSigningSecret: "test-secret",
			},
			Core: CoreConfig{
				Assets: AssetsConfig{
					SigningSecret: "test-asset-secret",
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
				CookieSigningSecret: "test-secret",
			},
			Core: CoreConfig{
				Assets: AssetsConfig{
					SigningSecret: "test-asset-secret",
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

func TestChattoConfig_Validate_SMTP(t *testing.T) {
	baseConfig := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{
				Port:                4000,
				CookieSigningSecret: "test-secret",
			},
			Core: CoreConfig{
				Assets: AssetsConfig{
					SigningSecret: "test-asset-secret",
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
				c.SMTP.Enabled = true
				c.SMTP.Host = "smtp.example.com"
				c.SMTP.Port = 587
				c.SMTP.From = "noreply@example.com"
			},
			wantError: false,
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
				CookieSigningSecret: "test-secret",
			},
			Core: CoreConfig{
				Assets: AssetsConfig{
					SigningSecret: "test-asset-secret",
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
					SecretAccessKey: "test-secret",
				}
			},
			wantError: false,
		},
		{
			name: "S3 backend without endpoint fails",
			modify: func(c *ChattoConfig) {
				c.Core.Assets.StorageBackend = StorageBackendS3
				c.Core.Assets.S3 = S3Config{
					Bucket:          "test-bucket",
					AccessKeyID:     "test-key",
					SecretAccessKey: "test-secret",
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
					SecretAccessKey: "test-secret",
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
					SecretAccessKey: "test-secret",
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
				CookieSigningSecret: "test-secret",
			},
			Core: CoreConfig{
				Assets: AssetsConfig{
					SigningSecret: "test-asset-secret",
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

func TestAdminConfig_IsInstanceAdminEmail(t *testing.T) {
	cfg := &AdminConfig{Emails: []string{"Admin@Example.com", "  ops@example.com  "}}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"exact match", "Admin@Example.com", true},
		{"different case in user input", "admin@example.com", true},
		{"different case in config", "ADMIN@EXAMPLE.COM", true},
		{"surrounding whitespace tolerated on input", "  admin@example.com  ", true},
		{"surrounding whitespace tolerated in config", "ops@example.com", true},
		{"non-admin email", "other@example.com", false},
		{"empty string", "", false},
		{"substring is not enough", "admin@example.co", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.IsInstanceAdminEmail(tt.input); got != tt.want {
				t.Errorf("IsInstanceAdminEmail(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
