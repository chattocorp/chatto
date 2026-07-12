//go:build !test_endpoints

package http_server

import (
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/email"
)

// createMailer creates the configured transactional email sender for production builds.
// Returns (nil, mailer) since mock mailer is not used in production.
func createMailer(emailConfig config.EmailConfig, smtpConfig config.SMTPConfig) (*email.MockSender, email.Sender) {
	return nil, email.NewSender(emailConfig, smtpConfig)
}

// registerTestEndpoints is a no-op in production builds.
// Test endpoints are only available when built with -tags test_endpoints.
func registerTestEndpoints(_ *gin.RouterGroup, _ *HTTPServer) {
	// No-op in production
}

// registerTestWebhookEndpoints is a no-op in production builds.
func registerTestWebhookEndpoints(_ *gin.RouterGroup, _ *HTTPServer) {
	// No-op in production
}
