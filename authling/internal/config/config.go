// Package config loads and validates Authling's standalone configuration.
package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"hmans.de/chatto/pkg/appconfig"
)

const (
	DefaultPath                           = "authling.toml"
	DefaultPasswordMinimumLength          = 10
	DefaultSigningKeyRotationIntervalDays = 90
)

// Config is Authling's canonical process configuration.
type Config struct {
	HTTP           HTTPConfig           `toml:"http"`
	Authentication AuthenticationConfig `toml:"authentication"`
	OIDC           OIDCConfig           `toml:"oidc"`
	NATS           NATSConfig           `toml:"nats"`
	SMTP           SMTPConfig           `toml:"smtp"`
}

// OIDCConfig controls Authling's OpenID Provider and conventional clients.
// URL-identified CIMD clients require no configuration.
type OIDCConfig struct {
	Clients                        []OIDCClientConfig `toml:"clients"`
	CIMDTrustedPrivateHosts        []string           `toml:"cimd_trusted_private_hosts" env:"AUTHLING_OIDC_CIMD_TRUSTED_PRIVATE_HOSTS"`
	CIMDTrustedLoopbackHosts       []string           `toml:"cimd_trusted_loopback_hosts" env:"AUTHLING_OIDC_CIMD_TRUSTED_LOOPBACK_HOSTS"`
	SigningKeyRotationIntervalDays int                `toml:"signing_key_rotation_interval_days" env:"AUTHLING_OIDC_SIGNING_KEY_ROTATION_INTERVAL_DAYS"`
}

// SigningKeyRotationInterval returns how long an active OIDC signing key may
// remain in use before Authling automatically starts a rollover.
func (c OIDCConfig) SigningKeyRotationInterval() time.Duration {
	days := c.SigningKeyRotationIntervalDays
	if days == 0 {
		days = DefaultSigningKeyRotationIntervalDays
	}
	return time.Duration(days) * 24 * time.Hour
}

// OIDCClientConfig declares one conventional OpenID Connect client. An empty
// secret creates a public client; a non-empty secret enables client_secret_basic.
type OIDCClientConfig struct {
	ID           string   `toml:"id"`
	Name         string   `toml:"name"`
	Secret       string   `toml:"secret"`
	RedirectURIs []string `toml:"redirect_uris"`
}

// TrustedPrivateCIMDHosts returns normalized hostnames whose CIMD documents
// may resolve to private addresses. Other special-use destinations remain
// forbidden even for these explicitly trusted hosts.
func (c OIDCConfig) TrustedPrivateCIMDHosts() []string {
	return normalizeCIMDHosts(c.CIMDTrustedPrivateHosts)
}

// TrustedLoopbackCIMDHosts returns normalized hostnames whose CIMD documents
// may resolve to loopback addresses. This explicit development exception does
// not relax restrictions for any other special-use destination.
func (c OIDCConfig) TrustedLoopbackCIMDHosts() []string {
	return normalizeCIMDHosts(c.CIMDTrustedLoopbackHosts)
}

func normalizeCIMDHosts(configured []string) []string {
	hosts := make([]string, 0, len(configured))
	for _, host := range configured {
		hosts = append(hosts, normalizeCIMDHost(host))
	}
	return hosts
}

func normalizeCIMDHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

// AuthenticationConfig controls local authentication policy.
type AuthenticationConfig struct {
	PasswordMinimumLength int `toml:"password_minimum_length" env:"AUTHLING_AUTHENTICATION_PASSWORD_MINIMUM_LENGTH"`
}

// PasswordMinimumLengthOrDefault returns the configured minimum number of
// Unicode characters accepted for a local password.
func (c AuthenticationConfig) PasswordMinimumLengthOrDefault() int {
	if c.PasswordMinimumLength == 0 {
		return DefaultPasswordMinimumLength
	}
	return c.PasswordMinimumLength
}

// SMTPTLSPolicy controls SMTP transport encryption.
type SMTPTLSPolicy string

const (
	SMTPTLSMandatory     SMTPTLSPolicy = "mandatory"
	SMTPTLSOpportunistic SMTPTLSPolicy = "opportunistic"
	SMTPTLSImplicit      SMTPTLSPolicy = "implicit"
)

