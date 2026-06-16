package cmd

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	initCmd.Run(initCmd, nil)

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
	if cfg.SMTP.Enabled {
		t.Fatal("generated SMTP config should be disabled by default")
	}
	if cfg.SMTP.Port != 587 {
		t.Fatalf("generated SMTP port = %d, want 587", cfg.SMTP.Port)
	}
	if cfg.SMTP.TLS != config.SMTPTLSMandatory {
		t.Fatalf("generated SMTP TLS policy = %q, want %q", cfg.SMTP.TLS, config.SMTPTLSMandatory)
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
	if !strings.Contains(rawText, "allowed_origins = ['*']") {
		t.Fatal("generated config should explicitly allow bearer-token CORS clients")
	}
	if !strings.Contains(rawText, "oauth_redirect_origins = []") {
		t.Fatal("generated config should not allow additional OAuth redirect origins by default")
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
	if !strings.Contains(rawText, "\n# [nats.client]\n") {
		t.Fatal("generated config should include a commented external NATS client example")
	}
	if strings.Contains(rawText, "\n[nats.client]\n") {
		t.Fatal("generated embedded config should not include an active [nats.client] table")
	}
}
