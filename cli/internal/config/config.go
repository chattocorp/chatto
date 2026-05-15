package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/c2h5oh/datasize"
	"github.com/caarlos0/env/v11"
	"github.com/pelletier/go-toml/v2"
	str2duration "github.com/xhit/go-str2duration/v2"
	"hmans.de/chatto/pkg/natsauth"
)

// Duration is a time.Duration that supports extended parsing including days (d), weeks (w),
// months (mo), and years (y). Examples: "7d", "1w", "168h", "24h30m"
type Duration time.Duration

// UnmarshalText implements encoding.TextUnmarshaler for TOML/env parsing.
func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := str2duration.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	*d = Duration(parsed)
	return nil
}

// Duration returns the underlying time.Duration.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

type GeneralConfig struct {
	LogLevel string `toml:"log_level" env:"CHATTO_LOG_LEVEL" comment:"Log level. Possible values: debug, info, warn, error."`
}

// TLSConfig contains settings for automatic TLS via Let's Encrypt.
// Note: Default ports 80/443 require elevated privileges (sudo, CAP_NET_BIND_SERVICE, or root).
type TLSConfig struct {
	Enabled  bool   `toml:"enabled" env:"CHATTO_WEBSERVER_TLS_ENABLED" comment:"Enable automatic TLS via Let's Encrypt. Note: default ports 80/443 require elevated privileges."`
	Domain   string `toml:"domain" env:"CHATTO_WEBSERVER_TLS_DOMAIN" comment:"Domain name for the TLS certificate. Required when TLS is enabled."`
	Email    string `toml:"email" env:"CHATTO_WEBSERVER_TLS_EMAIL" comment:"Email address for Let's Encrypt notifications. Required when TLS is enabled."`
	CacheDir string `toml:"cache_dir,commented" env:"CHATTO_WEBSERVER_TLS_CACHE_DIR" comment:"Directory to cache TLS certificates. Default: .chatto/certs"`
	HTTPPort int    `toml:"http_port,commented" env:"CHATTO_WEBSERVER_TLS_HTTP_PORT" comment:"Port for HTTP server (ACME challenges and HTTPS redirect). Default: 80. Use a higher port if running without elevated privileges."`
}

// CacheDirOrDefault returns the cache directory, or the default if not set.
func (c *TLSConfig) CacheDirOrDefault() string {
	if c.CacheDir == "" {
		return ".chatto/certs"
	}
	return c.CacheDir
}

// HTTPPortOrDefault returns the HTTP port for ACME challenges, or 80 if not set.
func (c *TLSConfig) HTTPPortOrDefault() int {
	if c.HTTPPort == 0 {
		return 80
	}
	return c.HTTPPort
}

type WebserverConfig struct {
	URL                    string    `toml:"url" env:"CHATTO_WEBSERVER_URL" comment:"Public URL where the webserver is accessible. Used for generating absolute URLs."`
	Port                   int       `toml:"port" env:"CHATTO_WEBSERVER_PORT" comment:"Port for the webserver to listen on."`
	AllowedOrigins         []string  `toml:"allowed_origins" env:"CHATTO_WEBSERVER_ALLOWED_ORIGINS" comment:"Additional origins allowed for CORS and WebSocket connections. Defaults to wildcard (*) for multi-instance support. Set explicitly to restrict cross-origin access."`
	WebSocketCompression   *bool     `toml:"websocket_compression" env:"CHATTO_WEBSERVER_WEBSOCKET_COMPRESSION" comment:"Enable WebSocket compression for GraphQL connections. Reduces bandwidth but uses more CPU. Default: true."`
	RequestLogging         *bool     `toml:"request_logging" env:"CHATTO_WEBSERVER_REQUEST_LOGGING" comment:"Log HTTP requests. Useful for debugging but can be noisy in production. Default: false."`
	CookieSigningSecret    string    `toml:"cookie_signing_secret" env:"CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET" comment:"Secret for signing session cookies. NEVER SHARE THIS!\nIf it leaks, change it immediately, but please note that all existing sessions will become invalid."`
	CookieEncryptionSecret string    `toml:"cookie_encryption_secret" env:"CHATTO_WEBSERVER_COOKIE_ENCRYPTION_SECRET" comment:"Optional hex-encoded secret used to encrypt session cookies (in addition to signing). Must decode to 16, 24, or 32 bytes (AES-128/192/256). If unset, cookies are signed but not encrypted — anything ever written to the session is readable by anyone who steals the cookie."`
	TLS                    TLSConfig `toml:"tls" comment:"Automatic TLS configuration via Let's Encrypt."`
}

