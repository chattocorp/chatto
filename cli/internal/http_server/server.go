package http_server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
	"golang.org/x/crypto/acme/autocert"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
	"hmans.de/chatto/internal/search"
)

// HTTPServerConfig holds configuration for creating an HTTPServer.
type HTTPServerConfig struct {
	Config  config.ChattoConfig
	NC      *nats.Conn
	Core    *core.ChattoCore
	Addr    string
	Version string
}

// HTTPServer serves the HTTP APIs and static frontend.
type HTTPServer struct {
	config           config.ChattoConfig
	nc               *nats.Conn
	router           *gin.Engine
	core             *core.ChattoCore
	connectAPI       *connectapi.API
	mailer           email.Sender
	mockMailer       *email.MockSender // Non-nil when test email endpoint is enabled
	addr             string
	version          string
	logger           *log.Logger
	metrics          *processMetrics
	realtimeCatchUps *realtimeCatchUpAdmission
	trustedProxies   trustedProxySet

	// Optional test hook used to make password-login revocation races deterministic.
	passwordLoginSessionCreatedHook func(*gin.Context, string, uint64)
}

const (
	httpServerReadHeaderTimeout = 10 * time.Second
	httpServerIdleTimeout       = 2 * time.Minute
	httpServerShutdownTimeout   = 5 * time.Second
	legacyRequestBodyLimit      = 64 * 1024
	legacyRequestBodyTimeout    = 30 * time.Second
)

type requestConnectionContextKey struct{}

// limitLegacyRequestBody bounds the small JSON/form requests handled outside
// ConnectRPC. It deliberately applies only to legacy route groups so uploads
// and long-lived realtime connections keep their dedicated limits.
func limitLegacyRequestBody() gin.HandlerFunc {
	return limitRequestBody(legacyRequestBodyLimit, legacyRequestBodyTimeout)
}

func limitRequestBody(maxBytes int64, readTimeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.Next()
			return
		}
		if c.Request.ContentLength > maxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Request body is too large"})
			return
		}

		controller := http.NewResponseController(c.Writer)
		deadlineSet := controller.SetReadDeadline(time.Now().Add(readTimeout)) == nil
		if !deadlineSet && c.Request.ProtoMajor == 1 {
			if conn, ok := c.Request.Context().Value(requestConnectionContextKey{}).(net.Conn); ok {
				deadlineSet = conn.SetReadDeadline(time.Now().Add(readTimeout)) == nil
				defer conn.SetReadDeadline(time.Time{})
			}
		}
		if deadlineSet {
			defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// NewHTTPServer creates a new HTTP server with the provided dependencies.
func NewHTTPServer(cfg HTTPServerConfig) (*HTTPServer, error) {
	logger := log.WithPrefix("server.HTTP")

	// Create email mailer (mock if built with -tags test_endpoints, real otherwise)
	mockMailer, mailer := createMailer(cfg.Config.SMTP)

	// Warn at startup if test endpoints are enabled (security-bypassing endpoints)
	if mockMailer != nil {
		logger.Warn("TEST ENDPOINTS ENABLED - This build includes security-bypassing endpoints. DO NOT use in production!")
	}

	// Create Gin router with Recovery middleware, and optionally Logger
	router := gin.New()
	if err := router.SetTrustedProxies(cfg.Config.Webserver.TrustedProxies); err != nil {
		return nil, fmt.Errorf("configure trusted proxies: %w", err)
	}
	router.Use(gin.Recovery())
	if cfg.Config.Webserver.RequestLoggingEnabled() {
		router.Use(requestLogger(logger))
	}

	trustedProxies, err := newTrustedProxySet(cfg.Config.Webserver.TrustedProxies)
	if err != nil {
		return nil, err
	}
	s := &HTTPServer{
		config:           cfg.Config,
		nc:               cfg.NC,
		router:           router,
		core:             cfg.Core,
		connectAPI:       connectapi.New(cfg.Core, cfg.Config, cfg.Version, connectapi.WithMessageSearchProviderClient(search.NewClient(cfg.NC))),
		mailer:           mailer,
		mockMailer:       mockMailer,
		addr:             cfg.Addr,
		version:          cfg.Version,
		logger:           logger,
		metrics:          newProcessMetrics(),
		realtimeCatchUps: newRealtimeCatchUpAdmission(),
		trustedProxies:   trustedProxies,
	}

	// Set up all routes
	if err := s.setupRoutes(); err != nil {
		return nil, err
	}

	return s, nil
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: httpServerReadHeaderTimeout,
		IdleTimeout:       httpServerIdleTimeout,
		ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
			return context.WithValue(ctx, requestConnectionContextKey{}, conn)
		},
	}
}

