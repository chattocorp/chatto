package http_server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/pkg/signedurl"
)

func (s *HTTPServer) setupAssetRoutes() {
	// Server assets use *path which catches everything including /t/signedPath for transforms
	// The serveServerAsset handler detects and routes transform requests appropriately
	// These handlers probe both NATS and S3 backends automatically
	s.router.GET("/assets/server/*path", s.serveServerAsset)
	// Attachment routes are kind-less. Both new uploads and legacy
	// attachments (whose S3 objects still live at
	// `spaces/{server|DM}/attachments/{id}`) are served through these
	// routes; the storage layer probes both layouts transparently.
	s.router.GET("/assets/attachments/:attachmentId", s.serveAttachment)
	s.router.GET("/assets/attachments/:attachmentId/t/:signedPath", s.serveTransformedAttachment)
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

// serveAttachment serves an attachment by ID. Authorization runs off
// the canonical Attachment record in SERVER_BODIES: we look up the
// record by ID, verify room membership, then redirect to a presigned
// S3 URL (if applicable) or stream from NATS. The storage layer probes
// both the post-ADR-030-Phase-4 kind-less S3 layout and the legacy
// `spaces/{server|DM}/attachments/{id}` layout transparently.
func (s *HTTPServer) serveAttachment(c *gin.Context) {
	attachmentID := c.Param("attachmentId")
	ctx := c.Request.Context()

	s.logger.Debug("Serving attachment", "attachment_id", attachmentID)

	if !s.requireAttachmentAccess(c, ctx, attachmentID) {
		return
	}

	// Try S3 presigned redirect first (zero-copy, full Range support).
	if presignedURL, err := s.core.TryPresignedAttachmentURL(ctx, "", attachmentID); err == nil {
		// Cache the redirect itself — the attachment URL is immutable
		c.Header("Cache-Control", "public, max-age=3600")
		c.Redirect(http.StatusFound, presignedURL)
		return
	}

	// Fall back to probing both NATS and S3 backends (handles transient S3 errors
	// where the presigned URL fails but direct S3 fetch succeeds).
	reader, info, err := s.core.GetAttachmentFromAnyBackend(ctx, "", attachmentID)
	if err != nil {
		s.logger.Error("Failed to get attachment", "error", err, "attachment_id", attachmentID)
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

	// Immutable asset - cache forever
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("ETag", fmt.Sprintf("\"%s\"", attachmentID))
	c.Header("Vary", "Accept-Encoding")

	// Stream directly — no io.ReadAll, no memory buffering
	c.DataFromReader(http.StatusOK, info.Size, contentType, reader, nil)
}

// requireAttachmentAccess gates an attachment HTTP request. Looks up
// the canonical Attachment record in SERVER_BODIES, then verifies
// the caller is a member of that record's room. Returns true if the
// request may proceed; writes the appropriate error response and
// returns false otherwise.
//
// A missing record (e.g. an orphan binary in S3 left over from a
// pre-record-bucket upload that the boot migration hasn't touched
// yet) is treated as not-found — we refuse to authorize a download
// we can't attribute to a room.
func (s *HTTPServer) requireAttachmentAccess(c *gin.Context, ctx context.Context, attachmentID string) bool {
	userID := s.getUserIDFromSession(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return false
	}

	record, err := s.core.GetAttachmentRecord(ctx, attachmentID)
	if err != nil {
		s.logger.Error("Failed to look up attachment record", "attachment_id", attachmentID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return false
	}
	if record == nil || record.RoomId == "" {
		s.logger.Warn("Attachment access denied: no metadata record", "attachment_id", attachmentID, "user_id", userID)
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return false
	}

	kind, err := s.core.FindRoomKind(ctx, record.RoomId)
	if err != nil {
		s.logger.Error("Failed to resolve room kind for attachment auth", "error", err, "room_id", record.RoomId)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return false
	}

	isMember, err := s.core.RoomMembershipExists(ctx, kind, userID, record.RoomId)
	if err != nil {
		s.logger.Error("Failed to check room membership", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify access"})
		return false
	}

	if !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: not a member of the room"})
		return false
	}

	return true
}

// getUserIDFromSession extracts the user ID from the Gin session.
// Returns empty string if not authenticated.
func (s *HTTPServer) getUserIDFromSession(c *gin.Context) string {
	session := sessions.Default(c)
	if session == nil {
		return ""
	}

	userIDRaw := session.Get("user_id")
	if userIDRaw == nil {
		return ""
	}

	userID, ok := userIDRaw.(string)
	if !ok {
		return ""
	}

	return userID
}

// serveTransformedAsset handles the common logic for serving transformed images.
// It parses the signed path, checks cache, fetches the asset, transforms it, and serves the result.
func (s *HTTPServer) serveTransformedAsset(c *gin.Context, req transformRequest) {
	ctx := c.Request.Context()

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

		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Header("ETag", fmt.Sprintf("\"%s-%d-%d-%s\"", req.AssetID, params.Width, params.Height, params.Fit))
		c.Header("Vary", "Accept-Encoding")
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
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("ETag", fmt.Sprintf("\"%s-%d-%d-%s\"", req.AssetID, params.Width, params.Height, params.Fit))
	c.Header("Vary", "Accept-Encoding")
	c.Header("X-Cache", "MISS")

	// Serve the transformed image with appropriate content type
	c.Data(http.StatusOK, result.ContentType, transformedData)
}

// serveTransformedServerAsset serves a dynamically transformed version of an server asset.
// URL format: /assets/server/{key}/t/{signedPath}
// Called by serveServerAsset when it detects a transform pattern in the path.
// Probes both NATS and S3 backends for the asset.
func (s *HTTPServer) serveTransformedServerAsset(c *gin.Context, key, signedPath string) {
	s.logger.Debug("Serving transformed server asset", "asset_id", key, "signed_path", signedPath)

	s.serveTransformedAsset(c, transformRequest{
		ResourceID1: "server",
		ResourceID2: key,
		SignedPath:  signedPath,
		CachePrefix: "server",
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

// serveTransformedAttachment serves a dynamically transformed version of an attachment.
// URL format: /assets/attachments/{attachmentId}/t/{signedPath}
// where signedPath is {base64params}.{signature}
// Probes both NATS and S3 backends for the attachment.
func (s *HTTPServer) serveTransformedAttachment(c *gin.Context) {
	attachmentID := c.Param("attachmentId")
	signedPath := c.Param("signedPath")

	s.logger.Debug("Serving transformed attachment", "attachment_id", attachmentID)

	s.serveTransformedAsset(c, transformRequest{
		ResourceID1: core.AttachmentSignResource,
		ResourceID2: attachmentID,
		SignedPath:  signedPath,
		CachePrefix: core.AttachmentSignResource,
		AssetID:     attachmentID,
		FetchAsset: func(ctx context.Context) (io.Reader, string, error) {
			reader, info, err := s.core.GetAttachmentFromAnyBackend(ctx, "", attachmentID)
			if err != nil {
				return nil, "", err
			}
			return reader, info.ContentType, nil
		},
		Authorize: func(c *gin.Context) bool {
			return s.requireAttachmentAccess(c, c.Request.Context(), attachmentID)
		},
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
