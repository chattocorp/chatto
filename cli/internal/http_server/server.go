package http_server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
	"golang.org/x/crypto/acme/autocert"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
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
	config     config.ChattoConfig
	nc         *nats.Conn
	router     *gin.Engine
	core       *core.ChattoCore
	mailer     email.Sender
	mockMailer *email.MockSender // Non-nil when test email endpoint is enabled
	addr       string
	version    string
	logger     *log.Logger
	metrics    *processMetrics

	// Optional test hook used to make password-login revocation races deterministic.
	passwordLoginSessionCreatedHook func(*gin.Context, string, uint64)
	failAssetProxyRequests          atomic.Int64
}

const (
	httpServerReadHeaderTimeout = 10 * time.Second
	httpServerIdleTimeout       = 2 * time.Minute
	httpServerShutdownTimeout   = 5 * time.Second
)

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
	router.Use(gin.Recovery())
	if cfg.Config.Webserver.RequestLoggingEnabled() {
		router.Use(requestLogger(logger))
	}

	s := &HTTPServer{
		config:     cfg.Config,
		nc:         cfg.NC,
		router:     router,
		core:       cfg.Core,
		mailer:     mailer,
		mockMailer: mockMailer,
		addr:       cfg.Addr,
		version:    cfg.Version,
		logger:     logger,
		metrics:    newProcessMetrics(),
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
	}
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
			logger.Info("HTTP request", fields...)
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

	if s.config.Webserver.TLS.Enabled {
		tlsConfig := s.config.Webserver.TLS

		// Ensure certificate cache directory exists
		cacheDir := tlsConfig.CacheDirOrDefault()
		if err := os.MkdirAll(cacheDir, 0700); err != nil {
			return fmt.Errorf("failed to create certificate cache directory: %w", err)
		}

		// Create autocert manager for Let's Encrypt
		certManager := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(tlsConfig.Domain),
			Cache:      autocert.DirCache(cacheDir),
			Email:      tlsConfig.Email,
		}

		// HTTPS server (started separately with ListenAndServeTLS)
		tlsServer = newHTTPServer(s.addr, s.router)
		tlsServer.TLSConfig = &tls.Config{
			GetCertificate: certManager.GetCertificate,
			MinVersion:     tls.VersionTLS12,
		}

		// HTTP server for ACME challenges and HTTPS redirect
		httpAddr := fmt.Sprintf(":%d", tlsConfig.HTTPPortOrDefault())
		servers = append(servers, newHTTPServer(httpAddr, certManager.HTTPHandler(http.HandlerFunc(s.redirectToHTTPS))))
	} else {
		// Plain HTTP server
		servers = append(servers, newHTTPServer(s.addr, s.router))
	}

	if s.config.Metrics.Enabled {
		var err error
		metricsServer, err = s.newMetricsServer()
		if err != nil {
			return err
		}
		servers = append(servers, metricsServer)
	}

	serverErr := make(chan error, len(servers)+1)

	// Start HTTP servers
	for _, srv := range servers {
		if srv == metricsServer {
			s.logger.Info("Starting metrics server", "url", metricsServerURL(srv.Addr, s.config.Metrics.PathOrDefault()))
		} else {
			s.logger.Info("Starting HTTP server", "addr", srv.Addr, "url", s.config.Webserver.URL)
		}
		go func(srv *http.Server) {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

func metricsServerURL(addr, path string) string {
	return (&url.URL{Scheme: "http", Host: addr, Path: path}).String()
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
