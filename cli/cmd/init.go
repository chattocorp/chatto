package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/c2h5oh/datasize"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/pkg/natsauth"
)

var initConfigFile string
var initWithLiveKit bool
var initWithSearch bool
var initInteractive bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initializes the chatto server and generates a configuration file",

	RunE: func(cmd *cobra.Command, args []string) error {
		opts := defaultInitOptions()
		opts.ConfigPath = initConfigFile
		if opts.ConfigPath == "" {
			opts.ConfigPath = "chatto.toml"
		}
		opts.WithLiveKit, opts.WithSearch = initWithLiveKit, initWithSearch
		if initInteractive {
			confirmed, err := promptInitOptions(cmd, &opts)
			if err != nil {
				return err
			}
			if !confirmed {
				cmd.Println("Setup cancelled. No files were created.")
				return nil
			}
		}
		if err := writeInitialConfig(cmd, opts); err != nil {
			return err
		}
		if initInteractive {
			printInitNextSteps(cmd, opts)
		}
		return nil
	},
}

// writeInitialConfig writes the selected configuration only after all prompts complete.
// Both modes use this path for secret generation and exclusive file creation.
func writeInitialConfig(cmd *cobra.Command, opts initOptions) error {
	if err := validateInitAuth(opts); err != nil {
		return err
	}
	configPath := opts.ConfigPath
	liveKitPath := filepath.Join(filepath.Dir(configPath), "livekit.yaml")
	paths := []string{configPath}
	if opts.WithLiveKit {
		if filepath.Clean(configPath) == liveKitPath {
			return fmt.Errorf("Chatto config path must differ from LiveKit config path %s", liveKitPath)
		}
		paths = append(paths, liveKitPath)
	}
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("config file already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check config path: %w", err)
		}
	}

	// Generate a random session signing secret (32 bytes = 256 bits)
	sessionSecret := make([]byte, 32)
	if _, err := rand.Read(sessionSecret); err != nil {
		return fmt.Errorf("generate session secret: %w", err)
	}
	sessionSecretString := hex.EncodeToString(sessionSecret)

	// Generate a random session encryption secret (32 bytes = AES-256).
	// Decoded back to raw bytes at server startup.
	cookieEncryptionSecret := make([]byte, 32)
	if _, err := rand.Read(cookieEncryptionSecret); err != nil {
		return fmt.Errorf("generate cookie encryption secret: %w", err)
	}
	cookieEncryptionSecretString := hex.EncodeToString(cookieEncryptionSecret)

	// Generate a random signing secret for assets (32 bytes = 256 bits)
	signingSecret := make([]byte, 32)
	if _, err := rand.Read(signingSecret); err != nil {
		return fmt.Errorf("generate signing secret: %w", err)
	}
	signingSecretString := hex.EncodeToString(signingSecret)

	// Generate a random server-wide core secret for token verifiers.
	coreSecret := make([]byte, 32)
	if _, err := rand.Read(coreSecret); err != nil {
		return fmt.Errorf("generate core secret: %w", err)
	}
	coreSecretString := hex.EncodeToString(coreSecret)

	// Generate a random auth token for NATS connections (32 bytes = 256 bits)
	authToken := make([]byte, 32)
	if _, err := rand.Read(authToken); err != nil {
		return fmt.Errorf("generate auth token: %w", err)
	}
	authTokenString := hex.EncodeToString(authToken)

	// Build configuration
	directRegistration := opts.DirectRegistration
	directLogin := opts.DirectLogin
	unlimited := -1
	cfg := config.ChattoConfig{
		General: config.GeneralConfig{
			LogLevel:  "info",
			LogFormat: "auto",
		},
		Auth: config.AuthConfig{
			DirectRegistration: &directRegistration,
			DirectLogin:        &directLogin,
			EmailOTP: config.EmailOTPConfig{
				ThrottlingEnabled: new(true),
				TTL:               config.Duration(config.DefaultEmailOTPTTL),
				MaxDeliveredCodes: 10,
				MaxWrongAttempts:  5,
			},
		},
		Limits: config.LimitsConfig{
			MaxUsers: &unlimited,
		},
		Webserver: config.WebserverConfig{
			Port:                   opts.Port,
			URL:                    opts.PublicURL,
			CookieSigningSecret:    sessionSecretString,
			CookieEncryptionSecret: cookieEncryptionSecretString,
		},
		Core: config.CoreConfig{
			SecretKey: coreSecretString,
			Assets: config.AssetsConfig{
				SigningSecret:  signingSecretString,
				MaxUploadSize:  25 * datasize.MB,
				StorageBackend: config.StorageBackendNATS,
			},
		},
		SMTP: opts.SMTP,
		Email: config.EmailConfig{
			Transport: config.EmailTransportSMTP,
		},
		AssetProcessing: config.AssetProcessingConfig{
			Enabled: true,
		},
		NATS: config.NATSConfig{
			Replicas: 1,
			Client: config.NATSClientConfig{
				URL:        "nats://nats.example.com:4222",
				AuthMethod: natsauth.AuthToken,
				Token:      "replace-me",
			},
			Embedded: config.EmbeddedNATSConfig{
				Enabled:      true,
				Port:         4222,
				BindAddress:  "127.0.0.1",
				HTTPPort:     8222,
				DataDir:      "./data",
				SyncInterval: "always",
				AuthToken:    authTokenString,
			},
		},
	}

	// Omit NATS here for external setups; its active client table is appended below.
	type initBaseConfig struct {
		config.ChattoConfig
		NATS *config.NATSConfig `toml:"nats,omitempty"`
		Auth any                `toml:"auth"`
	}
	baseConfig := initBaseConfig{ChattoConfig: cfg, Auth: initAuthConfig(opts, cfg.Auth)}
	if opts.EmbeddedNATS {
		baseConfig.NATS = &cfg.NATS
	} else if err := validateInitNATSClient(opts.NATSClient); err != nil {
		return err
	}

	// Write config file
	// Override only explicitly enabled tables; retain commented examples for the others.
	type searchInitConfig struct {
		initBaseConfig
		Search         config.SearchConfig         `toml:"search" comment:"Message search configuration."`
		SearchProvider config.SearchProviderConfig `toml:"search_provider" comment:"Bundled Bleve message search provider."`
	}
	searchConfig := searchInitConfig{
		initBaseConfig: baseConfig,
		Search:         config.SearchConfig{Enabled: true},
		SearchProvider: config.SearchProviderConfig{Enabled: true, Directory: "./data/search"},
	}
	var configDocument any = baseConfig
	if opts.WithSearch {
		configDocument = searchConfig
	}
	var liveKitConfig []byte
	if opts.WithLiveKit {
		apiKey := rand.Text()
		// LiveKit requires an API secret of at least 32 characters.
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return fmt.Errorf("generate LiveKit API secret: %w", err)
		}
		apiSecret := hex.EncodeToString(secretBytes)
		liveKitConfig = []byte(fmt.Sprintf(`# Local LiveKit server for Chatto. Configure public URLs and TLS before deployment.
port: 7880
rtc:
  tcp_port: 7881
  udp_port: 7882
  use_external_ip: false
  enable_loopback_candidate: true
keys:
  %q: %q
webhook:
  urls:
    - %q
  api_key: %q
`, apiKey, apiSecret, strings.TrimRight(opts.PublicURL, "/")+"/webhooks/livekit", apiKey))
		liveKit := config.LiveKitConfig{
			Enabled:   true,
			URL:       "ws://localhost:7880",
			APIKey:    apiKey,
			APISecret: apiSecret,
		}
		if opts.WithSearch {
			configDocument = struct {
				searchInitConfig
				LiveKit config.LiveKitConfig `toml:"livekit" comment:"LiveKit voice and video call configuration. Credentials match the generated livekit.yaml."`
			}{searchConfig, liveKit}
		} else {
			configDocument = struct {
				initBaseConfig
				LiveKit config.LiveKitConfig `toml:"livekit" comment:"LiveKit voice and video call configuration. Credentials match the generated livekit.yaml."`
			}{baseConfig, liveKit}
		}
	}
	b, err := toml.Marshal(configDocument)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if !opts.EmbeddedNATS {
		natsConfig, err := marshalInitExternalNATS(opts.NATSClient)
		if err != nil {
			return fmt.Errorf("marshal external NATS config: %w", err)
		}
		b = append(append(b, '\n'), natsConfig...)
	}
	text := addAuthProviderExamples(string(b))
	text = addEmailOTPDefaults(text)

	if err := writeInitConfig(configPath, []byte(text)); err != nil {
		return err
	}
	if opts.WithLiveKit {
		if err := writeInitConfig(liveKitPath, liveKitConfig); err != nil {
			// Remove only the Chatto config created by this invocation, so init can be retried.
			if cleanupErr := os.Remove(configPath); cleanupErr != nil {
				return fmt.Errorf("%w (remove partial Chatto config: %v)", err, cleanupErr)
			}
			return err
		}
		cmd.Printf("LiveKit configuration written to %s\n", liveKitPath)
	}
	cmd.Printf("Configuration written to %s\n", configPath)
	return nil
}