// WebSocketCompressionEnabled returns whether WebSocket compression is enabled (default: true)
func (c *WebserverConfig) WebSocketCompressionEnabled() bool {
	if c.WebSocketCompression == nil {
		return true
	}
	return *c.WebSocketCompression
}

// RequestLoggingEnabled returns whether HTTP request logging is enabled (default: false)
func (c *WebserverConfig) RequestLoggingEnabled() bool {
	if c.RequestLogging == nil {
		return false
	}
	return *c.RequestLogging
}

// EffectivePort returns the port to listen on. When TLS is enabled and no port
// is explicitly set (port == 0), defaults to 443. Otherwise returns the configured port.
func (c *WebserverConfig) EffectivePort() int {
	if c.TLS.Enabled && c.Port == 0 {
		return 443
	}
	return c.Port
}

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
	Region          string `toml:"region" env:"CHATTO_CORE_ASSETS_S3_REGION" comment:"AWS region. Optional for non-AWS S3-compatible services."`
	AccessKeyID     string `toml:"access_key_id" env:"CHATTO_CORE_ASSETS_S3_ACCESS_KEY_ID" comment:"S3 access key ID."`
	SecretAccessKey string `toml:"secret_access_key" env:"CHATTO_CORE_ASSETS_S3_SECRET_ACCESS_KEY" comment:"S3 secret access key. NEVER SHARE THIS!"`
	UseSSL          *bool  `toml:"use_ssl" env:"CHATTO_CORE_ASSETS_S3_USE_SSL" comment:"Use HTTPS for S3 connections. Default: true."`
	PathStyle       *bool  `toml:"path_style" env:"CHATTO_CORE_ASSETS_S3_PATH_STYLE" comment:"Use path-style URLs (bucket in path). Required for MinIO and most S3-compatible services. Default: false."`
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
	Assets       AssetsConfig  `toml:"assets"`
	AuthTokenTTL time.Duration `toml:"-" env:"-"` // Set by caller from AuthConfig.TokenTTLOrDefault()
	Replicas     int           `toml:"-" env:"-"` // Set by caller from NATSConfig.ReplicasOrDefault()
	Limits       LimitsConfig  `toml:"-" env:"-"` // Set by caller from ChattoConfig.Limits
	Owners       OwnersConfig  `toml:"-" env:"-"` // Set by caller from ChattoConfig.Owners — used by core to auto-promote on email verification
}

// OIDCConfig contains settings for a generic OIDC provider (e.g. Chatto Hub via Zitadel).
type OIDCConfig struct {
	Enabled      bool   `toml:"enabled" env:"CHATTO_AUTH_OIDC_ENABLED" comment:"Enable OIDC login (e.g. via Chatto Hub)."`
	IssuerURL    string `toml:"issuer_url" env:"CHATTO_AUTH_OIDC_ISSUER_URL" comment:"OIDC issuer URL. Used for discovery (/.well-known/openid-configuration)."`
	ClientID     string `toml:"client_id" env:"CHATTO_AUTH_OIDC_CLIENT_ID" comment:"OIDC client ID, obtained from Chatto Hub or your OIDC provider."`
	ClientSecret string `toml:"client_secret" env:"CHATTO_AUTH_OIDC_CLIENT_SECRET" comment:"OIDC client secret. NEVER SHARE THIS!"`
	Label        string `toml:"label,commented" env:"CHATTO_AUTH_OIDC_LABEL" comment:"Button label shown on the login page. Default: 'Chatto Hub'."`
}

// LabelOrDefault returns the configured label, or "Chatto Hub" if not set.
func (c *OIDCConfig) LabelOrDefault() string {
	if c.Label == "" {
		return "Chatto Hub"
	}
	return c.Label
}

// IsConfigured returns true if OIDC is enabled and all required fields are set.
func (c *OIDCConfig) IsConfigured() bool {
	return c.Enabled && c.IssuerURL != "" && c.ClientID != ""
}

