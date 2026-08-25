package config

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/idna"
)

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
	URL                    string        `toml:"url" env:"CHATTO_WEBSERVER_URL" comment:"Public URL where the webserver is accessible. Used for generating absolute URLs."`
	Port                   int           `toml:"port" env:"CHATTO_WEBSERVER_PORT" comment:"Port for the webserver to listen on."`
	TrustedProxies         []string      `toml:"trusted_proxies,commented" env:"CHATTO_WEBSERVER_TRUSTED_PROXIES" comment:"IP addresses or CIDR ranges of reverse proxies allowed to supply forwarded host and client-IP headers. Default: none."`
	APICompression         *bool         `toml:"api_compression" env:"CHATTO_WEBSERVER_API_COMPRESSION" comment:"Compress eligible ConnectRPC API responses with gzip. Disable to reduce compressor memory and CPU at the cost of higher network usage. Default: true."`
	APICompressionMinBytes *int          `toml:"api_compression_min_bytes" env:"CHATTO_WEBSERVER_API_COMPRESSION_MIN_BYTES" comment:"Minimum uncompressed ConnectRPC response size eligible for gzip compression. Default: 1024."`
	WebSocketCompression   *bool         `toml:"websocket_compression" env:"CHATTO_WEBSERVER_WEBSOCKET_COMPRESSION" comment:"Enable WebSocket compression for eligible realtime frames. Default: true."`
	RequestLogging         *bool         `toml:"request_logging" env:"CHATTO_WEBSERVER_REQUEST_LOGGING" comment:"Log HTTP requests. Successful requests are debug-level; 4xx responses are warnings; 5xx responses are errors. Useful for debugging but can be noisy in production. Default: false."`
	CookieSigningSecret    string        `toml:"cookie_signing_secret" env:"CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET" comment:"Secret for signing browser-flow cookies and CSRF proofs. NEVER SHARE THIS!\nIf it leaks, change it immediately. Existing browser flows must restart and CSRF cookies will refresh; opaque chatto_auth_* sessions use core.secret_key instead."`
	CookieEncryptionSecret string        `toml:"cookie_encryption_secret" env:"CHATTO_WEBSERVER_COOKIE_ENCRYPTION_SECRET" comment:"Optional hex-encoded secret used to encrypt session cookies (in addition to signing). Must decode to 16, 24, or 32 bytes (AES-128/192/256). If unset, cookies are signed but not encrypted — anything ever written to the session is readable by anyone who steals the cookie."`
	TLS                    TLSConfig     `toml:"tls" comment:"Automatic TLS configuration via Let's Encrypt."`
	Shields                ShieldsConfig `toml:"shields,commented" comment:"Public Shields.io-compatible community badges. Disabled by default."`
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

// SearchConfig controls Chatto's consumer-facing search API and UI.
type SearchConfig struct {
	Enabled bool `toml:"enabled" env:"CHATTO_SEARCH_ENABLED" comment:"Enable consumer-facing message search queries. Default: false."`
}

// SearchProviderConfig controls the bundled Bleve search provider.
type SearchProviderConfig struct {
	Enabled   bool     `toml:"enabled" env:"CHATTO_SEARCH_PROVIDER_ENABLED" comment:"Start the bundled Bleve search provider from chatto run. Default: false."`
	Directory string   `toml:"directory,commented" env:"CHATTO_SEARCH_PROVIDER_DIRECTORY" comment:"Directory for the disposable local Bleve index. Default: ./data/search."`
	Languages []string `toml:"languages,commented" env:"CHATTO_SEARCH_PROVIDER_LANGUAGES" comment:"Bleve language analyzers used for message indexing and queries. Omit to enable all bundled analyzers; use an empty list for literal matching only."`
}

var searchProviderLanguageCodes = []string{
	"ar", "cjk", "ckb", "da", "de", "en", "es", "fa", "fi", "fr", "hi",
	"hr", "hu", "it", "nl", "no", "pl", "pt", "ro", "ru", "sv", "tr",
}

// SupportedSearchProviderLanguages returns the language analyzer codes accepted
// by the bundled Bleve provider.
func SupportedSearchProviderLanguages() []string {
	return append([]string(nil), searchProviderLanguageCodes...)
}

// DirectoryOrDefault returns the bundled provider's local index directory.
func (c SearchProviderConfig) DirectoryOrDefault() string {
	if strings.TrimSpace(c.Directory) == "" {
		return "./data/search"
	}
	return strings.TrimSpace(c.Directory)
}

