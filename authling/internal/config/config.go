// Package config loads and validates Authling's standalone configuration.
package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"hmans.de/chatto/pkg/appconfig"
)

const (
	DefaultPath                  = "authling.toml"
	DefaultPasswordMinimumLength = 10
)

// Config is Authling's canonical process configuration.
type Config struct {
	HTTP           HTTPConfig           `toml:"http"`
	Authentication AuthenticationConfig `toml:"authentication"`
	NATS           NATSConfig           `toml:"nats"`
	SMTP           SMTPConfig           `toml:"smtp"`
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