type AuthConfig struct {
	DirectRegistration *bool      `toml:"direct_registration" env:"CHATTO_AUTH_DIRECT_REGISTRATION" comment:"Enable direct (email/password) registration. When false, users can only sign in via SSO providers. Default: true."`
	TokenTTL           Duration   `toml:"token_ttl,commented" env:"CHATTO_AUTH_TOKEN_TTL" comment:"TTL for bearer auth tokens. Supports human-readable durations like '90d', '2160h'. Default: 90d."`
	OIDC               OIDCConfig `toml:"oidc,commented" comment:"OIDC provider configuration (e.g. Chatto Hub)."`
}

// TokenTTLOrDefault returns the configured bearer token TTL, or 90 days if not set.
func (c *AuthConfig) TokenTTLOrDefault() time.Duration {
	if c.TokenTTL == 0 {
		return 90 * 24 * time.Hour
	}
	return c.TokenTTL.Duration()
}

// DirectRegistrationOrDefault returns whether direct (email/password) registration is enabled (default: true).
func (c *AuthConfig) DirectRegistrationOrDefault() bool {
	if c.DirectRegistration == nil {
		return true
	}
	return *c.DirectRegistration
}

// EnabledProviders returns a list of enabled SSO provider names.
func (c *AuthConfig) EnabledProviders() []string {
	var providers []string
	if c.OIDC.IsConfigured() {
		providers = append(providers, "oidc")
	}
	return providers
}

type EmbeddedNATSConfig struct {
	Enabled     bool   `toml:"enabled" env:"CHATTO_NATS_EMBEDDED_ENABLED" comment:"Enable embedded NATS server."`
	Port        int    `toml:"port" env:"CHATTO_NATS_EMBEDDED_PORT" comment:"NATS server port. Required for CLI commands to connect."`
	BindAddress string `toml:"bind_address,commented" env:"CHATTO_NATS_EMBEDDED_BIND_ADDRESS" comment:"Address to bind NATS ports. Default: 127.0.0.1 (localhost only)."`
	HTTPPort    int    `toml:"http_port,commented" env:"CHATTO_NATS_EMBEDDED_HTTP_PORT" comment:"NATS monitoring/stats HTTP port. Set to 0 to disable."`
	DataDir     string `toml:"data_dir" env:"CHATTO_NATS_EMBEDDED_DATA_DIR" comment:"Directory where the embedded NATS server stores its data."`
	AuthToken   string `toml:"auth_token" env:"CHATTO_NATS_EMBEDDED_AUTH_TOKEN" comment:"Authentication token for NATS connections. Auto-generated on init."`
}

// BindAddressOrDefault returns the bind address, defaulting to localhost for security.
func (c *EmbeddedNATSConfig) BindAddressOrDefault() string {
	if c.BindAddress == "" {
		return "127.0.0.1"
	}
	return c.BindAddress
}

// NATSAuthMethod is an alias for natsauth.AuthMethod, kept for backward compatibility.
type NATSAuthMethod = natsauth.AuthMethod

const (
	NATSAuthNone        = natsauth.AuthNone
	NATSAuthToken       = natsauth.AuthToken
	NATSAuthUserPass    = natsauth.AuthUserPass
	NATSAuthCredentials = natsauth.AuthCredentials
	NATSAuthNKey        = natsauth.AuthNKey
)

// NATSClientConfig contains settings for connecting to an external NATS server.
// Also used for CLI commands (like chatto admin) to connect to the embedded server.
type NATSClientConfig struct {
	URL             string         `toml:"url" env:"CHATTO_NATS_CLIENT_URL" comment:"NATS server URL. For embedded server, use nats://localhost:4222."`
	AuthMethod      NATSAuthMethod `toml:"auth_method" env:"CHATTO_NATS_CLIENT_AUTH_METHOD" comment:"Authentication method: none, token, userpass, credentials, nkey"`
	Token           string         `toml:"token" env:"CHATTO_NATS_CLIENT_TOKEN" comment:"Token for token auth. NEVER SHARE THIS!"`
	Username        string         `toml:"username,commented" env:"CHATTO_NATS_CLIENT_USERNAME" comment:"Username for userpass auth."`
	Password        string         `toml:"password,commented" env:"CHATTO_NATS_CLIENT_PASSWORD" comment:"Password for userpass auth. NEVER SHARE THIS!"`
	CredentialsFile string         `toml:"credentials_file,commented" env:"CHATTO_NATS_CLIENT_CREDENTIALS_FILE" comment:"Path to .creds file for credentials auth."`
	NKeySeed        string         `toml:"nkey_seed,commented" env:"CHATTO_NATS_CLIENT_NKEY_SEED" comment:"NKey seed for nkey auth. NEVER SHARE THIS!"`
	CACert          string         `toml:"ca_cert,commented" env:"CHATTO_NATS_CLIENT_CA_CERT" comment:"PEM-encoded CA certificate for verifying the NATS server's TLS cert. When set, the connection uses TLS."`
}

