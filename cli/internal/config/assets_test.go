package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadConfig_CoreProjectionSnapshotsFromEnv(t *testing.T) {
	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_CORE_PROJECTION_SNAPSHOTS", "true")
	t.Setenv("CHATTO_CORE_PROJECTION_SNAPSHOT_RETENTION", "10d")
	t.Setenv("CHATTO_CORE_PROJECTION_SNAPSHOT_S3_CLEANUP", "false")

	cfg, err := ReadConfig(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if !cfg.Core.ProjectionSnapshots {
		t.Error("expected projection snapshots from environment")
	}
	if cfg.Core.ProjectionSnapshotRetentionOrDefault() != 10*24*time.Hour {
		t.Errorf("projection snapshot retention = %s", cfg.Core.ProjectionSnapshotRetentionOrDefault())
	}
	if cfg.Core.ProjectionSnapshotS3CleanupOrDefault() {
		t.Error("expected S3 snapshot cleanup to be disabled from environment")
	}
}

func TestCoreProjectionSnapshotLifecycleDefaults(t *testing.T) {
	var cfg CoreConfig
	if got := cfg.ProjectionSnapshotRetentionOrDefault(); got != 7*24*time.Hour {
		t.Fatalf("default projection snapshot retention = %s", got)
	}
	if !cfg.ProjectionSnapshotS3CleanupOrDefault() {
		t.Fatal("S3 projection snapshot cleanup should default to enabled")
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

func TestChattoConfig_ValidateProjectionSnapshotRetention(t *testing.T) {
	cfg := validTestConfig()
	cfg.Core.ProjectionSnapshotRetention = Duration(-time.Hour)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "core.projection_snapshot_retention must be positive") {
		t.Fatalf("Validate() error = %v, want projection snapshot retention error", err)
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
