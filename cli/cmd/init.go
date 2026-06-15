package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/c2h5oh/datasize"
	"github.com/charmbracelet/log"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"hmans.de/chatto/internal/config"
)

var initConfigFile string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initializes the chatto server and generates a configuration file",

	Run: func(cmd *cobra.Command, args []string) {
		configPath := initConfigFile
		if configPath == "" {
			configPath = "chatto.toml"
		}

		// Check if config file already exists
		if _, err := os.Stat(configPath); err == nil {
			log.Error("Config file already exists, aborting to prevent overwrite", "path", configPath)
			os.Exit(1)
		}

		// Generate a random session signing secret (32 bytes = 256 bits)
		sessionSecret := make([]byte, 32)
		if _, err := rand.Read(sessionSecret); err != nil {
			log.Fatal("Failed to generate session secret", "error", err)
		}
		sessionSecretString := hex.EncodeToString(sessionSecret)

		// Generate a random session encryption secret (32 bytes = AES-256).
		// Decoded back to raw bytes at server startup.
		cookieEncryptionSecret := make([]byte, 32)
		if _, err := rand.Read(cookieEncryptionSecret); err != nil {
			log.Fatal("Failed to generate cookie encryption secret", "error", err)
		}
		cookieEncryptionSecretString := hex.EncodeToString(cookieEncryptionSecret)

		// Generate a random signing secret for assets (32 bytes = 256 bits)
		signingSecret := make([]byte, 32)
		if _, err := rand.Read(signingSecret); err != nil {
			log.Fatal("Failed to generate signing secret", "error", err)
		}
		signingSecretString := hex.EncodeToString(signingSecret)

		// Generate a random server-wide core secret for token verifiers.
		coreSecret := make([]byte, 32)
		if _, err := rand.Read(coreSecret); err != nil {
			log.Fatal("Failed to generate core secret", "error", err)
		}
		coreSecretString := hex.EncodeToString(coreSecret)

		// Generate a random auth token for NATS connections (32 bytes = 256 bits)
		authToken := make([]byte, 32)
		if _, err := rand.Read(authToken); err != nil {
			log.Fatal("Failed to generate auth token", "error", err)
		}
		authTokenString := hex.EncodeToString(authToken)

		// Build configuration
		directRegistration := true
		unlimited := -1
		cfg := config.ChattoConfig{
			General: config.GeneralConfig{
				LogLevel:  "info",
				LogFormat: "auto",
			},
			Auth: config.AuthConfig{
				DirectRegistration: &directRegistration,
			},
			Limits: config.LimitsConfig{
				MaxUsers: &unlimited,
			},
			Webserver: config.WebserverConfig{
				Port:                   4000,
				URL:                    "http://localhost:4000",
				AllowedOrigins:         []string{"*"},
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
			NATS: config.NATSConfig{
				Replicas: 1,
				Client: config.NATSClientConfig{
					URL:        "nats://nats.example.com:4222",
					AuthMethod: config.NATSAuthToken,
					Token:      "replace-me",
				},
				Embedded: config.EmbeddedNATSConfig{
					Enabled:     true,
					Port:        4222,
					BindAddress: "127.0.0.1",
					HTTPPort:    8222,
					DataDir:     "./data",
					AuthToken:   authTokenString,
				},
			},
		}

		// Write config file
		log.Info("Writing configuration", "path", configPath)
		b, err := toml.Marshal(cfg)
		if err != nil {
			log.Fatal("Failed to marshal config", "error", err)
		}

		if err := os.WriteFile(configPath, b, 0600); err != nil {
			log.Fatal("Failed to write config file", "error", err)
		}
		fmt.Printf("Configuration written to %s\n", configPath)
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringVarP(&initConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
}