// NATSAuthConfig returns the auth configuration suitable for natsauth.ConnectOptions.
func (c *NATSClientConfig) NATSAuthConfig() natsauth.Config {
	return natsauth.Config{
		AuthMethod:      c.AuthMethod,
		Token:           c.Token,
		Username:        c.Username,
		Password:        c.Password,
		CredentialsFile: c.CredentialsFile,
		NKeySeed:        c.NKeySeed,
		CACert:          c.CACert,
	}
}

type NATSConfig struct {
	Replicas int                `toml:"replicas,commented" env:"CHATTO_NATS_REPLICAS" comment:"Number of replicas for JetStream streams, KV buckets, and object stores. Must be 1, 3, or 5 (odd numbers for quorum). Default: 1. Set to 3 or 5 when running a NATS cluster for fault tolerance."`
	Client   NATSClientConfig   `toml:"client" comment:"Client settings for CLI commands to connect to NATS."`
	Embedded EmbeddedNATSConfig `toml:"embedded"`
}

// ReplicasOrDefault returns the configured replicas count, defaulting to 1.
func (c *NATSConfig) ReplicasOrDefault() int {
	if c.Replicas <= 0 {
		return 1
	}
	return c.Replicas
}

// LimitsConfig contains instance-wide resource limits. A value of -1 means unlimited
// (the default when unset); 0 means no creation is allowed; any positive integer caps
// the count at that value.
//
// Enforcement note: limits are checked at the entry point of each gated operation
// (CreateUser) by counting current entries in KV. The check is not atomic with
// the subsequent write, so a burst of concurrent requests at the boundary can
// briefly overshoot by one or two. Tightening this requires an instance-stats
// counter system with CAS-incrementing gates — tracked as a follow-up to this PR.
type LimitsConfig struct {
	MaxUsers *int `toml:"max_users,commented" env:"CHATTO_LIMITS_MAX_USERS" comment:"Maximum number of verified users allowed in this instance. -1 = unlimited (default), 0 = no new signups, positive = cap. Counts users with at least one verified email."`
}

// MaxUsersOrDefault returns the configured max-users limit, defaulting to -1 (unlimited).
func (c *LimitsConfig) MaxUsersOrDefault() int {
	if c.MaxUsers == nil {
		return -1
	}
	return *c.MaxUsers
}

// OwnersConfig declares the email addresses that confer owner status.
// A user with a matching verified email is treated as having all instance
// permissions (owner-level), which includes access to /admin routes. This is
// the operator-driven mechanism for designating an instance owner — useful
// for both Chatto Cloud (the control plane writes the customer's email here at
// provision time) and self-hosters (who set their own email here in chatto.toml).
type OwnersConfig struct {
	Emails []string `toml:"emails" env:"CHATTO_OWNERS_EMAILS" comment:"Email addresses that confer owner status. Users with these verified emails get full instance access, including /admin routes."`
}

// IsInstanceOwnerEmail checks if an email is in the owners list.
//
// The comparison is case-insensitive and trims surrounding whitespace on both
// sides. Both `c.Emails` and the user-supplied `email` are normalized at the
// call site rather than at config load so that mutations to `c.Emails` (rare)
// don't need to remember to re-normalize.
func (c *OwnersConfig) IsInstanceOwnerEmail(email string) bool {
	needle := strings.TrimSpace(email)
	for _, e := range c.Emails {
		if strings.EqualFold(strings.TrimSpace(e), needle) {
			return true
		}
	}
	return false
}

