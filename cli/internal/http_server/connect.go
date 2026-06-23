package http_server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/connectapi"
)

const connectAPIPrefix = connectapi.Prefix

func (s *HTTPServer) setupConnectAPI() {
	api := connectapi.New(s.core, s.config, s.version)
	for _, handler := range api.Handlers() {
		s.mountConnectHandler(handler.ServicePath, handler.Handler)
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

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
