package cmd

import (
	"bytes"
	"context"
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

func TestRootDoesNotPrintUsageOnError(t *testing.T) {
	resetOperatorGlobals(t)
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
	resetOperatorGlobals(t)
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
	resetOperatorGlobals(t)
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
	if err := printOperatorOutput(&humanOut, &adminv1.GetMemberResponse{Member: member}, func() {
		printOperatorUserLine(&humanOut, member)
	}); err != nil {
		t.Fatalf("printOperatorOutput human: %v", err)
	}
	if got := humanOut.String(); !strings.Contains(got, "Uwriter\twriter\tWriter User\troles=admin\temails=writer@example.com") {
		t.Fatalf("human output = %q", got)
	}

	operatorOutputJSON = true
	var jsonOut bytes.Buffer
	if err := printOperatorOutput(&jsonOut, &adminv1.GetMemberResponse{Member: member}, func() {
		t.Fatal("human callback should not run for JSON output")
	}); err != nil {
		t.Fatalf("printOperatorOutput JSON: %v", err)
	}
	if got := jsonOut.String(); !strings.Contains(got, `"user"`) || !strings.Contains(got, `"Uwriter"`) {
		t.Fatalf("JSON output = %q", got)
	}
}

type operatorCLITestEnv struct {
	ctx        context.Context
	core       *core.ChattoCore
	server     *http.Server
	socketPath string
}

func newOperatorCLITestEnv(t *testing.T) *operatorCLITestEnv {
	t.Helper()
	resetOperatorGlobals(t)

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
	startOperatorCLITestCore(t, c)
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

	env := &operatorCLITestEnv{ctx: ctx, core: c, server: server, socketPath: socketPath}
	operatorSocketPath = socketPath
	operatorOutputJSON = false
	return env
}

func startOperatorCLITestCore(t *testing.T, c *core.ChattoCore) {
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

func (env *operatorCLITestEnv) run(t *testing.T, args ...string) string {
	t.Helper()
	out, err := env.execute(t, args...)
	if err != nil {
		t.Fatalf("chatto %s: %v\noutput:\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func (env *operatorCLITestEnv) execute(t *testing.T, args ...string) (string, error) {
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
	err = rootCmd.Execute()
	return out.String(), err
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

func resetOperatorGlobals(t *testing.T) {
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