// SMTPConfig contains settings for sending transactional emails.
type SMTPConfig struct {
	Enabled  bool   `toml:"enabled" env:"CHATTO_SMTP_ENABLED" comment:"Enable SMTP for sending transactional emails (verification, password reset, etc.)."`
	Host     string `toml:"host" env:"CHATTO_SMTP_HOST" comment:"SMTP server hostname. Example: smtp.example.com"`
	Port     int    `toml:"port" env:"CHATTO_SMTP_PORT" comment:"SMTP server port. Common values: 587 (TLS), 465 (SSL), 25 (unencrypted)."`
	Username string `toml:"username" env:"CHATTO_SMTP_USERNAME" comment:"SMTP authentication username."`
	Password string `toml:"password" env:"CHATTO_SMTP_PASSWORD" comment:"SMTP authentication password. NEVER SHARE THIS!"`
	From     string `toml:"from" env:"CHATTO_SMTP_FROM" comment:"From address for outgoing emails. Example: noreply@example.com"`
}

// PushConfig contains settings for Web Push notifications.
// Push notifications allow messages to be delivered even when the browser is closed.
type PushConfig struct {
	Enabled         bool   `toml:"enabled" env:"CHATTO_PUSH_ENABLED" comment:"Enable Web Push notifications. Default: false (opt-in to avoid third-party server contact)."`
	VAPIDPublicKey  string `toml:"vapid_public_key" env:"CHATTO_PUSH_VAPID_PUBLIC_KEY" comment:"VAPID public key (base64-encoded). Generate with: openssl ecparam -genkey -name prime256v1 | openssl ec -pubout"`
	VAPIDPrivateKey string `toml:"vapid_private_key" env:"CHATTO_PUSH_VAPID_PRIVATE_KEY" comment:"VAPID private key (base64-encoded). NEVER SHARE THIS!"`
	VAPIDSubject    string `toml:"vapid_subject" env:"CHATTO_PUSH_VAPID_SUBJECT" comment:"VAPID subject (mailto: or https: URL). Used by push services to contact the operator."`
}

// IsConfigured returns true if push notifications are enabled and all required VAPID fields are set.
func (c *PushConfig) IsConfigured() bool {
	return c.Enabled && c.VAPIDPublicKey != "" && c.VAPIDPrivateKey != "" && c.VAPIDSubject != ""
}

// VideoConfig contains settings for the video processing service.
type VideoConfig struct {
	Enabled       bool              `toml:"enabled" env:"CHATTO_VIDEO_ENABLED" comment:"Enable video processing (transcoding, thumbnails). Requires ffmpeg installed on the system."`
	FFmpegPath    string            `toml:"ffmpeg_path,commented" env:"CHATTO_VIDEO_FFMPEG_PATH" comment:"Path to ffmpeg binary. Auto-detected from PATH if empty."`
	FFprobePath   string            `toml:"ffprobe_path,commented" env:"CHATTO_VIDEO_FFPROBE_PATH" comment:"Path to ffprobe binary. Auto-detected from PATH if empty."`
	MaxConcurrent int               `toml:"max_concurrent,commented" env:"CHATTO_VIDEO_MAX_CONCURRENT" comment:"Maximum number of videos to process simultaneously. Default: 2."`
	MaxUploadSize datasize.ByteSize `toml:"max_upload_size,commented" env:"CHATTO_VIDEO_MAX_UPLOAD_SIZE" comment:"Maximum size for video uploads. Supports human-readable formats like '100 MB'. Default: 100 MB."`
	TempDir       string            `toml:"temp_dir,commented" env:"CHATTO_VIDEO_TEMP_DIR" comment:"Temporary directory for video processing. Default: system temp directory."`
}

// DefaultVideoMaxUploadSize is the default maximum size for video uploads (100 MB).
const DefaultVideoMaxUploadSize datasize.ByteSize = 100 * datasize.MB

// MaxConcurrentOrDefault returns the max concurrent workers, defaulting to 2.
func (c *VideoConfig) MaxConcurrentOrDefault() int {
	if c.MaxConcurrent <= 0 {
		return 2
	}
	return c.MaxConcurrent
}

