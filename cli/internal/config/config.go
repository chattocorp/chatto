package config

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"path/filepath"
	"strings"

	"hmans.de/chatto/pkg/appconfig"
	"hmans.de/chatto/pkg/natsauth"
)

// ChattoConfig is the canonical aggregate configuration decoded from TOML and
// environment variables before derived defaults, normalization, and validation.
type ChattoConfig struct {
	General         GeneralConfig         `toml:"general"`
	Owners          OwnersConfig          `toml:"owners" comment:"Email addresses that confer owner status."`
	Webserver       WebserverConfig       `toml:"webserver"`
	Metrics         MetricsConfig         `toml:"metrics,commented" comment:"Process-local Prometheus metrics endpoint."`
	Exporter        ExporterConfig        `toml:"exporter,commented" comment:"Deployment-wide Prometheus metrics exporter."`
	Search          SearchConfig          `toml:"search,commented" comment:"Consumer-facing message search configuration."`
	SearchProvider  SearchProviderConfig  `toml:"search_provider,commented" comment:"Bundled Bleve message search provider."`
	Diagnostics     DiagnosticsConfig     `toml:"diagnostics,commented" comment:"Opt-in diagnostics for local benchmarking and operator troubleshooting."`
	OperatorAPI     OperatorAPIConfig     `toml:"operator_api,commented" comment:"Local root-equivalent operator API Unix socket. Disabled by default."`
	Core            CoreConfig            `toml:"core" comment:"Core service configuration."`
	Auth            AuthConfig            `toml:"auth" comment:"Authentication configuration."`
	Limits          LimitsConfig          `toml:"limits,commented" comment:"Instance-wide resource limits. Use -1 for unlimited."`
	Email           EmailConfig           `toml:"email" comment:"Transactional email transport configuration. SMTP is the default transport."`
	SMTP            SMTPConfig            `toml:"smtp" comment:"SMTP configuration for transactional emails."`
	Push            PushConfig            `toml:"push,commented" comment:"Web Push notification configuration."`
	Video           VideoConfig           `toml:"video,commented" comment:"Video uploads and derivative settings."`
	AssetProcessing AssetProcessingConfig `toml:"asset_processing" comment:"Built-in durable asset-processing worker."`
	LiveKit         LiveKitConfig         `toml:"livekit,commented" comment:"LiveKit voice call configuration."`
	NATS            NATSConfig            `toml:"nats"`
	Bootstrap       BootstrapConfig       `toml:"bootstrap,commented" comment:"Dev/E2E-only: users and spaces auto-created on startup. ONLY honored by builds compiled with the 'bootstrap' build tag; release binaries ignore this section entirely."`
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
				c.NATS.Client.AuthMethod = natsauth.AuthToken
			} else {
				c.NATS.Client.AuthMethod = natsauth.AuthNone
			}
		}
		if c.NATS.Client.AuthMethod == natsauth.AuthToken && c.NATS.Client.Token == "" {
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
	if c.SearchProvider.Languages != nil {
		c.SearchProvider.Languages = normalizeSearchProviderLanguages(c.SearchProvider.Languages)
	}
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
	if c.OperatorAPI.Enabled {
		if strings.TrimSpace(c.OperatorAPI.SocketPathOrDefault()) == "" {
			errs = append(errs, "operator_api.socket_path is required when operator_api.enabled is true")
		}
		if strings.TrimSpace(c.OperatorAPI.SocketMode) != "" {
			errs = append(errs, "operator_api.socket_mode is no longer supported; operator API sockets always use mode 0600")
		}
	}

	// Port ranges (port 0 is allowed when TLS is enabled, as it defaults to 443)
	if c.Webserver.Port < 0 || c.Webserver.Port > 65535 {
		errs = append(errs, "webserver.port must be between 0 and 65535")
	}
	if c.Webserver.Port == 0 && !c.Webserver.TLS.Enabled {
		errs = append(errs, "webserver.port is required when TLS is disabled")
	}
	if c.Webserver.APICompressionMinBytes != nil && *c.Webserver.APICompressionMinBytes < 0 {
		errs = append(errs, "webserver.api_compression_min_bytes must not be negative")
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
	if c.SearchProvider.Enabled || strings.TrimSpace(c.SearchProvider.Directory) != "" || c.SearchProvider.Languages != nil {
		searchDirectory := filepath.Clean(c.SearchProvider.DirectoryOrDefault())
		if searchDirectory == "." || filepath.IsAbs(searchDirectory) && searchDirectory == filepath.VolumeName(searchDirectory)+string(filepath.Separator) {
			errs = append(errs, "search_provider.directory must name a dedicated index directory")
		}
		if dataDirectory := strings.TrimSpace(c.NATS.Embedded.DataDir); dataDirectory != "" {
			absoluteSearch, searchErr := filepath.Abs(searchDirectory)
			absoluteData, dataErr := filepath.Abs(filepath.Clean(dataDirectory))
			if searchErr == nil && dataErr == nil {
				relativeData, relErr := filepath.Rel(absoluteSearch, absoluteData)
				if relErr == nil && relativeData != ".." && !strings.HasPrefix(relativeData, ".."+string(filepath.Separator)) {
					errs = append(errs, "search_provider.directory must not contain the embedded NATS data directory")
				}
			}
		}
		supportedLanguages := make(map[string]struct{}, len(searchProviderLanguageCodes))
		for _, language := range searchProviderLanguageCodes {
			supportedLanguages[language] = struct{}{}
		}
		languages := normalizeSearchProviderLanguages(c.SearchProvider.Languages)
		seenLanguages := make(map[string]struct{}, len(languages))
		for _, language := range languages {
			if language == "" {
				errs = append(errs, "search_provider.languages must not contain empty language codes")
				continue
			}
			if _, duplicate := seenLanguages[language]; duplicate {
				errs = append(errs, fmt.Sprintf("search_provider.languages contains duplicate language %q", language))
				continue
			}
			seenLanguages[language] = struct{}{}
			if _, ok := supportedLanguages[language]; !ok {
				errs = append(errs, fmt.Sprintf("search_provider.languages contains unsupported language %q", language))
			}
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
	configuredOriginByRequestHost := make(map[string]string)
	registerOrigin := func(name, raw string) {
		origin, requestHost, ok := canonicalHTTPOriginAndRequestHost(raw)
		if !ok {
			return
		}
		if previous, exists := configuredOriginByRequestHost[requestHost]; exists && previous != origin {
			errs = append(errs, fmt.Sprintf("%s must not use both HTTP and HTTPS for request host %q", name, requestHost))
			return
		}
		configuredOriginByRequestHost[requestHost] = origin
	}
	if c.Webserver.URL != "" {
		if err := validateAbsoluteHTTPURL("webserver.url", c.Webserver.URL); err != nil {
			errs = append(errs, err.Error())
		} else {
			registerOrigin("webserver.url", c.Webserver.URL)
		}
	}
	for _, origin := range c.Webserver.AllowedOrigins {
		if origin == "*" {
			continue
		}
		if err := validateHTTPOrigin("webserver.allowed_origins", origin); err != nil {
			errs = append(errs, err.Error())
		} else {
			registerOrigin("webserver.allowed_origins", origin)
		}
	}
	if c.NATS.Client.URL != "" {
		if _, err := url.Parse(c.NATS.Client.URL); err != nil {
			errs = append(errs, fmt.Sprintf("nats.client.url is invalid: %v", err))
		}
	}
	for _, proxy := range c.Webserver.TrustedProxies {
		if net.ParseIP(proxy) == nil {
			if _, _, err := net.ParseCIDR(proxy); err != nil {
				errs = append(errs, fmt.Sprintf("webserver.trusted_proxies contains invalid IP address or CIDR %q", proxy))
			}
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
	switch c.Auth.AccountCreationPolicyOrDefault() {
	case AccountCreationPolicyOpen, AccountCreationPolicyInviteOnly:
	default:
		errs = append(errs, "auth.account_creation_policy must be one of: open, invite_only")
	}
	if c.Auth.TokenTTL.Duration() < 0 {
		errs = append(errs, "auth.token_ttl must not be negative")
	}
	if c.Auth.AccessTokenTTL.Duration() < 0 {
		errs = append(errs, "auth.access_token_ttl must not be negative")
	}

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
		if provider.ClientSecret == "" && provider.Type != AuthProviderTypeOpenIDConnect {
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

	// Transactional email configuration
	switch c.Email.TransportOrDefault() {
	case EmailTransportSMTP:
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
	case EmailTransportJMAP:
		if c.Webserver.URL == "" {
			errs = append(errs, "webserver.url is required when JMAP email is enabled")
		}
		if c.Email.JMAP.SessionURL == "" {
			errs = append(errs, "email.jmap.session_url is required when email.transport is jmap")
		} else if err := validateAbsoluteHTTPSURL("email.jmap.session_url", c.Email.JMAP.SessionURL); err != nil {
			errs = append(errs, err.Error())
		}
		if c.Email.JMAP.AccessToken == "" {
			errs = append(errs, "email.jmap.access_token is required when email.transport is jmap")
		}
		if c.Email.JMAP.From == "" {
			errs = append(errs, "email.jmap.from is required when email.transport is jmap")
		} else if _, err := mail.ParseAddress(c.Email.JMAP.From); err != nil {
			errs = append(errs, "email.jmap.from must be a valid email address")
		}
	default:
		errs = append(errs, "email.transport must be one of: smtp, jmap")
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
	if c.Core.ProjectionSnapshotRetention.Duration() < 0 {
		errs = append(errs, "core.projection_snapshot_retention must be positive")
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
		c.NATS.Client.AuthMethod == natsauth.AuthToken &&
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
	cfg, err := appconfig.Load[ChattoConfig](appconfig.Options{
		Path:        configPath,
		DefaultPath: "chatto.toml",
	})
	if err != nil {
		return ChattoConfig{}, err
	}

	// Apply Chatto-specific compatibility environment variables that cannot be
	// represented by fixed struct tags.
	if err := applyAuthProviderEnv(&cfg); err != nil {
		return cfg, err
	}
	if err := applyBootstrapEnv(&cfg); err != nil {
		return cfg, err
	}

	// Apply derived defaults and normalize harmless spelling differences.
	cfg.ApplyDefaults()
	cfg.Normalize()

	// Validate the product-owned schema.
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}
