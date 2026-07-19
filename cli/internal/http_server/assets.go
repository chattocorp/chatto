package http_server

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/signedurl"
)

const protectedAssetCacheControl = "private, no-store"

func (s *HTTPServer) setupAssetRoutes() {
	// Server assets use *path which catches everything including /t/signedPath for transforms
	// The serveServerAsset handler detects and routes transform requests appropriately
	// These handlers probe both NATS and S3 backends automatically
	s.router.GET("/assets/server/*path", s.serveServerAsset)
	s.router.GET("/assets/files/:assetID", s.serveStableAttachment)
	s.router.GET("/assets/files/:assetID/image/:dimensions/:fit", s.serveStableTransformedAttachment)
}

// transformRequest holds the parameters for a transformed asset request.
// This allows sharing the transformation logic between different asset types.
type transformRequest struct {
	// ResourceID1 and ResourceID2 are used for signing verification.
	// For attachments: ("attachment", attachmentID)
	// For server assets: ("server", key)
	ResourceID1 string
	ResourceID2 string
	SignedPath  string
	// CachePrefix distinguishes cache keys between asset types (e.g., "attachment", "server")
	CachePrefix string
	// AssetID is used for ETag generation and logging
	AssetID string
	// JPEGQuality overrides the default quality for opaque static derivatives.
	JPEGQuality int
	// FetchAsset returns the asset data and content type.
	// The reader will be closed if it implements io.Closer.
	FetchAsset func(ctx context.Context) (io.Reader, string, error)
	// Authorize checks if access is allowed. Return true if authorized.
	// If nil, asset is considered public and no authorization is needed.
	Authorize func(c *gin.Context) bool
}

type assetDeliveryMode int

const (
	deliveryChattoStream assetDeliveryMode = iota
	deliveryS3Redirect
)

const largeAttachmentRedirectThreshold = 32 << 20

func protectedAssetDeliveryMode(attachment *corev1.Attachment) assetDeliveryMode {
	if attachment == nil {
		return deliveryChattoStream
	}
	if !attachmentCanUsePresignedRedirect(attachment.GetContentType()) {
		return deliveryChattoStream
	}
	if storage := attachment.GetStorage(); storage != nil {
		if _, ok := storage.GetAsset().(*corev1.DeprecatedAsset_S3); !ok {
			return deliveryChattoStream
		}
	}
	contentType := strings.ToLower(attachment.GetContentType())
	if strings.HasPrefix(contentType, "video/") || strings.HasPrefix(contentType, "audio/") {
		return deliveryS3Redirect
	}
	if attachment.GetSize() >= largeAttachmentRedirectThreshold {
		return deliveryS3Redirect
	}
	return deliveryChattoStream
}

func (s *HTTPServer) serveServerAsset(c *gin.Context) {
	path := c.Param("path")

	// Trim leading slash
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	// /assets/server/* is intentionally unauthenticated and may only serve
	// explicitly public, server-scoped assets. Classify the base key before
	// transform signature parsing, derivative-cache access, object reads, or
	// image transformation so shared-store private objects always look absent.
	key := path
	signedPath := ""
	transformRequest := false
	if idx := strings.LastIndex(path, "/t/"); idx != -1 {
		transformRequest = true
		key = path[:idx]
		signedPath = path[idx+3:]
	}
	location, public := s.core.ResolvePublicServerAsset(c.Request.Context(), key)
	if key == "" || !public {
		c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found"})
		return
	}
	if transformRequest && signedPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found"})
		return
	}

	// Check if this is a transform request: path ends with /t/{signedPath}
	// Pattern: {key}/t/{signedPath}
	if transformRequest {
		s.serveTransformedServerAsset(c, key, signedPath, location)
		return
	}

	s.logger.Debug("Serving server asset", "asset_id", key)

	// Probe both NATS and S3 backends
	reader, info, err := s.core.GetPublicServerAsset(c.Request.Context(), location)
	if err != nil {
		s.logger.Error("Failed to get server asset", "error", err, "asset_id", key)
		c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found"})
		return
	}
	// Close the reader if it implements io.Closer
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}

	// Get content type, fall back to extension-based detection
	contentType := info.ContentType
	if contentType == "" {
		contentType = getContentType(key)
	}

	// Immutable asset - cache forever
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("ETag", fmt.Sprintf("\"%s\"", key))
	c.Header("Vary", "Accept-Encoding")

	c.DataFromReader(
		http.StatusOK,
		info.Size,
		contentType,
		reader,
		nil,
	)
}

