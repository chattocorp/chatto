package http_server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	csrfCookieName              = "chatto_csrf"
	csrfHeaderName              = "X-CSRF-Token"
	csrfGraphQLRequestHeader    = "X-REQUEST-TYPE"
	csrfGraphQLRequestHeaderVal = "GraphQL"
	csrfSessionKey              = "csrf_token"
	csrfTokenBytes              = 32
)

func (s *HTTPServer) csrfMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.requiresCSRF(c) && !s.validCSRFToken(c) && !validGraphQLRequestHeader(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "CSRF token missing or invalid"})
			return
		}

		session := sessions.Default(c)
		if isSafeHTTPMethod(c.Request.Method) && session.Get("user_id") != nil {
			if err := s.ensureCSRFToken(c, session); err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare CSRF token"})
				return
			}
		}

		c.Next()
	}
}

func (s *HTTPServer) ensureCSRFToken(c *gin.Context, session sessions.Session) error {
	token, ok := session.Get(csrfSessionKey).(string)
	if !ok || token == "" {
		generated, err := generateCSRFToken()
		if err != nil {
			return err
		}
		token = generated
		session.Set(csrfSessionKey, token)
		if err := session.Save(); err != nil {
			return err
		}
	}
	s.setCSRFCookie(c, token)
	return nil
}

func (s *HTTPServer) requiresCSRF(c *gin.Context) bool {
	if isSafeHTTPMethod(c.Request.Method) {
		return false
	}
	if sessions.Default(c).Get("user_id") == nil {
		return false
	}
	return !isCSRFExemptUnsafePath(c.Request.URL.Path)
}

func isCSRFExemptUnsafePath(path string) bool {
	if strings.HasPrefix(path, "/auth/test/") || strings.HasPrefix(path, "/webhooks/") {
		return true
	}
	switch path {
	case "/auth/login",
		"/auth/register",
		"/auth/register/verify-code",
		"/auth/register/complete",
		"/auth/forgot-password",
		"/auth/reset-password",
		"/oauth/token":
		return true
	default:
		return false
	}
}

func (s *HTTPServer) validCSRFToken(c *gin.Context) bool {
	sessionToken, ok := sessions.Default(c).Get(csrfSessionKey).(string)
	if !ok || sessionToken == "" {
		return false
	}

	headerToken := c.GetHeader(csrfHeaderName)
	cookieToken, err := c.Cookie(csrfCookieName)
	if err != nil || headerToken == "" || cookieToken == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(headerToken), []byte(sessionToken)) == 1 &&
		subtle.ConstantTimeCompare([]byte(cookieToken), []byte(sessionToken)) == 1
}

func validGraphQLRequestHeader(c *gin.Context) bool {
	return c.Request.URL.Path == "/api/graphql" &&
		strings.EqualFold(c.GetHeader(csrfGraphQLRequestHeader), csrfGraphQLRequestHeaderVal)
}

func isSafeHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func generateCSRFToken() (string, error) {
	buf := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *HTTPServer) setCSRFCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		csrfCookieName,
		token,
		60*60*24*90,
		"/",
		"",
		strings.HasPrefix(s.config.Webserver.URL, "https"),
		false,
	)
}

func clearCSRFCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(csrfCookieName, "", -1, "/", "", false, false)
}