func newAppHTTPServer(addr string, handler http.Handler) *http.Server {
	server := newHTTPServer(addr, handler)
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)
	server.Protocols = protocols
	return server
}

func requestLogger(logger *log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		hasQuery := c.Request.URL.RawQuery != ""

		c.Next()

		status := c.Writer.Status()
		fields := []any{
			"status", status,
			"method", c.Request.Method,
			"path", path,
			"latency", time.Since(start).String(),
			"client_ip_present", c.ClientIP() != "",
			"user_agent", c.Request.UserAgent(),
			"bytes", c.Writer.Size(),
		}
		if hasQuery {
			fields = append(fields, "query_present", true)
		}
		if len(c.Errors) > 0 {
			fields = append(fields, "error_count", len(c.Errors.ByType(gin.ErrorTypePrivate)))
		}

		switch {
		case status >= http.StatusInternalServerError:
			logger.Error("HTTP request", fields...)
		case status >= http.StatusBadRequest:
			logger.Warn("HTTP request", fields...)
		default:
			logger.Debug("HTTP request", fields...)
		}
	}
}

func (s *HTTPServer) setupRoutes() error {
	// SESSION MANAGEMENT

	// Configure session middleware
	authKey := []byte(s.config.Webserver.CookieSigningSecret)
	var sessionStore sessions.Store
	encKey, err := s.config.Webserver.CookieEncryptionKey()
	if err != nil {
		return err
	}
	if len(encKey) > 0 {
		sessionStore = cookie.NewStore(authKey, encKey)
	} else {
		s.logger.Warn("webserver.cookie_encryption_secret is not set; session cookies are signed but NOT encrypted. Run `chatto init` on a fresh server to generate one, or add a hex-encoded 32-byte value to chatto.toml.")
		sessionStore = cookie.NewStore(authKey)
	}
	sessionStore.Options(cookieSessionOptions(s.config.Auth.TokenTTLOrDefault(), strings.HasPrefix(s.config.Webserver.URL, "https")))
	sessionStore = newDebugSessionStore(sessionStore, s.logger)
	s.router.Use(sessions.Sessions("chatto_session", sessionStore))

	// Build allowed origins list once and share between CORS middleware and WebSocket CheckOrigin
	allowedOrigins := s.buildAllowedOrigins()

	// CORS middleware for cross-origin API access (token-based auth)
	s.router.Use(s.corsMiddleware(allowedOrigins))
	s.router.Use(s.csrfMiddleware())

	// Set up feature-specific routes
	s.setupHealthRoutes()
	s.setupWebhookRoutes()
	s.setupConnectAPI()
	s.setupRealtimeAPI(allowedOrigins)
	s.setupOIDCRoutes()
	s.setupAuthRoutes()
	s.setupOAuthRoutes()
	s.setupAssetRoutes()
	s.setupShieldRoutes()

	if err := s.setupFrontendRoutes(); err != nil {
		return err
	}

	return nil
}

