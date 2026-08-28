package appconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hmans.de/chatto/pkg/appconfig"
)

type testConfig struct {
	Name string `toml:"name" env:"APPCONFIG_TEST_NAME"`
	Port int    `toml:"port" env:"APPCONFIG_TEST_PORT"`
}

func TestLoadAppliesEnvironmentAfterTOML(t *testing.T) {
	path := writeConfig(t, "name = \"from-toml\"\nport = 4222\n")
	t.Setenv("APPCONFIG_TEST_NAME", "from-environment")

	config, err := appconfig.Load[testConfig](appconfig.Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "from-environment" {
		t.Fatalf("name = %q, want environment override", config.Name)
	}
	if config.Port != 4222 {
		t.Fatalf("port = %d, want TOML value", config.Port)
	}
}

func TestLoadUsesDefaultPath(t *testing.T) {
	path := writeConfig(t, "name = \"default-file\"\n")

	config, err := appconfig.Load[testConfig](appconfig.Options{DefaultPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "default-file" {
		t.Fatalf("name = %q, want default-file", config.Name)
	}
}

func TestLoadExplicitPathTakesPrecedenceOverDefaultPath(t *testing.T) {
	explicitPath := writeConfig(t, "name = \"explicit-file\"\n")
	defaultPath := writeConfig(t, "name = \"default-file\"\n")

	config, err := appconfig.Load[testConfig](appconfig.Options{
		Path:        explicitPath,
		DefaultPath: defaultPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "explicit-file" {
		t.Fatalf("name = %q, want explicit-file", config.Name)
	}
}

func TestLoadAllowsMissingDefaultFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")
	t.Setenv("APPCONFIG_TEST_NAME", "environment-only")

	config, err := appconfig.Load[testConfig](appconfig.Options{DefaultPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "environment-only" {
		t.Fatalf("name = %q, want environment-only", config.Name)
	}
}

func TestLoadExplicitMissingFilePolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")

	_, err := appconfig.Load[testConfig](appconfig.Options{
		Path:                path,
		RequireExplicitFile: true,
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("required explicit file error = %v, want os.ErrNotExist", err)
	}

	config, err := appconfig.Load[testConfig](appconfig.Options{Path: path})
	if err != nil {
		t.Fatalf("optional explicit file: %v", err)
	}
	if config != (testConfig{}) {
		t.Fatalf("config = %#v, want zero value", config)
	}
}

func TestLoadUnknownFieldPolicy(t *testing.T) {
	path := writeConfig(t, "name = \"known\"\nsurprise = true\n")

	_, err := appconfig.Load[testConfig](appconfig.Options{
		Path:                  path,
		DisallowUnknownFields: true,
	})
	if err == nil || !strings.Contains(err.Error(), "strict mode") {
		t.Fatalf("strict error = %v, want unknown-field error", err)
	}

	config, err := appconfig.Load[testConfig](appconfig.Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "known" {
		t.Fatalf("name = %q, want known", config.Name)
	}
}

func TestLoadReturnsZeroValueAfterDecodeFailure(t *testing.T) {
	path := writeConfig(t, "name = \"partially-decoded\"\nport = \"not-an-integer\"\n")

	config, err := appconfig.Load[testConfig](appconfig.Options{Path: path})
	if err == nil || !strings.Contains(err.Error(), "decode "+path) {
		t.Fatalf("decode error = %v", err)
	}
	if config != (testConfig{}) {
		t.Fatalf("config = %#v, want zero value", config)
	}
}

func TestLoadReturnsZeroValueAfterEnvironmentFailure(t *testing.T) {
	path := writeConfig(t, "name = \"partially-decoded\"\nport = 4222\n")
	t.Setenv("APPCONFIG_TEST_PORT", "not-an-integer")

	config, err := appconfig.Load[testConfig](appconfig.Options{Path: path})
	if err == nil || !strings.Contains(err.Error(), "parse environment") {
		t.Fatalf("environment error = %v", err)
	}
	if config != (testConfig{}) {
		t.Fatalf("config = %#v, want zero value", config)
	}
}

func TestLoadWithoutPathsUsesEnvironmentOnly(t *testing.T) {
	t.Setenv("APPCONFIG_TEST_NAME", "environment-only")

	config, err := appconfig.Load[testConfig](appconfig.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "environment-only" {
		t.Fatalf("name = %q, want environment-only", config.Name)
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
