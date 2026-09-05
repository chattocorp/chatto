package config

import (
	"strings"
	"testing"
)

func TestApplyBootstrapEnvironment(t *testing.T) {
	t.Setenv("CHATTO_BOOTSTRAP_USERS_0_LOGIN", "owner")
	t.Setenv("CHATTO_BOOTSTRAP_USERS_0_DISPLAY_NAME", "Compose Owner")
	t.Setenv("CHATTO_BOOTSTRAP_USERS_0_EMAIL", "owner@example.com")
	t.Setenv("CHATTO_BOOTSTRAP_USERS_0_PASSWORD", "development-password")
	t.Setenv("CHATTO_BOOTSTRAP_USERS_0_SERVER_ROLE", "owner")
	t.Setenv("CHATTO_BOOTSTRAP_BOTS_0_LOGIN", "test_bot")
	t.Setenv("CHATTO_BOOTSTRAP_BOTS_0_DISPLAY_NAME", "TestBot")
	t.Setenv("CHATTO_BOOTSTRAP_BOTS_0_OWNER_LOGIN", "owner")
	t.Setenv("CHATTO_BOOTSTRAP_BOTS_0_API_KEY_NAME", "Local development")
	t.Setenv("CHATTO_BOOTSTRAP_BOTS_0_CREDENTIAL_FILE", "./data/bootstrap/test_bot.key")
	t.Setenv("CHATTO_BOOTSTRAP_BOTS_0_PERMISSIONS", "room.join, message.read")
	t.Setenv("CHATTO_BOOTSTRAP_BOTS_0_ROOMS", "general")
	t.Setenv("CHATTO_BOOTSTRAP_SERVER_NAME", "Compose Server")
	t.Setenv("CHATTO_BOOTSTRAP_SERVER_ROOMS", "announcements, general")

	var cfg ChattoConfig
	if err := applyBootstrapEnv(&cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Bootstrap.Users) != 1 || cfg.Bootstrap.Users[0].Login != "owner" || cfg.Bootstrap.Users[0].ServerRole != "owner" {
		t.Fatalf("bootstrap users = %#v", cfg.Bootstrap.Users)
	}
	if len(cfg.Bootstrap.Bots) != 1 {
		t.Fatalf("bootstrap bots = %#v", cfg.Bootstrap.Bots)
	}
	bot := cfg.Bootstrap.Bots[0]
	if bot.Login != "test_bot" || bot.DisplayName != "TestBot" || bot.OwnerLogin != "owner" || bot.APIKeyName != "Local development" || bot.CredentialFile != "./data/bootstrap/test_bot.key" {
		t.Fatalf("bootstrap bot = %#v", bot)
	}
	if len(bot.Permissions) != 2 || bot.Permissions[0] != "room.join" || bot.Permissions[1] != "message.read" || len(bot.Rooms) != 1 || bot.Rooms[0] != "general" {
		t.Fatalf("bootstrap bot lists = %#v", bot)
	}
	if cfg.Bootstrap.Server == nil || cfg.Bootstrap.Server.Name != "Compose Server" || len(cfg.Bootstrap.Server.Rooms) != 2 {
		t.Fatalf("bootstrap server = %#v", cfg.Bootstrap.Server)
	}
}

func TestBootstrapEnvironmentRequiresContiguousBotIndexes(t *testing.T) {
	t.Setenv("CHATTO_BOOTSTRAP_BOTS_1_LOGIN", "test_bot")

	var cfg ChattoConfig
	err := applyBootstrapEnv(&cfg)
	if err == nil || !strings.Contains(err.Error(), "missing index 0") {
		t.Fatalf("error = %v, want missing-index error", err)
	}
}

func TestBootstrapEnvironmentRejectsUnknownBotFields(t *testing.T) {
	t.Setenv("CHATTO_BOOTSTRAP_BOTS_0_UNKNOWN", "value")

	var cfg ChattoConfig
	err := applyBootstrapEnv(&cfg)
	if err == nil || !strings.Contains(err.Error(), "unknown bootstrap bot field") {
		t.Fatalf("error = %v, want unknown-field error", err)
	}
}

func TestBootstrapEnvironmentRequiresContiguousUserIndexes(t *testing.T) {
	t.Setenv("CHATTO_BOOTSTRAP_USERS_1_LOGIN", "owner")

	var cfg ChattoConfig
	err := applyBootstrapEnv(&cfg)
	if err == nil || !strings.Contains(err.Error(), "missing index 0") {
		t.Fatalf("error = %v, want missing-index error", err)
	}
}

func TestBootstrapEnvironmentRejectsUnknownUserFields(t *testing.T) {
	t.Setenv("CHATTO_BOOTSTRAP_USERS_0_UNKNOWN", "value")

	var cfg ChattoConfig
	err := applyBootstrapEnv(&cfg)
	if err == nil || !strings.Contains(err.Error(), "unknown bootstrap user field") {
		t.Fatalf("error = %v, want unknown-field error", err)
	}
}
