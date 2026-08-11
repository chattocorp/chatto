package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"hmans.de/chatto/pkg/natsauth"
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
	t.Setenv("CHATTO_WEBSERVER_API_COMPRESSION", "false")
	t.Setenv("CHATTO_WEBSERVER_API_COMPRESSION_MIN_BYTES", "8192")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_SEARCH_ENABLED", "true")
	t.Setenv("CHATTO_SEARCH_PROVIDER_ENABLED", "true")
	t.Setenv("CHATTO_SEARCH_PROVIDER_DIRECTORY", "./custom-search")
	t.Setenv("CHATTO_SEARCH_PROVIDER_LANGUAGES", "de,en,cjk")

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
	if cfg.Webserver.APICompressionEnabled() {
		t.Error("expected API response compression disabled from env var")
	}
	if got := cfg.Webserver.APICompressionMinBytesOrDefault(); got != 8192 {
		t.Errorf("APICompressionMinBytesOrDefault() = %d, want 8192", got)
	}
	if cfg.Core.SecretKey != "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" {
		t.Errorf("expected core secret to be set from env var")
	}
	if cfg.Webserver.Shields.Enabled {
		t.Errorf("expected shields to default disabled")
	}
	if !cfg.Search.Enabled || !cfg.SearchProvider.Enabled {
		t.Error("expected search and bundled provider enabled from environment")
	}
	if got := cfg.SearchProvider.DirectoryOrDefault(); got != "./custom-search" {
		t.Errorf("search provider directory = %q, want %q", got, "./custom-search")
	}
	if got := cfg.SearchProvider.LanguagesOrDefault(); !slices.Equal(got, []string{"cjk", "de", "en"}) {
		t.Errorf("search provider languages = %v", got)
	}
}

func TestReadConfig_AllowsUnknownTOMLFieldsForCompatibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chatto.toml")
	if err := os.WriteFile(path, []byte(`
future_option = true

[webserver]
port = 4000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadConfig(path); err != nil {
		t.Fatalf("ReadConfig() rejected an unknown compatibility field: %v", err)
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
projection_snapshots = true
projection_snapshot_retention = "9d"
projection_snapshot_s3_cleanup = false

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

[search]
enabled = true

[search_provider]
enabled = true
directory = "./search-index"
languages = ["fr", "en"]
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
	if !cfg.Core.ProjectionSnapshots {
		t.Error("expected projection snapshots from file")
	}
	if cfg.Core.ProjectionSnapshotRetentionOrDefault() != 9*24*time.Hour {
		t.Errorf("projection snapshot retention = %s", cfg.Core.ProjectionSnapshotRetentionOrDefault())
	}
	if cfg.Core.ProjectionSnapshotS3CleanupOrDefault() {
		t.Error("expected S3 snapshot cleanup to be disabled from file")
	}
	if !cfg.Search.Enabled || !cfg.SearchProvider.Enabled {
		t.Error("expected search and bundled provider enabled from config file")
	}
	if got := cfg.SearchProvider.DirectoryOrDefault(); got != "./search-index" {
		t.Errorf("search provider directory = %q, want %q", got, "./search-index")
	}
	if got := cfg.SearchProvider.LanguagesOrDefault(); !slices.Equal(got, []string{"en", "fr"}) {
		t.Errorf("search provider languages = %v", got)
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

func TestChattoConfig_Validate_AccountCreationPolicy(t *testing.T) {
	for _, policy := range []string{"", AccountCreationPolicyOpen, AccountCreationPolicyInviteOnly} {
		cfg := validTestConfig()
		cfg.Auth.AccountCreationPolicy = policy
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() policy %q: %v", policy, err)
		}
	}

	cfg := validTestConfig()
	cfg.Auth.AccountCreationPolicy = "closed"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "auth.account_creation_policy") {
		t.Fatalf("Validate() invalid policy error = %v", err)
	}
}

func TestChattoConfig_Validate_FrontendAuthling(t *testing.T) {
	tests := []struct {
		name      string
		publicURL string
		issuer    string
		wantError string
	}{
		{name: "accepts HTTPS origins", publicURL: "https://chat.example", issuer: "https://id.example"},
		{name: "rejects missing public URL", issuer: "https://id.example", wantError: "webserver.url is required"},
		{name: "rejects HTTP public URL", publicURL: "http://chat.example", issuer: "https://id.example", wantError: "webserver.url must use https"},
		{name: "rejects insecure issuer", publicURL: "https://chat.example", issuer: "http://id.example", wantError: "frontend.authling_issuer must use https"},
		{name: "accepts loopback development issuer", publicURL: "https://chat.example", issuer: "http://localhost:8080"},
		{name: "rejects issuer path", publicURL: "https://chat.example", issuer: "https://id.example/tenant", wantError: "without user info, path, query, or fragment"},
		{name: "rejects issuer query", publicURL: "https://chat.example", issuer: "https://id.example?tenant=one", wantError: "without user info, path, query, or fragment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.Webserver.URL = tt.publicURL
			cfg.Frontend.AuthlingIssuer = tt.issuer
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

func TestReadConfig_FrontendAuthlingIssuerFromEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chatto.toml")
	if err := os.WriteFile(path, []byte(`
[webserver]
url = "https://chat.example"
port = 4000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHATTO_FRONTEND_AUTHLING_ISSUER", "https://id.example")

	cfg, err := ReadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Frontend.AuthlingIssuer != "https://id.example" {
		t.Fatalf("frontend Authling issuer = %q", cfg.Frontend.AuthlingIssuer)
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
	if cfg.NATS.Client.AuthMethod != natsauth.AuthToken || cfg.NATS.Client.Token != "nats-token" {
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
