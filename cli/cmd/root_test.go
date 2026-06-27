package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"hmans.de/chatto/internal/config"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestRootHelpShowsBannerAndNoResetCommand(t *testing.T) {
	originalVersion := Version
	t.Cleanup(func() {
		SetVersion(originalVersion)
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
	})

	SetVersion("9.8.7-test")

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)

	if err := rootCmd.Help(); err != nil {
		t.Fatalf("render root help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"Chatto is a self-hostable chat server for teams and communities.",
		"Version: 9.8.7-test | Self-hosting docs: https://docs.chatto.run",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}

	if strings.Contains(help, "\n  reset ") {
		t.Fatalf("root help should not list reset command:\n%s", help)
	}
}

func TestRootRegistersExporterCommand(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"exporter", "--help"})
	if err != nil {
		t.Fatalf("find exporter command: %v", err)
	}
	if cmd == nil || cmd.Use != "exporter" {
		t.Fatalf("root command did not register exporter, got %#v", cmd)
	}
}

func TestRootRegistersAdminUserCommands(t *testing.T) {
	for _, args := range [][]string{
		{"admin", "user", "create", "--help"},
		{"admin", "user", "set-password", "--help"},
		{"admin", "user", "role", "add", "--help"},
	} {
		cmd, _, err := rootCmd.Find(args)
		if err != nil {
			t.Fatalf("find %v: %v", args, err)
		}
		if cmd == nil {
			t.Fatalf("root command did not register %v", args)
		}
	}
}

