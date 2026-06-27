package http_server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"strings"

	"connectrpc.com/authn"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
	graphauth "hmans.de/chatto/internal/graph/auth"
)

const connectAPIPrefix = connectapi.Prefix

func (s *HTTPServer) setupConnectAPI() {
	api := connectapi.New(s.core, s.config, s.version)
	authMiddleware := authn.NewMiddleware(authenticateConnectRequest, connectapi.HandlerOptions()...)
	adminAuthMiddleware := authn.NewMiddleware(func(ctx context.Context, req *http.Request) (any, error) {
		return authenticateAdminConnectRequest(ctx, req, s.config.AdminAPI)
	}, connectapi.HandlerOptions()...)
	for _, handler := range api.Handlers() {
		serviceHandler := handler.Handler
		switch handler.AuthPolicy {
		case connectapi.AuthPolicyPublic:
		case connectapi.AuthPolicyAuthenticatedUser:
			serviceHandler = authMiddleware.Wrap(serviceHandler)
		case connectapi.AuthPolicyAdminToken:
			serviceHandler = adminAuthMiddleware.Wrap(serviceHandler)
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

func authenticateAdminConnectRequest(_ context.Context, req *http.Request, cfg config.AdminAPIConfig) (any, error) {
	if !cfg.Enabled {
		return nil, authn.Errorf("admin API is disabled")
	}
	if req == nil {
		return nil, authn.Errorf("admin token required")
	}
	token, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		return nil, authn.Errorf("admin token required")
	}
	token = strings.TrimSpace(token)
	tokenHash := sha256.Sum256([]byte(token))
	for _, configured := range cfg.Tokens {
		configuredHash := sha256.Sum256([]byte(configured.Token))
		if subtle.ConstantTimeCompare(tokenHash[:], configuredHash[:]) != 1 {
			continue
		}
		allowed, err := adminRequestSourceAllowed(req, configured)
		if err != nil {
			return nil, authn.Errorf("invalid admin API configuration")
		}
		if !allowed {
			return nil, authn.Errorf("admin token required")
		}
		return connectapi.AdminCaller{}, nil
	}
	return nil, authn.Errorf("admin token required")
}

func adminRequestSourceAllowed(req *http.Request, token config.AdminAPITokenConfig) (bool, error) {
	ip, err := requestRemoteIP(req)
	if err != nil {
		return false, nil
	}
	nets, err := token.AllowedIPNetsOrDefault()
	if err != nil {
		return false, err
	}
	for _, allowed := range nets {
		if allowed.Contains(ip) {
			return true, nil
		}
	}
	return false, nil
}

func requestRemoteIP(req *http.Request) (net.IP, error) {
	if req == nil || strings.TrimSpace(req.RemoteAddr) == "" {
		return nil, errors.New("missing remote address")
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return nil, errors.New("invalid remote address")
	}
	return ip, nil
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
