package cmd

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"hmans.de/chatto/internal/config"
)

func TestInitGeneratesCoreSecret(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	originalConfigFile := initConfigFile
	initConfigFile = ""
	t.Cleanup(func() { initConfigFile = originalConfigFile })

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	cfg, err := config.ReadConfig(filepath.Join(tmpDir, "chatto.toml"))
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	if len(cfg.Core.SecretKey) != 64 {
		t.Fatalf("generated core secret length = %d, want 64", len(cfg.Core.SecretKey))
	}
	if _, err := hex.DecodeString(cfg.Core.SecretKey); err != nil {
		t.Fatalf("generated core secret should be hex: %v", err)
	}
	if cfg.Core.Assets.StorageBackend != config.StorageBackendNATS {
		t.Fatalf("generated storage backend = %q, want %q", cfg.Core.Assets.StorageBackend, config.StorageBackendNATS)
	}
	if cfg.NATS.Replicas != 1 {
		t.Fatalf("generated NATS replicas = %d, want 1", cfg.NATS.Replicas)
	}
	if cfg.NATS.Embedded.Port != 0 {
		t.Fatalf("generated embedded NATS port = %d, want 0 when port is commented out", cfg.NATS.Embedded.Port)
	}
	if cfg.NATS.Client.URL != "" {
		t.Fatalf("generated embedded NATS client URL = %q, want empty when TCP listener is disabled", cfg.NATS.Client.URL)
	}
	if got := cfg.Auth.EmailOTP.ThrottlingEnabledOrDefault(); got != true {
		t.Fatalf("generated email OTP throttling enabled = %v, want true", got)
	}
	if cfg.Core.SkipSetupWizard {
		t.Fatal("generated config must enable first-run setup")
	}
	if cfg.SMTP.Enabled {
		t.Fatal("generated SMTP config should be disabled by default")
	}
	if cfg.SMTP.Port != 587 {
		t.Fatalf("generated SMTP port = %d, want 587", cfg.SMTP.Port)
	}
	if cfg.SMTP.TLS != config.SMTPTLSMandatory {
		t.Fatalf("generated SMTP TLS policy = %q, want %q", cfg.SMTP.TLS, config.SMTPTLSMandatory)
	}
	if cfg.Email.Transport != config.EmailTransportSMTP {
		t.Fatalf("generated email transport = %q, want %q", cfg.Email.Transport, config.EmailTransportSMTP)
	}
	if !cfg.AssetProcessing.Enabled {
		t.Fatal("generated config should enable the built-in asset-processing worker")
	}
	raw, err := os.ReadFile(filepath.Join(tmpDir, "chatto.toml"))
	if err != nil {
		t.Fatalf("read generated raw config: %v", err)
	}
	rawText := string(raw)
	generalIndex := strings.Index(rawText, "\n[general]\n")
	if generalIndex == -1 && strings.HasPrefix(rawText, "[general]\n") {
		generalIndex = 0
	}
	ownersIndex := strings.Index(rawText, "\n[owners]\n")
	webserverIndex := strings.Index(rawText, "\n[webserver]\n")
	if generalIndex == -1 || ownersIndex == -1 || webserverIndex == -1 || !(generalIndex < ownersIndex && ownersIndex < webserverIndex) {
		t.Fatal("generated config should place [owners] between [general] and [webserver]")
	}
	if !strings.Contains(rawText, "log_level = 'info'") {
		t.Fatal("generated config should set general.log_level to 'info'")
	}
	assetProcessingIndex := strings.Index(rawText, "\n[asset_processing]\n")
	if assetProcessingIndex == -1 {
		t.Fatal("generated config should include an active [asset_processing] section")
	}
	assetProcessingBlock := rawText[assetProcessingIndex:]
	if nextSection := strings.Index(assetProcessingBlock[len("\n[asset_processing]\n"):], "\n["); nextSection >= 0 {
		assetProcessingBlock = assetProcessingBlock[:len("\n[asset_processing]\n")+nextSection]
	}
	if !strings.Contains(assetProcessingBlock, "\nenabled = true\n") {
		t.Fatal("generated config should explicitly enable [asset_processing]")
	}
	for _, setting := range []string{"# ffmpeg_path", "# ffprobe_path", "# max_concurrent_jobs", "# temp_dir"} {
		if !strings.Contains(assetProcessingBlock, setting) {
			t.Fatalf("generated [asset_processing] config should include %q", setting)
		}
	}
	if strings.Contains(rawText, "oauth_redirect_origins") {
		t.Fatal("generated config should not include the retired OAuth redirect-origin setting")
	}
	if strings.Contains(rawText, "\nproviders = []") {
		t.Fatal("generated config should not include an active empty auth.providers array")
	}
	if !strings.Contains(rawText, "\n# [[auth.providers]]\n# id = 'chatto-hub'\n# type = 'oidc'") {
		t.Fatal("generated config should include a commented OIDC auth provider example")
	}
	if !strings.Contains(rawText, "\n# [[auth.providers]]\n# id = 'github'\n# type = 'github'") {
		t.Fatal("generated config should include a commented GitHub auth provider example")
	}
	if !strings.Contains(rawText, "\ndirect_login = true\n") {
		t.Fatal("generated config should explicitly enable password login")
	}
	if !strings.Contains(rawText, "\n[auth.email_otp]\n") {
		t.Fatal("generated config should include an active auth.email_otp section")
	}
	if strings.Contains(rawText, "\n# [auth.email_otp]\n") {
		t.Fatal("generated config should not comment out the auth.email_otp section")
	}
	if !strings.Contains(rawText, "\nthrottling_enabled = true\n") {
		t.Fatal("generated config should explicitly enable email OTP throttling")
	}
	if !strings.Contains(rawText, "\n# ttl = '30m'\n") {
		t.Fatal("generated config should include commented default email OTP TTL")
	}
	if !strings.Contains(rawText, "\n# max_delivered_codes = 10\n") {
		t.Fatal("generated config should include commented default delivered-code limit")
	}
	if !strings.Contains(rawText, "\n# max_wrong_attempts = 5\n") {
		t.Fatal("generated config should include commented default wrong-attempt limit")
	}
	if !strings.Contains(rawText, "\n# domain = ''") {
		t.Fatal("generated config should comment out webserver.tls.domain by default")
	}
	if !strings.Contains(rawText, "\n# email = ''") {
		t.Fatal("generated config should comment out webserver.tls.email by default")
	}
	if !strings.Contains(rawText, "storage_backend = 'nats'") {
		t.Fatal("generated config should set core.assets.storage_backend to 'nats'")
	}
	if !strings.Contains(rawText, "\n[smtp]\n") {
		t.Fatal("generated config should include SMTP defaults")
	}
	if !strings.Contains(rawText, "\nport = 587\n") {
		t.Fatal("generated SMTP config should default to STARTTLS submission port 587")
	}
	if !strings.Contains(rawText, "\ntls = 'mandatory'\n") {
		t.Fatal("generated SMTP config should default to mandatory STARTTLS")
	}
	if !strings.Contains(rawText, "\nreplicas = 1\n") {
		t.Fatal("generated config should set nats.replicas to 1")
	}
	if strings.Contains(rawText, "\n# replicas =") {
		t.Fatal("generated config should not comment out nats.replicas")
	}
	if !strings.Contains(rawText, "\n# port = 4222") {
		t.Fatal("generated config should comment out nats.embedded.port by default")
	}
	if strings.Contains(rawText, "\nport = 4222") {
		t.Fatal("generated config should not enable the embedded NATS TCP port by default")
	}
	if !strings.Contains(rawText, "\n# sync_interval = 'always'") {
		t.Fatal("generated config should recommend sync_interval = 'always'")
	}
	if !strings.Contains(rawText, "\n# [nats.client]\n") {
		t.Fatal("generated config should include a commented external NATS client example")
	}
	if strings.Contains(rawText, "\n[nats.client]\n") {
		t.Fatal("generated embedded config should not include an active [nats.client] table")
	}
}

