package managementserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/managementapi"
)

const shutdownTimeout = 5 * time.Second

type Server struct {
	socketPath string
	handler    http.Handler
	logger     *log.Logger
}

func New(cfg config.ManagementConfig, c *core.ChattoCore) *Server {
	mux := http.NewServeMux()
	api := managementapi.New(c)
	for _, handler := range api.Handlers() {
		mux.Handle(handler.ServicePath, handler.Handler)
	}
	return &Server{
		socketPath: cfg.SocketPathOrDefault(),
		handler:    mux,
		logger:     log.WithPrefix("server.management"),
	}
}

func (s *Server) Run(ctx context.Context) error {
	if err := prepareSocketPath(s.socketPath); err != nil {
		return err
	}
	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on management socket: %w", err)
	}
	if err := os.Chmod(s.socketPath, 0600); err != nil {
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

func prepareSocketPath(path string) error {
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
	if err := validateSocketDirectory(dir); err != nil {
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

func validateSocketDirectory(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("inspect management socket directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("management socket parent path is not a directory: %s", dir)
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("management socket directory must not be accessible by group or others: %s", dir)
	}
	return nil
}
