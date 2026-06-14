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
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/auth"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/signedurl"
)

func (s *HTTPServer) setupAssetRoutes() {
	// Server assets use *path which catches everything including /t/signedPath for transforms
	// The serveServerAsset handler detects and routes transform requests appropriately
	// These handlers probe both NATS and S3 backends automatically
	s.router.GET("/assets/server/*path", s.serveServerAsset)
	// Attachment routes carry a signed locator (`{base64payload}.{hexHMAC}`)
	// as the path segment. The payload encodes roomId + (bodyKey | videoOrigin)
	// + attachmentId — everything the handler needs to authorize and serve
	// the binary without a separate index lookup.
	s.router.GET("/assets/files/:assetID", s.serveStableAttachment)
	s.router.GET("/assets/files/:assetID/image/:dimensions/:fit", s.serveStableTransformedAttachment)
	s.router.GET("/assets/attachments/:locator", s.serveAttachment)
	s.router.GET("/assets/attachments/:locator/t/:signedPath", s.serveTransformedAttachment)
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
	// FetchAsset returns the asset data and content type.
	// The reader will be closed if it implements io.Closer.
	FetchAsset func(ctx context.Context) (io.Reader, string, error)
	// Authorize checks if access is allowed. Return true if authorized.
	// If nil, asset is considered public and no authorization is needed.
	Authorize func(c *gin.Context) bool
}

func (s *HTTPServer) serveServerAsset(c *gin.Context) {
	path := c.Param("path")

	// Trim leading slash
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	// Check if this is a transform request: path ends with /t/{signedPath}
	// Pattern: {key}/t/{signedPath}
	if idx := strings.LastIndex(path, "/t/"); idx != -1 {
		key := path[:idx]
		signedPath := path[idx+3:] // skip "/t/"
		if key != "" && signedPath != "" {
			s.serveTransformedServerAsset(c, key, signedPath)
			return
		}
	}

	s.logger.Debug("Serving server asset", "asset_id", path)

	// Probe both NATS and S3 backends
	reader, info, err := s.core.GetServerAssetFromAnyBackend(c.Request.Context(), path)
	if err != nil {
		s.logger.Error("Failed to get server asset", "error", err, "asset_id", path)
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
		contentType = getContentType(path)
	}

	// Immutable asset - cache forever
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("ETag", fmt.Sprintf("\"%s\"", path))
	c.Header("Vary", "Accept-Encoding")

	c.DataFromReader(
		http.StatusOK,
		info.Size,
		contentType,
		reader,
		nil,
	)
}

