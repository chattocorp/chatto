package http_server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const maxAuditUserAgentBytes = 256

func (s *HTTPServer) auditRequestMetadata(c *gin.Context) *corev1.AuditRequestMetadata {
	metadata := &corev1.AuditRequestMetadata{}
	if c == nil || c.Request == nil {
		return metadata
	}

	metadata.UserAgent = capAuditUserAgent(c.Request.UserAgent())
	if ip := auditSourceIP(c); ip != "" && s.config.Webserver.CookieSigningSecret != "" {
		metadata.IpHash = hmacSHA256Hex(s.config.Webserver.CookieSigningSecret, ip)
	}
	return metadata
}

func (s *HTTPServer) requestContextWithAuditMetadata(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	ctx := core.WithAuditRequestMetadata(c.Request.Context(), s.auditRequestMetadata(c))
	c.Request = c.Request.WithContext(ctx)
}

func capAuditUserAgent(userAgent string) string {
	userAgent = strings.ToValidUTF8(userAgent, "")
	if len(userAgent) <= maxAuditUserAgentBytes {
		return userAgent
	}
	capped := userAgent[:maxAuditUserAgentBytes]
	for !utf8.ValidString(capped) && len(capped) > 0 {
		capped = capped[:len(capped)-1]
	}
	return capped
}

func auditSourceIP(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return c.ClientIP()
}

func hmacSHA256Hex(secret, value string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}