// serveStableAttachment serves the canonical authenticated asset URL:
//
//	/assets/files/{assetID}
//
// The URL identifies the binary, while the access ticket (or, for API clients,
// the request's cookie/bearer token) authorizes access.
func (s *HTTPServer) serveStableAttachment(c *gin.Context) {
	ctx := c.Request.Context()
	assetID := c.Param("assetID")

	attachment, ok := s.resolveStableAttachment(c, ctx, assetID, nil)
	if !ok {
		return
	}

	if protectedAssetDeliveryMode(attachment) == deliveryS3Redirect {
		if presignedURL, err := s.core.TryPresignedAttachmentURL(ctx, attachment, core.S3AssetRedirectTTL); err == nil {
			c.Header("Cache-Control", protectedAssetCacheControl)
			c.Redirect(http.StatusFound, presignedURL)
			return
		}
	}

	reader, info, err := s.core.GetAttachmentReader(ctx, attachment)
	if err != nil {
		s.logger.Error("Failed to get stable attachment", "error", err, "attachment_id", assetID)
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}

	contentType := info.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	setOriginalAttachmentSecurityHeaders(c, contentType)

	c.Header("Cache-Control", protectedAssetCacheControl)
	c.Header("ETag", fmt.Sprintf("\"%s\"", assetID))
	c.Header("Vary", "Accept-Encoding, Authorization, Cookie")
	// Chatto-backed streams are sequential. Seekable media delivery requires an
	// S3 redirect, whose object server handles byte ranges directly.
	c.Header("Accept-Ranges", "none")
	c.DataFromReader(http.StatusOK, info.Size, contentType, reader, nil)
}

const originalAttachmentSandboxCSP = "sandbox"

func setOriginalAttachmentSecurityHeaders(c *gin.Context, contentType string) {
	c.Header("X-Content-Type-Options", "nosniff")
	if originalAttachmentNeedsSandbox(contentType) {
		c.Header("Content-Security-Policy", originalAttachmentSandboxCSP)
	}
}

func attachmentCanUsePresignedRedirect(contentType string) bool {
	return !originalAttachmentNeedsSandbox(contentType)
}

func originalAttachmentNeedsSandbox(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}
	mediaType = strings.ToLower(mediaType)

	switch mediaType {
	case "text/html", "application/xhtml+xml", "image/svg+xml", "application/xml", "text/xml":
		return true
	default:
		return strings.HasSuffix(mediaType, "+xml")
	}
}

// serveStableTransformedAttachment serves an authenticated image derivative:
//
//	/assets/files/{assetID}/image/{width}x{height}/{fit}
//
// Transform dimensions remain visible and stable in the URL. Authorization
// comes from the asset-scoped access ticket or request credentials.
func (s *HTTPServer) serveStableTransformedAttachment(c *gin.Context) {
	ctx := c.Request.Context()
	assetID := c.Param("assetID")
	params, err := parseStableTransformParams(c.Param("dimensions"), c.Param("fit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	attachment, ok := s.resolveStableAttachment(c, ctx, assetID, params)
	if !ok {
		return
	}

	s.serveTransformedAssetWithParams(c, transformRequest{
		CachePrefix: AttachmentStableCachePrefix,
		AssetID:     assetID,
		JPEGQuality: AttachmentDerivativeJPEGQuality,
		FetchAsset: func(ctx context.Context) (io.Reader, string, error) {
			reader, info, err := s.core.GetAttachmentReader(ctx, attachment)
			if err != nil {
				return nil, "", err
			}
			return reader, info.ContentType, nil
		},
		Authorize: func(c *gin.Context) bool { return true },
	}, params)
}

const (
	// AttachmentDerivativeJPEGQuality keeps displayed attachment images compact
	// without changing the encoding quality of public server assets.
	AttachmentDerivativeJPEGQuality = 75
	// AttachmentStableCachePrefix is versioned whenever attachment derivative
	// encoding changes so older cached bytes cannot be reused.
	AttachmentStableCachePrefix = core.AttachmentDerivativeCacheResource
)

func parseStableTransformParams(dimensions, fit string) (*signedurl.TransformParams, error) {
	widthText, heightText, ok := strings.Cut(dimensions, "x")
	if !ok {
		return nil, fmt.Errorf("invalid dimensions")
	}
	width, err := strconv.Atoi(widthText)
	if err != nil {
		return nil, fmt.Errorf("invalid width")
	}
	height, err := strconv.Atoi(heightText)
	if err != nil {
		return nil, fmt.Errorf("invalid height")
	}
	params := &signedurl.TransformParams{Width: width, Height: height, Fit: fit}
	if params.Width < 1 || params.Width > 2048 {
		return nil, fmt.Errorf("width out of range [1, 2048]: %d", params.Width)
	}
	if params.Height < 1 || params.Height > 2048 {
		return nil, fmt.Errorf("height out of range [1, 2048]: %d", params.Height)
	}
	if params.Fit != "contain" && params.Fit != "cover" && params.Fit != "exact" {
		return nil, fmt.Errorf("invalid fit mode: %s", params.Fit)
	}
	return params, nil
}

func (s *HTTPServer) resolveStableAttachment(c *gin.Context, ctx context.Context, assetID string, params *signedurl.TransformParams) (*corev1.Attachment, bool) {
	if assetID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return nil, false
	}

	userID, ok := s.resolveStableAssetViewerID(c, assetID, params)
	if !ok {
		return nil, false
	}

	declared, ok := s.core.Assets.AssetCreation(assetID)
	if !ok || declared == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return nil, false
	}
	roomID, ok := s.core.Assets.AssetRoomID(assetID)
	if !ok {
		s.logger.Warn("Asset has no room scope", "attachment_id", assetID)
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return nil, false
	}

	kind, err := s.core.FindRoomKind(ctx, roomID)
	if err != nil {
		s.logger.Error("Failed to resolve room kind for stable attachment auth", "error", err, "room_id", roomID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return nil, false
	}
	isMember, err := s.core.RoomMembershipExists(ctx, kind, userID, roomID)
	if err != nil {
		s.logger.Error("Failed to check stable attachment room membership", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return nil, false
	}
	if !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: not a member of the room"})
		return nil, false
	}

	attachment := core.AttachmentFromAsset(declared.GetAsset())
	if attachment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return nil, false
	}
	attachment.RoomId = roomID
	return attachment, true
}