// serveAttachment serves an attachment whose location is encoded in
// the signed locator path segment. Verifying the locator gives us the
// roomId (for authz) and the source pointer (body key or video-origin
// id) plus the attachment id, with no index lookup needed.
func (s *HTTPServer) serveAttachment(c *gin.Context) {
	ctx := c.Request.Context()

	loc, attachment, ok := s.resolveLocatorAttachment(c, ctx, c.Param("locator"))
	if !ok {
		return
	}

	s.logger.Debug("Serving attachment", "attachment_id", loc.AttachmentID)

	// Try S3 presigned redirect first (zero-copy, full Range support) for
	// attachment types that do not need Chatto-served security headers.
	if attachmentCanUsePresignedRedirect(attachment.GetContentType()) {
		if presignedURL, err := s.core.TryPresignedAttachmentURL(ctx, attachment); err == nil {
			// Cache the redirect itself — the attachment URL is immutable
			c.Header("Cache-Control", "public, max-age=3600")
			c.Redirect(http.StatusFound, presignedURL)
			return
		}
	}

	// Otherwise stream from the recorded backend.
	reader, info, err := s.core.GetAttachmentReader(ctx, attachment)
	if err != nil {
		s.logger.Error("Failed to get attachment", "error", err, "attachment_id", loc.AttachmentID)
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

	c.Header("Cache-Control", legacyAttachmentCacheControl(contentType))
	c.Header("ETag", fmt.Sprintf("\"%s\"", loc.AttachmentID))
	c.Header("Vary", "Accept-Encoding")

	// Stream directly — no io.ReadAll, no memory buffering
	c.DataFromReader(http.StatusOK, info.Size, contentType, reader, nil)
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

	if s.failAssetProxyRequest(c) {
		return
	}

	attachment, ok := s.resolveStableAttachment(c, ctx, assetID, nil)
	if !ok {
		return
	}

	if c.GetHeader("X-Chatto-Asset-Proxy") != "1" {
		// Try S3 presigned redirect first (zero-copy, full Range support) after
		// validating the asset ticket/request credentials and room membership.
		// Active document formats must stream through Chatto so the sandbox CSP
		// below cannot be bypassed by S3's response headers.
		if attachmentCanUsePresignedRedirect(attachment.GetContentType()) {
			if presignedURL, err := s.core.TryPresignedAttachmentURL(ctx, attachment); err == nil {
				c.Header("Cache-Control", "private, max-age=3600")
				c.Redirect(http.StatusFound, presignedURL)
				return
			}
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

	c.Header("Cache-Control", "private, max-age=3600")
	c.Header("ETag", fmt.Sprintf("\"%s\"", assetID))
	c.Header("Vary", "Accept-Encoding, Authorization, Cookie, X-Chatto-Asset-Proxy")
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

func legacyAttachmentCacheControl(contentType string) string {
	if originalAttachmentNeedsSandbox(contentType) {
		return fmt.Sprintf("private, max-age=%d", int(core.AttachmentURLTTL.Seconds()))
	}
	return "public, max-age=31536000, immutable"
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

	if s.failAssetProxyRequest(c) {
		return
	}

	attachment, ok := s.resolveStableAttachment(c, ctx, assetID, params)
	if !ok {
		return
	}

	s.serveTransformedAssetWithParams(c, transformRequest{
		CachePrefix: AttachmentStableCachePrefix,
		AssetID:     assetID,
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

func (s *HTTPServer) failAssetProxyRequest(c *gin.Context) bool {
	if c.GetHeader("X-Chatto-Asset-Proxy") != "1" {
		return false
	}

	for {
		remaining := s.failAssetProxyRequests.Load()
		if remaining <= 0 {
			return false
		}
		if s.failAssetProxyRequests.CompareAndSwap(remaining, remaining-1) {
			c.JSON(http.StatusForbidden, gin.H{"error": "expired asset access ticket"})
			return true
		}
	}
}

const AttachmentStableCachePrefix = "attachment-stable"

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
	user := auth.ForContext(reqWithUser.Context())
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return "", false
	}
	return user.Id, true
}

// resolveLocatorAttachment parses the signed locator, validates its
// expiry, verifies the signed user is still a member of the room, and
// looks up the source Attachment proto. On success returns the locator,
// the attachment, and true. On any failure, writes the appropriate HTTP
// response and returns ok=false.
//
// The signed locator is the capability — no session cookie or bearer
// token is consulted. This is what lets cross-origin <img> tags work
// for remote-server attachments, which can't carry either credential.
// Auto-revocation on kick/leave still works because we re-check
// membership for the signed user on every request.
func (s *HTTPServer) resolveLocatorAttachment(c *gin.Context, ctx context.Context, signedLocator string) (*signedurl.AttachmentLocator, *corev1.Attachment, bool) {
	loc, err := signedurl.ParseSignedAttachmentLocator(s.config.Core.Assets.SigningSecret, signedLocator)
	if err != nil {
		s.logger.Warn("Invalid attachment locator", "error", err)
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid attachment URL"})
		return nil, nil, false
	}

	if loc.Expired(time.Now().Unix()) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Attachment URL expired"})
		return nil, nil, false
	}

	kind, err := s.core.FindRoomKind(ctx, loc.RoomID)
	if err != nil {
		s.logger.Error("Failed to resolve room kind for attachment auth", "error", err, "room_id", loc.RoomID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return nil, nil, false
	}
	isMember, err := s.core.RoomMembershipExists(ctx, kind, loc.UserID, loc.RoomID)
	if err != nil {
		s.logger.Error("Failed to check room membership", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return nil, nil, false
	}
	if !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: not a member of the room"})
		return nil, nil, false
	}

	attachment, err := s.core.LookupAttachment(ctx, *loc)
	if err != nil {
		s.logger.Error("Failed to look up attachment", "error", err, "attachment_id", loc.AttachmentID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve attachment"})
		return nil, nil, false
	}
	if attachment == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return nil, nil, false
	}
	return loc, attachment, true
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
	result, err := assets.TransformImage(data, params.Width, params.Height, assets.FitMode(params.Fit))
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
	return "private, max-age=3600"
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
// Probes both NATS and S3 backends for the asset.
func (s *HTTPServer) serveTransformedServerAsset(c *gin.Context, key, signedPath string) {
	s.logger.Debug("Serving transformed server asset", "asset_id", key, "signed_path", signedPath)

	s.serveTransformedAsset(c, transformRequest{
		ResourceID1: core.ServerAssetSignResource,
		ResourceID2: key,
		SignedPath:  signedPath,
		CachePrefix: core.ServerAssetSignResource,
		AssetID:     key,
		FetchAsset: func(ctx context.Context) (io.Reader, string, error) {
			// Probe both NATS and S3 backends
			reader, info, err := s.core.GetServerAssetFromAnyBackend(ctx, key)
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

// serveTransformedAttachment serves a dynamically transformed version
// of an attachment identified by the locator path segment.
// URL format: /assets/attachments/{locator}/t/{signedPath}
// The locator is verified, then the transform path is verified against
// it; both signatures must be valid.
func (s *HTTPServer) serveTransformedAttachment(c *gin.Context) {
	signedLocator := c.Param("locator")
	signedPath := c.Param("signedPath")
	ctx := c.Request.Context()

	loc, attachment, ok := s.resolveLocatorAttachment(c, ctx, signedLocator)
	if !ok {
		return
	}

	s.logger.Debug("Serving transformed attachment", "attachment_id", loc.AttachmentID)

	// resolveLocatorAttachment already authorized; the inner helper
	// runs the transform-path signature check too.
	s.serveTransformedAsset(c, transformRequest{
		ResourceID1: core.AttachmentSignResource,
		ResourceID2: signedLocator,
		SignedPath:  signedPath,
		CachePrefix: core.AttachmentSignResource,
		AssetID:     loc.AttachmentID,
		FetchAsset: func(ctx context.Context) (io.Reader, string, error) {
			reader, info, err := s.core.GetAttachmentReader(ctx, attachment)
			if err != nil {
				return nil, "", err
			}
			return reader, info.ContentType, nil
		},
		Authorize: nil, // Already authorized above.
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
