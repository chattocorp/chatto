package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLimitsConfig_Defaults(t *testing.T) {
	c := &LimitsConfig{}
	if got := c.MaxUsersOrDefault(); got != -1 {
		t.Errorf("MaxUsersOrDefault() with unset = %d, want -1", got)
	}

	zero := 0
	c = &LimitsConfig{MaxUsers: &zero}
	if got := c.MaxUsersOrDefault(); got != 0 {
		t.Errorf("MaxUsersOrDefault() with explicit 0 = %d, want 0", got)
	}

	c = &LimitsConfig{MaxUsers: intPtr(100)}
	if got := c.MaxUsersOrDefault(); got != 100 {
		t.Errorf("MaxUsersOrDefault() with 100 = %d, want 100", got)
	}
}

func TestReadConfig_LimitsFromTOML(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	configContent := `
[webserver]
port = 4000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

[limits]
max_users = -1
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if got := cfg.Limits.MaxUsersOrDefault(); got != -1 {
		t.Errorf("MaxUsers from TOML = %d, want -1", got)
	}
}

func TestReadConfig_LimitsFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_LIMITS_MAX_USERS", "0")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if got := cfg.Limits.MaxUsersOrDefault(); got != 0 {
		t.Errorf("MaxUsers from env (explicit 0) = %d, want 0", got)
	}
}

func TestChattoConfig_Validate_Limits(t *testing.T) {
	base := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{Port: 4000, CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			Core:      CoreConfig{SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", Assets: AssetsConfig{SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"}},
		}
	}

	t.Run("rejects max_users below -1", func(t *testing.T) {
		c := base()
		c.Limits.MaxUsers = intPtr(-5)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "limits.max_users") {
			t.Errorf("expected limits.max_users validation error, got %v", err)
		}
	})

	t.Run("accepts -1, 0, positive", func(t *testing.T) {
		for _, v := range []int{-1, 0, 1, 100} {
			c := base()
			c.Limits.MaxUsers = intPtr(v)
			if err := c.Validate(); err != nil {
				t.Errorf("validate failed for %d: %v", v, err)
			}
		}
	})
}

func TestOwnersConfig_IsServerOwnerEmail(t *testing.T) {
	cfg := &OwnersConfig{Emails: []string{"Owner@Example.com", "  ops@example.com  "}}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"exact match", "Owner@Example.com", true},
		{"different case in user input", "owner@example.com", true},
		{"different case in config", "OWNER@EXAMPLE.COM", true},
		{"surrounding whitespace tolerated on input", "  owner@example.com  ", true},
		{"surrounding whitespace tolerated in config", "ops@example.com", true},
		{"non-owner email", "other@example.com", false},
		{"empty string", "", false},
		{"substring is not enough", "owner@example.co", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.IsServerOwnerEmail(tt.input); got != tt.want {
				t.Errorf("IsServerOwnerEmail(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
