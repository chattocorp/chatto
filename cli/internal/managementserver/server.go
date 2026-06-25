package managementserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/managementapi"
)

const shutdownTimeout = 5 * time.Second

type Server struct {
	socketPath    string
	socketMode    os.FileMode
	socketModeErr error
	socketGroup   string
	handler       http.Handler
	logger        *log.Logger
}

func New(cfg config.ManagementConfig, c *core.ChattoCore) *Server {
	mux := http.NewServeMux()
	api := managementapi.New(c)
	for _, handler := range api.Handlers() {
		mux.Handle(handler.ServicePath, handler.Handler)
	}
	socketMode, socketModeErr := cfg.SocketFileMode()
	return &Server{
		socketPath:    cfg.SocketPathOrDefault(),
		socketMode:    socketMode,
		socketModeErr: socketModeErr,
		socketGroup:   strings.TrimSpace(cfg.SocketGroup),
		handler:       mux,
		logger:        log.WithPrefix("server.management"),
	}
}

func (s *Server) Run(ctx context.Context) error {
	if s.socketModeErr != nil {
		return s.socketModeErr
	}
	if err := prepareSocketPath(s.socketPath, s.socketMode, s.socketGroup); err != nil {
		return err
	}
	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on management socket: %w", err)
	}
	if err := os.Chmod(s.socketPath, s.socketMode); err != nil {
		_ = ln.Close()
		return fmt.Errorf("secure management socket permissions: %w", err)
	}
	defer func() {
		_ = os.Remove(s.socketPath)
	}()

	httpServer := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	serverErr := make(chan error, 1)
	go func() {
		if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	s.logger.Info("Starting management server", "socket", s.socketPath)
	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			_ = httpServer.Close()
			return err
		}
		return nil
	}
}

func prepareSocketPath(path string, socketMode os.FileMode, socketGroup string) error {
	if path == "" {
		return fmt.Errorf("management socket path is required")
	}
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create management socket directory: %w", err)
		}
	}
	if err := validateSocketDirectory(dir, socketMode, socketGroup); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("management socket path exists and is not a socket: %s", path)
		}
		if err := removeStaleSocket(path); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect management socket path: %w", err)
	}
	return nil
}

func removeStaleSocket(path string) error {
	conn, err := net.DialTimeout("unix", path, 100*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("management socket path is already in use: %s", path)
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("inspect existing management socket: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale management socket: %w", err)
	}
	return nil
}

func validateSocketDirectory(dir string, socketMode os.FileMode, socketGroup string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("inspect management socket directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("management socket parent path is not a directory: %s", dir)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("inspect management socket directory ownership: unsupported stat type")
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("management socket directory must be owned by the server user: %s", dir)
	}
	perm := info.Mode().Perm()
	if perm&0007 != 0 {
		return fmt.Errorf("management socket directory must not be accessible by other users: %s", dir)
	}
	switch socketMode {
	case 0600:
		if perm&0070 != 0 {
			return fmt.Errorf("management socket directory must not be accessible by group users when socket_mode is 0600: %s", dir)
		}
	case 0660:
		expectedGID, err := resolveSocketGroupID(socketGroup)
		if err != nil {
			return err
		}
		if stat.Gid != expectedGID {
			return fmt.Errorf("management socket directory group does not match management.socket_group: %s", dir)
		}
		if perm&0010 == 0 {
			return fmt.Errorf("management socket directory must be group-executable when socket_mode is 0660: %s", dir)
		}
		if perm&0020 != 0 {
			return fmt.Errorf("management socket directory must not be group-writable: %s", dir)
		}
	default:
		return fmt.Errorf("management socket mode must be 0600 or 0660")
	}
	return nil
}

func resolveSocketGroupID(socketGroup string) (uint32, error) {
	socketGroup = strings.TrimSpace(socketGroup)
	if socketGroup == "" {
		return 0, fmt.Errorf("management.socket_group is required when management.socket_mode is 0660")
	}
	if gid, err := strconv.ParseUint(socketGroup, 10, 32); err == nil {
		return uint32(gid), nil
	}
	group, err := user.LookupGroup(socketGroup)
	if err != nil {
		return 0, fmt.Errorf("lookup management.socket_group: %w", err)
	}
	gid, err := strconv.ParseUint(group.Gid, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse management.socket_group gid: %w", err)
	}
	return uint32(gid), nil
}
