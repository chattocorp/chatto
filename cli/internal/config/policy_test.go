package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountsConfig_Defaults(t *testing.T) {
	c := &AccountsConfig{}
	if got := c.MaxUsersOrDefault(); got != -1 {
		t.Errorf("MaxUsersOrDefault() with unset = %d, want -1", got)
	}

	zero := 0
	c = &AccountsConfig{MaxUsers: &zero}
	if got := c.MaxUsersOrDefault(); got != 0 {
		t.Errorf("MaxUsersOrDefault() with explicit 0 = %d, want 0", got)
	}

	c = &AccountsConfig{MaxUsers: new(100)}
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
url = "http://localhost:4000"
port = 4000
cookie_signing_secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

[core]
secret_key = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

[core.assets]
signing_secret = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

[core.calls]
max_participants = 8

[core.accounts]
max_users = -1
max_login_length = 90
`
	if err := os.WriteFile(filepath.Join(tmpDir, "chatto.toml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if cfg.Core.Calls.MaxParticipantsOrDefault() != 8 {
		t.Fatal("call limit from TOML")
	}
	if got := cfg.Core.Accounts.MaxLoginLengthOrDefault(); got != 90 {
		t.Fatalf("TOML login limit = %d, want 90", got)
	}
	if got := cfg.Core.Accounts.MaxUsersOrDefault(); got != -1 {
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
	t.Setenv("CHATTO_WEBSERVER_URL", "http://localhost:4000")
	t.Setenv("CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("CHATTO_CORE_SECRET_KEY", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	t.Setenv("CHATTO_CORE_ASSETS_SIGNING_SECRET", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	t.Setenv("CHATTO_CORE_CALLS_MAX_PARTICIPANTS", "0")
	t.Setenv("CHATTO_CORE_ACCOUNTS_MAX_USERS", "0")
	t.Setenv("CHATTO_CORE_ACCOUNTS_MAX_LOGIN_LENGTH", "12")

	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatalf("ReadConfig() failed: %v", err)
	}
	if cfg.Core.Calls.MaxParticipantsOrDefault() != 0 {
		t.Fatal("call limit from environment")
	}
	if got := cfg.Core.Accounts.MaxLoginLengthOrDefault(); got != 12 {
		t.Fatalf("ENV login limit = %d, want 12", got)
	}
	if got := cfg.Core.Accounts.MaxUsersOrDefault(); got != 0 {
		t.Errorf("MaxUsers from env (explicit 0) = %d, want 0", got)
	}
}

func TestChattoConfig_Validate_Limits(t *testing.T) {
	base := func() ChattoConfig {
		return ChattoConfig{
			Webserver: WebserverConfig{URL: "http://localhost:4000", Port: 4000, CookieSigningSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			Core:      CoreConfig{SecretKey: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", Assets: AssetsConfig{SigningSecret: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"}},
		}
	}

	t.Run("call limit", func(t *testing.T) {
		c := base()
		if c.Core.Calls.MaxParticipantsOrDefault() != -1 {
			t.Fatal("expected unlimited default")
		}
		c.Core.Calls.MaxParticipants = new(-2)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "core.calls.max_participants") {
			t.Fatalf("invalid call limit: %v", err)
		}
	})
	t.Run("login length", func(t *testing.T) {
		c := base()
		if got := c.Core.Accounts.MaxLoginLengthOrDefault(); got != 64 {
			t.Fatalf("default login limit = %d, want 64", got)
		}
		for _, value := range []int{-1, 0, 1, 2, 64, 254, 255} {
			c.Core.Accounts.MaxLoginLength = &value
			if err := c.Validate(); (err == nil) != (value >= 2 && value <= 254) {
				t.Errorf("limit %d: %v", value, err)
			}
		}
	})

	t.Run("rejects max_users below -1", func(t *testing.T) {
		c := base()
		c.Core.Accounts.MaxUsers = new(-5)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "core.accounts.max_users") {
			t.Errorf("expected core.accounts.max_users validation error, got %v", err)
		}
	})

	t.Run("accepts -1, 0, positive", func(t *testing.T) {
		for _, v := range []int{-1, 0, 1, 100} {
			c := base()
			c.Core.Accounts.MaxUsers = new(v)
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

func TestMaxUsersConfigCompatibility(t *testing.T) {
	const oldTOML = "[limits]\nmax_users=3\n"
	const newTOML = "[core.accounts]\nmax_users=4\n"
	for _, tc := range []struct {
		name, file, legacyEnv, canonicalEnv string
		want                                int
	}{
		{"old file", oldTOML, "", "", 3},
		{"old environment", "", "0", "", 0},
		{"both tables", oldTOML + newTOML, "", "", 4},
		{"old environment overrides new file", newTOML, "0", "", 0},
		{"new environment overrides old file", oldTOML, "", "6", 6},
		{"both environments", oldTOML + newTOML, "0", "6", 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validCheckEnvironment(t)
			if tc.legacyEnv != "" {
				t.Setenv("CHATTO_LIMITS_MAX_USERS", tc.legacyEnv)
			}
			if tc.canonicalEnv != "" {
				t.Setenv("CHATTO_CORE_ACCOUNTS_MAX_USERS", tc.canonicalEnv)
			}
			path := filepath.Join(t.TempDir(), "chatto.toml")
			if err := os.WriteFile(path, []byte(tc.file), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := ReadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.Core.Accounts.MaxUsersOrDefault(); got != tc.want {
				t.Errorf("max_users = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestOwnersDIDValidation(t *testing.T) {
	for _, value := range []string{"did:plc:abcdefghijklmnopqrstuvwx", "did:web:example.com", "did:web:example.com:user:alice", "did:web:example.com%3A8443"} {
		cfg := OwnersConfig{DIDs: []string{value}}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("valid DID rejected: %v", err)
		}
		if !cfg.IsServerOwnerDID(value) {
			t.Fatal("DID did not match")
		}
	}
	for _, value := range []string{"alice.example.com", "did:plc:", "did:plc:short", "did:web:", "did:key:abc", " did:web:example.com", "did:web:example.com/path", "did:web:example.com?query"} {
		cfg := OwnersConfig{DIDs: []string{value}}
		if err := cfg.Validate(); err == nil {
			t.Fatal("invalid DID accepted")
		} else if strings.Contains(err.Error(), value) {
			t.Fatal("error disclosed DID")
		}
	}
}

func TestCheckConfigOwnerDIDs(t *testing.T) {
	validCheckEnvironment(t)
	t.Setenv("CHATTO_OWNERS_DIDS", "did:web:owner.example")
	if err := CheckConfig(""); err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Owners.IsServerOwnerDID("did:web:owner.example") {
		t.Fatal("ENV did not load")
	}
	t.Setenv("CHATTO_OWNERS_DIDS", "private-sentinel")
	if err := CheckConfig(""); err == nil || strings.Contains(err.Error(), "private-sentinel") {
		t.Fatalf("unsafe validation result: %v", err)
	}
}
