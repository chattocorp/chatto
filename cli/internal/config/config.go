package config

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
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
	LogLevel  string `toml:"log_level" env:"CHATTO_LOG_LEVEL" comment:"Log level. Possible values: debug, info, warn, error."`
	LogFormat string `toml:"log_format,commented" env:"CHATTO_LOG_FORMAT" comment:"Log output format. Possible values: auto, text, json, logfmt. Default: auto (text on terminals, JSON otherwise)."`
}

// TLSConfig contains settings for automatic TLS via Let's Encrypt.
// Note: Default ports 80/443 require elevated privileges (sudo, CAP_NET_BIND_SERVICE, or root).
type TLSConfig struct {
	Enabled  bool   `toml:"enabled" env:"CHATTO_WEBSERVER_TLS_ENABLED" comment:"Enable automatic TLS via Let's Encrypt. Note: default ports 80/443 require elevated privileges."`
	Domain   string `toml:"domain,commented" env:"CHATTO_WEBSERVER_TLS_DOMAIN" comment:"Domain name for the TLS certificate. Required when TLS is enabled."`
	Email    string `toml:"email,commented" env:"CHATTO_WEBSERVER_TLS_EMAIL" comment:"Email address for Let's Encrypt notifications. Required when TLS is enabled."`
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
	AllowedOrigins         []string  `toml:"allowed_origins" env:"CHATTO_WEBSERVER_ALLOWED_ORIGINS" comment:"Origins allowed for cross-server browser API access. Use [\"*\"] to allow bearer-token clients without cookies; use exact origins to allow credentialed CORS/WebSocket access. Exact non-wildcard entries are also trusted for OAuth redirect callbacks."`
	OAuthRedirectOrigins   []string  `toml:"oauth_redirect_origins" env:"CHATTO_WEBSERVER_OAUTH_REDIRECT_ORIGINS" comment:"Additional origins trusted only for OAuth redirect callbacks. Leave empty unless another web origin must complete OAuth. Use exact HTTPS origins in production; loopback development origins may use HTTP."`
	WebSocketCompression   *bool     `toml:"websocket_compression" env:"CHATTO_WEBSERVER_WEBSOCKET_COMPRESSION" comment:"Enable WebSocket compression for GraphQL connections. Reduces bandwidth but uses more CPU. Default: true."`
	RequestLogging         *bool     `toml:"request_logging" env:"CHATTO_WEBSERVER_REQUEST_LOGGING" comment:"Log HTTP requests. Useful for debugging but can be noisy in production. Default: false."`
	CookieSigningSecret    string    `toml:"cookie_signing_secret" env:"CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET" comment:"Secret for signing session cookies. NEVER SHARE THIS!\nIf it leaks, change it immediately, but please note that all existing sessions will become invalid."`
	CookieEncryptionSecret string    `toml:"cookie_encryption_secret" env:"CHATTO_WEBSERVER_COOKIE_ENCRYPTION_SECRET" comment:"Optional hex-encoded secret used to encrypt session cookies (in addition to signing). Must decode to 16, 24, or 32 bytes (AES-128/192/256). If unset, cookies are signed but not encrypted — anything ever written to the session is readable by anyone who steals the cookie."`
	TLS                    TLSConfig `toml:"tls" comment:"Automatic TLS configuration via Let's Encrypt."`
}

// MetricsConfig controls the process-local Prometheus scrape endpoint.
type MetricsConfig struct {
	Enabled     bool   `toml:"enabled" env:"CHATTO_METRICS_ENABLED" comment:"Expose a Prometheus-compatible metrics endpoint on a separate internal HTTP listener. Default: false."`
	BindAddress string `toml:"bind_address,commented" env:"CHATTO_METRICS_BIND_ADDRESS" comment:"Address to bind the metrics listener. Default: 127.0.0.1 (localhost only)."`
	Port        int    `toml:"port,commented" env:"CHATTO_METRICS_PORT" comment:"Port for the metrics listener. Default: 9090."`
	Path        string `toml:"path,commented" env:"CHATTO_METRICS_PATH" comment:"HTTP path for Prometheus scrapes. Default: /metrics."`
	Pprof       bool   `toml:"pprof,commented" env:"CHATTO_METRICS_PPROF" comment:"Expose Go pprof debug endpoints on the metrics listener under /debug/pprof/. Default: false."`
}

// ExporterConfig controls deployment-wide Prometheus metrics for a Chatto instance.
type ExporterConfig struct {
	Enabled           bool     `toml:"enabled" env:"CHATTO_EXPORTER_ENABLED" comment:"Start the deployment-wide Prometheus exporter from chatto run. Default: false."`
	BindAddress       string   `toml:"bind_address,commented" env:"CHATTO_EXPORTER_BIND_ADDRESS" comment:"Address to bind the exporter listener. Default: 127.0.0.1 (localhost only)."`
	Port              int      `toml:"port,commented" env:"CHATTO_EXPORTER_PORT" comment:"Port for the exporter listener. Default: 9100."`
	Path              string   `toml:"path,commented" env:"CHATTO_EXPORTER_PATH" comment:"HTTP path for Prometheus scrapes. Default: /metrics."`
	S3RefreshInterval Duration `toml:"s3_refresh_interval,commented" env:"CHATTO_EXPORTER_S3_REFRESH_INTERVAL" comment:"How often to refresh cached S3 bucket size metrics. Default: 15m."`
	S3Timeout         Duration `toml:"s3_timeout,commented" env:"CHATTO_EXPORTER_S3_TIMEOUT" comment:"Timeout for one S3 bucket-size refresh. Default: 30s."`
}

