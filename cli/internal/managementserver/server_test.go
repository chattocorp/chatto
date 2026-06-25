package managementserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	managementv1 "hmans.de/chatto/internal/pb/chatto/management/v1"
	"hmans.de/chatto/internal/pb/chatto/management/v1/managementv1connect"
	"hmans.de/chatto/internal/testutil"
)

func TestServerServesUserAdminServiceOnUnixSocket(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := context.Background()
	chattoCore, err := core.NewChattoCore(ctx, nc, config.CoreConfig{})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

	socketPath := filepath.Join(privateTempDir(t), "admin.sock")
	server := New(config.ManagementConfig{SocketPath: socketPath}, chattoCore)
	serverCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(serverCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("management server stopped with error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("management server did not stop")
		}
	})
	waitForSocket(t, socketPath)

	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("socket permissions = %o, want 0600", got)
	}

	client := managementv1connect.NewUserAdminServiceClient(unixHTTPClient(socketPath), "http://chatto-management")
	resp, err := client.CreateUser(ctx, connect.NewRequest(&managementv1.CreateUserRequest{
		Login:       "socket-user",
		DisplayName: "Socket User",
		Password:    "password123",
	}))
	if err != nil {
		t.Fatalf("CreateUser over socket: %v", err)
	}
	if got := resp.Msg.GetUser(); got.GetLogin() != "socket-user" {
		t.Fatalf("created user = %+v", got)
	}
}

func TestServerUsesConfiguredGroupSocketMode(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := context.Background()
	chattoCore, err := core.NewChattoCore(ctx, nc, config.CoreConfig{})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startCoreServices(t, chattoCore)

	socketDir := filepath.Join(t.TempDir(), "group-readable")
	if err := os.Mkdir(socketDir, 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Chmod(socketDir, 0750); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	socketPath := filepath.Join(socketDir, "admin.sock")
	server := New(config.ManagementConfig{
		SocketPath:  socketPath,
		SocketMode:  "0660",
		SocketGroup: dirGroup(t, socketDir),
	}, chattoCore)
	serverCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(serverCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("management server stopped with error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("management server did not stop")
		}
	})
	waitForSocket(t, socketPath)

	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if got := info.Mode().Perm(); got != 0660 {
		t.Fatalf("socket permissions = %o, want 0660", got)
	}
}

func TestServerRejectsInvalidSocketMode(t *testing.T) {
	server := New(config.ManagementConfig{
		SocketPath: filepath.Join(privateTempDir(t), "admin.sock"),
		SocketMode: "0640",
	}, nil)

	err := server.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "management.socket_mode must be 0600 or 0660") {
		t.Fatalf("Run() error = %v, want invalid socket mode error", err)
	}
}

func TestPrepareSocketPathRefusesNonSocketPath(t *testing.T) {
	path := filepath.Join(privateTempDir(t), "admin.sock")
	if err := os.WriteFile(path, []byte("do not delete"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := prepareSocketPath(path, 0600, "")
	if err == nil || !strings.Contains(err.Error(), "not a socket") {
		t.Fatalf("prepareSocketPath error = %v, want not-a-socket error", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("existing non-socket path was removed: %v", err)
	}
}

func TestPrepareSocketPathRejectsSharedParentDirectory(t *testing.T) {
	for _, mode := range []os.FileMode{0755, 0777} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "shared")
			if err := os.Mkdir(dir, mode); err != nil {
				t.Fatalf("Mkdir: %v", err)
			}
			if err := os.Chmod(dir, mode); err != nil {
				t.Fatalf("Chmod: %v", err)
			}

			err := prepareSocketPath(filepath.Join(dir, "admin.sock"), 0600, "")
			if err == nil || !strings.Contains(err.Error(), "must not be accessible by other users") {
				t.Fatalf("prepareSocketPath error = %v, want shared-parent error", err)
			}
		})
	}
}

func TestPrepareSocketPathAllowsExplicitGroupReadableParent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "group-readable")
	if err := os.Mkdir(dir, 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Chmod(dir, 0750); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	err := prepareSocketPath(filepath.Join(dir, "admin.sock"), 0660, dirGroup(t, dir))
	if err != nil {
		t.Fatalf("prepareSocketPath: %v", err)
	}
}

func TestPrepareSocketPathRejectsGroupModeWithoutExpectedGroup(t *testing.T) {
	err := prepareSocketPath(filepath.Join(privateTempDir(t), "admin.sock"), 0660, "")
	if err == nil || !strings.Contains(err.Error(), "management.socket_group is required") {
		t.Fatalf("prepareSocketPath error = %v, want missing-group error", err)
	}
}

func TestPrepareSocketPathRejectsGroupWritableParentDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "group-writable")
	if err := os.Mkdir(dir, 0770); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Chmod(dir, 0770); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	err := prepareSocketPath(filepath.Join(dir, "admin.sock"), 0660, dirGroup(t, dir))
	if err == nil || !strings.Contains(err.Error(), "must not be group-writable") {
		t.Fatalf("prepareSocketPath error = %v, want group-writable error", err)
	}
}

func TestPrepareSocketPathRejectsUnexpectedParentDirectoryGroup(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wrong-group")
	if err := os.Mkdir(dir, 0750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Chmod(dir, 0750); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	expectedGID := dirGroupID(t, dir) + 1
	err := prepareSocketPath(filepath.Join(dir, "admin.sock"), 0660, fmt.Sprint(expectedGID))
	if err == nil || !strings.Contains(err.Error(), "directory group does not match") {
		t.Fatalf("prepareSocketPath error = %v, want group mismatch error", err)
	}
}

func TestPrepareSocketPathRemovesStaleSocket(t *testing.T) {
	path := filepath.Join(privateTempDir(t), "admin.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := prepareSocketPath(path, 0600, ""); err != nil {
		t.Fatalf("prepareSocketPath: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale socket still exists, stat err = %v", err)
	}
}

func TestPrepareSocketPathRefusesActiveSocket(t *testing.T) {
	path := filepath.Join(privateTempDir(t), "admin.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	err = prepareSocketPath(path, 0600, "")
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("prepareSocketPath error = %v, want already-in-use error", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("active socket was removed: %v", err)
	}
}

func unixHTTPClient(socketPath string) *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}}
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("socket %s did not appear", path)
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatalf("Mkdir private temp dir: %v", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatalf("Chmod private temp dir: %v", err)
	}
	return dir
}

func dirGroup(t *testing.T, dir string) string {
	t.Helper()
	return fmt.Sprint(dirGroupID(t, dir))
}

func dirGroupID(t *testing.T, dir string) uint32 {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat %s: %v", dir, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("Stat %s returned unsupported stat type", dir)
	}
	return stat.Gid
}

func startCoreServices(t *testing.T, c *core.ChattoCore) {
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
