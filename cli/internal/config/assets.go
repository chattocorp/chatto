package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/c2h5oh/datasize"
)

// AssetsCacheConfig contains settings for caching resized images.
type AssetsCacheConfig struct {
	Enabled bool     `toml:"enabled" env:"CHATTO_CORE_ASSETS_CACHE_ENABLED" comment:"Enable caching for resized images. Default: false (opt-in)."`
	TTL     Duration `toml:"ttl" env:"CHATTO_CORE_ASSETS_CACHE_TTL" comment:"Time-to-live for cached images. Supports '7d', '1w', '168h', etc. Default: 7d."`
}

// StorageBackend defines where new asset uploads are stored.
type StorageBackend string

const (
	StorageBackendNATS StorageBackend = "nats" // Default: store assets in NATS ObjectStore
	StorageBackendS3   StorageBackend = "s3"   // Store assets in S3-compatible object storage
)

// S3Config contains settings for S3-compatible object storage.
type S3Config struct {
	Endpoint        string `toml:"endpoint" env:"CHATTO_CORE_ASSETS_S3_ENDPOINT" comment:"S3 endpoint URL. Use 's3.amazonaws.com' for AWS, or custom endpoint for MinIO, Wasabi, etc."`
	Bucket          string `toml:"bucket" env:"CHATTO_CORE_ASSETS_S3_BUCKET" comment:"S3 bucket name for storing assets."`
	PathPrefix      string `toml:"path_prefix" env:"CHATTO_CORE_ASSETS_S3_PATH_PREFIX" comment:"Optional object key prefix for all S3 assets. Stored asset references remain prefix-free so this can be changed after moving objects in S3."`
	Region          string `toml:"region" env:"CHATTO_CORE_ASSETS_S3_REGION" comment:"AWS region. Optional for non-AWS S3-compatible services."`
	AccessKeyID     string `toml:"access_key_id" env:"CHATTO_CORE_ASSETS_S3_ACCESS_KEY_ID" comment:"S3 access key ID."`
	SecretAccessKey string `toml:"secret_access_key" env:"CHATTO_CORE_ASSETS_S3_SECRET_ACCESS_KEY" comment:"S3 secret access key. NEVER SHARE THIS!"`
	UseSSL          *bool  `toml:"use_ssl" env:"CHATTO_CORE_ASSETS_S3_USE_SSL" comment:"Use HTTPS for S3 connections. Default: true."`
	PathStyle       *bool  `toml:"path_style" env:"CHATTO_CORE_ASSETS_S3_PATH_STYLE" comment:"Use path-style URLs (bucket in path). Required for MinIO and most S3-compatible services. Default: auto (virtual-hosted for AWS S3, path-style for custom endpoints)."`
}

// UseSSLOrDefault returns whether to use SSL, defaulting to true.
func (c *S3Config) UseSSLOrDefault() bool {
	if c.UseSSL == nil {
		return true
	}
	return *c.UseSSL
}

// PathStyleOrDefault returns whether to use path-style URLs, defaulting to false.
func (c *S3Config) PathStyleOrDefault() bool {
	if c.PathStyle == nil {
		return false
	}
	return *c.PathStyle
}

// UsePathStyleForEndpoint returns the AWS SDK addressing mode for this
// endpoint. When path_style is omitted, AWS S3 endpoints use virtual-hosted
// addressing while custom endpoints use path-style addressing, matching the
// old MinIO client's automatic bucket lookup behavior.
func (c *S3Config) UsePathStyleForEndpoint() bool {
	if c.PathStyle != nil {
		return *c.PathStyle
	}
	return !c.IsAWSEndpoint()
}

// IsAWSEndpoint reports whether the configured endpoint looks like an AWS S3
// endpoint rather than a custom S3-compatible service.
func (c *S3Config) IsAWSEndpoint() bool {
	host := strings.TrimSpace(c.Endpoint)
	if host == "" {
		return false
	}

	if u, err := url.Parse(host); err == nil && u.Host != "" {
		host = u.Hostname()
	} else if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}

	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == "s3.amazonaws.com" ||
		strings.HasSuffix(host, ".amazonaws.com") ||
		host == "s3.amazonaws.com.cn" ||
		strings.HasSuffix(host, ".amazonaws.com.cn")
}

// NormalizedPathPrefix returns PathPrefix with harmless leading/trailing slashes
// removed. Empty and "/" both preserve the historical bucket-root layout.
func (c *S3Config) NormalizedPathPrefix() string {
	return strings.Trim(c.PathPrefix, "/")
}

// NormalizePathPrefix trims harmless leading/trailing slashes from the S3
// object prefix. Empty and "/" both preserve the historical bucket-root layout.
func (c *S3Config) NormalizePathPrefix() {
	c.PathPrefix = c.NormalizedPathPrefix()
}

// ValidatePathPrefix rejects ambiguous prefixes before they become physical
// object keys. Call NormalizePathPrefix first so "/" is accepted as empty.
func (c *S3Config) ValidatePathPrefix() error {
	return validateS3PathPrefix(c.PathPrefix)
}