// DiagnosticsConfig controls opt-in local/operator diagnostics.
type DiagnosticsConfig struct {
	StartupCPUProfile string `toml:"startup_cpu_profile,commented" env:"CHATTO_DIAGNOSTICS_STARTUP_CPU_PROFILE" comment:"Write a Go CPU profile covering process startup through core boot to this path. Disabled when empty."`
}

// BindAddressOrDefault returns the metrics bind address, defaulting to localhost.
func (c *MetricsConfig) BindAddressOrDefault() string {
	if c.BindAddress == "" {
		return "127.0.0.1"
	}
	return c.BindAddress
}

// PortOrDefault returns the metrics listener port, defaulting to 9090.
func (c *MetricsConfig) PortOrDefault() int {
	if c.Port == 0 {
		return 9090
	}
	return c.Port
}

// PathOrDefault returns the metrics scrape path, defaulting to /metrics.
func (c *MetricsConfig) PathOrDefault() string {
	if c.Path == "" {
		return "/metrics"
	}
	return c.Path
}

// BindAddressOrDefault returns the exporter bind address, defaulting to localhost.
func (c *ExporterConfig) BindAddressOrDefault() string {
	if c.BindAddress == "" {
		return "127.0.0.1"
	}
	return c.BindAddress
}

// PortOrDefault returns the exporter listener port, defaulting to 9100.
func (c *ExporterConfig) PortOrDefault() int {
	if c.Port == 0 {
		return 9100
	}
	return c.Port
}

// PathOrDefault returns the exporter scrape path, defaulting to /metrics.
func (c *ExporterConfig) PathOrDefault() string {
	if c.Path == "" {
		return "/metrics"
	}
	return c.Path
}

// S3RefreshIntervalOrDefault returns the S3 refresh interval, defaulting to 15 minutes.
func (c *ExporterConfig) S3RefreshIntervalOrDefault() time.Duration {
	if c.S3RefreshInterval == 0 {
		return 15 * time.Minute
	}
	return c.S3RefreshInterval.Duration()
}

// S3TimeoutOrDefault returns the S3 refresh timeout, defaulting to 30 seconds.
func (c *ExporterConfig) S3TimeoutOrDefault() time.Duration {
	if c.S3Timeout == 0 {
		return 30 * time.Second
	}
	return c.S3Timeout.Duration()
}

func validateHexSecret(name, value string, required bool) error {
	if value == "" {
		if required {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return fmt.Errorf("%s must be hex-encoded: %w", name, err)
	}
	if len(decoded) != 32 {
		return fmt.Errorf("%s must decode to 32 bytes (got %d)", name, len(decoded))
	}
	return nil
}

func validateAbsoluteHTTPURL(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", name, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", name)
	}
	if u.Host == "" || u.User != nil {
		return fmt.Errorf("%s must include a host and must not include user info", name)
	}
	return nil
}

