package http_server

import (
	"context"
	"net/http"
	"net/url"

	"connectrpc.com/authn"
	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/connectapi"
)

const connectAPIPrefix = connectapi.Prefix

func (s *HTTPServer) setupConnectAPI() {
	if s.logger == nil {
		s.logger = log.WithPrefix("server.HTTP")
	}
	s.setupConnectAPIOnRouter(s.router)
}

func (s *HTTPServer) newOperatorAPIServer() *http.Server {
	if s.logger == nil {
		s.logger = log.WithPrefix("server.HTTP")
	}
	router := gin.New()
	router.Use(gin.Recovery())
	if s.config.Webserver.RequestLoggingEnabled() {
		router.Use(requestLogger(s.logger))
	}
	s.setupOperatorConnectAPI(router)
	return newHTTPServer(s.config.OperatorAPI.SocketPathOrDefault(), router)
}

func (s *HTTPServer) setupConnectAPIOnRouter(router gin.IRouter) {
	api := connectapi.New(s.core, s.config, s.version)
	authMiddleware := authn.NewMiddleware(authenticateConnectRequest, connectapi.HandlerOptions()...)
	for _, handler := range api.Handlers() {
		serviceHandler := handler.Handler
		switch handler.AuthPolicy {
		case connectapi.AuthPolicyPublic:
		case connectapi.AuthPolicyAuthenticatedUser:
			serviceHandler = authMiddleware.Wrap(serviceHandler)
		default:
			panic("unknown ConnectRPC auth policy for " + handler.ServicePath)
		}
		s.mountConnectHandler(router, handler.ServicePath, serviceHandler)
	}
}

func (s *HTTPServer) setupOperatorConnectAPI(router gin.IRouter) {
	api := connectapi.New(s.core, s.config, s.version)
	for _, handler := range api.OperatorHandlers() {
		s.mountOperatorConnectHandler(router, handler.ServicePath, handler.Handler)
	}
}

func (s *HTTPServer) mountConnectHandler(router gin.IRouter, servicePath string, serviceHandler http.Handler) {
	handler := http.StripPrefix(connectAPIPrefix, serviceHandler)
	router.Any(connectAPIPrefix+servicePath+"*connectPath", func(c *gin.Context) {
		req := s.injectUserIntoContext(c)
		req = req.WithContext(connectapi.WithRequestBaseURL(req.Context(), s.requestBaseURL(c.Request)))
		req = req.WithContext(connectapi.WithBrowserSessionCreator(req.Context(), func(ctx context.Context, userID, source string) (connectapi.BrowserSession, error) {
			return s.createConnectBrowserSession(c, userID, source)
		}))
		handler.ServeHTTP(c.Writer, req)
	})
}

func (s *HTTPServer) mountOperatorConnectHandler(router gin.IRouter, servicePath string, serviceHandler http.Handler) {
	handler := http.StripPrefix(connectAPIPrefix, serviceHandler)
	router.Any(connectAPIPrefix+servicePath+"*connectPath", func(c *gin.Context) {
		req := c.Request.WithContext(connectapi.WithRequestBaseURL(c.Request.Context(), s.requestBaseURL(c.Request)))
		handler.ServeHTTP(c.Writer, req)
	})
}

func authenticateConnectRequest(ctx context.Context, _ *http.Request) (any, error) {
	user := authctx.ForContext(ctx)
	if user == nil {
		return nil, authn.Errorf("authentication required")
	}
	return connectapi.Caller{UserID: user.Id}, nil
}

func (s *HTTPServer) requestBaseURL(r *http.Request) string {
	if baseURL := configuredWebserverOrigin(s.config.Webserver.URL); baseURL != "" {
		return baseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func configuredWebserverOrigin(raw string) string {
	if raw == "" {
		return ""
	}
	base, err := url.Parse(raw)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return ""
	}
	return base.Scheme + "://" + base.Host
}