// writeInitConfig creates a private configuration file without replacing an existing file.
func writeInitConfig(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create config: %w", err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return errors.Join(fmt.Errorf("write config: %w", err), os.Remove(path))
	}
	return nil
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

	// go-toml separates tables with a blank line. Stop at that boundary so
	// provider tables and unrelated sections are preserved regardless of order.
	end := len(tomlText)
	if separator := strings.Index(tomlText[start:], "\n\n"); separator >= 0 {
		end = start + separator + 1
	}

	defaultTTL := fmt.Sprintf("%dm", config.DefaultEmailOTPTTL/time.Minute)
	emailOTPDefaults := fmt.Sprintf(`# Email OTP guardrails for registration and email verification.
[auth.email_otp]
# Enable email OTP throttling for registration and email verification. Default: true.
throttling_enabled = true
# How long registration and email-verification codes stay valid. Default: %[1]s.
# ttl = '%[1]s'
# Maximum successfully delivered codes per email challenge before throttling. Default: 10.
# max_delivered_codes = 10
# Maximum wrong-code attempts per email challenge before throttling. Default: 5.
# max_wrong_attempts = 5
`, defaultTTL)

	return tomlText[:start] + emailOTPDefaults + tomlText[end:]
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVarP(&initInteractive, "interactive", "i", false, "configure your server with a guided terminal setup")
	initCmd.Flags().BoolVar(&initWithSearch, "with-search", false, "enable message search and the bundled search provider")
	initCmd.Flags().StringVarP(&initConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
	initCmd.Flags().BoolVar(&initWithLiveKit, "with-livekit", false, "generate matching Chatto and livekit.yaml settings for a local LiveKit server")
}