func TestInitWithLiveKit(t *testing.T) {
	// Serial: the command flags use package variables.
	originalConfigFile, originalWithLiveKit := initConfigFile, initWithLiveKit
	t.Cleanup(func() {
		initConfigFile, initWithLiveKit = originalConfigFile, originalWithLiveKit
	})

	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%v", enabled), func(t *testing.T) {
			dir := t.TempDir()
			initConfigFile = filepath.Join(dir, "custom.toml")
			if err := initCmd.Flags().Set("with-livekit", strconv.FormatBool(enabled)); err != nil {
				t.Fatal(err)
			}
			if err := initCmd.RunE(initCmd, nil); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.ReadConfig(initConfigFile)
			if err != nil {
				t.Fatal(err)
			}
			liveKitPath := filepath.Join(dir, "livekit.yaml")
			if !enabled {
				if cfg.LiveKit.Enabled {
					t.Fatal("LiveKit must remain disabled without the flag")
				}
				if _, err := os.Stat(liveKitPath); !os.IsNotExist(err) {
					t.Fatalf("unexpected LiveKit config: %v", err)
				}
				return
			}
			if !cfg.LiveKit.IsConfigured() || cfg.LiveKit.URL != "ws://localhost:7880" {
				t.Fatal("generated config must enable local LiveKit")
			}
			if len(cfg.LiveKit.APISecret) < 32 || cfg.LiveKit.APIKey == cfg.LiveKit.APISecret {
				t.Fatal("LiveKit must use separate random credentials")
			}
			raw, err := os.ReadFile(liveKitPath)
			if err != nil {
				t.Fatal(err)
			}
			var liveKit struct {
				Port    int               `yaml:"port"`
				Keys    map[string]string `yaml:"keys"`
				Webhook struct {
					URLs   []string `yaml:"urls"`
					APIKey string   `yaml:"api_key"`
				} `yaml:"webhook"`
			}
			if err := yaml.Unmarshal(raw, &liveKit); err != nil {
				t.Fatal(err)
			}
			if liveKit.Port != 7880 || liveKit.Keys[cfg.LiveKit.APIKey] != cfg.LiveKit.APISecret {
				t.Fatal("LiveKit listener and credentials must match Chatto")
			}
			if liveKit.Webhook.APIKey != cfg.LiveKit.APIKey || len(liveKit.Webhook.URLs) != 1 || liveKit.Webhook.URLs[0] != cfg.LiveKit.WebhookURL {
				t.Fatal("LiveKit webhooks must match Chatto")
			}
			for _, path := range []string{initConfigFile, liveKitPath} {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0600 {
					t.Fatalf("config permissions = %o, want 600", info.Mode().Perm())
				}
			}
		})
	}
}

