package http_server

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const shieldCacheControl = "public, max-age=60"

var (
	shieldLabelColor      = color.RGBA{R: 85, G: 85, B: 85, A: 255}
	shieldOnlineColor     = color.RGBA{R: 46, G: 160, B: 67, A: 255}
	shieldRegisteredColor = color.RGBA{R: 9, G: 105, B: 218, A: 255}
	shieldTextColor       = color.RGBA{R: 255, G: 255, B: 255, A: 255}
)

type shieldMetric struct {
	label string
	color color.Color
	count func() (int, error)
}

func (s *HTTPServer) setupShieldRoutes() {
	s.router.GET("/shields/:name", s.serveShield)
}

func (s *HTTPServer) serveShield(c *gin.Context) {
	if !s.config.Shields.Enabled {
		c.Status(http.StatusNotFound)
		return
	}

	metricName, ok := parseShieldName(c.Param("name"))
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}

	metric, ok := s.shieldMetric(metricName, c)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}

	count, err := metric.count()
	if err != nil {
		s.logger.Error("Failed to render shield", "metric", metricName, "error", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	etag := shieldETag(metricName, count)
	setShieldHeaders(c, etag)
	if requestETagMatches(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return
	}

	body, err := renderShieldPNG(metric.label, strconv.Itoa(count), metric.color)
	if err != nil {
		s.logger.Error("Failed to encode shield", "metric", metricName, "error", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(http.StatusOK, "image/png", body)
}

func parseShieldName(name string) (string, bool) {
	metric, ok := strings.CutSuffix(name, ".png")
	if !ok || metric == "" || strings.Contains(metric, "/") {
		return "", false
	}
	return metric, true
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

func renderShieldPNG(label, value string, valueColor color.Color) ([]byte, error) {
	const (
		height      = 20
		textPadding = 6
	)

	labelWidth := shieldTextWidth(label) + textPadding*2
	valueWidth := shieldTextWidth(value) + textPadding*2
	width := labelWidth + valueWidth

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, image.Rect(0, 0, labelWidth, height), image.NewUniform(shieldLabelColor), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(labelWidth, 0, width, height), image.NewUniform(valueColor), image.Point{}, draw.Src)

	drawShieldText(img, textPadding, label)
	drawShieldText(img, labelWidth+textPadding, value)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func shieldTextWidth(text string) int {
	return font.MeasureString(basicfont.Face7x13, text).Ceil()
}

func drawShieldText(img draw.Image, x int, text string) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(shieldTextColor),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, 14),
	}
	d.DrawString(text)
}
