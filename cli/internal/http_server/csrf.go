package http_server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	csrfCookieName     = "chatto_csrf"
	csrfHeaderName     = "X-CSRF-Token"
	csrfTokenBytes     = 32
	csrfTokenSeparator = "."
)

func (s *HTTPServer) csrfMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.requiresCSRF(c) {
			var valid bool
			var err error
			if c.Request.URL.Path == "/auth/browser/logout" {
				valid, err = s.validBrowserLogoutCSRF(c)
			} else {
				valid, err = s.validCSRFToken(c)
			}
			if err != nil {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Authentication service temporarily unavailable"})
				return
			}
			if !valid {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "CSRF token missing or invalid"})
				return
			}
		}

		if isSafeHTTPMethod(c.Request.Method) &&
			s.hasCookieCredential(c) &&
			c.Request.URL.Path != serverDiscoveryConnectPath &&
			!isImmutableFrontendAsset(c.Request.URL.Path) {
			if err := s.ensureCSRFToken(c); err != nil {
				if errors.Is(err, errAuthenticationServiceUnavailable) {
					c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Authentication service temporarily unavailable"})
					return
				}
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare CSRF token"})
				return
			}
		}

		c.Next()
	}
}

func (s *HTTPServer) ensureCSRFToken(c *gin.Context) error {
	binding, ok, err := s.csrfBinding(c)
	if err != nil {
		return errAuthenticationServiceUnavailable
	}
	if !ok {
		return nil
	}
	if existingToken, err := c.Cookie(csrfCookieName); err == nil && s.validSignedCSRFToken(existingToken, binding) {
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
	if !s.hasCookieCredential(c) {
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
		"/auth/browser/login",
		"/auth/browser/session/migrate",
		"/auth/browser/revoke-bearer-session",
		"/auth/register",
		"/auth/register/verify-code",
		"/auth/register/complete",
		"/auth/browser/register/complete",
		"/auth/forgot-password",
		"/auth/reset-password",
		"/oauth/token":
		return true
	default:
		return false
	}
}

func (s *HTTPServer) validCSRFToken(c *gin.Context) (bool, error) {
	token, ok := s.matchingCSRFToken(c)
	if !ok {
		return false, nil
	}
	binding, ok, err := s.csrfBinding(c)
	if err != nil {
		return false, err
	}
	return ok && s.validSignedCSRFToken(token, binding), nil
}

// validBrowserLogoutCSRF keeps the signed CSRF proof mandatory while a valid
// cookie authority exists. If every presented handle is already invalid, there
// is no ambient authority left to protect. In that case, the independent
// browser-route proof can authorize clearing the stale browser cookies.
func (s *HTTPServer) validBrowserLogoutCSRF(c *gin.Context) (bool, error) {
	binding, ok, err := s.csrfBinding(c)
	if err != nil {
		return false, err
	}
	if !ok {
		return isJSONAuthenticationRequest(c) &&
			requestsCookieOnlyAuthentication(c) &&
			s.requestIsSameOrigin(c.Request), nil
	}
	token, ok := s.matchingCSRFToken(c)
	return ok && s.validSignedCSRFToken(token, binding), nil
}

func (s *HTTPServer) matchingCSRFToken(c *gin.Context) (string, bool) {
	headerToken := c.GetHeader(csrfHeaderName)
	cookieToken, err := c.Cookie(csrfCookieName)
	if err != nil || headerToken == "" || cookieToken == "" {
		return "", false
	}

	if subtle.ConstantTimeCompare([]byte(headerToken), []byte(cookieToken)) != 1 {
		return "", false
	}
	return cookieToken, true
}

type csrfBinding struct {
	userID         string
	authGeneration uint64
}

func (s *HTTPServer) csrfBinding(c *gin.Context) (csrfBinding, bool, error) {
	credential, ok, err := s.cookiePresentedCredential(c)
	if err != nil {
		return csrfBinding{}, false, err
	}
	if !ok {
		return csrfBinding{}, false, nil
	}
	return csrfBindingForSession(credential.auth.UserID, credential.cookieRecord), true, nil
}

func (s *HTTPServer) hasCookieCredential(c *gin.Context) bool {
	cookies, err := browserSessionCookies(c.Request)
	return err != nil || len(cookies) != 0
}

func csrfBindingForSession(userID string, record *runtimestatev1.CookieSession) csrfBinding {
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
		int(s.config.Auth.TokenTTLOrDefault().Seconds()),
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