func TestConnectBaseURL(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "https://chat.example", want: "https://chat.example/api/connect"},
		{raw: "https://chat.example/api/connect", want: "https://chat.example/api/connect"},
		{raw: "https://chat.example/base/", want: "https://chat.example/base/api/connect"},
		{raw: "http://localhost:4000", want: "http://localhost:4000/api/connect"},
		{raw: "http://127.0.0.1:4000", want: "http://127.0.0.1:4000/api/connect"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := connectBaseURL(tt.raw)
			if err != nil {
				t.Fatalf("connectBaseURL(): %v", err)
			}
			if got != tt.want {
				t.Fatalf("connectBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConnectBaseURLRejectsPlainHTTPForNonLoopbackHosts(t *testing.T) {
	if _, err := connectBaseURL("http://chat.example"); err == nil || !strings.Contains(err.Error(), "must use https") {
		t.Fatalf("connectBaseURL() error = %v, want https requirement", err)
	}
}

func TestResolveAdminAPIClientConfigRefusesConfigTokenForOverriddenURL(t *testing.T) {
	resetAdminGlobals(t)
	adminConfigFile = writeAdminTestConfig(t, "https://safe.example")
	adminAPIURL = "https://evil.example"

	_, err := resolveAdminAPIClientConfig()
	if err == nil || !strings.Contains(err.Error(), "refusing to send admin_api.tokens from config") {
		t.Fatalf("resolveAdminAPIClientConfig() error = %v, want config-token refusal", err)
	}
}

func TestResolveAdminAPIClientConfigAllowsExplicitTokenForOverriddenURL(t *testing.T) {
	resetAdminGlobals(t)
	adminConfigFile = writeAdminTestConfig(t, "https://safe.example")
	adminAPIURL = "https://ops.example"
	adminAPIToken = "explicit-token"

	got, err := resolveAdminAPIClientConfig()
	if err != nil {
		t.Fatalf("resolveAdminAPIClientConfig(): %v", err)
	}
	if got.connectBaseURL != "https://ops.example/api/connect" {
		t.Fatalf("connectBaseURL = %q, want overridden URL", got.connectBaseURL)
	}
	if got.token != "explicit-token" {
		t.Fatalf("token = %q, want explicit token", got.token)
	}
}

func TestResolveAdminAPIClientConfigUsesDedicatedListenerURL(t *testing.T) {
	resetAdminGlobals(t)
	adminConfigFile = writeAdminListenerTestConfig(t, "https://public.example")

	got, err := resolveAdminAPIClientConfig()
	if err != nil {
		t.Fatalf("resolveAdminAPIClientConfig(): %v", err)
	}
	if got.connectBaseURL != "http://127.0.0.1:4021/api/connect" {
		t.Fatalf("connectBaseURL = %q, want dedicated listener URL", got.connectBaseURL)
	}
	if got.token != "config-token-value" {
		t.Fatalf("token = %q, want config token", got.token)
	}
}

func TestResolveAdminAPIClientConfigUsesDedicatedListenerEnv(t *testing.T) {
	resetAdminGlobals(t)
	adminConfigFile = writeAdminTestConfig(t, "https://safe.example")
	t.Setenv("CHATTO_WEBSERVER_URL", "https://public-env.example")
	t.Setenv("CHATTO_ADMIN_API_LISTENER_ENABLED", "true")
	t.Setenv("CHATTO_ADMIN_API_LISTENER_BIND_ADDRESS", "0.0.0.0")
	t.Setenv("CHATTO_ADMIN_API_LISTENER_PORT", "4123")

	got, err := resolveAdminAPIClientConfig()
	if err != nil {
		t.Fatalf("resolveAdminAPIClientConfig(): %v", err)
	}
	if got.connectBaseURL != "http://127.0.0.1:4123/api/connect" {
		t.Fatalf("connectBaseURL = %q, want dedicated listener URL from env", got.connectBaseURL)
	}
	if got.token != "config-token-value" {
		t.Fatalf("token = %q, want config token", got.token)
	}
}

func TestResolveAdminAPIClientConfigReadsAdminTokenFile(t *testing.T) {
	resetAdminGlobals(t)
	adminConfigFile = writeAdminTestConfig(t, "https://safe.example")
	adminAPIURL = "https://ops.example"
	adminAPITokenFile = t.TempDir() + "/admin-token"
	if err := os.WriteFile(adminAPITokenFile, []byte("file-token-value\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	t.Setenv("CHATTO_ADMIN_API_TOKEN", "env-token-value")

	got, err := resolveAdminAPIClientConfig()
	if err != nil {
		t.Fatalf("resolveAdminAPIClientConfig(): %v", err)
	}
	if got.token != "file-token-value" {
		t.Fatalf("token = %q, want file token", got.token)
	}
}

func TestResolveAdminAPIClientConfigRejectsAmbiguousAdminTokenSources(t *testing.T) {
	resetAdminGlobals(t)
	adminAPIToken = "flag-token"
	adminAPITokenFile = "token-file"

	_, err := resolveAdminAPIClientConfig()
	if err == nil || !strings.Contains(err.Error(), "provide only one of --admin-token, --admin-token-file") {
		t.Fatalf("resolveAdminAPIClientConfig() error = %v, want ambiguous token error", err)
	}
}

func TestResolveAdminAPIClientConfigAllowsEnvTokenEntriesForEnvURL(t *testing.T) {
	resetAdminGlobals(t)
	adminConfigFile = writeAdminTestConfig(t, "https://safe.example")
	t.Setenv("CHATTO_WEBSERVER_URL", "https://ops.example")
	t.Setenv("CHATTO_ADMIN_API_TOKENS_0_NAME", "env-token")
	t.Setenv("CHATTO_ADMIN_API_TOKENS_0_TOKEN", "env-token-value")
	t.Setenv("CHATTO_ADMIN_API_TOKENS_0_ALLOWED_CIDRS", "127.0.0.1/32")

	got, err := resolveAdminAPIClientConfig()
	if err != nil {
		t.Fatalf("resolveAdminAPIClientConfig(): %v", err)
	}
	if got.connectBaseURL != "https://ops.example/api/connect" {
		t.Fatalf("connectBaseURL = %q, want env URL", got.connectBaseURL)
	}
	if got.token != "env-token-value" {
		t.Fatalf("token = %q, want counted env token", got.token)
	}
}

func TestResolveAdminAPIClientConfigRefusesEnvTokenEntriesForOverriddenURL(t *testing.T) {
	resetAdminGlobals(t)
	adminConfigFile = writeAdminTestConfig(t, "https://safe.example")
	adminAPIURL = "https://evil.example"
	t.Setenv("CHATTO_ADMIN_API_TOKENS_0_NAME", "env-token")
	t.Setenv("CHATTO_ADMIN_API_TOKENS_0_TOKEN", "env-token-value")
	t.Setenv("CHATTO_ADMIN_API_TOKENS_0_ALLOWED_CIDRS", "127.0.0.1/32")

	_, err := resolveAdminAPIClientConfig()
	if err == nil || !strings.Contains(err.Error(), "refusing to send admin_api.tokens from config/env") {
		t.Fatalf("resolveAdminAPIClientConfig() error = %v, want env-token URL refusal", err)
	}
}

func TestSelectAdminAPIConfigToken(t *testing.T) {
	tokens := []config.AdminAPITokenConfig{
		{Name: "local-cli", Token: "local-secret"},
		{Name: "ops-sidecar", Token: "ops-secret"},
	}

	got, err := selectAdminAPIConfigToken(tokens, "")
	if err != nil {
		t.Fatalf("select default token: %v", err)
	}
	if got != "local-secret" {
		t.Fatalf("default token = %q, want local-secret", got)
	}

	got, err = selectAdminAPIConfigToken(tokens, "ops-sidecar")
	if err != nil {
		t.Fatalf("select named token: %v", err)
	}
	if got != "ops-secret" {
		t.Fatalf("named token = %q, want ops-secret", got)
	}

	_, err = selectAdminAPIConfigToken(tokens, "missing")
	if err == nil || !strings.Contains(err.Error(), `admin API token named "missing" not found`) {
		t.Fatalf("missing token err = %v", err)
	}
}

func TestAdminOutputUsesProvidedWriter(t *testing.T) {
	originalJSON := adminOutputJSON
	t.Cleanup(func() { adminOutputJSON = originalJSON })

	user := &apiv1.AdminUser{
		UserId:      "Uwriter",
		Login:       "writer",
		DisplayName: "Writer User",
		RoleNames:   []string{"admin"},
		VerifiedEmails: []*apiv1.AdminVerifiedEmail{
			{Email: "writer@example.com"},
		},
	}

	adminOutputJSON = false
	var humanOut bytes.Buffer
	if err := printAdminOutput(&humanOut, &apiv1.GetAdminUserResponse{User: user}, func() {
		printAdminUserLine(&humanOut, user)
	}); err != nil {
		t.Fatalf("printAdminOutput human: %v", err)
	}
	if got := humanOut.String(); !strings.Contains(got, "Uwriter\twriter\tWriter User\troles=admin\temails=writer@example.com") {
		t.Fatalf("human output = %q", got)
	}

	adminOutputJSON = true
	var jsonOut bytes.Buffer
	if err := printAdminOutput(&jsonOut, &apiv1.GetAdminUserResponse{User: user}, func() {
		t.Fatal("human callback should not run for JSON output")
	}); err != nil {
		t.Fatalf("printAdminOutput JSON: %v", err)
	}
	if got := jsonOut.String(); !strings.Contains(got, `"userId"`) || !strings.Contains(got, `"Uwriter"`) {
		t.Fatalf("JSON output = %q", got)
	}
}

func TestAdminSecretReaders(t *testing.T) {
	path := t.TempDir() + "/secret"
	if err := os.WriteFile(path, []byte("secret value\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if got, err := readSecretFile(path); err != nil || got != "secret value" {
		t.Fatalf("readSecretFile() = %q, %v; want secret value, nil", got, err)
	}

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { os.Stdin = oldStdin })
	os.Stdin = r
	if _, err := w.WriteString("stdin secret\n"); err != nil {
		t.Fatalf("write stdin pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	if got, err := readSecretStdin(); err != nil || got != "stdin secret" {
		t.Fatalf("readSecretStdin() = %q, %v; want stdin secret, nil", got, err)
	}
}

func resetAdminGlobals(t *testing.T) {
	t.Helper()
	oldConfigFile := adminConfigFile
	oldAPIURL := adminAPIURL
	oldToken := adminAPIToken
	oldTokenFile := adminAPITokenFile
	oldTokenName := adminAPITokenName
	oldEnv := make(map[string]*string)
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if name == "CHATTO_WEBSERVER_URL" || name == "CHATTO_ADMIN_API_TOKEN" || name == "CHATTO_ADMIN_API_TOKEN_NAME" || strings.HasPrefix(name, "CHATTO_ADMIN_API_TOKENS_") || strings.HasPrefix(name, "CHATTO_ADMIN_API_LISTENER_") {
			value := os.Getenv(name)
			oldEnv[name] = &value
			if err := os.Unsetenv(name); err != nil {
				t.Fatalf("unset %s: %v", name, err)
			}
		}
	}
	t.Cleanup(func() {
		adminConfigFile = oldConfigFile
		adminAPIURL = oldAPIURL
		adminAPIToken = oldToken
		adminAPITokenFile = oldTokenFile
		adminAPITokenName = oldTokenName
		for name, value := range oldEnv {
			if value == nil {
				_ = os.Unsetenv(name)
			} else {
				_ = os.Setenv(name, *value)
			}
		}
	})
	adminConfigFile = ""
	adminAPIURL = ""
	adminAPIToken = ""
	adminAPITokenFile = ""
	adminAPITokenName = ""
}

func writeAdminTestConfig(t *testing.T, webserverURL string) string {
	t.Helper()
	path := t.TempDir() + "/chatto.toml"
	body := `[webserver]
url = "` + webserverURL + `"

[admin_api]
enabled = true

[[admin_api.tokens]]
name = "local-cli"
token = "config-token-value"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func writeAdminListenerTestConfig(t *testing.T, webserverURL string) string {
	t.Helper()
	path := t.TempDir() + "/chatto.toml"
	body := `[webserver]
url = "` + webserverURL + `"

[admin_api]
enabled = true

[admin_api.listener]
enabled = true

[[admin_api.tokens]]
name = "local-cli"
token = "config-token-value"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
