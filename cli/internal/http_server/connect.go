package http_server

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/authctx"
	"github.com/chattocorp/chatto/cli/internal/connectapi"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/discovery/v1/discoveryv1connect"
	"github.com/chattocorp/chatto/cli/internal/search"
	"github.com/gin-gonic/gin"
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
	api := s.connectAPI
	if api == nil {
		api = connectapi.New(s.core, s.config, s.version, connectapi.WithMessageSearchProviderClient(search.NewClient(s.nc)), connectapi.WithEmailSender(s.mailer))
		s.connectAPI = api
	}
	authMiddleware := authn.NewMiddleware(authenticateConnectRequest, connectapi.HandlerOptionsForWebserver(s.config.Webserver)...)
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
		if handler.ServicePath == "/"+discoveryv1connect.ServerDiscoveryServiceName+"/" {
			// Serve the canonical response through the same codec and cache logic,
			// without session hydration or caller-controlled RPC parameters.
			router.GET(serverDiscoveryWellKnownPath, func(c *gin.Context) {
				req := c.Request.Clone(connectapi.WithRequestBaseURL(c.Request.Context(), s.requestBaseURL(c.Request)))
				req.URL.Path = discoveryv1connect.ServerDiscoveryServiceGetServerProcedure
				req.URL.RawQuery = "connect=v1&encoding=json&message=%7B%7D"
				serviceHandler.ServeHTTP(c.Writer, req)
			})
		}
	}
}

func (s *HTTPServer) setupOperatorConnectAPI(router gin.IRouter) {
	api := connectapi.New(s.core, s.config, s.version)
	for _, handler := range api.OperatorHandlers() {
		s.mountOperatorConnectHandler(router, handler.ServicePath, handler.Handler)
	}
}

func (s *HTTPServer) mountConnectHandler(router gin.IRouter, servicePath string, serviceHandler http.Handler) {
	handler := http.StripPrefix(connectAPIPrefix, withEmptyConnectGETMessage(serviceHandler))
	router.Any(connectAPIPrefix+servicePath+"*connectPath", func(c *gin.Context) {
		req := s.injectUserIntoContext(c)
		if s.requestIsSameOrigin(c.Request) {
			req = req.WithContext(connectapi.WithRegistrationAdmissionCompleter(req.Context(), func(invitationID string) { clearRegistrationAdmission(c, invitationID) }))
		}
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

// withEmptyConnectGETMessage supplies an empty request when a caller omits
// message on an uncompressed GET. Connect still enforces method safety and
// request validation. Explicit messages and compressed requests pass unchanged.
func withEmptyConnectGETMessage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err == nil && !query.Has("message") && !query.Has("compression") {
			var message string
			switch query.Get("encoding") {
			case "json":
				message = "{}"
			case "proto":
			default:
				next.ServeHTTP(w, r)
				return
			}
			if query.Get("base64") == "1" {
				message = base64.RawURLEncoding.EncodeToString([]byte(message))
			}
			query.Set("message", message)
			r = r.Clone(r.Context())
			r.URL.RawQuery = query.Encode()
		}
		next.ServeHTTP(w, r)
	})
}

func authenticateConnectRequest(ctx context.Context, _ *http.Request) (any, error) {
	if err := authenticationValidationError(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("authentication service temporarily unavailable"))
	}
	user := authctx.ForContext(ctx)
	if user == nil {
		return nil, authn.Errorf("authentication required")
	}
	return connectapi.Caller{UserID: user.Id}, nil
}

// requestBaseURL returns the public origin for absolute URLs in a response to
// r. A request to webserver.url or to an exact webserver.allowed_origins host
// gets that configured origin, so a client that uses a hostname alias receives
// URLs on the same alias. Another host gets webserver.url. Without
// webserver.url, the direct request scheme and host apply.
func (s *HTTPServer) requestBaseURL(r *http.Request) string {
	if origin, ok := s.configuredOriginForHost(r.Host); ok {
		return origin
	}
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
	return canonicalOrigin(base)
}
