package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Config tests are serial because the loader reads process environment state.
func cleanCheckEnvironment(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "CHATTO_") {
			t.Setenv(name, "")
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func validCheckEnvironment(t *testing.T) {
	t.Helper()
	cleanCheckEnvironment(t)
	t.Setenv("CHATTO_WEBSERVER_PORT", "4000")
	for _, name := range []string{"CHATTO_WEBSERVER_COOKIE_SIGNING_SECRET", "CHATTO_CORE_SECRET_KEY", "CHATTO_CORE_ASSETS_SIGNING_SECRET"} {
		t.Setenv(name, strings.Repeat("ab", 32))
	}
}

func TestCheckConfigDiagnostics(t *testing.T) {
	for _, tc := range []struct{ name, toml, env, want string }{
		{"unknown field", "[auth]\ntoken_tll='private-sentinel'", "", "auth.token_tll"},
		{"unknown empty table", "[unknown]", "", "unknown"},
		{"indexed TOML", "[[auth.providers]]\nclient_secert='private-sentinel'", "", "auth.providers[0].client_secert"},
		{"removed TOML", "[livekit]\nwebhook_url='private-sentinel'", "", "configure webhook.urls in LiveKit"},
		{"removed provider", "[[auth.providers]]\nprovider_options={secret='private-sentinel'}", "", "remove this unused setting"},
		{"alias TOML", "[livekit]\ninstance_id='private-sentinel'", "", "use livekit.server_id"},
		{"bootstrap alias", "[[bootstrap.users]]\ninstance_role='private-sentinel'", "", "bootstrap.users[].server_role"},
		{"unknown ENV", "", "CHATTO_AUTH_TOKEN_TLL", "unknown ENV name"},
		{"empty unknown ENV", "", "CHATTO_UNKNOWN", "CHATTO_UNKNOWN"},
		{"indexed ENV", "", "CHATTO_AUTH_PROVIDERS_0_CLIENT_SECERT", "unknown ENV name"},
		{"invalid index", "", "CHATTO_AUTH_PROVIDERS_no_ID", "indexed fields"},
		{"OIDC alias", "", "CHATTO_AUTH_OIDC_CLIENT_SECRET", "CHATTO_AUTH_PROVIDERS_<index>_<field>"},
		{"removed ENV", "", "CHATTO_LIVEKIT_WEBHOOK_URL", "configure webhook.urls in LiveKit"},
		{"invalid syntax", "password = 'private-sentinel", "", "invalid TOML syntax"},
		{"invalid value", "[auth]\ntoken_ttl='private-sentinel'", "", "values omitted"},
		{"invalid ENV value", "", "CHATTO_AUTH_PROVIDERS_0_REQUEST_EMAIL", "values omitted"},
		{"invalid PII", "[webserver]\ntrusted_proxies=['private-sentinel']", "", "values omitted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validCheckEnvironment(t)
			if tc.env != "" {
				t.Setenv(tc.env, "private-sentinel")
			}
			path := filepath.Join(t.TempDir(), "chatto.toml")
			if err := os.WriteFile(path, []byte(tc.toml), 0600); err != nil {
				t.Fatal(err)
			}
			err := CheckConfig(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			if strings.Contains(err.Error(), "private-sentinel") {
				t.Fatal("diagnostic exposed a value")
			}
		})
	}
}

func TestCheckConfigENVOnlyAndFilePolicy(t *testing.T) {
	validCheckEnvironment(t)
	t.Chdir(t.TempDir())
	if err := CheckConfig(""); err != nil {
		t.Fatal(err)
	}
	if err := CheckConfig("missing.toml"); err == nil {
		t.Fatal("explicit missing file accepted")
	}
	if _, err := ReadConfig("missing.toml"); err != nil {
		t.Fatalf("normal missing-file policy changed: %v", err)
	}
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_ID", "oidc")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_TYPE", "oidc")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_CLIENT_ID", "example")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_ISSUER_URL", "https://id.example.com")
	t.Setenv("CHATTO_AUTH_PROVIDERS_0_REQUEST_EMAIL", "false")
	t.Setenv("CHATTO_WEBSERVER_URL", "https://chat.example.com")
	if err := CheckConfig(""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHATTO_AUTH_PROVIDERS_2_ID", "gap")
	if err := CheckConfig(""); err == nil {
		t.Fatal("provider index gap accepted")
	}
}

func TestCheckConfigPreservesNormalCompatibility(t *testing.T) {
	validCheckEnvironment(t)
	path := filepath.Join(t.TempDir(), "chatto.toml")
	if err := os.WriteFile(path, []byte("[livekit]\ninstance_id='legacy'\nwebhook_url='ignored'\nunknown='ignored'"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHATTO_UNKNOWN", "ignored")
	cfg, err := ReadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LiveKit.ServerID != "legacy" {
		t.Fatal("legacy alias no longer applied")
	}
	if err := CheckConfig(path); err == nil {
		t.Fatal("strict check accepted obsolete names")
	}
	t.Setenv("CHATTO_AUTH_OIDC_ENABLED", "true")
	t.Setenv("CHATTO_AUTH_OIDC_CLIENT_ID", "example")
	t.Setenv("CHATTO_AUTH_OIDC_ISSUER_URL", "https://id.example.com")
	t.Setenv("CHATTO_WEBSERVER_URL", "https://chat.example.com")
	cfg, err = ReadConfig(path)
	if err != nil || len(cfg.Auth.Providers) != 1 {
		t.Fatalf("legacy OIDC no longer works: %v", err)
	}
}

func TestEmailOTPDefaultAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, toml, env string
		want            time.Duration
	}{
		{"omitted", "", "", 30 * time.Minute},
		{"explicit", "[auth.email_otp]\nttl='12m'", "", 12 * time.Minute},
		{"ENV only", "", "8m", 8 * time.Minute},
		{"ENV override", "[auth.email_otp]\nttl='12m'", "8m", 8 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validCheckEnvironment(t)
			path := filepath.Join(t.TempDir(), "chatto.toml")
			if err := os.WriteFile(path, []byte(tc.toml), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.env != "" {
				t.Setenv("CHATTO_AUTH_EMAIL_OTP_TTL", tc.env)
			}
			cfg, err := ReadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.Auth.EmailOTP.TTLOrDefault(); got != tc.want {
				t.Fatalf("TTL = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestCheckEnvNamesCoverage(t *testing.T) {
	names := []string{"PATH=ignored", "CHATTO_AUTH_PROVIDERS_0_TOKEN_ENDPOINT_AUTH_METHOD=none", "CHATTO_AUTH_PROVIDERS_0_SCOPES=openid", "CHATTO_AUTH_PROVIDERS_0_AUTO_PROVISION=false", "CHATTO_BOOTSTRAP_USERS_0_SERVER_ROLE=admin", "CHATTO_BOOTSTRAP_BOTS_0_OUTBOUND_WEBHOOK_URL=https://example.com", "CHATTO_BOOTSTRAP_SERVER_NAME=example"}
	if got := checkEnvNames(names); len(got) != 0 {
		t.Fatal(got)
	}
}
