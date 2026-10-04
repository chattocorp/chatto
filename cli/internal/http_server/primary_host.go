package http_server

import (
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// redirectFrontendToPrimaryHost runs only when serving an HTML frontend page.
// Exact configured aliases may redirect; forwarded headers and unknown hosts
// never select an alias or the destination. Protocols and callbacks keep their
// original origin, including the frontend's OAuth popup callback.
func (s *HTTPServer) redirectFrontendToPrimaryHost(c *gin.Context) bool {
	if !s.config.Webserver.RedirectToPrimaryHost ||
		(c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) ||
		c.IsWebsocket() || !acceptsFrontendHTML(c.Request) {
		return false
	}
	requestPath := c.Request.URL.Path
	for _, prefix := range []string{"/servers/callback", "/oauth", "/mcp", "/_app", "/icons", "/healthz", "/readyz"} {
		if hasPathSegmentPrefix(requestPath, prefix) {
			return false
		}
	}
	if isReservedNonFrontendPath(requestPath) {
		return false
	}
	primary := configuredWebserverOrigin(s.config.Webserver.URL)
	origin, ok := s.configuredOriginForHost(c.Request.Host)
	if !ok || primary == "" || origin == primary {
		return false
	}
	primaryURL, err := url.Parse(primary)
	if err != nil {
		return false
	}
	target := url.URL{
		Scheme: primaryURL.Scheme, Host: primaryURL.Host,
		Path: c.Request.URL.Path, RawPath: c.Request.URL.RawPath,
		RawQuery: c.Request.URL.RawQuery, ForceQuery: c.Request.URL.ForceQuery,
	}
	c.Header("Cache-Control", "no-store")
	c.Redirect(http.StatusTemporaryRedirect, target.String())
	c.Abort()
	return true
}

func acceptsFrontendHTML(r *http.Request) bool {
	if mode := r.Header.Get("Sec-Fetch-Mode"); mode != "" && mode != "navigate" {
		return false
	}
	for _, entry := range strings.Split(r.Header.Get("Accept"), ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(entry))
		if err != nil || mediaType != "text/html" {
			continue
		}
		if quality, present := params["q"]; present {
			value, err := strconv.ParseFloat(quality, 64)
			if err != nil || !(value > 0 && value <= 1) {
				continue
			}
		}
		return true
	}
	return false
}
