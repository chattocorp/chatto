package config

import (
	"strings"

	"github.com/c2h5oh/datasize"
)

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

// VideoConfig controls whether video uploads are accepted and their upload limit.
type VideoConfig struct {
	Enabled       bool              `toml:"enabled" env:"CHATTO_VIDEO_ENABLED" comment:"Allow video uploads and enqueue derivative processing. Requires at least one asset-processing worker."`
	MaxUploadSize datasize.ByteSize `toml:"max_upload_size,commented" env:"CHATTO_VIDEO_MAX_UPLOAD_SIZE" comment:"Maximum size for video uploads. Supports human-readable formats like '100 MB'. Default: 100 MB."`
}

// AssetProcessingConfig controls the durable asset-processing worker. Enabled
// determines whether chatto run embeds the worker; the standalone chatto
// asset-processing command runs explicitly but uses the remaining settings.
type AssetProcessingConfig struct {
	Enabled           bool   `toml:"enabled" env:"CHATTO_ASSET_PROCESSING_ENABLED" comment:"Start the built-in asset-processing worker inside chatto run."`
	FFmpegPath        string `toml:"ffmpeg_path,commented" env:"CHATTO_ASSET_PROCESSING_FFMPEG_PATH" comment:"Path to ffmpeg binary. Auto-detected from PATH if empty."`
	FFprobePath       string `toml:"ffprobe_path,commented" env:"CHATTO_ASSET_PROCESSING_FFPROBE_PATH" comment:"Path to ffprobe binary. Auto-detected from PATH if empty."`
	MaxConcurrentJobs int    `toml:"max_concurrent_jobs,commented" env:"CHATTO_ASSET_PROCESSING_MAX_CONCURRENT_JOBS" comment:"Maximum number of asset-processing jobs to run simultaneously in this process. Default: 2."`
	TempDir           string `toml:"temp_dir,commented" env:"CHATTO_ASSET_PROCESSING_TEMP_DIR" comment:"Temporary directory for asset processing. Default: system temp directory."`
}

// DefaultVideoMaxUploadSize is the default maximum size for video uploads (100 MB).
const DefaultVideoMaxUploadSize datasize.ByteSize = 100 * datasize.MB

// MaxConcurrentJobsOrDefault returns the maximum concurrent jobs for one
// asset-processing worker process, defaulting to 2.
func (c *AssetProcessingConfig) MaxConcurrentJobsOrDefault() int {
	if c.MaxConcurrentJobs <= 0 {
		return 2
	}
	return c.MaxConcurrentJobs
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