// MaxUploadSizeOrDefault returns the max video upload size, defaulting to 100 MB.
func (c *VideoConfig) MaxUploadSizeOrDefault() datasize.ByteSize {
	if c.MaxUploadSize == 0 {
		return DefaultVideoMaxUploadSize
	}
	return c.MaxUploadSize
}

// LiveKitConfig contains settings for LiveKit voice call integration.
// LiveKit is an external media server that handles WebRTC voice/video connections.
type LiveKitConfig struct {
	Enabled          bool   `toml:"enabled" env:"CHATTO_LIVEKIT_ENABLED" comment:"Enable LiveKit voice call support. Requires a running LiveKit server."`
	URL              string `toml:"url" env:"CHATTO_LIVEKIT_URL" comment:"LiveKit server WebSocket URL. Example: ws://localhost:7880 (dev) or wss://livekit.example.com (prod)."`
	APIKey           string `toml:"api_key" env:"CHATTO_LIVEKIT_API_KEY" comment:"LiveKit API key."`
	APISecret        string `toml:"api_secret" env:"CHATTO_LIVEKIT_API_SECRET" comment:"LiveKit API secret. NEVER SHARE THIS!"`
	WebhookURL       string `toml:"webhook_url" env:"CHATTO_LIVEKIT_WEBHOOK_URL" comment:"URL where LiveKit sends webhook events. Defaults to {webserver.url}/webhooks/livekit."`
	ServerID       string `toml:"instance_id,commented" env:"CHATTO_LIVEKIT_INSTANCE_ID" comment:"Unique identifier for this instance, prefixed to LiveKit room names. Required when multiple Chatto instances share the same LiveKit cluster."`
	WebhookAPIKey    string `toml:"webhook_api_key,commented" env:"CHATTO_LIVEKIT_WEBHOOK_API_KEY" comment:"API key LiveKit uses to sign webhooks. Falls back to api_key if not set. Required when the webhook signing key differs from the per-instance API key."`
	WebhookAPISecret string `toml:"webhook_api_secret,commented" env:"CHATTO_LIVEKIT_WEBHOOK_API_SECRET" comment:"API secret for webhook signature validation. Falls back to api_secret if not set."`
}

// WebhookKeyPair returns the key/secret used to validate incoming LiveKit webhooks.
// In shared deployments, LiveKit signs webhooks with a dedicated webhook key that
// differs from the per-tenant API key. Falls back to the tenant API key/secret
// when webhook-specific credentials are not configured.
func (c *LiveKitConfig) WebhookKeyPair() (key, secret string) {
	if c.WebhookAPIKey != "" && c.WebhookAPISecret != "" {
		return c.WebhookAPIKey, c.WebhookAPISecret
	}
	return c.APIKey, c.APISecret
}

// IsConfigured returns true if LiveKit is enabled and all required fields are set.
func (c *LiveKitConfig) IsConfigured() bool {
	return c.Enabled && c.URL != "" && c.APIKey != "" && c.APISecret != ""
}

// BootstrapConfig declares users and the instance config to be auto-applied
// on startup, for fast iteration while developing and for E2E test fixtures.
// ONLY honored by builds compiled with the `bootstrap` build tag — release
// binaries parse the section but ignore its contents. Plaintext passwords
// are fine here for the same reason.
type BootstrapConfig struct {
	Users    []BootstrapUser    `toml:"users"`
	Server *BootstrapServer `toml:"instance,commented" comment:"Seeds the server config (name) and the deployment's primary room set on first boot."`
}

// BootstrapUser describes a user to create on startup in bootstrap-tag builds.
type BootstrapUser struct {
	Login        string `toml:"login" comment:"Required. The user's login (username)."`
	DisplayName  string `toml:"display_name,commented" comment:"Defaults to Login if empty."`
	Email        string `toml:"email,commented" comment:"Optional. If set, added as a verified email."`
	Password     string `toml:"password,commented" comment:"Optional. Required to log in via password; safe in plaintext because bootstrap-tag builds only."`
	ServerRole string `toml:"instance_role,commented" comment:"Optional: owner | admin | moderator."`
}