// Run starts the HTTP server(s) and blocks until ctx is cancelled or an error occurs.
func (s *HTTPServer) Run(ctx context.Context) error {

	var servers []*http.Server
	var tlsServer *http.Server
	var metricsServer *http.Server
	var operatorServer *http.Server
	var operatorListener net.Listener
	var operatorSocketInfo os.FileInfo

	if s.config.Webserver.TLS.Enabled {
		tlsConfig := s.config.Webserver.TLS

		// Ensure certificate cache directory exists and remains private even when
		// reusing a path created with more permissive permissions.
		cacheDir := tlsConfig.CacheDirOrDefault()
		if err := ensureAutocertCacheDir(cacheDir); err != nil {
			return err
		}

		// Create autocert manager for Let's Encrypt
		certManager := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(tlsConfig.Domain),
			Cache:      autocert.DirCache(cacheDir),
			Email:      tlsConfig.Email,
		}

		// HTTPS server (started separately with ListenAndServeTLS)
		tlsServer = newAppHTTPServer(s.addr, s.router)
		tlsServer.TLSConfig = &tls.Config{
			GetCertificate: certManager.GetCertificate,
			MinVersion:     tls.VersionTLS12,
		}

		// HTTP server for ACME challenges and HTTPS redirect
		httpAddr := fmt.Sprintf(":%d", tlsConfig.HTTPPortOrDefault())
		servers = append(servers, newHTTPServer(httpAddr, certManager.HTTPHandler(http.HandlerFunc(s.redirectToHTTPS))))
	} else {
		// Plain HTTP server
		servers = append(servers, newAppHTTPServer(s.addr, s.router))
	}

	if s.config.Metrics.Enabled {
		var err error
		metricsServer, err = s.newMetricsServer()
		if err != nil {
			return err
		}
		servers = append(servers, metricsServer)
	}

	if s.config.OperatorAPI.Enabled {
		operatorServer = s.newOperatorAPIServer()
		var err error
		operatorListener, operatorSocketInfo, err = s.prepareOperatorAPISocket()
		if err != nil {
			return err
		}
		defer s.cleanupOperatorAPISocket(operatorSocketInfo)
		servers = append(servers, operatorServer)
	}

	serverErr := make(chan error, len(servers)+1)

	// Start HTTP servers
	for _, srv := range servers {
		if srv == metricsServer {
			s.logger.Info("Starting metrics server", "url", metricsServerURL(srv.Addr, s.config.Metrics.PathOrDefault()))
		} else if srv == operatorServer {
			s.logger.Info("Starting operator API server", "socket", srv.Addr)
		} else {
			s.logger.Info("Starting HTTP server", "addr", srv.Addr, "url", s.config.Webserver.URL)
		}
		go func(srv *http.Server) {
			var err error
			if srv == operatorServer {
				err = srv.Serve(operatorListener)
			} else {
				err = srv.ListenAndServe()
			}
			if err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		}(srv)
	}

	// Start HTTPS server if TLS is enabled
	if tlsServer != nil {
		s.logger.Info("Starting HTTPS server with Let's Encrypt", "addr", tlsServer.Addr, "domain", s.config.Webserver.TLS.Domain)
		go func() {
			if err := tlsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		}()
	}

	// Wait for context cancellation or server error
	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		// Shutdown all servers gracefully
		for _, srv := range servers {
			if err := s.shutdownServer(srv); err != nil {
				s.logger.Error("Server shutdown error", "addr", srv.Addr, "error", err)
			}
		}
		if tlsServer != nil {
			if err := s.shutdownServer(tlsServer); err != nil {
				s.logger.Error("Server shutdown error", "addr", tlsServer.Addr, "error", err)
			}
		}
		return nil
	}
}

const autocertCacheDirMode os.FileMode = 0o700

