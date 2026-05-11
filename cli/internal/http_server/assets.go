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
	// Instance assets use *path which catches everything including /t/signedPath for transforms
	// The serveInstanceAsset handler detects and routes transform requests appropriately
	// These handlers probe both NATS and S3 backends automatically
	s.router.GET("/assets/instance/*path", s.serveInstanceAsset)
	s.router.GET("/assets/space/:spaceId/attachments/:attachmentId", s.serveSpaceAttachment)
	s.router.GET("/assets/space/:spaceId/attachments/:attachmentId/t/:signedPath", s.serveTransformedAttachment)
}

// transformRequest holds the parameters for a transformed asset request.
// This allows sharing the transformation logic between different asset types.
type transformRequest struct {
	// ResourceID1 and ResourceID2 are used for signing verification.
	// For space attachments: (spaceID, attachmentID)
	// For instance assets: ("instance", key)
	ResourceID1 string
	ResourceID2 string
	SignedPath  string
	// CachePrefix distinguishes cache keys between asset types (e.g., "space", "instance")
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

func (s *HTTPServer) serveInstanceAsset(c *gin.Context) {
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
			s.serveTransformedInstanceAsset(c, key, signedPath)
			return
		}
	}

	s.logger.Debug("Serving instance asset", "asset_id", path)

	// Probe both NATS and S3 backends
	reader, info, err := s.core.GetServerAssetFromAnyBackend(c.Request.Context(), path)
	if err != nil {
		s.logger.Error("Failed to get instance asset", "error", err, "asset_id", path)
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

// serveSpaceAttachment serves an attachment from a space's storage (NATS or S3).
//
// For S3-stored attachments: redirects to a presigned S3 URL. The browser talks
// directly to S3, with full Range request support. Zero memory/CPU on the Go process.
//
// For NATS-stored attachments: streams directly from NATS ObjectStore without
// buffering the entire file into memory. Progressive playback works (faststart),
// but seeking to unbuffered positions requires waiting for the buffer to catch up.
func (s *HTTPServer) serveSpaceAttachment(c *gin.Context) {
	spaceID := c.Param("spaceId")
	attachmentID := c.Param("attachmentId")
	ctx := c.Request.Context()

	s.logger.Debug("Serving space attachment", "space_id", spaceID, "attachment_id", attachmentID)

	// Try S3 presigned redirect first (zero-copy, full Range support)
	if presignedURL, err := s.core.TryPresignedAttachmentURL(ctx, spaceID, attachmentID); err == nil {
		// S3 attachments don't store roomID, so authorizeAttachmentAccess passes
		// with empty roomID (same as the previous proxying behavior).
		if !s.authorizeAttachmentAccess(c, spaceID, "") {
			return
		}

		// Cache the redirect itself — the attachment URL is immutable
		c.Header("Cache-Control", "public, max-age=3600")
		c.Redirect(http.StatusFound, presignedURL)
		return
	}

	// Fall back to probing both NATS and S3 backends (handles transient S3 errors
	// where the presigned URL fails but direct S3 fetch succeeds)
	reader, info, err := s.core.GetAttachmentFromAnyBackend(ctx, spaceID, attachmentID)
	if err != nil {
		s.logger.Error("Failed to get attachment", "error", err, "space_id", spaceID, "attachment_id", attachmentID)
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}

	if !s.authorizeAttachmentAccess(c, spaceID, info.RoomID) {
		return
	}

	contentType := info.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Immutable asset - cache forever
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Header("ETag", fmt.Sprintf("\"%s-%s\"", spaceID, attachmentID))
	c.Header("Vary", "Accept-Encoding")

	// Stream directly — no io.ReadAll, no memory buffering
	c.DataFromReader(http.StatusOK, info.Size, contentType, reader, nil)
}

// authorizeAttachmentAccess checks if the current user can access an attachment.
// Returns true if authorized, false if not (and sends appropriate error response).
func (s *HTTPServer) authorizeAttachmentAccess(c *gin.Context, spaceID, roomID string) bool {
	if roomID == "" {
		return true
	}

	userID := s.getUserIDFromSession(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return false
	}

	isMember, err := s.core.RoomMembershipExists(c.Request.Context(), spaceID, userID, roomID)
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

// serveTransformedInstanceAsset serves a dynamically transformed version of an instance asset.
// URL format: /assets/instance/{key}/t/{signedPath}
// Called by serveInstanceAsset when it detects a transform pattern in the path.
// Probes both NATS and S3 backends for the asset.
func (s *HTTPServer) serveTransformedInstanceAsset(c *gin.Context, key, signedPath string) {
	s.logger.Debug("Serving transformed instance asset", "asset_id", key, "signed_path", signedPath)

	s.serveTransformedAsset(c, transformRequest{
		ResourceID1: "instance",
		ResourceID2: key,
		SignedPath:  signedPath,
		CachePrefix: "instance",
		AssetID:     key,
		FetchAsset: func(ctx context.Context) (io.Reader, string, error) {
			// Probe both NATS and S3 backends
			reader, info, err := s.core.GetServerAssetFromAnyBackend(ctx, key)
			if err != nil {
				s.logger.Debug("Failed to fetch instance asset",
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
			s.logger.Debug("Fetched instance asset",
				"asset_id", key,
				"content_type", contentType,
				"size", info.Size)
			return reader, contentType, nil
		},
		Authorize: nil, // Instance assets are public
	})
}

// serveTransformedAttachment serves a dynamically transformed version of an attachment.
// URL format: /assets/space/{spaceId}/attachments/{attachmentId}/t/{signedPath}
// where signedPath is {base64params}.{signature}
// Probes both NATS and S3 backends for the attachment.
func (s *HTTPServer) serveTransformedAttachment(c *gin.Context) {
	spaceID := c.Param("spaceId")
	attachmentID := c.Param("attachmentId")
	signedPath := c.Param("signedPath")

	s.logger.Debug("Serving transformed attachment",
		"space_id", spaceID,
		"attachment_id", attachmentID)

	// We need to fetch the attachment info for authorization, and cache the room ID
	// since we may need it both for the authorize callback and for FetchAsset
	var cachedRoomID string
	var roomIDFetched bool

	s.serveTransformedAsset(c, transformRequest{
		ResourceID1: spaceID,
		ResourceID2: attachmentID,
		SignedPath:  signedPath,
		CachePrefix: spaceID,
		AssetID:     attachmentID,
		FetchAsset: func(ctx context.Context) (io.Reader, string, error) {
			// Probe both NATS and S3 backends
			reader, info, err := s.core.GetAttachmentFromAnyBackend(ctx, spaceID, attachmentID)
			if err != nil {
				return nil, "", err
			}
			// Cache the room ID for authorization
			cachedRoomID = info.RoomID
			roomIDFetched = true
			return reader, info.ContentType, nil
		},
		Authorize: func(c *gin.Context) bool {
			// If we haven't fetched the room ID yet, we need to fetch it for authorization
			if !roomIDFetched {
				_, info, err := s.core.GetAttachmentFromAnyBackend(c.Request.Context(), spaceID, attachmentID)
				if err != nil {
					s.logger.Error("Failed to get attachment for auth", "error", err)
					c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
					return false
				}
				cachedRoomID = info.RoomID
				roomIDFetched = true
			}
			return s.authorizeAttachmentAccess(c, spaceID, cachedRoomID)
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
