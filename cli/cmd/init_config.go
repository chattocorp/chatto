package cmd

import (
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/c2h5oh/datasize"
	"github.com/pelletier/go-toml/v2"
	"hmans.de/chatto/internal/config"
)

func buildInitialConfig(answers initAnswers, entropy io.Reader) (config.ChattoConfig, error) {
	port, err := strconv.Atoi(answers.ListenPort)
	if err != nil || port < 1 || port > 65535 {
		return config.ChattoConfig{}, fmt.Errorf("listen port must be between 1 and 65535")
	}

	sessionSecret, err := randomHex(entropy, 32)
	if err != nil {
		return config.ChattoConfig{}, fmt.Errorf("generate cookie signing secret: %w", err)
	}
	cookieEncryptionSecret, err := randomHex(entropy, 32)
	if err != nil {
		return config.ChattoConfig{}, fmt.Errorf("generate cookie encryption secret: %w", err)
	}
	signingSecret, err := randomHex(entropy, 32)
	if err != nil {
		return config.ChattoConfig{}, fmt.Errorf("generate asset signing secret: %w", err)
	}
	coreSecret, err := randomHex(entropy, 32)
	if err != nil {
		return config.ChattoConfig{}, fmt.Errorf("generate core secret: %w", err)
	}
	authToken, err := randomHex(entropy, 32)
	if err != nil {
		return config.ChattoConfig{}, fmt.Errorf("generate embedded NATS token: %w", err)
	}

	directRegistration := true
	unlimited := -1
	cfg := config.ChattoConfig{
		General: config.GeneralConfig{LogLevel: "info", LogFormat: "auto"},
		Auth: config.AuthConfig{
			DirectRegistration: &directRegistration,
			EmailOTP: config.EmailOTPConfig{
				ThrottlingEnabled: &directRegistration,
				TTL:               config.Duration(15 * time.Minute),
				MaxDeliveredCodes: 10,
				MaxWrongAttempts:  5,
			},
		},
		Limits: config.LimitsConfig{MaxUsers: &unlimited},
		Webserver: config.WebserverConfig{
			Port:                   port,
			URL:                    strings.TrimSpace(answers.PublicURL),
			AllowedOrigins:         []string{"*"},
			CookieSigningSecret:    sessionSecret,
			CookieEncryptionSecret: cookieEncryptionSecret,
		},
		Core: config.CoreConfig{
			SecretKey: coreSecret,
			Assets: config.AssetsConfig{
				SigningSecret:  signingSecret,
				MaxUploadSize:  25 * datasize.MB,
				StorageBackend: config.StorageBackendNATS,
			},
		},
		SMTP: config.SMTPConfig{Enabled: false, Port: 587, TLS: config.SMTPTLSMandatory},
		NATS: config.NATSConfig{
			Replicas: answers.NATSReplicas,
			Client: config.NATSClientConfig{
				URL:             strings.TrimSpace(answers.ExternalNATSURL),
				AuthMethod:      answers.NATSAuthMethod,
				Token:           answers.NATSToken,
				Username:        answers.NATSUsername,
				Password:        answers.NATSPassword,
				CredentialsFile: answers.NATSCredentialsFile,
				NKeySeed:        answers.NATSNKeySeed,
			},
			Embedded: config.EmbeddedNATSConfig{
				Enabled:     answers.NATSMode == initNATSEmbedded,
				Port:        4222,
				BindAddress: "127.0.0.1",
				HTTPPort:    8222,
				DataDir:     strings.TrimSpace(answers.EmbeddedDataDir),
				AuthToken:   authToken,
			},
		},
	}
	if answers.NATSMode == initNATSEmbedded {
		// The client block is emitted as a commented example for embedded mode.
		cfg.NATS.Client = config.NATSClientConfig{
			URL:        "nats://nats.example.com:4222",
			AuthMethod: config.NATSAuthToken,
			Token:      "replace-me",
		}
	}
	return cfg, nil
}