func (s *HTTPServer) resolveStableAssetViewerID(c *gin.Context, assetID string, params *signedurl.TransformParams) (string, bool) {
	if access := c.Query("access"); access != "" {
		ticket, err := signedurl.ParseSignedAssetAccessTicket(s.config.Core.Assets.SigningSecret, access)
		if err != nil {
			s.logger.Warn("Invalid asset access ticket", "error", err, "asset_id", assetID)
			c.JSON(http.StatusForbidden, gin.H{"error": "Invalid asset access ticket"})
			return "", false
		}
		if ticket.Expired(time.Now().Unix()) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Asset access ticket expired"})
			return "", false
		}
		if ticket.AssetID != assetID {
			c.JSON(http.StatusForbidden, gin.H{"error": "Asset access ticket does not match asset"})
			return "", false
		}
		if !ticket.MatchesTransform(params) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Asset access ticket does not match derivative"})
			return "", false
		}
		return ticket.UserID, true
	}

	if params != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Asset derivative URL requires a signed access ticket"})
		return "", false
	}

	reqWithUser := s.injectUserIntoContext(c)
	if authenticationValidationError(reqWithUser.Context()) != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Authentication service temporarily unavailable"})
		return "", false
	}
	user := authctx.ForContext(reqWithUser.Context())
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return "", false
	}
	return user.Id, true
}

// serveTransformedAsset handles the common logic for serving transformed images.
// It parses the signed path, checks cache, fetches the asset, transforms it, and serves the result.
func (s *HTTPServer) serveTransformedAsset(c *gin.Context, req transformRequest) {
	// Parse and verify the signed path
	params, err := signedurl.ParseSignedTransformPath(s.config.Core.Assets.SigningSecret, req.ResourceID1, req.ResourceID2, req.SignedPath)
	if err != nil {
		s.logger.Warn("Invalid transform path",
			"resource_id1", req.ResourceID1,
			"resource_id2", req.ResourceID2,
			"error", err)
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid or expired transform URL"})
		return
	}

	s.serveTransformedAssetWithParams(c, req, params)
}

