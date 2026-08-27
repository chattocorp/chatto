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
	t.Setenv("CHATTO_BOOTSTRAP_SERVER_NAME", "Compose Server")
	t.Setenv("CHATTO_BOOTSTRAP_SERVER_ROOMS", "announcements, general")

	var cfg ChattoConfig
	if err := applyBootstrapEnv(&cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Bootstrap.Users) != 1 || cfg.Bootstrap.Users[0].Login != "owner" || cfg.Bootstrap.Users[0].ServerRole != "owner" {
		t.Fatalf("bootstrap users = %#v", cfg.Bootstrap.Users)
	}
	if cfg.Bootstrap.Server == nil || cfg.Bootstrap.Server.Name != "Compose Server" || len(cfg.Bootstrap.Server.Rooms) != 2 {
		t.Fatalf("bootstrap server = %#v", cfg.Bootstrap.Server)
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
