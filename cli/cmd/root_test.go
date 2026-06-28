package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/authn"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/internal/testutil"
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
	adminConfigFile = writeAdminTestConfig(t, "https://public.example")

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
	t.Setenv("CHATTO_ADMIN_API_ENABLED", "true")
	t.Setenv("CHATTO_ADMIN_API_BIND_ADDRESS", "0.0.0.0")
	t.Setenv("CHATTO_ADMIN_API_PORT", "4123")

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
	adminConfigFile = writeWebserverOnlyTestConfig(t, "https://safe.example")
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

func TestAdminUserCommandsExerciseAdminAPI(t *testing.T) {
	env := newAdminCLITestEnv(t)

	createOut := env.run(t, "admin", "user", "create",
		"--login", "cli-admin-user",
		"--display-name", "CLI Admin User",
		"--password-stdin",
		"--verified-email", "cli-admin@example.com",
		"--role", "cli-test-role",
		"--json",
	)
	var created struct {
		User struct {
			Login     string   `json:"login"`
			RoleNames []string `json:"roleNames"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(createOut), &created); err != nil {
		t.Fatalf("unmarshal create output: %v\n%s", err, createOut)
	}
	if created.User.Login != "cli-admin-user" || strings.Join(created.User.RoleNames, ",") != "cli-test-role" {
		t.Fatalf("create output = %+v", created.User)
	}
	user, err := env.core.GetUserByLogin(env.ctx, "cli-admin-user")
	if err != nil {
		t.Fatalf("GetUserByLogin after create: %v", err)
	}
	emails, err := env.core.GetVerifiedEmails(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("GetVerifiedEmails: %v", err)
	}
	if len(emails) != 1 || emails[0].Email != "cli-admin@example.com" {
		t.Fatalf("verified emails = %+v, want cli-admin@example.com", emails)
	}
	roles, err := env.core.GetUserRoles(env.ctx, user.Id)
	if err != nil {
		t.Fatalf("GetUserRoles: %v", err)
	}
	if strings.Join(roles, ",") != "cli-test-role" {
		t.Fatalf("roles = %v, want cli-test-role", roles)
	}

	getOut := env.run(t, "admin", "user", "get", "--login", "cli-admin-user")
	if !strings.Contains(getOut, user.Id+"\tcli-admin-user\tCLI Admin User") {
		t.Fatalf("get output = %q", getOut)
	}

	updateOut := env.run(t, "admin", "user", "update", user.Id, "--display-name", "CLI Renamed")
	if !strings.Contains(updateOut, "\tCLI Renamed\t") {
		t.Fatalf("update output = %q", updateOut)
	}

	passwordPath := t.TempDir() + "/password"
	if err := os.WriteFile(passwordPath, []byte("new-password-123\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	env.run(t, "admin", "user", "set-password", user.Id, "--password-file", passwordPath)
	if _, _, err := env.core.VerifyPasswordWithAuthGeneration(env.ctx, "cli-admin-user", "new-password-123"); err != nil {
		t.Fatalf("VerifyPasswordWithAuthGeneration after set-password: %v", err)
	}

	emailOut := env.run(t, "admin", "user", "add-email", user.Id, "cli-admin-2@example.com")
	if !strings.Contains(emailOut, "cli-admin-2@example.com") {
		t.Fatalf("add-email output = %q", emailOut)
	}

	roleAddOut := env.run(t, "admin", "user", "role", "add", user.Id, "cli-extra-role")
	if !strings.Contains(roleAddOut, "cli-extra-role") {
		t.Fatalf("role add output = %q", roleAddOut)
	}
	roleRemoveOut := env.run(t, "admin", "user", "role", "remove", user.Id, "cli-extra-role")
	if strings.Contains(roleRemoveOut, "cli-extra-role") {
		t.Fatalf("role remove output still contains cli-extra-role: %q", roleRemoveOut)
	}

	listOut := env.run(t, "admin", "user", "list", "--search", "cli-admin", "--limit", "101")
	if !strings.Contains(listOut, "total=1 has_more=false") || !strings.Contains(listOut, "cli-admin-user") {
		t.Fatalf("list output = %q", listOut)
	}

	deleteOut := env.run(t, "admin", "user", "delete", user.Id, "--yes")
	if !strings.Contains(deleteOut, "deleted user "+user.Id) {
		t.Fatalf("delete output = %q", deleteOut)
	}
	if _, err := env.core.GetUser(env.ctx, user.Id); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("GetUser after delete err = %v, want ErrNotFound", err)
	}
}

func TestAdminUserCommandReadsAdminTokenFile(t *testing.T) {
	env := newAdminCLITestEnv(t)
	tokenPath := t.TempDir() + "/admin-token"
	if err := os.WriteFile(tokenPath, []byte(adminCLITestToken+"\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	env.token = ""

	out := env.run(t, "admin", "--admin-token-file", tokenPath, "user", "create",
		"--login", "token-file-user",
		"--password", "password123",
	)
	if !strings.Contains(out, "\ttoken-file-user\t") {
		t.Fatalf("create with token file output = %q", out)
	}
}

const adminCLITestToken = "cli-admin-token"

type adminCLITestEnv struct {
	ctx    context.Context
	core   *core.ChattoCore
	server *httptest.Server
	token  string
}

func newAdminCLITestEnv(t *testing.T) *adminCLITestEnv {
	t.Helper()
	resetAdminGlobals(t)

	_, nc := testutil.StartSharedNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	c, err := core.NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startAdminCLITestCore(t, c)
	if _, err := c.CreateServerRole(ctx, core.SystemActorID, "cli-test-role", "CLI Test Role", ""); err != nil {
		t.Fatalf("CreateServerRole cli-test-role: %v", err)
	}
	if _, err := c.CreateServerRole(ctx, core.SystemActorID, "cli-extra-role", "CLI Extra Role", ""); err != nil {
		t.Fatalf("CreateServerRole cli-extra-role: %v", err)
	}

	cfg := config.ChattoConfig{
		AdminAPI: config.AdminAPIConfig{
			Enabled: true,
			Tokens: []config.AdminAPITokenConfig{{
				Name:         "local-cli",
				Token:        adminCLITestToken,
				AllowedCIDRs: []string{"127.0.0.1/32"},
			}},
		},
	}
	mux := http.NewServeMux()
	api := connectapi.New(c, cfg, "test")
	authMiddleware := authn.NewMiddleware(func(ctx context.Context, req *http.Request) (any, error) {
		if req.Header.Get("Authorization") != "Bearer "+adminCLITestToken {
			return nil, authn.Errorf("admin token required")
		}
		return connectapi.AdminCaller{TokenName: "local-cli"}, nil
	}, connectapi.HandlerOptions()...)
	for _, handler := range api.Handlers() {
		if handler.AuthPolicy != connectapi.AuthPolicyAdminToken {
			continue
		}
		mux.Handle(connectapi.Prefix+handler.ServicePath, http.StripPrefix(connectapi.Prefix, authMiddleware.Wrap(handler.Handler)))
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	env := &adminCLITestEnv{ctx: ctx, core: c, server: server, token: adminCLITestToken}
	adminAPIURL = server.URL + connectapi.Prefix
	adminAPIToken = adminCLITestToken
	adminOutputJSON = false
	return env
}

func startAdminCLITestCore(t *testing.T, c *core.ChattoCore) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("core.Run did not stop within timeout")
		}
	})

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bootCancel()
	if err := c.WaitForBoot(bootCtx); err != nil {
		t.Fatalf("WaitForBoot: %v", err)
	}
}

func (env *adminCLITestEnv) run(t *testing.T, args ...string) string {
	t.Helper()
	resetCommandFlags(rootCmd)
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if _, err := w.WriteString("password123\n"); err != nil {
		t.Fatalf("write stdin password: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()

	adminAPIURL = env.server.URL + connectapi.Prefix
	adminAPIToken = env.token
	adminOutputJSON = false

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	defer func() {
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
		rootCmd.SetArgs(nil)
		adminOutputJSON = false
	}()
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("chatto %s: %v\noutput:\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

func resetCommandFlags(cmd *cobra.Command) {
	resetFlagSet(cmd.Flags())
	resetFlagSet(cmd.PersistentFlags())
	for _, child := range cmd.Commands() {
		resetCommandFlags(child)
	}
}

func resetFlagSet(flags *pflag.FlagSet) {
	flags.VisitAll(func(flag *pflag.Flag) {
		if replacer, ok := flag.Value.(interface{ Replace([]string) error }); ok {
			_ = replacer.Replace(nil)
		} else {
			_ = flag.Value.Set(flag.DefValue)
		}
		flag.Changed = false
	})
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
		if name == "CHATTO_WEBSERVER_URL" || name == "CHATTO_ADMIN_API_TOKEN" || name == "CHATTO_ADMIN_API_TOKEN_NAME" || name == "CHATTO_ADMIN_API_ENABLED" || name == "CHATTO_ADMIN_API_BIND_ADDRESS" || name == "CHATTO_ADMIN_API_PORT" || strings.HasPrefix(name, "CHATTO_ADMIN_API_TOKENS_") {
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

func writeWebserverOnlyTestConfig(t *testing.T, webserverURL string) string {
	t.Helper()
	path := t.TempDir() + "/chatto.toml"
	body := `[webserver]
url = "` + webserverURL + `"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
