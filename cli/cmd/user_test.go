package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveManagementSocketPrecedence(t *testing.T) {
	t.Setenv("CHATTO_MANAGEMENT_SOCKET", "/env/admin.sock")
	t.Setenv("CHATTO_MANAGEMENT_SOCKET_PATH", "/env-path/admin.sock")

	got, err := resolveManagementSocket("/flag/admin.sock", "")
	if err != nil {
		t.Fatalf("resolve with flag: %v", err)
	}
	if got != "/flag/admin.sock" {
		t.Fatalf("flag socket = %q, want /flag/admin.sock", got)
	}

	got, err = resolveManagementSocket("", "")
	if err != nil {
		t.Fatalf("resolve with env: %v", err)
	}
	if got != "/env/admin.sock" {
		t.Fatalf("env socket = %q, want /env/admin.sock", got)
	}
}

func TestResolveManagementSocketPathEnvAnchorsRelativePath(t *testing.T) {
	t.Setenv("CHATTO_MANAGEMENT_SOCKET", "")
	t.Setenv("CHATTO_MANAGEMENT_SOCKET_PATH", ".chatto/admin.sock")

	configPath := filepath.Join(t.TempDir(), "chatto.toml")
	got, err := resolveManagementSocket("", configPath)
	if err != nil {
		t.Fatalf("resolve with socket path env: %v", err)
	}
	want := filepath.Join(filepath.Dir(configPath), ".chatto", "admin.sock")
	if got != want {
		t.Fatalf("env socket path = %q, want %q", got, want)
	}
}

func TestResolveManagementSocketFromConfig(t *testing.T) {
	t.Setenv("CHATTO_MANAGEMENT_SOCKET", "")
	t.Setenv("CHATTO_MANAGEMENT_SOCKET_PATH", "")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "chatto.toml")
	if err := os.WriteFile(configPath, []byte(`
[webserver]
port = 4000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[management]
socket_path = "/configured/admin.sock"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := resolveManagementSocket("", configPath)
	if err != nil {
		t.Fatalf("resolve from config: %v", err)
	}
	if got != "/configured/admin.sock" {
		t.Fatalf("config socket = %q, want /configured/admin.sock", got)
	}
}

func TestResolveManagementSocketFromConfigAnchorsRelativePath(t *testing.T) {
	t.Setenv("CHATTO_MANAGEMENT_SOCKET", "")
	t.Setenv("CHATTO_MANAGEMENT_SOCKET_PATH", "")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config", "chatto.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`
	[webserver]
	port = 4000
	cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	[management]
	socket_path = ".chatto/admin.sock"

	[core]
	secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	[core.assets]
	signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := resolveManagementSocket("", configPath)
	if err != nil {
		t.Fatalf("resolve from config: %v", err)
	}
	want := filepath.Join(filepath.Dir(configPath), ".chatto", "admin.sock")
	if got != want {
		t.Fatalf("config socket = %q, want %q", got, want)
	}
}

func TestUserSelector(t *testing.T) {
	if got := userSelector("alice", false).GetLogin(); got != "alice" {
		t.Fatalf("login selector = %q, want alice", got)
	}
	if got := userSelector("U123", true).GetUserId(); got != "U123" {
		t.Fatalf("user ID selector = %q, want U123", got)
	}
}