func ensureAutocertCacheDir(cacheDir string) error {
	if err := os.MkdirAll(cacheDir, autocertCacheDirMode); err != nil {
		return fmt.Errorf("failed to create certificate cache directory: %w", err)
	}

	info, err := os.Lstat(cacheDir)
	if err != nil {
		return fmt.Errorf("failed to inspect certificate cache directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("certificate cache path %q is not a directory", cacheDir)
	}
	uid, _, ownerAvailable := fileOwnerIDs(info)
	if ownerAvailable && uid != uint32(os.Geteuid()) {
		return fmt.Errorf("certificate cache directory %q is owned by uid %d, want uid %d", cacheDir, uid, os.Geteuid())
	}
	if ownerAvailable {
		parent := filepath.Dir(filepath.Clean(cacheDir))
		parentInfo, err := os.Lstat(parent)
		if err != nil {
			return fmt.Errorf("failed to inspect certificate cache parent directory: %w", err)
		}
		if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
			return fmt.Errorf("certificate cache parent path %q is not a directory", parent)
		}
		parentUID, _, ok := fileOwnerIDs(parentInfo)
		if !ok {
			return fmt.Errorf("failed to inspect owner of certificate cache parent directory %q", parent)
		}
		if parentUID != uint32(os.Geteuid()) && parentUID != 0 {
			return fmt.Errorf("certificate cache parent directory %q is owned by uid %d, want uid %d or root", parent, parentUID, os.Geteuid())
		}
		if got := parentInfo.Mode().Perm(); got&0o022 != 0 {
			return fmt.Errorf("certificate cache parent directory %q is writable by group or other users; mode is %04o", parent, got)
		}
	}

	if err := os.Chmod(cacheDir, autocertCacheDirMode); err != nil {
		return fmt.Errorf("failed to secure certificate cache directory: %w", err)
	}
	info, err = os.Lstat(cacheDir)
	if err != nil {
		return fmt.Errorf("failed to verify certificate cache directory permissions: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("certificate cache path %q changed while securing it", cacheDir)
	}
	if verifiedUID, _, ok := fileOwnerIDs(info); ownerAvailable && (!ok || verifiedUID != uint32(os.Geteuid())) {
		return fmt.Errorf("certificate cache directory ownership changed while securing it")
	}
	if got := info.Mode().Perm(); ownerAvailable && got != autocertCacheDirMode {
		return fmt.Errorf("certificate cache directory has mode %04o after securing, want %04o", got, autocertCacheDirMode)
	}
	return nil
}

func metricsServerURL(addr, path string) string {
	return (&url.URL{Scheme: "http", Host: addr, Path: path}).String()
}

func (s *HTTPServer) prepareOperatorAPISocket() (net.Listener, os.FileInfo, error) {
	socketPath := s.config.OperatorAPI.SocketPathOrDefault()
	socketMode := config.OperatorAPISocketMode
	parent := filepath.Dir(socketPath)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create operator API socket directory %s: %w", parent, err)
	}
	if err := validateOperatorAPISocketParent(parent); err != nil {
		return nil, nil, err
	}
	if existing, err := os.Lstat(socketPath); err == nil {
		if existing.Mode()&os.ModeSocket == 0 {
			return nil, nil, fmt.Errorf("operator API socket path %s exists and is not a Unix socket", socketPath)
		}
		if existing.Mode().Perm() != socketMode {
			return nil, nil, fmt.Errorf("operator API socket %s has mode %04o, want %04o", socketPath, existing.Mode().Perm(), socketMode)
		}
		conn, err := net.DialTimeout("unix", socketPath, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil, nil, fmt.Errorf("operator API socket %s is already in use", socketPath)
		}
		if !isStaleOperatorSocketError(err) {
			return nil, nil, fmt.Errorf("operator API socket %s exists but could not be verified as stale: %w", socketPath, err)
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, nil, fmt.Errorf("remove stale operator API socket %s: %w", socketPath, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("inspect operator API socket %s: %w", socketPath, err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, nil, fmt.Errorf("listen on operator API socket %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, socketMode); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, nil, fmt.Errorf("set operator API socket mode %s to %04o: %w", socketPath, socketMode, err)
	}
	created, err := os.Lstat(socketPath)
	if err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, nil, fmt.Errorf("inspect created operator API socket %s: %w", socketPath, err)
	}
	if created.Mode().Perm() != socketMode {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, nil, fmt.Errorf("operator API socket %s has mode %04o after bind, want %04o", socketPath, created.Mode().Perm(), socketMode)
	}
	return listener, created, nil
}

func validateOperatorAPISocketParent(parent string) error {
	info, err := os.Lstat(parent)
	if err != nil {
		return fmt.Errorf("inspect operator API socket directory %s: %w", parent, err)
	}
	mode := info.Mode()
	if mode&os.ModeSymlink != 0 || !mode.IsDir() {
		return fmt.Errorf("operator API socket directory %s is not a directory", parent)
	}
	uid, _, ok := fileOwnerIDs(info)
	if !ok {
		return fmt.Errorf("inspect owner of operator API socket directory %s", parent)
	}
	if uid != uint32(os.Geteuid()) {
		return fmt.Errorf("operator API socket directory %s is owned by uid %d, want uid %d", parent, uid, os.Geteuid())
	}
	if mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return fmt.Errorf("operator API socket directory %s has unsafe mode bits %s", parent, mode.String())
	}
	perm := mode.Perm()
	if perm&0o077 != 0 {
		return fmt.Errorf("operator API socket directory %s must not be accessible by group or other users; mode is %04o", parent, perm)
	}
	return nil
}

func isStaleOperatorSocketError(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, os.ErrNotExist)
}

func (s *HTTPServer) cleanupOperatorAPISocket(created os.FileInfo) {
	if created == nil {
		return
	}
	socketPath := s.config.OperatorAPI.SocketPathOrDefault()
	current, err := os.Lstat(socketPath)
	if err != nil {
		return
	}
	if os.SameFile(created, current) {
		if err := os.Remove(socketPath); err != nil {
			s.logger.Warn("Failed to remove operator API socket", "socket", socketPath, "error", err)
		}
	}
}

func (s *HTTPServer) shutdownServer(server *http.Server) error {
	return s.shutdownServerWithTimeout(server, httpServerShutdownTimeout)
}

func (s *HTTPServer) shutdownServerWithTimeout(server *http.Server, timeout time.Duration) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		s.logger.Error("Server forced to shutdown", "addr", server.Addr, "error", err)
		if closeErr := server.Close(); closeErr != nil {
			return fmt.Errorf("graceful shutdown: %w; forced close: %w", err, closeErr)
		}
		return err
	}

	return nil
}

func (s *HTTPServer) redirectToHTTPS(w http.ResponseWriter, r *http.Request) {
	// Build HTTPS URL, including port if non-standard
	port := s.config.Webserver.EffectivePort()
	var target string
	if port == 443 {
		target = "https://" + s.config.Webserver.TLS.Domain + r.URL.RequestURI()
	} else {
		target = fmt.Sprintf("https://%s:%d%s", s.config.Webserver.TLS.Domain, port, r.URL.RequestURI())
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}