// SMTPConfig controls transactional email delivery.
type SMTPConfig struct {
	Enabled       bool          `toml:"enabled" env:"AUTHLING_SMTP_ENABLED"`
	Host          string        `toml:"host" env:"AUTHLING_SMTP_HOST"`
	Port          int           `toml:"port" env:"AUTHLING_SMTP_PORT"`
	TLS           SMTPTLSPolicy `toml:"tls" env:"AUTHLING_SMTP_TLS"`
	TLSServerName string        `toml:"tls_server_name" env:"AUTHLING_SMTP_TLS_SERVER_NAME"`
	TLSSkipVerify bool          `toml:"tls_skip_verify" env:"AUTHLING_SMTP_TLS_SKIP_VERIFY"`
	Username      string        `toml:"username" env:"AUTHLING_SMTP_USERNAME"`
	Password      string        `toml:"password" env:"AUTHLING_SMTP_PASSWORD"`
	From          string        `toml:"from" env:"AUTHLING_SMTP_FROM"`
}

// TLSPolicyOrDefault returns a fail-closed transport policy. Port 465 uses
// implicit TLS even when the policy is omitted or written as mandatory.
func (c SMTPConfig) TLSPolicyOrDefault() SMTPTLSPolicy {
	policy := SMTPTLSPolicy(strings.ToLower(strings.TrimSpace(string(c.TLS))))
	if c.Port == 465 && (policy == "" || policy == SMTPTLSMandatory) {
		return SMTPTLSImplicit
	}
	if policy == "" {
		return SMTPTLSMandatory
	}
	return policy
}

// HTTPConfig controls Authling's public HTTP listener.
type HTTPConfig struct {
	BindAddress string `toml:"bind_address" env:"AUTHLING_HTTP_BIND_ADDRESS"`
	// PublicURL is the externally visible origin. It determines whether browser
	// cookies require HTTPS and will become the basis of Authling's issuer URL.
	PublicURL string `toml:"public_url" env:"AUTHLING_HTTP_PUBLIC_URL"`
	// TrustProxyHeaders accepts the sanitized X-Forwarded-Host and
	// X-Forwarded-Proto supplied by the deployment's sole reverse proxy.
	TrustProxyHeaders bool `toml:"trust_proxy_headers" env:"AUTHLING_HTTP_TRUST_PROXY_HEADERS"`
}

// PublicURLOrDefault returns the configured browser origin. A loopback
// listener defaults to its own plain-HTTP origin for local development.
func (c HTTPConfig) PublicURLOrDefault() string {
	if strings.TrimSpace(c.PublicURL) != "" {
		return c.PublicURL
	}
	return "http://" + c.BindAddressOrDefault()
}

// SecureCookies reports whether the public origin requires HTTPS cookies.
// Validate must be called before this method.
func (c HTTPConfig) SecureCookies() bool {
	parsed, err := url.Parse(c.PublicURLOrDefault())
	return err == nil && strings.EqualFold(parsed.Scheme, "https")
}

// BindAddressOrDefault returns the configured listener address or the safe
// loopback-only default.
func (c HTTPConfig) BindAddressOrDefault() string {
	if strings.TrimSpace(c.BindAddress) == "" {
		return "127.0.0.1:8080"
	}
	return c.BindAddress
}

// NATSConfig selects Authling's dedicated NATS account and storage policy.
type NATSConfig struct {
	Replicas int                `toml:"replicas" env:"AUTHLING_NATS_REPLICAS"`
	Client   NATSClientConfig   `toml:"client"`
	Embedded EmbeddedNATSConfig `toml:"embedded"`
}

// ReplicasOrDefault returns the configured JetStream replica count.
func (c NATSConfig) ReplicasOrDefault() int {
	if c.Replicas == 0 {
		return 1
	}
	return c.Replicas
}

// NATSClientConfig connects Authling to an external, dedicated NATS account.
type NATSClientConfig struct {
	URL             string `toml:"url" env:"AUTHLING_NATS_CLIENT_URL"`
	CredentialsFile string `toml:"credentials_file" env:"AUTHLING_NATS_CLIENT_CREDENTIALS_FILE"`
}