func randomHex(source io.Reader, size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(source, value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func renderInitialConfig(cfg config.ChattoConfig, mode initNATSMode) (string, error) {
	b, err := toml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	text := addAuthProviderExamples(string(b))
	text = addEmailOTPDefaults(text)
	if mode == initNATSExternal {
		text, err = activateExternalNATSClient(text, cfg.NATS.Client)
		if err != nil {
			return "", err
		}
	}
	return text, nil
}

type externalNATSClientTOML struct {
	URL             string                `toml:"url"`
	AuthMethod      config.NATSAuthMethod `toml:"auth_method"`
	Token           string                `toml:"token,omitempty"`
	Username        string                `toml:"username,omitempty"`
	Password        string                `toml:"password,omitempty"`
	CredentialsFile string                `toml:"credentials_file,omitempty"`
	NKeySeed        string                `toml:"nkey_seed,omitempty"`
}

func activateExternalNATSClient(text string, client config.NATSClientConfig) (string, error) {
	const startMarker = "# External NATS client settings."
	const endMarker = "\n[nats.embedded]\n"
	start := strings.Index(text, startMarker)
	if start == -1 {
		return "", errorsNewGeneratedConfigMarker(startMarker)
	}
	endOffset := strings.Index(text[start:], endMarker)
	if endOffset == -1 {
		return "", errorsNewGeneratedConfigMarker("[nats.embedded]")
	}
	end := start + endOffset

	clientText, err := toml.Marshal(externalNATSClientTOML{
		URL:             client.URL,
		AuthMethod:      client.AuthMethod,
		Token:           client.Token,
		Username:        client.Username,
		Password:        client.Password,
		CredentialsFile: client.CredentialsFile,
		NKeySeed:        client.NKeySeed,
	})
	if err != nil {
		return "", err
	}
	section := "# External NATS client settings selected during chatto init.\n[nats.client]\n" + string(clientText)
	return text[:start] + section + text[end:], nil
}

func errorsNewGeneratedConfigMarker(marker string) error {
	return fmt.Errorf("generated configuration is missing %q marker", marker)
}

func addAuthProviderExamples(tomlText string) string {
	const generatedEmptyProviders = "# External login providers. Configure as repeated [[auth.providers]] tables.\nproviders = []"
	const providerExamples = `# External login providers. Uncomment and adapt one or more [[auth.providers]] tables.
#
# [[auth.providers]]
# id = 'chatto-hub'
# type = 'oidc'
# label = 'Chatto Hub'
# issuer_url = 'https://id.example.com/realms/chatto'
# client_id = 'chatto'
# client_secret = 'replace-me'
# request_email = true
#
# [[auth.providers]]
# id = 'github'
# type = 'github'
# client_id = 'replace-me'
# client_secret = 'replace-me'`

	return strings.Replace(tomlText, generatedEmptyProviders, providerExamples, 1)
}

func addEmailOTPDefaults(tomlText string) string {
	const marker = "# Email OTP guardrails for registration and email verification."
	start := strings.Index(tomlText, marker)
	if start == -1 {
		return tomlText
	}

	endMarker := "\n# Instance-wide resource limits."
	end := strings.Index(tomlText[start:], endMarker)
	if end == -1 {
		return tomlText
	}
	end += start

	const emailOTPDefaults = `# Email OTP guardrails for registration and email verification.
[auth.email_otp]
# Enable email OTP throttling for registration and email verification. Default: true.
throttling_enabled = true
# How long registration and email-verification codes stay valid. Default: 15m.
# ttl = '15m'
# Maximum successfully delivered codes per email challenge before throttling. Default: 10.
# max_delivered_codes = 10
# Maximum wrong-code attempts per email challenge before throttling. Default: 5.
# max_wrong_attempts = 5
`

	return tomlText[:start] + emailOTPDefaults + tomlText[end:]
}