// BootstrapServer describes the instance to seed on startup in bootstrap-tag
// builds. Per ADR-027 there is no separate "space" concept any more — the
// instance is the server. The bootstrap creates whatever underlying storage
// records (notably a primary space) the data layer still needs, but those
// are internal: operators only configure the instance's name.
type BootstrapServer struct {
	Name  string   `toml:"name" comment:"Required. The instance's display name."`
	Rooms []string `toml:"rooms,commented" comment:"Optional. Auto-join rooms created on the instance; defaults to announcements + general."`
}

type ChattoConfig struct {
	General      GeneralConfig      `toml:"general"`
	Webserver    WebserverConfig    `toml:"webserver"`
	Core         CoreConfig         `toml:"core" comment:"Core service configuration."`
	Auth         AuthConfig         `toml:"auth" comment:"Authentication configuration."`
	Owners       OwnersConfig       `toml:"owners" comment:"Email addresses that confer owner status."`
	Limits       LimitsConfig       `toml:"limits,commented" comment:"Instance-wide resource limits. Use -1 for unlimited."`
	SMTP         SMTPConfig         `toml:"smtp" comment:"SMTP configuration for transactional emails."`
	Push         PushConfig         `toml:"push,commented" comment:"Web Push notification configuration."`
	Video        VideoConfig        `toml:"video,commented" comment:"Video processing configuration. Requires ffmpeg."`
	LiveKit      LiveKitConfig      `toml:"livekit,commented" comment:"LiveKit voice call configuration."`
	NATS         NATSConfig         `toml:"nats"`
	Bootstrap    BootstrapConfig    `toml:"bootstrap,commented" comment:"Dev/E2E-only: users and spaces auto-created on startup. ONLY honored by builds compiled with the 'bootstrap' build tag; release binaries ignore this section entirely."`
}