// EmbeddedNATSConfig runs a private, in-process NATS server for simple
// single-process deployments.
type EmbeddedNATSConfig struct {
	Enabled bool   `toml:"enabled" env:"AUTHLING_NATS_EMBEDDED_ENABLED"`
	DataDir string `toml:"data_dir" env:"AUTHLING_NATS_EMBEDDED_DATA_DIR"`
}

// Read loads TOML, applies environment overrides, defaults derived values, and
// validates the result. A missing default file is allowed for environment-only
// deployments; an explicitly named missing file is an error.
func Read(path string) (Config, error) {
	cfg, err := appconfig.Load[Config](appconfig.Options{
		Path:                  path,
		DefaultPath:           DefaultPath,
		RequireExplicitFile:   true,
		DisallowUnknownFields: true,
	})
	if err != nil {
		return Config{}, err
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.NATS.Embedded.Enabled && strings.TrimSpace(c.NATS.Embedded.DataDir) == "" {
		c.NATS.Embedded.DataDir = ".authling/nats"
	}
}

// Validate checks that Authling has exactly one usable NATS deployment mode.
func (c Config) Validate() error {
	var problems []string
	if days := c.OIDC.SigningKeyRotationIntervalDays; days < 0 || days > 3650 {
		problems = append(problems, "oidc.signing_key_rotation_interval_days must be between 1 and 3650 when set")
	}
	if minimum := c.Authentication.PasswordMinimumLengthOrDefault(); minimum < 8 || minimum > 128 {
		problems = append(problems, "authentication.password_minimum_length must be between 8 and 128")
	}
	host, portText, err := net.SplitHostPort(c.HTTP.BindAddressOrDefault())
	if err != nil {
		problems = append(problems, "http.bind_address must be a host:port listener address")
	} else {
		port, portErr := strconv.Atoi(portText)
		if portErr != nil || port < 0 || port > 65535 {
			problems = append(problems, "http.bind_address must contain a port from 0 to 65535")
		}
		if strings.ContainsAny(host, "\r\n") {
			problems = append(problems, "http.bind_address contains invalid characters")
		}
	}
	publicURL, publicURLErr := url.Parse(c.HTTP.PublicURLOrDefault())
	if publicURLErr != nil || publicURL.Host == "" ||
		(publicURL.Scheme != "http" && publicURL.Scheme != "https") ||
		publicURL.User != nil || (publicURL.Path != "" && publicURL.Path != "/") ||
		publicURL.RawQuery != "" || publicURL.Fragment != "" {
		problems = append(problems, "http.public_url must be an absolute HTTP(S) origin without credentials, paths, queries, or fragments")
	} else if publicURL.Scheme == "http" {
		bindHost, _, bindErr := net.SplitHostPort(c.HTTP.BindAddressOrDefault())
		if bindErr != nil || !isLoopbackHost(bindHost) || !isLoopbackHost(publicURL.Hostname()) {
			problems = append(problems, "http.public_url may use plain HTTP only when both the public URL and listener are loopback")
		}
	}
	seenClients := make(map[string]struct{}, len(c.OIDC.Clients))
	for index, client := range c.OIDC.Clients {
		field := fmt.Sprintf("oidc.clients[%d]", index)
		clientID := strings.TrimSpace(client.ID)
		if !validConventionalClientID(client.ID) {
			problems = append(problems, field+".id must be a non-URL identifier no longer than 256 characters")
		} else if _, exists := seenClients[clientID]; exists {
			problems = append(problems, field+".id is duplicated")
		} else {
			seenClients[clientID] = struct{}{}
		}
		if strings.TrimSpace(client.Name) == "" || len(client.Name) > 100 {
			problems = append(problems, field+".name must contain between 1 and 100 characters")
		}
		if client.Secret != "" && len(client.Secret) < 32 {
			problems = append(problems, field+".secret must contain at least 32 characters when configured")
		}
		if len(client.RedirectURIs) == 0 {
			problems = append(problems, field+".redirect_uris must contain at least one URI")
		}
		seenRedirects := make(map[string]struct{}, len(client.RedirectURIs))
		for _, redirect := range client.RedirectURIs {
			if len(redirect) > 2048 {
				problems = append(problems, field+".redirect_uris contains a URI longer than 2048 characters")
				continue
			}
			if _, exists := seenRedirects[redirect]; exists {
				problems = append(problems, field+".redirect_uris contains a duplicate URI")
				continue
			}
			seenRedirects[redirect] = struct{}{}
			if !validOIDCRedirectURI(redirect, publicURL) {
				problems = append(problems, field+".redirect_uris must contain absolute HTTPS URIs, or loopback HTTP URIs for loopback development")
			}
		}
	}
	seenCIMDHosts := make(map[string]string, len(c.OIDC.CIMDTrustedPrivateHosts)+len(c.OIDC.CIMDTrustedLoopbackHosts))
	validateCIMDHosts := func(name string, configured []string) {
		for index, raw := range configured {
			host := normalizeCIMDHost(raw)
			field := fmt.Sprintf("oidc.%s[%d]", name, index)
			parsed, err := url.Parse("https://" + host)
			if host == "" || err != nil || parsed.Host != host || parsed.Hostname() != host || parsed.Port() != "" {
				problems = append(problems, field+" must be one hostname without a scheme, port, path, query, or fragment")
				continue
			}
			if previous, exists := seenCIMDHosts[host]; exists {
				problems = append(problems, field+" duplicates "+previous)
			}
			seenCIMDHosts[host] = field
		}
	}
	validateCIMDHosts("cimd_trusted_private_hosts", c.OIDC.CIMDTrustedPrivateHosts)
	validateCIMDHosts("cimd_trusted_loopback_hosts", c.OIDC.CIMDTrustedLoopbackHosts)
	replicas := c.NATS.ReplicasOrDefault()
	if replicas != 1 && replicas != 3 && replicas != 5 {
		problems = append(problems, "nats.replicas must be 1, 3, or 5")
	}
	if c.SMTP.Enabled {
		if strings.TrimSpace(c.SMTP.Host) == "" {
			problems = append(problems, "smtp.host is required when SMTP is enabled")
		}
		if c.SMTP.Port < 1 || c.SMTP.Port > 65535 {
			problems = append(problems, "smtp.port must be between 1 and 65535 when SMTP is enabled")
		}
		if strings.TrimSpace(c.SMTP.From) == "" {
			problems = append(problems, "smtp.from is required when SMTP is enabled")
		}
	}
	switch c.SMTP.TLSPolicyOrDefault() {
	case SMTPTLSMandatory, SMTPTLSOpportunistic, SMTPTLSImplicit:
	default:
		problems = append(problems, "smtp.tls must be one of: mandatory, opportunistic, implicit")
	}

	externalConfigured := strings.TrimSpace(c.NATS.Client.URL) != "" ||
		strings.TrimSpace(c.NATS.Client.CredentialsFile) != ""
	switch {
	case c.NATS.Embedded.Enabled && externalConfigured:
		problems = append(problems, "configure either nats.embedded or nats.client, not both")
	case !c.NATS.Embedded.Enabled && !externalConfigured:
		problems = append(problems, "enable nats.embedded or configure nats.client")
	case c.NATS.Embedded.Enabled:
		if strings.TrimSpace(c.NATS.Embedded.DataDir) == "" {
			problems = append(problems, "nats.embedded.data_dir is required")
		}
	default:
		parsed, err := url.Parse(c.NATS.Client.URL)
		if err != nil ||
			parsed.Host == "" ||
			!validNATSScheme(parsed.Scheme) ||
			parsed.User != nil ||
			parsed.Path != "" ||
			parsed.RawQuery != "" ||
			parsed.Fragment != "" {
			problems = append(problems, "nats.client.url must be an absolute NATS URL without credentials, paths, queries, or fragments")
		}
		if strings.TrimSpace(c.NATS.Client.CredentialsFile) == "" {
			problems = append(problems, "nats.client.credentials_file is required for Authling's dedicated NATS account")
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

func validOIDCRedirectURI(raw string, issuer *url.URL) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && issuer != nil && isLoopbackHost(issuer.Hostname()) && isLoopbackHost(parsed.Hostname())
}

func validConventionalClientID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 256 || strings.HasPrefix(strings.ToLower(value), "https://") {
		return false
	}
	for _, char := range value {
		if char <= 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func validNATSScheme(scheme string) bool {
	switch strings.ToLower(scheme) {
	case "nats", "tls", "ws", "wss":
		return true
	default:
		return false
	}
}
