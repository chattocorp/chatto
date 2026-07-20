package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
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

func TestRootRegistersSearchProviderCommand(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"search-provider"})
	if err != nil {
		t.Fatalf("find search-provider command: %v", err)
	}
	if command != searchProviderCmd {
		t.Fatalf("found command %q, want search-provider", command.Name())
	}
}

func TestRootRegistersOperatorUserCommands(t *testing.T) {
	for _, args := range [][]string{
		{"operator", "user", "create", "--help"},
		{"operator", "user", "set-password", "--help"},
		{"operator", "user", "role", "add", "--help"},
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

func TestRootDoesNotPrintUsageOnError(t *testing.T) {
	resetAdminGlobals(t)
	resetCommandFlags(rootCmd)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"operator", "user", "create", "ignored", "--login", "alice"})
	t.Cleanup(func() {
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
		rootCmd.SetArgs(nil)
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("Execute() err = nil, want argument error")
	}
	if strings.Contains(out.String(), "Usage:") {
		t.Fatalf("unexpected usage output on error:\n%s", out.String())
	}
}

func TestResolveOperatorAPIClientConfigUsesConfigSocket(t *testing.T) {
	resetAdminGlobals(t)
	operatorConfigFile = writeOperatorTestConfig(t, "/tmp/config-operator.sock")

	got, err := resolveOperatorAPIClientConfig()
	if err != nil {
		t.Fatalf("resolveOperatorAPIClientConfig(): %v", err)
	}
	if got.socketPath != "/tmp/config-operator.sock" {
		t.Fatalf("socketPath = %q, want config path", got.socketPath)
	}
	if got.connectBaseURL != "http://chatto-operator/api/connect" {
		t.Fatalf("connectBaseURL = %q, want Unix-socket base URL", got.connectBaseURL)
	}
}

func TestResolveOperatorAPIClientConfigEnvOverridesConfigSocket(t *testing.T) {
	resetAdminGlobals(t)
	operatorConfigFile = writeOperatorTestConfig(t, "/tmp/config-operator.sock")
	t.Setenv("CHATTO_OPERATOR_API_SOCKET_PATH", "/tmp/env-operator.sock")

	got, err := resolveOperatorAPIClientConfig()
	if err != nil {
		t.Fatalf("resolveOperatorAPIClientConfig(): %v", err)
	}
	if got.socketPath != "/tmp/env-operator.sock" {
		t.Fatalf("socketPath = %q, want env path", got.socketPath)
	}
}

func TestOperatorOutputUsesProvidedWriter(t *testing.T) {
	originalJSON := operatorOutputJSON
	t.Cleanup(func() { operatorOutputJSON = originalJSON })

	member := &adminv1.AdminMember{
		Roles:          []string{"admin"},
		VerifiedEmails: []string{"writer@example.com"},
		User: &apiv1.User{
			Id:          "Uwriter",
			Login:       "writer",
			DisplayName: "Writer User",
		},
	}

	operatorOutputJSON = false
	var humanOut bytes.Buffer
	if err := printAdminOutput(&humanOut, &adminv1.GetMemberResponse{Member: member}, func() {
		printAdminMemberLine(&humanOut, member)
	}); err != nil {
		t.Fatalf("printAdminOutput human: %v", err)
	}
	if got := humanOut.String(); !strings.Contains(got, "Uwriter\twriter\tWriter User\troles=admin\temails=writer@example.com") {
		t.Fatalf("human output = %q", got)
	}

	operatorOutputJSON = true
	var jsonOut bytes.Buffer
	if err := printAdminOutput(&jsonOut, &adminv1.GetMemberResponse{Member: member}, func() {
		t.Fatal("human callback should not run for JSON output")
	}); err != nil {
		t.Fatalf("printAdminOutput JSON: %v", err)
	}
	if got := jsonOut.String(); !strings.Contains(got, `"user"`) || !strings.Contains(got, `"Uwriter"`) {
		t.Fatalf("JSON output = %q", got)
	}
}

func TestOperatorUserCommandsExerciseOperatorAPI(t *testing.T) {
	env := newAdminCLITestEnv(t)

	createOut := env.run(t, "operator", "user", "create",
		"--login", "cli-admin-user",
		"--display-name", "CLI Admin User",
		"--password-stdin",
		"--verified-email", "cli-admin@example.com",
		"--role", "cli-test-role",
		"--json",
	)
	var created struct {
		Member struct {
			User struct {
				Login string `json:"login"`
			} `json:"user"`
			Roles []string `json:"roles"`
		} `json:"member"`
	}
	if err := json.Unmarshal([]byte(createOut), &created); err != nil {
		t.Fatalf("unmarshal create output: %v\n%s", err, createOut)
	}
	if created.Member.User.Login != "cli-admin-user" || strings.Join(created.Member.Roles, ",") != "cli-test-role" {
		t.Fatalf("create output = %+v", created.Member)
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

	getOut := env.run(t, "operator", "user", "get", user.Id)
	if !strings.Contains(getOut, user.Id+"\tcli-admin-user\tCLI Admin User") {
		t.Fatalf("get output = %q", getOut)
	}

	updateOut := env.run(t, "operator", "user", "update", user.Id, "--display-name", "CLI Renamed")
	if !strings.Contains(updateOut, "\tCLI Renamed\t") {
		t.Fatalf("update output = %q", updateOut)
	}

	passwordPath := t.TempDir() + "/password"
	if err := os.WriteFile(passwordPath, []byte("new-password-123\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	env.run(t, "operator", "user", "set-password", user.Id, "--password-file", passwordPath)
	if _, _, err := env.core.VerifyPasswordWithAuthGeneration(env.ctx, "cli-admin-user", "new-password-123"); err != nil {
		t.Fatalf("VerifyPasswordWithAuthGeneration after set-password: %v", err)
	}

	emailOut := env.run(t, "operator", "user", "add-email", user.Id, "--email", "cli-admin-2@example.com")
	if !strings.Contains(emailOut, "cli-admin-2@example.com") {
		t.Fatalf("add-email output = %q", emailOut)
	}

	roleAddOut := env.run(t, "operator", "user", "role", "add", user.Id, "cli-extra-role")
	if !strings.Contains(roleAddOut, "cli-extra-role") {
		t.Fatalf("role add output = %q", roleAddOut)
	}
	roleRemoveOut := env.run(t, "operator", "user", "role", "remove", user.Id, "cli-extra-role")
	if strings.Contains(roleRemoveOut, "cli-extra-role") {
		t.Fatalf("role remove output still contains cli-extra-role: %q", roleRemoveOut)
	}

	listOut := env.run(t, "operator", "user", "list", "--search", "cli-admin", "--limit", "101")
	if !strings.Contains(listOut, "total=1 has_more=false") || !strings.Contains(listOut, "cli-admin-user") {
		t.Fatalf("list output = %q", listOut)
	}
	negativeLimitListOut := env.run(t, "operator", "user", "list", "--search", "cli-admin", "--limit", "-1")
	if !strings.Contains(negativeLimitListOut, "total=1 has_more=false") || !strings.Contains(negativeLimitListOut, "cli-admin-user") {
		t.Fatalf("list with negative limit output = %q", negativeLimitListOut)
	}
	emailListOut := env.run(t, "operator", "user", "list", "--search", "cli-admin-2@example.com")
	if !strings.Contains(emailListOut, "total=1 has_more=false") || !strings.Contains(emailListOut, user.Id) {
		t.Fatalf("list with email search output = %q", emailListOut)
	}

	deleteOut := env.run(t, "operator", "user", "delete", user.Id, "--yes")
	if !strings.Contains(deleteOut, "deleted user "+user.Id) {
		t.Fatalf("delete output = %q", deleteOut)
	}
	if _, err := env.core.GetUser(env.ctx, user.Id); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("GetUser after delete err = %v, want ErrNotFound", err)
	}
}

type adminCLITestEnv struct {
	ctx        context.Context
	core       *core.ChattoCore
	server     *http.Server
	socketPath string
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

	socketPath := fmt.Sprintf("/tmp/chatto-operator-%d.sock", time.Now().UnixNano())
	cfg := config.ChattoConfig{}
	mux := http.NewServeMux()
	api := connectapi.New(c, cfg, "test")
	for _, handler := range api.OperatorHandlers() {
		serviceHandler := handler.Handler
		mux.Handle(connectapi.Prefix+handler.ServicePath, http.StripPrefix(connectapi.Prefix, serviceHandler))
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen operator socket: %v", err)
	}
	server := &http.Server{Handler: mux}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("operator test server did not stop within timeout")
		}
		_ = os.Remove(socketPath)
	})

	env := &adminCLITestEnv{ctx: ctx, core: c, server: server, socketPath: socketPath}
	operatorSocketPath = socketPath
	operatorOutputJSON = false
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

	operatorSocketPath = env.socketPath
	operatorOutputJSON = false

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	defer func() {
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
		rootCmd.SetArgs(nil)
		operatorOutputJSON = false
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
	oldConfigFile := operatorConfigFile
	oldSocketPath := operatorSocketPath
	oldEnv := make(map[string]*string)
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if name == "CHATTO_OPERATOR_API_SOCKET_PATH" {
			value := os.Getenv(name)
			oldEnv[name] = &value
			if err := os.Unsetenv(name); err != nil {
				t.Fatalf("unset %s: %v", name, err)
			}
		}
	}
	t.Cleanup(func() {
		operatorConfigFile = oldConfigFile
		operatorSocketPath = oldSocketPath
		for name, value := range oldEnv {
			if value == nil {
				_ = os.Unsetenv(name)
			} else {
				_ = os.Setenv(name, *value)
			}
		}
	})
	operatorConfigFile = ""
	operatorSocketPath = ""
}

func writeOperatorTestConfig(t *testing.T, socketPath string) string {
	t.Helper()
	path := t.TempDir() + "/chatto.toml"
	body := `[operator_api]
enabled = true
socket_path = "` + socketPath + `"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
