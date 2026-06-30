package http_server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	csrfCookieName     = "chatto_csrf"
	csrfHeaderName     = "X-CSRF-Token"
	csrfTokenBytes     = 32
	csrfTokenSeparator = "."
)

func (s *HTTPServer) csrfMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.requiresCSRF(c) && !s.validCSRFToken(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "CSRF token missing or invalid"})
			return
		}

		session := sessions.Default(c)
		if isSafeHTTPMethod(c.Request.Method) && hasCookieCredential(session) {
			if err := s.ensureCSRFToken(c); err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare CSRF token"})
				return
			}
		}

		c.Next()
	}
}

func (s *HTTPServer) ensureCSRFToken(c *gin.Context) error {
	binding, ok := s.csrfBinding(c)
	if !ok {
		return nil
	}
	if existingToken, err := c.Cookie(csrfCookieName); err == nil && s.validSignedCSRFToken(existingToken, binding) {
		s.setCSRFCookie(c, existingToken)
		return nil
	}
	token, err := s.generateCSRFToken(binding)
	if err != nil {
		return err
	}
	s.setCSRFCookie(c, token)
	return nil
}

func (s *HTTPServer) requiresCSRF(c *gin.Context) bool {
	if isSafeHTTPMethod(c.Request.Method) {
		return false
	}
	if !hasCookieCredential(sessions.Default(c)) {
		return false
	}
	return !isCSRFExemptUnsafePath(c.Request.URL.Path)
}

func isCSRFExemptUnsafePath(path string) bool {
	if strings.HasPrefix(path, "/auth/test/") || strings.HasPrefix(path, "/webhooks/") {
		return true
	}
	// ConnectRPC is programmatic API traffic. Browsers cannot submit
	// application/proto or application/connect+proto via plain HTML forms.
	if strings.HasPrefix(path, connectAPIPrefix+"/") {
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
	headerToken := c.GetHeader(csrfHeaderName)
	cookieToken, err := c.Cookie(csrfCookieName)
	if err != nil || headerToken == "" || cookieToken == "" {
		return false
	}

	if subtle.ConstantTimeCompare([]byte(headerToken), []byte(cookieToken)) != 1 {
		return false
	}

	binding, ok := s.csrfBinding(c)
	return ok && s.validSignedCSRFToken(cookieToken, binding)
}

type csrfBinding struct {
	userID         string
	authGeneration uint64
}

func (s *HTTPServer) csrfBinding(c *gin.Context) (csrfBinding, bool) {
	credential, ok := s.cookiePresentedCredential(c)
	if !ok {
		return csrfBinding{}, false
	}
	return csrfBindingForSession(credential.auth.UserID, credential.cookieRecord), true
}

func hasCookieCredential(session sessions.Session) bool {
	_, ok := cookieCredentialFromSession(session)
	return ok
}

func csrfBindingForSession(userID string, record *corev1.CookieSession) csrfBinding {
	if record == nil {
		return csrfBinding{userID: userID}
	}
	return csrfBinding{
		userID:         userID,
		authGeneration: record.GetAuthGeneration(),
	}
}

func (s *HTTPServer) generateCSRFToken(binding csrfBinding) (string, error) {
	nonce, err := generateCSRFNonce()
	if err != nil {
		return "", err
	}
	return nonce + csrfTokenSeparator + s.signCSRFToken(nonce, binding), nil
}

func (s *HTTPServer) validSignedCSRFToken(token string, binding csrfBinding) bool {
	nonce, signature, ok := strings.Cut(token, csrfTokenSeparator)
	if !ok || nonce == "" || signature == "" {
		return false
	}
	expected := s.signCSRFToken(nonce, binding)
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
}

func (s *HTTPServer) signCSRFToken(nonce string, binding csrfBinding) string {
	mac := hmac.New(sha256.New, []byte(s.config.Webserver.CookieSigningSecret))
	mac.Write([]byte(nonce))
	mac.Write([]byte{0})
	mac.Write([]byte(binding.userID))
	mac.Write([]byte{0})
	mac.Write([]byte(strconv.FormatUint(binding.authGeneration, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func isSafeHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func generateCSRFNonce() (string, error) {
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
