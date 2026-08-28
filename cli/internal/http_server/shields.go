package http_server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const shieldCacheControl = "public, max-age=60"
const shieldsIOEndpointURL = "https://img.shields.io/endpoint"
const shieldRoutePrefix = "/.well-known/chatto/shields"

const (
	shieldLabelColor      = "555"
	shieldOnlineColor     = "2ea043"
	shieldRegisteredColor = "0969da"
)

type shieldMetric struct {
	label string
	color string
	count func() (int, error)
}

type shieldEndpointResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Color         string `json:"color"`
	LabelColor    string `json:"labelColor"`
}

func (s *HTTPServer) setupShieldRoutes() {
	s.router.GET(shieldRoutePrefix+"/:name", s.serveShield)
}

func (s *HTTPServer) serveShield(c *gin.Context) {
	if !s.config.Webserver.Shields.Enabled {
		c.Status(http.StatusNotFound)
		return
	}

	metricName, format, ok := parseShieldName(c.Param("name"))
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}

	metric, ok := s.shieldMetric(metricName, c)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}

	if format == "png" {
		setShieldRedirectHeaders(c)
		c.Redirect(http.StatusFound, shieldRedirectURL(s.requestBaseURL(c.Request), metricName))
		return
	}

	count, err := metric.count()
	if err != nil {
		s.logger.Error("Failed to serve shield endpoint", "metric", metricName, "error", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	etag := shieldETag(metricName, count)
	setShieldHeaders(c, etag)
	if requestETagMatches(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return
	}

	body, err := json.Marshal(shieldEndpointResponse{
		SchemaVersion: 1,
		Label:         metric.label,
		Message:       strconv.Itoa(count),
		Color:         metric.color,
		LabelColor:    shieldLabelColor,
	})
	if err != nil {
		s.logger.Error("Failed to encode shield endpoint", "metric", metricName, "error", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}

func parseShieldName(name string) (string, string, bool) {
	for _, format := range []string{"json", "png"} {
		metric, ok := strings.CutSuffix(name, "."+format)
		if ok && metric != "" && !strings.Contains(metric, "/") {
			return metric, format, true
		}
	}
	return "", "", false
}

func (s *HTTPServer) shieldMetric(name string, c *gin.Context) (shieldMetric, bool) {
	switch name {
	case "online":
		return shieldMetric{
			label: "online",
			color: shieldOnlineColor,
			count: func() (int, error) {
				return s.core.LivePresenceCount(c.Request.Context())
			},
		}, true
	case "registered":
		return shieldMetric{
			label: "registered",
			color: shieldRegisteredColor,
			count: func() (int, error) {
				return s.core.CountVerifiedAccounts(c.Request.Context())
			},
		}, true
	default:
		return shieldMetric{}, false
	}
}

func setShieldHeaders(c *gin.Context, etag string) {
	c.Header("Cache-Control", shieldCacheControl)
	c.Header("ETag", etag)
	c.Header("X-Content-Type-Options", "nosniff")
}

func setShieldRedirectHeaders(c *gin.Context) {
	c.Header("Cache-Control", shieldCacheControl)
	c.Header("X-Content-Type-Options", "nosniff")
}

func shieldETag(metric string, count int) string {
	return fmt.Sprintf(`"chatto-shield-%s-%d"`, metric, count)
}

func requestETagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == etag || candidate == "W/"+etag {
			return true
		}
	}
	return false
}

func shieldRedirectURL(baseURL, metric string) string {
	endpoint, err := url.Parse(shieldsIOEndpointURL)
	if err != nil {
		panic(err)
	}
	q := endpoint.Query()
	q.Set("url", strings.TrimRight(baseURL, "/")+shieldRoutePrefix+"/"+metric+".json")
	endpoint.RawQuery = q.Encode()
	return endpoint.String()
}