func (s *HTTPServer) serveTransformedAssetWithParams(c *gin.Context, req transformRequest, params *signedurl.TransformParams) {
	ctx := c.Request.Context()

	// Build cache key with prefix to distinguish between asset types
	cacheKey := core.ImageCacheKey(req.CachePrefix, req.AssetID, params.Width, params.Height, params.Fit)

	// Try cache first
	if cached, err := s.core.GetCachedResize(ctx, cacheKey); err == nil && cached != nil {
		s.logger.Debug("Cache hit for transformed asset",
			"asset_id", req.AssetID,
			"cache_key", cacheKey)

		// Still need to check authorization if required
		if req.Authorize != nil && !req.Authorize(c) {
			return
		}

		c.Header("Cache-Control", transformedAssetCacheControl(req.Authorize == nil))
		c.Header("ETag", fmt.Sprintf("\"%s-%d-%d-%s\"", req.AssetID, params.Width, params.Height, params.Fit))
		c.Header("Vary", transformedAssetVary(req.Authorize == nil))
		c.Header("X-Cache", "HIT")
		c.Data(http.StatusOK, assets.DetectImageContentType(cached), cached)
		return
	}

	// Cache miss - fetch the asset first
	// (FetchAsset may cache metadata like room ID needed by Authorize)
	reader, contentType, err := req.FetchAsset(ctx)
	if err != nil {
		s.logger.Error("Failed to get asset", "error", err, "asset_id", req.AssetID)
		c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found"})
		return
	}
	// Close the reader if it implements io.Closer
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}

	// Check authorization after fetching (Authorize can use metadata cached by FetchAsset)
	if req.Authorize != nil && !req.Authorize(c) {
		return
	}

	// Check if content type is an image
	if contentType == "" || !isImageContentType(contentType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Asset is not an image"})
		return
	}

	// Read asset data into bytes for transformation
	data, err := io.ReadAll(reader)
	if err != nil {
		s.logger.Error("Failed to read asset", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read asset"})
		return
	}

	// Transform the image
	var result *assets.TransformResult
	if req.JPEGQuality > 0 {
		result, err = assets.TransformImageWithOptions(data, params.Width, params.Height, assets.FitMode(params.Fit), assets.TransformOptions{
			JPEGQuality: req.JPEGQuality,
		})
	} else {
		result, err = assets.TransformImage(data, params.Width, params.Height, assets.FitMode(params.Fit))
	}
	if err != nil {
		s.logger.Error("Failed to transform image", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to transform image"})
		return
	}

	// Read transformed bytes for caching and response
	transformedData, err := io.ReadAll(result.Reader)
	if err != nil {
		s.logger.Error("Failed to read transformed image", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read transformed image"})
		return
	}

	// Store in cache (fire-and-forget, skip animated GIFs which are large)
	if result.ContentType != "image/gif" && s.core.ImageCacheEnabled() {
		go func() {
			if err := s.core.StoreCachedResize(context.Background(), cacheKey, transformedData); err != nil {
				s.logger.Warn("Failed to cache transformed image", "error", err, "cache_key", cacheKey)
			}
		}()
	}

	// Set cache headers for long-term caching (immutable content)
	c.Header("Cache-Control", transformedAssetCacheControl(req.Authorize == nil))
	c.Header("ETag", fmt.Sprintf("\"%s-%d-%d-%s\"", req.AssetID, params.Width, params.Height, params.Fit))
	c.Header("Vary", transformedAssetVary(req.Authorize == nil))
	c.Header("X-Cache", "MISS")

	// Serve the transformed image with appropriate content type
	c.Data(http.StatusOK, result.ContentType, transformedData)
}

func transformedAssetCacheControl(public bool) string {
	if public {
		return "public, max-age=31536000, immutable"
	}
	return protectedAssetCacheControl
}

func transformedAssetVary(public bool) string {
	if public {
		return "Accept-Encoding"
	}
	return "Accept-Encoding, Authorization, Cookie"
}

// serveTransformedServerAsset serves a dynamically transformed version of an server asset.
// URL format: /assets/server/{key}/t/{signedPath}
// Called by serveServerAsset when it detects a transform pattern in the path.
// Opens only the backend object bound by pre-cache public classification.
func (s *HTTPServer) serveTransformedServerAsset(c *gin.Context, key, signedPath string, location *core.PublicServerAssetLocation) {
	s.logger.Debug("Serving transformed server asset", "asset_id", key, "signed_path", signedPath)

	s.serveTransformedAsset(c, transformRequest{
		ResourceID1: core.ServerAssetSignResource,
		ResourceID2: key,
		SignedPath:  signedPath,
		CachePrefix: core.ServerAssetSignResource,
		AssetID:     key,
		FetchAsset: func(ctx context.Context) (io.Reader, string, error) {
			reader, info, err := s.core.GetPublicServerAsset(ctx, location)
			if err != nil {
				s.logger.Debug("Failed to fetch server asset",
					"asset_id", key,
					"error", err)
				return nil, "", err
			}
			contentType := info.ContentType
			if contentType == "" {
				contentType = getContentType(key)
				s.logger.Debug("Content type from header is empty, using extension-based fallback",
					"asset_id", key,
					"fallback_content_type", contentType)
			}
			s.logger.Debug("Fetched server asset",
				"asset_id", key,
				"content_type", contentType,
				"size", info.Size)
			return reader, contentType, nil
		},
		Authorize: nil, // Instance assets are public
	})
}

// isImageContentType checks if the content type is an image.
func isImageContentType(contentType string) bool {
	return contentType == "image/jpeg" ||
		contentType == "image/png" ||
		contentType == "image/gif" ||
		contentType == "image/webp"
}

// getContentType returns the MIME type based on file extension.
func getContentType(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".webp":
		return "image/webp"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}