// Validate checks the configuration for errors and returns a descriptive error if any are found.
func (c *ChattoConfig) Validate() error {
	var errs []string

	// Required fields
	if c.Webserver.CookieSigningSecret == "" {
		errs = append(errs, "webserver.cookie_signing_secret is required")
	}
	if c.Core.Assets.SigningSecret == "" {
		errs = append(errs, "core.assets.signing_secret is required")
	}

	// Port ranges (port 0 is allowed when TLS is enabled, as it defaults to 443)
	if c.Webserver.Port < 0 || c.Webserver.Port > 65535 {
		errs = append(errs, "webserver.port must be between 0 and 65535")
	}
	if c.Webserver.Port == 0 && !c.Webserver.TLS.Enabled {
		errs = append(errs, "webserver.port is required when TLS is disabled")
	}
	if c.NATS.Embedded.Enabled {
		if c.NATS.Embedded.Port < 0 || c.NATS.Embedded.Port > 65535 {
			errs = append(errs, "nats.embedded.port must be between 0 and 65535")
		}
		if c.NATS.Embedded.HTTPPort < 0 || c.NATS.Embedded.HTTPPort > 65535 {
			errs = append(errs, "nats.embedded.http_port must be between 0 and 65535")
		}
		// Require auth token when TCP port is enabled
		if c.NATS.Embedded.Port > 0 && c.NATS.Embedded.AuthToken == "" {
			errs = append(errs, "nats.embedded.auth_token is required when TCP port is enabled")
		}
	}

	// NATS replicas
	if c.NATS.Replicas != 0 && c.NATS.Replicas != 1 && c.NATS.Replicas != 3 && c.NATS.Replicas != 5 {
		errs = append(errs, "nats.replicas must be 1, 3, or 5 (odd numbers for quorum)")
	}

	// URL format
	if c.Webserver.URL != "" {
		if _, err := url.Parse(c.Webserver.URL); err != nil {
			errs = append(errs, fmt.Sprintf("webserver.url is invalid: %v", err))
		}
	}
	if c.NATS.Client.URL != "" {
		if _, err := url.Parse(c.NATS.Client.URL); err != nil {
			errs = append(errs, fmt.Sprintf("nats.client.url is invalid: %v", err))
		}
	}

	// Log level
	if c.General.LogLevel != "" {
		validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
		if !validLevels[strings.ToLower(c.General.LogLevel)] {
			errs = append(errs, "general.log_level must be one of: debug, info, warn, error")
		}
	}

	// TLS configuration
	if c.Webserver.TLS.Enabled {
		if c.Webserver.TLS.Domain == "" {
			errs = append(errs, "webserver.tls.domain is required when TLS is enabled")
		}
		if c.Webserver.TLS.Email == "" {
			errs = append(errs, "webserver.tls.email is required when TLS is enabled")
		}
	}

	// SMTP configuration
	if c.SMTP.Enabled {
		if c.SMTP.Host == "" {
			errs = append(errs, "smtp.host is required when SMTP is enabled")
		}
		if c.SMTP.Port < 1 || c.SMTP.Port > 65535 {
			errs = append(errs, "smtp.port must be between 1 and 65535 when SMTP is enabled")
		}
		if c.SMTP.From == "" {
			errs = append(errs, "smtp.from is required when SMTP is enabled")
		}
	}

	// Push notification configuration
	if c.Push.Enabled {
		if c.Push.VAPIDPublicKey == "" {
			errs = append(errs, "push.vapid_public_key is required when push is enabled")
		}
		if c.Push.VAPIDPrivateKey == "" {
			errs = append(errs, "push.vapid_private_key is required when push is enabled")
		}
		if c.Push.VAPIDSubject == "" {
			errs = append(errs, "push.vapid_subject is required when push is enabled")
		}
	}

	// LiveKit configuration
	if c.LiveKit.Enabled {
		if c.LiveKit.URL == "" {
			errs = append(errs, "livekit.url is required when LiveKit is enabled")
		}
		if c.LiveKit.APIKey == "" {
			errs = append(errs, "livekit.api_key is required when LiveKit is enabled")
		}
		if c.LiveKit.APISecret == "" {
			errs = append(errs, "livekit.api_secret is required when LiveKit is enabled")
		}
		// Default webhook URL to {webserver.url}/webhooks/livekit
		if c.LiveKit.WebhookURL == "" && c.Webserver.URL != "" {
			c.LiveKit.WebhookURL = strings.TrimRight(c.Webserver.URL, "/") + "/webhooks/livekit"
		}
	}

	// Limits configuration: must be -1 (unlimited) or non-negative.
	if c.Limits.MaxUsers != nil && *c.Limits.MaxUsers < -1 {
		errs = append(errs, "limits.max_users must be -1 (unlimited) or a non-negative integer")
	}

	// Asset cache configuration
	if c.Core.Assets.Cache.Enabled && c.Core.Assets.Cache.TTL.Duration() < 0 {
		errs = append(errs, "core.assets.cache.ttl must be positive when cache is enabled")
	}

	// Storage backend validation
	if c.Core.Assets.StorageBackend != "" &&
		c.Core.Assets.StorageBackend != StorageBackendNATS &&
		c.Core.Assets.StorageBackend != StorageBackendS3 {
		errs = append(errs, "core.assets.storage_backend must be 'nats' or 's3'")
	}

	// S3 configuration (required when storage_backend = "s3")
	if c.Core.Assets.StorageBackend == StorageBackendS3 {
		if c.Core.Assets.S3.Endpoint == "" {
			errs = append(errs, "core.assets.s3.endpoint is required when storage_backend = 's3'")
		}
		if c.Core.Assets.S3.Bucket == "" {
			errs = append(errs, "core.assets.s3.bucket is required when storage_backend = 's3'")
		}
		if c.Core.Assets.S3.AccessKeyID == "" {
			errs = append(errs, "core.assets.s3.access_key_id is required when storage_backend = 's3'")
		}
		if c.Core.Assets.S3.SecretAccessKey == "" {
			errs = append(errs, "core.assets.s3.secret_access_key is required when storage_backend = 's3'")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// ReadConfig reads configuration from the specified file path (or "chatto.toml" if empty),
// then overrides with environment variables, and validates the result.
func ReadConfig(configPath string) (ChattoConfig, error) {
	var cfg ChattoConfig

	if configPath == "" {
		configPath = "chatto.toml"
	}

	// 1. Read TOML file if it exists (base config)
	b, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return cfg, err // Real error, not just missing file
	}
	if err == nil {
		if err := toml.Unmarshal(b, &cfg); err != nil {
			return cfg, err
		}
	}
	// If file doesn't exist, cfg remains zero-valued and env vars provide all config

	// 2. Override with environment variables
	if err := env.Parse(&cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse environment variables: %w", err)
	}

	// 3. Validate
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}
