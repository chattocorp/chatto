package http_server

import (
	"context"
	"net/http"

	"connectrpc.com/authn"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/connectapi"
	graphauth "hmans.de/chatto/internal/graph/auth"
)

const connectAPIPrefix = connectapi.Prefix

func (s *HTTPServer) setupConnectAPI() {
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
		s.mountConnectHandler(handler.ServicePath, serviceHandler)
	}
}

func (s *HTTPServer) mountConnectHandler(servicePath string, serviceHandler http.Handler) {
	handler := http.StripPrefix(connectAPIPrefix, serviceHandler)
	s.router.Any(connectAPIPrefix+servicePath+"*connectPath", func(c *gin.Context) {
		req := s.injectUserIntoContext(c)
		req = req.WithContext(connectapi.WithRequestBaseURL(req.Context(), requestBaseURL(c.Request)))
		handler.ServeHTTP(c.Writer, req)
	})
}

func authenticateConnectRequest(ctx context.Context, _ *http.Request) (any, error) {
	user := graphauth.ForContext(ctx)
	if user == nil {
		return nil, authn.Errorf("authentication required")
	}
	return connectapi.Caller{UserID: user.Id}, nil
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