func validateS3PathPrefix(prefix string) error {
	if strings.Contains(prefix, "//") {
		return fmt.Errorf("core.assets.s3.path_prefix must not contain empty path segments")
	}
	for _, r := range prefix {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("core.assets.s3.path_prefix must not contain control characters")
		}
	}
	return nil
}

// TTLOrDefault returns the configured TTL, or 7 days if not set.
func (c *AssetsCacheConfig) TTLOrDefault() time.Duration {
	if c.TTL == 0 {
		return 7 * 24 * time.Hour // 7 days
	}
	return c.TTL.Duration()
}

// AssetsConfig contains settings for asset storage (attachments, thumbnails, etc.).
type AssetsConfig struct {
	SigningSecret  string            `toml:"signing_secret" env:"CHATTO_CORE_ASSETS_SIGNING_SECRET" comment:"Secret for signing asset URLs. NEVER SHARE THIS!\nIf it leaks, regenerate it. Existing signed URLs will become invalid but will be regenerated on next request."`
	MaxUploadSize  datasize.ByteSize `toml:"max_upload_size" env:"CHATTO_CORE_ASSETS_MAX_UPLOAD_SIZE" comment:"Maximum size for uploaded files. Supports human-readable formats like '25 MB', '25MB', '25MiB'."`
	StorageBackend StorageBackend    `toml:"storage_backend" env:"CHATTO_CORE_ASSETS_STORAGE_BACKEND" comment:"Where to store new uploads: 'nats' (default) or 's3'. Existing assets are served from their original location regardless of this setting."`
	S3             S3Config          `toml:"s3,commented" comment:"S3-compatible storage configuration. Only used when storage_backend = 's3'."`
	Cache          AssetsCacheConfig `toml:"cache" comment:"Caching configuration for resized images."`
}

// CoreConfig contains settings for the Chatto core service.
type CoreConfig struct {
	SecretKey                   string         `toml:"secret_key" env:"CHATTO_CORE_SECRET_KEY" comment:"Server-wide secret for deriving HMAC verifiers for bearer tokens, account-flow credentials, and invite links, and for sealing public cursors. NEVER SHARE THIS!\nIf it changes, existing bearer tokens, invite links, public cursors, and pending registration, verification, password reset, account deletion, and OAuth authorization-code credentials become invalid. Projection snapshots also become unreadable and are rebuilt from EVT."`
	ProjectionSnapshots         bool           `toml:"projection_snapshots,commented" env:"CHATTO_CORE_PROJECTION_SNAPSHOTS" comment:"Persist encrypted projection snapshots and replay only the later EVT delta at startup. Missing or incompatible snapshots safely fall back to EVT replay. Default: false."`
	ProjectionSnapshotRetention Duration       `toml:"projection_snapshot_retention,commented" env:"CHATTO_CORE_PROJECTION_SNAPSHOT_RETENTION" comment:"How long projection snapshot generations are retained. NATS enforces this as an Object Store TTL; Chatto uses it for optional S3 cleanup. Supports '7d', '1w', '168h', etc. Default: 7d."`
	ProjectionSnapshotS3Cleanup *bool          `toml:"projection_snapshot_s3_cleanup,commented" env:"CHATTO_CORE_PROJECTION_SNAPSHOT_S3_CLEANUP" comment:"Delete S3 projection snapshot generations older than projection_snapshot_retention. Disable when an external S3 lifecycle policy owns expiry. Default: true."`
	Assets                      AssetsConfig   `toml:"assets"`
	AuthTokenTTL                time.Duration  `toml:"-" env:"-"` // Renewable bearer maximum and per-cookie lifetime, set from AuthConfig.TokenTTLOrDefault().
	AuthAccessTokenTTL          time.Duration  `toml:"-" env:"-"` // Set by caller from AuthConfig.AccessTokenTTLOrDefault().
	EmailOTP                    EmailOTPConfig `toml:"-" env:"-"` // Set by caller from AuthConfig.EmailOTP
	Replicas                    int            `toml:"-" env:"-"` // Set by caller from NATSConfig.ReplicasOrDefault()
	Limits                      LimitsConfig   `toml:"-" env:"-"` // Set by caller from ChattoConfig.Limits
	Owners                      OwnersConfig   `toml:"-" env:"-"` // Set by caller from ChattoConfig.Owners — used by core to auto-promote on email verification
	Version                     string         `toml:"-" env:"-"` // Set by caller from the running build version; diagnostics only
}

// ProjectionSnapshotRetentionOrDefault returns the configured retention, or
// seven days when it is unset.
func (c *CoreConfig) ProjectionSnapshotRetentionOrDefault() time.Duration {
	if c.ProjectionSnapshotRetention == 0 {
		return 7 * 24 * time.Hour
	}
	return c.ProjectionSnapshotRetention.Duration()
}

// ProjectionSnapshotS3CleanupOrDefault reports whether Chatto owns S3 expiry.
func (c *CoreConfig) ProjectionSnapshotS3CleanupOrDefault() bool {
	return c.ProjectionSnapshotS3Cleanup == nil || *c.ProjectionSnapshotS3Cleanup
}