// LanguagesOrDefault returns the normalized configured analyzer codes. An
// omitted setting enables every bundled analyzer, while an explicit empty list
// retains only language-neutral literal and fuzzy matching.
func (c SearchProviderConfig) LanguagesOrDefault() []string {
	if c.Languages == nil {
		return SupportedSearchProviderLanguages()
	}
	return normalizeSearchProviderLanguages(c.Languages)
}

func normalizeSearchProviderLanguages(languages []string) []string {
	normalized := make([]string, len(languages))
	for i, language := range languages {
		normalized[i] = strings.ToLower(strings.TrimSpace(language))
	}
	if len(normalized) == 1 && normalized[0] == "none" {
		return []string{}
	}
	sort.Strings(normalized)
	return normalized
}

// ShieldsConfig controls public Shields.io-compatible community badges.
type ShieldsConfig struct {
	Enabled bool `toml:"enabled" env:"CHATTO_WEBSERVER_SHIELDS_ENABLED" comment:"Expose public Shields.io-compatible badge endpoints for aggregate community counts. Disabled by default because counts reveal server size and activity."`
}

// DiagnosticsConfig controls opt-in local/operator diagnostics.
type DiagnosticsConfig struct {
	StartupCPUProfile string `toml:"startup_cpu_profile,commented" env:"CHATTO_DIAGNOSTICS_STARTUP_CPU_PROFILE" comment:"Write a Go CPU profile covering process startup through core boot to this path. Disabled when empty."`
}

// OperatorAPIConfig controls the local root-equivalent operator API socket.
type OperatorAPIConfig struct {
	Enabled    bool   `toml:"enabled" env:"CHATTO_OPERATOR_API_ENABLED" comment:"Enable the local operator API Unix socket. Default: false."`
	SocketPath string `toml:"socket_path,commented" env:"CHATTO_OPERATOR_API_SOCKET_PATH" comment:"Unix socket path for local operator commands. Default: /tmp/chatto/operator.sock."`
	SocketMode string `toml:"socket_mode,omitempty" env:"CHATTO_OPERATOR_API_SOCKET_MODE"`
}

const (
	defaultOperatorAPISocketPath = "/tmp/chatto/operator.sock"
	OperatorAPISocketMode        = os.FileMode(0o600)
)

// SocketPathOrDefault returns the configured operator API socket path.
func (c OperatorAPIConfig) SocketPathOrDefault() string {
	if strings.TrimSpace(c.SocketPath) == "" {
		return defaultOperatorAPISocketPath
	}
	return strings.TrimSpace(c.SocketPath)
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
	hostname, err := idna.Lookup.ToASCII(strings.ToLower(u.Hostname()))
	if err != nil {
		return fmt.Errorf("%s host must be a valid IDNA hostname: %w", name, err)
	}
	if _, err := netip.ParseAddr(hostname); err != nil && hostnameEndsInNumber(hostname) {
		return fmt.Errorf("%s must use canonical dotted IPv4 syntax", name)
	}
	if port := u.Port(); port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return fmt.Errorf("%s port must be between 0 and 65535", name)
		}
	}
	return nil
}

// hostnameEndsInNumber detects hostnames that browsers interpret using their
// legacy IPv4 parser. Requiring modern dotted IPv4 spelling prevents the
// browser and server from serializing the same configured origin differently.
func hostnameEndsInNumber(hostname string) bool {
	labels := strings.Split(strings.TrimSuffix(hostname, "."), ".")
	last := labels[len(labels)-1]
	if strings.HasPrefix(last, "0x") {
		last = strings.TrimPrefix(last, "0x")
		if last == "" {
			return false
		}
		for _, character := range last {
			if !strings.ContainsRune("0123456789abcdef", character) {
				return false
			}
		}
		return true
	}
	if last == "" {
		return false
	}
	for _, character := range last {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
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

const defaultAPICompressionMinBytes = 1024

// APICompressionEnabled returns whether ConnectRPC responses may be
// compressed, defaulting to true. Compressed requests remain supported.
func (c *WebserverConfig) APICompressionEnabled() bool {
	if c.APICompression == nil {
		return true
	}
	return *c.APICompression
}

// APICompressionMinBytesOrDefault returns the smallest uncompressed
// ConnectRPC response eligible for compression.
func (c *WebserverConfig) APICompressionMinBytesOrDefault() int {
	if c.APICompressionMinBytes == nil {
		return defaultAPICompressionMinBytes
	}
	return *c.APICompressionMinBytes
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