func validateOrigin(name, raw string, allowWildcard bool, requireHTTPSExceptLoopback bool) error {
	raw = strings.TrimSpace(raw)
	if allowWildcard && raw == "*" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s contains invalid origin %q: %w", name, raw, err)
	}
	if u.Scheme == "" || u.Host == "" || u.User != nil {
		return fmt.Errorf("%s contains invalid origin %q: must include scheme and host only", name, raw)
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s contains invalid origin %q: origins must not include path, query, or fragment", name, raw)
	}
	if requireHTTPSExceptLoopback && !isLoopbackHost(u.Hostname()) {
		if u.Scheme != "https" {
			return fmt.Errorf("%s contains invalid origin %q: non-loopback OAuth redirect origins must use https", name, raw)
		}
		return nil
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s contains invalid origin %q: origin must use http or https", name, raw)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// CookieEncryptionKey decodes the optional cookie encryption secret into an
// AES key suitable for securecookie. Empty means cookies are signed only.
func (c *WebserverConfig) CookieEncryptionKey() ([]byte, error) {
	if c.CookieEncryptionSecret == "" {
		return nil, nil
	}

	key, err := hex.DecodeString(c.CookieEncryptionSecret)
	if err != nil {
		return nil, fmt.Errorf("webserver.cookie_encryption_secret must be hex-encoded: %w", err)
	}

	switch len(key) {
	case 16, 24, 32:
		return key, nil
	default:
		return nil, fmt.Errorf("webserver.cookie_encryption_secret must decode to 16, 24, or 32 bytes (got %d)", len(key))
	}
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
	SecretKey    string         `toml:"secret_key" env:"CHATTO_CORE_SECRET_KEY" comment:"Server-wide secret for deriving HMAC verifiers for bearer tokens and account-flow credentials. NEVER SHARE THIS!\nIf it changes, existing bearer tokens and pending registration, verification, password reset, account deletion, and OAuth authorization-code credentials become invalid."`
	Assets       AssetsConfig   `toml:"assets"`
	AuthTokenTTL time.Duration  `toml:"-" env:"-"` // Set by caller from AuthConfig.TokenTTLOrDefault()
	EmailOTP     EmailOTPConfig `toml:"-" env:"-"` // Set by caller from AuthConfig.EmailOTP
	Replicas     int            `toml:"-" env:"-"` // Set by caller from NATSConfig.ReplicasOrDefault()
	Limits       LimitsConfig   `toml:"-" env:"-"` // Set by caller from ChattoConfig.Limits
	Owners       OwnersConfig   `toml:"-" env:"-"` // Set by caller from ChattoConfig.Owners — used by core to auto-promote on email verification
}

const (
	AuthProviderTypeOpenIDConnect = "oidc"
	AuthProviderTypeGitHub        = "github"
	AuthProviderTypeGitLab        = "gitlab"
	AuthProviderTypeGoogle        = "google"
	AuthProviderTypeDiscord       = "discord"
)

var authProviderDefaultLabels = map[string]string{
	AuthProviderTypeOpenIDConnect: "OpenID Connect",
	AuthProviderTypeGitHub:        "GitHub",
	AuthProviderTypeGitLab:        "GitLab",
	AuthProviderTypeGoogle:        "Google",
	AuthProviderTypeDiscord:       "Discord",
}

// AuthProviderConfig contains one configured external login provider. The ID is
// a stable local issuer namespace for OAuth-only providers and must not be
// changed after users link identities through it.
type AuthProviderConfig struct {
	ID              string            `toml:"id" comment:"Stable provider ID used in callback URLs and external identity links. Do not change after users link accounts."`
	Type            string            `toml:"type" comment:"Provider type: oidc, github, gitlab, google, or discord."`
	Label           string            `toml:"label,commented" comment:"Button label shown on the login page. Defaults to the provider type's display name."`
	ClientID        string            `toml:"client_id" comment:"OAuth/OIDC client ID."`
	ClientSecret    string            `toml:"client_secret" comment:"OAuth/OIDC client secret. NEVER SHARE THIS!"`
	IssuerURL       string            `toml:"issuer_url,commented" comment:"OIDC issuer URL. Required when type = 'oidc'."`
	Scopes          []string          `toml:"scopes,commented" comment:"Optional OAuth scopes. Defaults are provider-specific."`
	RequestEmail    *bool             `toml:"request_email,commented" comment:"Whether to request email scopes for providers that support it. Default: true."`
	ProviderOptions map[string]string `toml:"provider_options,commented" comment:"Provider-specific options reserved for future use."`
}

// LabelOrDefault returns the configured label, or a provider-specific default.
func (c AuthProviderConfig) LabelOrDefault() string {
	if c.Label != "" {
		return c.Label
	}
	if label, ok := authProviderDefaultLabels[c.Type]; ok {
		return label
	}
	return c.ID
}

func (c AuthProviderConfig) RequestEmailOrDefault() bool {
	if c.RequestEmail == nil {
		return true
	}
	return *c.RequestEmail
}

func IsAllowedAuthProviderType(providerType string) bool {
	_, ok := authProviderDefaultLabels[providerType]
	return ok
}

type AuthConfig struct {
	DirectRegistration *bool                `toml:"direct_registration" env:"CHATTO_AUTH_DIRECT_REGISTRATION" comment:"Enable direct (email/password) registration. When false, users can only sign in via SSO providers. Default: true."`
	TokenTTL           Duration             `toml:"token_ttl,commented" env:"CHATTO_AUTH_TOKEN_TTL" comment:"TTL for bearer auth tokens. Supports human-readable durations like '90d', '2160h'. Default: 90d."`
	EmailOTP           EmailOTPConfig       `toml:"email_otp,commented" comment:"Email OTP guardrails for registration and email verification."`
	Providers          []AuthProviderConfig `toml:"providers" comment:"External login providers. Configure as repeated [[auth.providers]] tables."`
}

// EmailOTPConfig controls registration and email-verification one-time-password guardrails.
type EmailOTPConfig struct {
	ThrottlingEnabled *bool    `toml:"throttling_enabled,commented" env:"CHATTO_AUTH_EMAIL_OTP_THROTTLING_ENABLED" comment:"Enable email OTP throttling for registration and email verification. Default: true."`
	TTL               Duration `toml:"ttl,commented" env:"CHATTO_AUTH_EMAIL_OTP_TTL" comment:"How long registration and email-verification codes stay valid. Default: 15m."`
	MaxDeliveredCodes int      `toml:"max_delivered_codes,commented" env:"CHATTO_AUTH_EMAIL_OTP_MAX_DELIVERED_CODES" comment:"Maximum successfully delivered codes per email challenge before throttling. Default: 10."`
	MaxWrongAttempts  int      `toml:"max_wrong_attempts,commented" env:"CHATTO_AUTH_EMAIL_OTP_MAX_WRONG_ATTEMPTS" comment:"Maximum wrong-code attempts per email challenge before throttling. Default: 5."`
}

// ThrottlingEnabledOrDefault returns whether email OTP throttling is enabled (default: true).
func (c *EmailOTPConfig) ThrottlingEnabledOrDefault() bool {
	if c.ThrottlingEnabled == nil {
		return true
	}
	return *c.ThrottlingEnabled
}

// TTLOrDefault returns the configured email OTP TTL, or 15 minutes if unset.
func (c *EmailOTPConfig) TTLOrDefault() time.Duration {
	if c.TTL == 0 {
		return 15 * time.Minute
	}
	return c.TTL.Duration()
}

// MaxDeliveredCodesOrDefault returns the delivered-code limit, or 10 if unset.
func (c *EmailOTPConfig) MaxDeliveredCodesOrDefault() int {
	if c.MaxDeliveredCodes == 0 {
		return 10
	}
	return c.MaxDeliveredCodes
}

// MaxWrongAttemptsOrDefault returns the wrong-code attempt limit, or 5 if unset.
func (c *EmailOTPConfig) MaxWrongAttemptsOrDefault() int {
	if c.MaxWrongAttempts == 0 {
		return 5
	}
	return c.MaxWrongAttempts
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

// EnabledProviders returns a list of configured SSO provider IDs.
func (c *AuthConfig) EnabledProviders() []string {
	providers := make([]string, 0, len(c.Providers))
	for _, provider := range c.Providers {
		providers = append(providers, provider.ID)
	}
	return providers
}

// EnabledProviderMethods returns legacy method-oriented SSO provider names.
// Provider-specific IDs are exposed through PublicProviders/AuthProvider.
func (c *AuthConfig) EnabledProviderMethods() []string {
	methods := make([]string, 0, len(c.Providers))
	seen := make(map[string]struct{}, len(c.Providers))
	for _, provider := range c.Providers {
		method := provider.Type
		if provider.Type == AuthProviderTypeOpenIDConnect {
			method = "oidc"
		}
		if _, ok := seen[method]; ok {
			continue
		}
		seen[method] = struct{}{}
		methods = append(methods, method)
	}
	return methods
}

// PublicProviders returns login metadata safe to expose before authentication.
func (c *AuthConfig) PublicProviders() []AuthProviderConfig {
	providers := make([]AuthProviderConfig, 0, len(c.Providers))
	for _, provider := range c.Providers {
		providers = append(providers, AuthProviderConfig{
			ID:    provider.ID,
			Type:  provider.Type,
			Label: provider.LabelOrDefault(),
		})
	}
	return providers
}

type EmbeddedNATSConfig struct {
	Enabled     bool   `toml:"enabled" env:"CHATTO_NATS_EMBEDDED_ENABLED" comment:"Enable embedded NATS server."`
	Port        int    `toml:"port,commented" env:"CHATTO_NATS_EMBEDDED_PORT" comment:"Uncomment to expose embedded NATS over TCP for nats CLI/admin commands. When left commented, Chatto connects in-process and no NATS port is opened."`
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
	URL             string         `toml:"url" env:"CHATTO_NATS_CLIENT_URL" comment:"NATS server URL. Use a comma-separated list for cluster failover, e.g. nats://n1:4222,nats://n2:4222."`
	AuthMethod      NATSAuthMethod `toml:"auth_method" env:"CHATTO_NATS_CLIENT_AUTH_METHOD" comment:"Authentication method for the external NATS server: none, token, userpass, credentials, or nkey."`
	Token           string         `toml:"token" env:"CHATTO_NATS_CLIENT_TOKEN" comment:"Token for token auth. Only used when auth_method = 'token'. NEVER SHARE THIS!"`
	Username        string         `toml:"username,commented" env:"CHATTO_NATS_CLIENT_USERNAME" comment:"Username for userpass auth. Only used when auth_method = 'userpass'."`
	Password        string         `toml:"password,commented" env:"CHATTO_NATS_CLIENT_PASSWORD" comment:"Password for userpass auth. Only used when auth_method = 'userpass'. NEVER SHARE THIS!"`
	CredentialsFile string         `toml:"credentials_file,commented" env:"CHATTO_NATS_CLIENT_CREDENTIALS_FILE" comment:"Path to a NATS .creds file. Only used when auth_method = 'credentials'."`
	NKeySeed        string         `toml:"nkey_seed,commented" env:"CHATTO_NATS_CLIENT_NKEY_SEED" comment:"NKey seed. Only used when auth_method = 'nkey'. NEVER SHARE THIS!"`
	CACert          string         `toml:"ca_cert,commented" env:"CHATTO_NATS_CLIENT_CA_CERT" comment:"PEM-encoded CA certificate for verifying the NATS server's TLS certificate. When set, the connection uses TLS."`
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
	Replicas int                `toml:"replicas" env:"CHATTO_NATS_REPLICAS" comment:"Number of replicas for JetStream streams, KV buckets, and object stores. Must be 1, 3, or 5 (odd numbers for quorum). Use 3 or 5 only with a matching NATS cluster."`
	Client   NATSClientConfig   `toml:"client,commented" comment:"External NATS client settings. To use an external server or cluster, set nats.embedded.enabled = false, then uncomment and update this section. Embedded NATS derives its client settings automatically."`
	Embedded EmbeddedNATSConfig `toml:"embedded"`
}

// ReplicasOrDefault returns the configured replicas count, defaulting to 1.
func (c *NATSConfig) ReplicasOrDefault() int {
	if c.Replicas <= 0 {
		return 1
	}
	return c.Replicas
}

// LimitsConfig contains server-wide resource limits. A value of -1 means unlimited
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
// the operator-driven mechanism for designating an server owner — useful
// for both Chatto Cloud (the control plane writes the customer's email here at
// provision time) and self-hosters (who set their own email here in chatto.toml).
type OwnersConfig struct {
	Emails []string `toml:"emails" env:"CHATTO_OWNERS_EMAILS" comment:"Email addresses that confer owner status. Users with these verified emails get full instance access, including /admin routes."`
}

// IsServerOwnerEmail checks if an email is in the owners list.
//
// The comparison is case-insensitive and trims surrounding whitespace on both
// sides. Both `c.Emails` and the user-supplied `email` are normalized at the
// call site rather than at config load so that mutations to `c.Emails` (rare)
// don't need to remember to re-normalize.
func (c *OwnersConfig) IsServerOwnerEmail(email string) bool {
	needle := strings.TrimSpace(email)
	for _, e := range c.Emails {
		if strings.EqualFold(strings.TrimSpace(e), needle) {
			return true
		}
	}
	return false
}

// SMTPTLSPolicy controls how the SMTP client encrypts the transport.
type SMTPTLSPolicy string

const (
	SMTPTLSMandatory     SMTPTLSPolicy = "mandatory"
	SMTPTLSOpportunistic SMTPTLSPolicy = "opportunistic"
	SMTPTLSImplicit      SMTPTLSPolicy = "implicit"
)

// TLSPolicyOrDefault returns the configured SMTP TLS policy, defaulting to
// mandatory STARTTLS so transactional email tokens are not sent in plaintext.
// Port 465 is the standard implicit TLS/SMTPS submission port, so treat the
// default/mandatory policy as implicit TLS there for operator compatibility.
func (c *SMTPConfig) TLSPolicyOrDefault() SMTPTLSPolicy {
	policy := SMTPTLSPolicy(strings.ToLower(strings.TrimSpace(string(c.TLS))))
	if policy == "" {
		if c.Port == 465 {
			return SMTPTLSImplicit
		}
		return SMTPTLSMandatory
	}
	if policy == SMTPTLSMandatory && c.Port == 465 {
		return SMTPTLSImplicit
	}
	return policy
}

// SMTPConfig contains settings for sending transactional emails.
type SMTPConfig struct {
	Enabled       bool          `toml:"enabled" env:"CHATTO_SMTP_ENABLED" comment:"Enable SMTP for sending transactional emails (verification, password reset, etc.)."`
	Host          string        `toml:"host" env:"CHATTO_SMTP_HOST" comment:"SMTP server hostname. Example: smtp.example.com"`
	Port          int           `toml:"port" env:"CHATTO_SMTP_PORT" comment:"SMTP server port. Common value: 587 (STARTTLS)."`
	TLS           SMTPTLSPolicy `toml:"tls" env:"CHATTO_SMTP_TLS" comment:"SMTP TLS policy: mandatory STARTTLS (default), implicit TLS/SMTPS, or opportunistic. Opportunistic allows plaintext fallback and should only be used when explicitly required."`
	TLSServerName string        `toml:"tls_server_name,commented" env:"CHATTO_SMTP_TLS_SERVER_NAME" comment:"SMTP TLS server name for certificate verification and SNI. Use when smtp.host is an IP address or internal alias but the certificate is issued for a DNS name."`
	TLSSkipVerify bool          `toml:"tls_skip_verify,commented" env:"CHATTO_SMTP_TLS_SKIP_VERIFY" comment:"Disable SMTP TLS certificate verification. Insecure; use only for trusted internal SMTP servers with self-signed or mismatched certificates."`
	Username      string        `toml:"username" env:"CHATTO_SMTP_USERNAME" comment:"SMTP authentication username."`
	Password      string        `toml:"password" env:"CHATTO_SMTP_PASSWORD" comment:"SMTP authentication password. NEVER SHARE THIS!"`
	From          string        `toml:"from" env:"CHATTO_SMTP_FROM" comment:"From address for outgoing emails. Example: noreply@example.com"`
}

// PushConfig contains settings for Web Push notifications.
// Push notifications allow messages to be delivered even when the browser is closed.
type PushConfig struct {
	Enabled         bool   `toml:"enabled" env:"CHATTO_PUSH_ENABLED" comment:"Enable Web Push notifications. Default: false (opt-in to avoid third-party server contact)."`
	VAPIDPublicKey  string `toml:"vapid_public_key" env:"CHATTO_PUSH_VAPID_PUBLIC_KEY" comment:"VAPID public key (base64-encoded). Generate with: openssl ecparam -genkey -name prime256v1 | openssl ec -pubout"`
	VAPIDPrivateKey string `toml:"vapid_private_key" env:"CHATTO_PUSH_VAPID_PRIVATE_KEY" comment:"VAPID private key (base64-encoded). NEVER SHARE THIS!"`
	VAPIDSubject    string `toml:"vapid_subject" env:"CHATTO_PUSH_VAPID_SUBJECT" comment:"VAPID subject (operator email, optional mailto: prefix, or https: URL). Used by push services to contact the operator."`
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
	ServerID         string `toml:"server_id,commented" env:"CHATTO_LIVEKIT_SERVER_ID" comment:"Unique identifier for this server, prefixed to LiveKit room names. Required when multiple Chatto servers share the same LiveKit cluster."`
	InstanceID       string `toml:"instance_id,commented" env:"CHATTO_LIVEKIT_INSTANCE_ID" comment:"Deprecated alias for server_id. Prefer server_id / CHATTO_LIVEKIT_SERVER_ID."`
	WebhookAPIKey    string `toml:"webhook_api_key,commented" env:"CHATTO_LIVEKIT_WEBHOOK_API_KEY" comment:"API key LiveKit uses to sign webhooks. Falls back to api_key if not set. Required when the webhook signing key differs from the per-server API key."`
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

// BootstrapConfig declares users and the server config to be auto-applied
// on startup, for fast iteration while developing and for E2E test fixtures.
// ONLY honored by builds compiled with the `bootstrap` build tag — release
// binaries parse the section but ignore its contents. Plaintext passwords
// are fine here for the same reason.
type BootstrapConfig struct {
	Users          []BootstrapUser  `toml:"users"`
	Server         *BootstrapServer `toml:"server,commented" comment:"Seeds the server config (name) and the deployment's primary room group on first boot."`
	LegacyInstance *BootstrapServer `toml:"instance,commented" comment:"Deprecated alias for [bootstrap.server]. Prefer [bootstrap.server]."`
}

// BootstrapUser describes a user to create on startup in bootstrap-tag builds.
type BootstrapUser struct {
	Login        string `toml:"login" comment:"Required. The user's login (username)."`
	DisplayName  string `toml:"display_name,commented" comment:"Defaults to Login if empty."`
	Email        string `toml:"email,commented" comment:"Optional. If set, added as a verified email."`
	Password     string `toml:"password,commented" comment:"Optional. Required to log in via password; safe in plaintext because bootstrap-tag builds only."`
	ServerRole   string `toml:"server_role,commented" comment:"Optional: owner | admin | moderator."`
	InstanceRole string `toml:"instance_role,commented" comment:"Deprecated alias for server_role. Prefer server_role."`
}

// RoleOrDefault returns the normalized bootstrap role, honoring the deprecated
// instance_role alias only when server_role is unset.
func (u BootstrapUser) RoleOrDefault() string {
	if u.ServerRole != "" {
		return u.ServerRole
	}
	return u.InstanceRole
}

// ServerOrDefault returns the normalized bootstrap server, honoring the
// deprecated [bootstrap.instance] alias only when [bootstrap.server] is unset.
func (c BootstrapConfig) ServerOrDefault() *BootstrapServer {
	if c.Server != nil {
		return c.Server
	}
	return c.LegacyInstance
}

// BootstrapServer describes the server to seed on startup in bootstrap-tag
// builds. Per ADR-027 there is no separate "space" concept any more — the
// server is the product surface. The bootstrap creates whatever underlying storage
// records (notably a primary space) the data layer still needs, but those
// are internal: operators only configure the server's name.
type BootstrapServer struct {
	Name  string   `toml:"name" comment:"Required. The instance's display name."`
	Rooms []string `toml:"rooms,commented" comment:"Optional. Auto-join rooms created on the instance; defaults to announcements + general."`
}

type ChattoConfig struct {
	General     GeneralConfig     `toml:"general"`
	Owners      OwnersConfig      `toml:"owners" comment:"Email addresses that confer owner status."`
	Webserver   WebserverConfig   `toml:"webserver"`
	Metrics     MetricsConfig     `toml:"metrics,commented" comment:"Process-local Prometheus metrics endpoint."`
	Exporter    ExporterConfig    `toml:"exporter,commented" comment:"Deployment-wide Prometheus metrics exporter."`
	Diagnostics DiagnosticsConfig `toml:"diagnostics,commented" comment:"Opt-in diagnostics for local benchmarking and operator troubleshooting."`
	Core        CoreConfig        `toml:"core" comment:"Core service configuration."`
	Auth        AuthConfig        `toml:"auth" comment:"Authentication configuration."`
	Limits      LimitsConfig      `toml:"limits,commented" comment:"Instance-wide resource limits. Use -1 for unlimited."`
	SMTP        SMTPConfig        `toml:"smtp" comment:"SMTP configuration for transactional emails."`
	Push        PushConfig        `toml:"push,commented" comment:"Web Push notification configuration."`
	Video       VideoConfig       `toml:"video,commented" comment:"Video processing configuration. Requires ffmpeg."`
	LiveKit     LiveKitConfig     `toml:"livekit,commented" comment:"LiveKit voice call configuration."`
	NATS        NATSConfig        `toml:"nats"`
	Bootstrap   BootstrapConfig   `toml:"bootstrap,commented" comment:"Dev/E2E-only: users and spaces auto-created on startup. ONLY honored by builds compiled with the 'bootstrap' build tag; release binaries ignore this section entirely."`
}

// ApplyDefaults fills derived config values that are safe to compute from other
// fields. Keep validation separate so Validate can remain a pure check.
func (c *ChattoConfig) ApplyDefaults() {
	if c.NATS.Embedded.Enabled && c.NATS.Embedded.Port > 0 {
		if c.NATS.Client.URL == "" {
			c.NATS.Client.URL = embeddedNATSClientURL(c.NATS.Embedded)
		}
		if c.NATS.Client.AuthMethod == "" {
			if c.NATS.Embedded.AuthToken != "" {
				c.NATS.Client.AuthMethod = NATSAuthToken
			} else {
				c.NATS.Client.AuthMethod = NATSAuthNone
			}
		}
		if c.NATS.Client.AuthMethod == NATSAuthToken && c.NATS.Client.Token == "" {
			c.NATS.Client.Token = c.NATS.Embedded.AuthToken
		}
	}

	if c.LiveKit.ServerID == "" {
		c.LiveKit.ServerID = c.LiveKit.InstanceID
	}
	if c.LiveKit.Enabled && c.LiveKit.WebhookURL == "" && c.Webserver.URL != "" {
		c.LiveKit.WebhookURL = strings.TrimRight(c.Webserver.URL, "/") + "/webhooks/livekit"
	}

	for i := range c.Bootstrap.Users {
		if c.Bootstrap.Users[i].ServerRole == "" {
			c.Bootstrap.Users[i].ServerRole = c.Bootstrap.Users[i].InstanceRole
		}
	}
	if c.Bootstrap.Server == nil {
		c.Bootstrap.Server = c.Bootstrap.LegacyInstance
	}
}

// Normalize canonicalizes harmless config spelling differences without applying
// semantic defaults.
func (c *ChattoConfig) Normalize() {
	c.Core.Assets.S3.NormalizePathPrefix()
}

func embeddedNATSClientURL(cfg EmbeddedNATSConfig) string {
	host := cfg.BindAddressOrDefault()
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return fmt.Sprintf("nats://%s", net.JoinHostPort(host, fmt.Sprint(cfg.Port)))
}

// Validate checks the configuration for errors and returns a descriptive error if any are found.
func (c *ChattoConfig) Validate() error {
	var errs []string

	// Required fields
	if err := validateHexSecret("webserver.cookie_signing_secret", c.Webserver.CookieSigningSecret, true); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validateHexSecret("core.assets.signing_secret", c.Core.Assets.SigningSecret, true); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validateHexSecret("core.secret_key", c.Core.SecretKey, true); err != nil {
		errs = append(errs, err.Error())
	}
	if _, err := c.Webserver.CookieEncryptionKey(); err != nil {
		errs = append(errs, err.Error())
	}

	// Port ranges (port 0 is allowed when TLS is enabled, as it defaults to 443)
	if c.Webserver.Port < 0 || c.Webserver.Port > 65535 {
		errs = append(errs, "webserver.port must be between 0 and 65535")
	}
	if c.Webserver.Port == 0 && !c.Webserver.TLS.Enabled {
		errs = append(errs, "webserver.port is required when TLS is disabled")
	}
	if c.Metrics.Enabled {
		if c.Metrics.Port < 0 || c.Metrics.Port > 65535 {
			errs = append(errs, "metrics.port must be between 0 and 65535")
		}
		metricsPath := c.Metrics.PathOrDefault()
		if !strings.HasPrefix(metricsPath, "/") {
			errs = append(errs, "metrics.path must start with /")
		}
		if strings.ContainsAny(metricsPath, "?#") {
			errs = append(errs, "metrics.path must not contain query strings or fragments")
		}
	}
	if c.Exporter.Enabled || c.Exporter.Port != 0 || c.Exporter.Path != "" || c.Exporter.BindAddress != "" || c.Exporter.S3RefreshInterval != 0 || c.Exporter.S3Timeout != 0 {
		if c.Exporter.Port < 0 || c.Exporter.Port > 65535 {
			errs = append(errs, "exporter.port must be between 0 and 65535")
		}
		exporterPath := c.Exporter.PathOrDefault()
		if !strings.HasPrefix(exporterPath, "/") {
			errs = append(errs, "exporter.path must start with /")
		}
		if strings.ContainsAny(exporterPath, "?#") {
			errs = append(errs, "exporter.path must not contain query strings or fragments")
		}
		if c.Exporter.S3RefreshInterval.Duration() < 0 {
			errs = append(errs, "exporter.s3_refresh_interval must not be negative")
		}
		if c.Exporter.S3Timeout.Duration() < 0 {
			errs = append(errs, "exporter.s3_timeout must not be negative")
		}
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
		if err := validateAbsoluteHTTPURL("webserver.url", c.Webserver.URL); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if c.NATS.Client.URL != "" {
		if _, err := url.Parse(c.NATS.Client.URL); err != nil {
			errs = append(errs, fmt.Sprintf("nats.client.url is invalid: %v", err))
		}
	}
	for _, origin := range c.Webserver.AllowedOrigins {
		if err := validateOrigin("webserver.allowed_origins", origin, true, false); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for _, origin := range c.Webserver.OAuthRedirectOrigins {
		if err := validateOrigin("webserver.oauth_redirect_origins", origin, true, true); err != nil {
			errs = append(errs, err.Error())
		}
	}

	// Log level
	if c.General.LogLevel != "" {
		validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
		if !validLevels[strings.ToLower(c.General.LogLevel)] {
			errs = append(errs, "general.log_level must be one of: debug, info, warn, error")
		}
	}
	if c.General.LogFormat != "" {
		validFormats := map[string]bool{"auto": true, "text": true, "json": true, "logfmt": true}
		if !validFormats[strings.ToLower(c.General.LogFormat)] {
			errs = append(errs, "general.log_format must be one of: auto, text, json, logfmt")
		}
	}

	// External auth providers
	seenProviderIDs := make(map[string]struct{}, len(c.Auth.Providers))
	for i, provider := range c.Auth.Providers {
		prefix := fmt.Sprintf("auth.providers[%d]", i)
		if c.Webserver.URL == "" {
			errs = append(errs, "webserver.url is required when auth providers are configured")
		}
		if provider.ID == "" {
			errs = append(errs, prefix+".id is required")
		} else if strings.ContainsAny(provider.ID, "/?#") || strings.TrimSpace(provider.ID) != provider.ID {
			errs = append(errs, prefix+".id must be a stable URL-safe identifier without spaces or path separators")
		} else if _, exists := seenProviderIDs[provider.ID]; exists {
			errs = append(errs, fmt.Sprintf("auth provider id %q is configured more than once", provider.ID))
		} else {
			seenProviderIDs[provider.ID] = struct{}{}
		}
		if !IsAllowedAuthProviderType(provider.Type) {
			errs = append(errs, prefix+".type must be one of: oidc, github, gitlab, google, discord")
		}
		if provider.ClientID == "" {
			errs = append(errs, prefix+".client_id is required")
		}
		if provider.ClientSecret == "" {
			errs = append(errs, prefix+".client_secret is required")
		}
		if provider.Type == AuthProviderTypeOpenIDConnect && provider.IssuerURL == "" {
			errs = append(errs, prefix+".issuer_url is required when type = 'oidc'")
		}
		if provider.IssuerURL != "" {
			if err := validateAbsoluteHTTPURL(prefix+".issuer_url", provider.IssuerURL); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if c.Auth.EmailOTP.TTL.Duration() < 0 {
		errs = append(errs, "auth.email_otp.ttl must be positive when set")
	}
	if c.Auth.EmailOTP.MaxDeliveredCodes < 0 {
		errs = append(errs, "auth.email_otp.max_delivered_codes must be positive when set")
	}
	if c.Auth.EmailOTP.MaxWrongAttempts < 0 {
		errs = append(errs, "auth.email_otp.max_wrong_attempts must be positive when set")
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
	switch c.SMTP.TLSPolicyOrDefault() {
	case SMTPTLSMandatory, SMTPTLSOpportunistic, SMTPTLSImplicit:
	default:
		errs = append(errs, "smtp.tls must be one of: mandatory, opportunistic, implicit")
	}
	if c.SMTP.Enabled {
		if c.Webserver.URL == "" {
			errs = append(errs, "webserver.url is required when SMTP is enabled")
		}
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
		if c.Webserver.URL == "" {
			errs = append(errs, "webserver.url is required when push is enabled")
		}
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
		if c.Webserver.URL == "" {
			errs = append(errs, "webserver.url is required when LiveKit is enabled")
		}
		if c.LiveKit.URL == "" {
			errs = append(errs, "livekit.url is required when LiveKit is enabled")
		}
		if c.LiveKit.APIKey == "" {
			errs = append(errs, "livekit.api_key is required when LiveKit is enabled")
		}
		if c.LiveKit.APISecret == "" {
			errs = append(errs, "livekit.api_secret is required when LiveKit is enabled")
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
		if err := validateS3PathPrefix(c.Core.Assets.S3.NormalizedPathPrefix()); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if c.NATS.Embedded.Enabled &&
		c.NATS.Embedded.Port > 0 &&
		c.NATS.Embedded.AuthToken != "" &&
		c.NATS.Client.AuthMethod == NATSAuthToken &&
		c.NATS.Client.Token != "" &&
		c.NATS.Client.Token != c.NATS.Embedded.AuthToken {
		errs = append(errs, "nats.client.token must match nats.embedded.auth_token when embedded NATS uses token auth")
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
	if err := applyAuthProviderEnv(&cfg); err != nil {
		return cfg, err
	}

	// 3. Apply derived defaults and normalize harmless spelling differences
	cfg.ApplyDefaults()
	cfg.Normalize()

	// 4. Validate
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func applyAuthProviderEnv(cfg *ChattoConfig) error {
	providers, providersSet, err := authProvidersFromEnv()
	if err != nil {
		return err
	}
	legacyOIDCEnabled := strings.TrimSpace(os.Getenv("CHATTO_AUTH_OIDC_ENABLED"))

	if providersSet {
		if legacyOIDCEnabled != "" {
			return fmt.Errorf("CHATTO_AUTH_PROVIDERS_* cannot be combined with legacy CHATTO_AUTH_OIDC_ENABLED")
		}
		cfg.Auth.Providers = providers
		return nil
	}

	if legacyOIDCEnabled == "" {
		return nil
	}
	enabled, err := strconv.ParseBool(legacyOIDCEnabled)
	if err != nil {
		return fmt.Errorf("CHATTO_AUTH_OIDC_ENABLED must be a boolean: %w", err)
	}
	if !enabled {
		cfg.Auth.Providers = nil
		return nil
	}
	label := os.Getenv("CHATTO_AUTH_OIDC_LABEL")
	if label == "" {
		label = "Chatto Hub"
	}
	cfg.Auth.Providers = []AuthProviderConfig{{
		ID:           "oidc",
		Type:         AuthProviderTypeOpenIDConnect,
		Label:        label,
		IssuerURL:    os.Getenv("CHATTO_AUTH_OIDC_ISSUER_URL"),
		ClientID:     os.Getenv("CHATTO_AUTH_OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("CHATTO_AUTH_OIDC_CLIENT_SECRET"),
	}}
	return nil
}

func authProvidersFromEnv() ([]AuthProviderConfig, bool, error) {
	const prefix = "CHATTO_AUTH_PROVIDERS_"
	providersByIndex := make(map[int]*AuthProviderConfig)

	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(name, prefix) {
			continue
		}

		rest := strings.TrimPrefix(name, prefix)
		indexPart, field, ok := strings.Cut(rest, "_")
		if !ok {
			return nil, false, fmt.Errorf("%s must use CHATTO_AUTH_PROVIDERS_<index>_<field>", name)
		}
		index, err := strconv.Atoi(indexPart)
		if err != nil || index < 0 {
			return nil, false, fmt.Errorf("%s uses invalid provider index %q", name, indexPart)
		}

		provider := providersByIndex[index]
		if provider == nil {
			provider = &AuthProviderConfig{}
			providersByIndex[index] = provider
		}
		if err := applyAuthProviderEnvField(provider, name, field, value); err != nil {
			return nil, false, err
		}
	}

	if len(providersByIndex) == 0 {
		return nil, false, nil
	}

	indices := make([]int, 0, len(providersByIndex))
	for index := range providersByIndex {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for expected, index := range indices {
		if index != expected {
			return nil, false, fmt.Errorf("CHATTO_AUTH_PROVIDERS_* indexes must be contiguous starting at 0; missing index %d", expected)
		}
	}

	providers := make([]AuthProviderConfig, 0, len(indices))
	for _, index := range indices {
		providers = append(providers, *providersByIndex[index])
	}
	return providers, true, nil
}

func applyAuthProviderEnvField(provider *AuthProviderConfig, name, field, value string) error {
	switch field {
	case "ID":
		provider.ID = value
	case "TYPE":
		provider.Type = value
	case "LABEL":
		provider.Label = value
	case "CLIENT_ID":
		provider.ClientID = value
	case "CLIENT_SECRET":
		provider.ClientSecret = value
	case "ISSUER_URL":
		provider.IssuerURL = value
	case "SCOPES":
		provider.Scopes = splitCommaSeparatedEnv(value)
	case "REQUEST_EMAIL":
		requestEmail, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s must be a boolean: %w", name, err)
		}
		provider.RequestEmail = &requestEmail
	default:
		const providerOptionsPrefix = "PROVIDER_OPTIONS_"
		if strings.HasPrefix(field, providerOptionsPrefix) {
			optionName := strings.ToLower(strings.TrimPrefix(field, providerOptionsPrefix))
			if optionName == "" {
				return fmt.Errorf("%s must include a provider option name", name)
			}
			if provider.ProviderOptions == nil {
				provider.ProviderOptions = make(map[string]string)
			}
			provider.ProviderOptions[optionName] = value
			return nil
		}
		return fmt.Errorf("%s uses unknown auth provider field %q", name, field)
	}
	return nil
}

func splitCommaSeparatedEnv(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