func TestInitRefusesExistingConfigs(t *testing.T) {
	// Serial: the command flags use package variables.
	originalConfigFile, originalWithLiveKit := initConfigFile, initWithLiveKit
	t.Cleanup(func() {
		initConfigFile, initWithLiveKit = originalConfigFile, originalWithLiveKit
	})
	initWithLiveKit = true
	for _, existing := range []string{"chatto.toml", "livekit.yaml"} {
		t.Run(existing, func(t *testing.T) {
			dir := t.TempDir()
			initConfigFile = filepath.Join(dir, "chatto.toml")
			path := filepath.Join(dir, existing)
			if err := os.WriteFile(path, []byte("keep me"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := initCmd.RunE(initCmd, nil); err == nil {
				t.Fatal("init must refuse existing configs")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "keep me" {
				t.Fatal("init changed the existing config")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatal("init must not leave a partial configuration")
			}
		})
	}
}

func TestInitWithSearch(t *testing.T) {
	// Serial: the command flags use package variables.
	originalConfigFile, originalWithSearch, originalWithLiveKit := initConfigFile, initWithSearch, initWithLiveKit
	t.Cleanup(func() {
		initConfigFile, initWithSearch, initWithLiveKit = originalConfigFile, originalWithSearch, originalWithLiveKit
	})
	for _, search := range []bool{false, true} {
		for _, liveKit := range []bool{false, true} {
			t.Run(fmt.Sprintf("search=%v/livekit=%v", search, liveKit), func(t *testing.T) {
				initConfigFile = filepath.Join(t.TempDir(), "chatto.toml")
				for name, enabled := range map[string]bool{"with-search": search, "with-livekit": liveKit} {
					if err := initCmd.Flags().Set(name, strconv.FormatBool(enabled)); err != nil {
						t.Fatal(err)
					}
				}
				if err := initCmd.RunE(initCmd, nil); err != nil {
					t.Fatal(err)
				}
				cfg, err := config.ReadConfig(initConfigFile)
				if err != nil {
					t.Fatal(err)
				}
				if cfg.Search.Enabled != search || cfg.SearchProvider.Enabled != search {
					t.Fatalf("search and provider enabled = %v, %v; want %v", cfg.Search.Enabled, cfg.SearchProvider.Enabled, search)
				}
				if cfg.LiveKit.IsConfigured() != liveKit {
					t.Fatal("search flag must compose with the LiveKit flag")
				}
				if cfg.SearchProvider.DirectoryOrDefault() != "./data/search" || !slices.Equal(cfg.SearchProvider.LanguagesOrDefault(), config.SupportedSearchProviderLanguages()) {
					t.Fatal("generated config must preserve search storage and language defaults")
				}
				if !cfg.NATS.Embedded.Enabled || len(cfg.Core.SecretKey) != 64 || !cfg.AssetProcessing.Enabled {
					t.Fatal("optional integrations must preserve the base config")
				}
				raw, err := os.ReadFile(initConfigFile)
				if err != nil {
					t.Fatal(err)
				}
				for _, table := range []string{"search", "search_provider"} {
					marker := "\n[" + table + "]\n"
					if !search {
						marker = "\n# [" + table + "]\n"
					}
					if !strings.Contains(string(raw), marker) {
						t.Fatalf("generated config is missing %q", marker)
					}
				}
			})
		}
	}
}
