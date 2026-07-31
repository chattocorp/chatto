package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadEmbeddedConfigWithEnvironmentOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authling.toml")
	if err := os.WriteFile(path, []byte(`
[nats]
replicas = 1

[nats.embedded]
enabled = true
data_dir = "/var/lib/authling"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	wantDataDir := filepath.Join(t.TempDir(), "nats")
	t.Setenv("AUTHLING_NATS_EMBEDDED_DATA_DIR", wantDataDir)

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !cfg.NATS.Embedded.Enabled {
		t.Fatal("embedded NATS is disabled, want enabled")
	}
	if cfg.NATS.Embedded.DataDir != wantDataDir {
		t.Fatalf("embedded NATS data directory = %q, want %q", cfg.NATS.Embedded.DataDir, wantDataDir)
	}
}

func TestReadRejectsUnknownTOMLFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authling.toml")
	if err := os.WriteFile(path, []byte(`
[nats.embedded]
enabled = true
data_dir = ".authling/nats"
surprise = true
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "strict mode") {
		t.Fatalf("read config error = %v, want unknown-field error", err)
	}
}

func TestReadRequiresOneNATSMode(t *testing.T) {
	t.Chdir(t.TempDir())

	if _, err := Read(""); err == nil || !strings.Contains(err.Error(), "enable nats.embedded") {
		t.Fatalf("read config error = %v, want missing NATS mode", err)
	}
}

func TestReadRequiresExplicitConfigFileToExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")

	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "read "+path) {
		t.Fatalf("read config error = %v, want missing explicit file error", err)
	}
}

func TestValidateRejectsCredentialsInNATSURL(t *testing.T) {
	cfg := Config{
		NATS: NATSConfig{
			Client: NATSClientConfig{
				URL:             "nats://user:password@nats.example:4222",
				CredentialsFile: "/run/secrets/authling.creds",
			},
		},
	}

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "without credentials") {
		t.Fatalf("validate config error = %v, want embedded-credential error", err)
	}
}

func TestDevelopmentConfigIsValid(t *testing.T) {
	cfg, err := Read(filepath.Join("..", "..", "authling.toml"))
	if err != nil {
		t.Fatalf("read development config: %v", err)
	}
	if !cfg.NATS.Embedded.Enabled {
		t.Fatal("development config does not enable embedded NATS")
	}
}
