//go:build integration

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestRunCommandShutsDownOnSignals(t *testing.T) {
	binary := buildRunCommandTestBinary(t)

	for _, tc := range []struct {
		name   string
		signal os.Signal
	}{
		{name: "SIGHUP", signal: syscall.SIGHUP},
		{name: "SIGTERM", signal: syscall.SIGTERM},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port := freeTCPPort(t)
			configPath := writeShutdownTestConfig(t, port)

			cmd := exec.Command(binary, "start", "-c", configPath)
			var logs lockedBuffer
			cmd.Stdout = &logs
			cmd.Stderr = &logs

			if err := cmd.Start(); err != nil {
				t.Fatalf("start server: %v", err)
			}
			t.Cleanup(func() {
				if cmd.ProcessState == nil {
					_ = cmd.Process.Kill()
					_, _ = cmd.Process.Wait()
				}
			})

			waitForHTTPReady(t, port, &logs)

			if err := cmd.Process.Signal(tc.signal); err != nil {
				t.Fatalf("send %s: %v\nlogs:\n%s", tc.name, err, logs.String())
			}

			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("server exited with error after %s: %v\nlogs:\n%s", tc.name, err, logs.String())
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("server did not exit after %s\nlogs:\n%s", tc.name, logs.String())
			}
		})
	}
}

func buildRunCommandTestBinary(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "chatto")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	cmd.Dir = ".."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build test binary: %v\n%s", err, string(output))
	}
	return binary
}

func writeShutdownTestConfig(t *testing.T, port int) string {
	t.Helper()

	dataDir := filepath.Join(t.TempDir(), "nats")
	configPath := filepath.Join(t.TempDir(), "chatto.toml")
	config := fmt.Sprintf(`
[general]
log_level = 'error'

[webserver]
url = 'http://127.0.0.1:%d'
port = %d
cookie_signing_secret = '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef'

[core]
secret_key = 'abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789'

[core.assets]
signing_secret = 'fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210'

[nats.embedded]
enabled = true
port = 0
data_dir = %q
`, port, port, dataDir)

	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}

func freeTCPPort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free TCP port: %v", err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener addr is %T, want *net.TCPAddr", ln.Addr())
	}
	return addr.Port
}

func waitForHTTPReady(t *testing.T, port int, logs *lockedBuffer) {
	t.Helper()

	client := &http.Client{Timeout: 250 * time.Millisecond}
	url := fmt.Sprintf("http://127.0.0.1:%d/readyz", port)
	deadline := time.Now().Add(20 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("server did not become ready at %s\nlogs:\n%s", url, logs.String())
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
