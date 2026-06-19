package http_server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/auth"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	roomAttachmentUploadFieldName = "attachments"
	roomAttachmentUploadMaxFiles  = 10
	userAvatarUploadFieldName     = "avatar"
	serverLogoUploadFieldName     = "logo"
	serverBannerUploadFieldName   = "banner"
	protobufRequestMaxBytes       = 64 * 1024
	protobufContentType           = "application/protobuf"
)

func (s *HTTPServer) setupAttachmentUploadRoutes() {
	s.router.POST("/api/rooms/:roomID/attachments", s.uploadRoomAttachments)
	s.router.POST("/api/rooms/:roomID/attachments/urls/refresh", s.refreshRoomAttachmentURLs)
	s.router.POST("/api/users/:userID/avatar", s.uploadUserAvatar)
	s.router.DELETE("/api/users/:userID/avatar", s.deleteUserAvatar)
	s.router.POST("/api/server/logo", s.uploadServerLogo)
	s.router.DELETE("/api/server/logo", s.deleteServerLogo)
	s.router.POST("/api/server/banner", s.uploadServerBanner)
	s.router.DELETE("/api/server/banner", s.deleteServerBanner)
}

func (s *HTTPServer) uploadRoomAttachments(c *gin.Context) {
	s.requestContextWithAuditMetadata(c)
	c.Request = s.injectUserIntoContext(c)
	ctx := c.Request.Context()
	user := auth.ForContext(ctx)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	roomID := c.Param("roomID")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Room ID is required"})
		return
	}

	kind, err := s.core.FindRoomKind(ctx, roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Room not found"})
		return
	}
	member, err := s.core.RoomMembershipExists(ctx, kind, user.Id, roomID)
	if err != nil {
		s.logger.Error("Failed to check room membership for attachment upload", "error", err, "room_id", roomID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify room membership"})
		return
	}
	if !member {
		c.JSON(http.StatusForbidden, gin.H{"error": "Room membership required"})
		return
	}

	maxMemory, maxUploadSize := s.multipartUploadLimits()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)
	if err := c.Request.ParseMultipartForm(maxMemory); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid multipart upload"})
		return
	}
	form := c.Request.MultipartForm
	if form == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid multipart upload"})
		return
	}
	defer form.RemoveAll()

	threadRootEventID := firstFormValue(form.Value["threadRootEventId"])
	canPost, err := s.canUploadAttachmentForMessage(ctx, user.Id, kind, roomID, threadRootEventID)
	if err != nil {
		s.logger.Error("Failed to check post permission for attachment upload", "error", err, "room_id", roomID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify post permission"})
		return
	}
	if !canPost {
		c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied"})
		return
	}

	files := form.File[roomAttachmentUploadFieldName]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No attachments provided"})
		return
	}
	if len(files) > roomAttachmentUploadMaxFiles {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Too many attachments; maximum is %d", roomAttachmentUploadMaxFiles)})
		return
	}

	response := &apiv1.UploadRoomAttachmentsResponse{}
	for _, fileHeader := range files {
		attachment, needsProcessing, err := s.uploadOneRoomAttachment(ctx, user.Id, roomID, fileHeader)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		response.Attachments = append(response.Attachments, &apiv1.UploadedAttachment{
			Attachment:           attachment,
			NeedsVideoProcessing: needsProcessing,
		})
		if needsProcessing {
			response.VideoProcessingAssetIds = append(response.VideoProcessingAssetIds, attachment.GetId())
		}
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal attachment upload response", "error", err, "room_id", roomID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode upload response"})
		return
	}
	c.Data(http.StatusOK, protobufContentType, data)
}

func (s *HTTPServer) refreshRoomAttachmentURLs(c *gin.Context) {
	s.requestContextWithAuditMetadata(c)
	c.Request = s.injectUserIntoContext(c)
	ctx := c.Request.Context()
	user := auth.ForContext(ctx)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	roomID := c.Param("roomID")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Room ID is required"})
		return
	}

	body, err := readProtobufRequest(c, protobufRequestMaxBytes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid protobuf request"})
		return
	}
	var req apiv1.RefreshMessageAttachmentUrlsRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid protobuf request"})
		return
	}
	if req.GetRoomId() != "" && req.GetRoomId() != roomID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Room ID mismatch"})
		return
	}
	req.RoomId = roomID
	if req.GetEventId() == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Event ID is required"})
		return
	}

	opts, err := attachmentViewOptionsFromRefreshRequest(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	kind, err := s.core.FindRoomKind(ctx, roomID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Room not found"})
		return
	}
	member, err := s.core.RoomMembershipExists(ctx, kind, user.Id, roomID)
	if err != nil {
		s.logger.Error("Failed to check room membership for attachment URL refresh", "error", err, "room_id", roomID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify room membership"})
		return
	}
	if !member {
		c.JSON(http.StatusForbidden, gin.H{"error": "Room membership required"})
		return
	}

	event, err := s.core.GetRoomEventByEventID(ctx, kind, roomID, req.GetEventId())
	if err != nil {
		s.logger.Error("Failed to resolve event for attachment URL refresh", "error", err, "room_id", roomID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve event"})
		return
	}
	if event == nil || event.GetMessagePosted() == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Message not found"})
		return
	}

	messageBody, err := s.core.GetFullMessageBodyByEventID(ctx, req.GetEventId())
	if err != nil {
		s.logger.Error("Failed to load message body for attachment URL refresh", "error", err, "room_id", roomID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load message"})
		return
	}
	response := &apiv1.RefreshMessageAttachmentUrlsResponse{}
	if messageBody != nil {
		viewConn := &wireConn{server: s}
		response.Attachments = viewConn.attachmentViewsWithOptions(user.Id, roomID, req.GetEventId(), messageBody.Attachments, opts)
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal attachment URL refresh response", "error", err, "room_id", roomID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode refresh response"})
		return
	}
	c.Data(http.StatusOK, protobufContentType, data)
}

func (s *HTTPServer) uploadUserAvatar(c *gin.Context) {
	ctx, actor, ok := s.authenticatedUploadRequest(c)
	if !ok {
		return
	}

	userID := c.Param("userID")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}
	canManage, err := s.canManageUserProfileAsset(ctx, actor.GetId(), userID)
	if err != nil {
		s.logger.Error("Failed to check user avatar upload permission", "error", err, "user_id", userID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify permission"})
		return
	}
	if !canManage {
		c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied"})
		return
	}

	fileHeader, cleanup, ok := s.singleMultipartFile(c, userAvatarUploadFieldName, "avatar")
	if !ok {
		return
	}
	defer cleanup()

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to open avatar"})
		return
	}
	defer file.Close()

	asset, err := s.core.UploadUserAvatar(ctx, userID, file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.core.SetUserAvatar(ctx, userID, asset); err != nil {
		s.core.CleanupAsset(ctx, core.DeprecatedAssetFromAsset(asset))
		s.logger.Error("Failed to save uploaded avatar", "error", err, "user_id", userID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save avatar"})
		return
	}

	response, err := s.userAvatarAssetResponse(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to build avatar upload response", "error", err, "user_id", userID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build avatar response"})
		return
	}
	s.writeProtobufResponse(c, response, "avatar upload", "user_id", userID)
}

func (s *HTTPServer) deleteUserAvatar(c *gin.Context) {
	ctx, actor, ok := s.authenticatedUploadRequest(c)
	if !ok {
		return
	}

	userID := c.Param("userID")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}
	canManage, err := s.canManageUserProfileAsset(ctx, actor.GetId(), userID)
	if err != nil {
		s.logger.Error("Failed to check user avatar delete permission", "error", err, "user_id", userID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify permission"})
		return
	}
	if !canManage {
		c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied"})
		return
	}

	if err := s.core.DeleteUserAvatar(ctx, userID); err != nil {
		s.logger.Error("Failed to delete avatar", "error", err, "user_id", userID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete avatar"})
		return
	}

	response, err := s.userAvatarAssetResponse(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to build avatar delete response", "error", err, "user_id", userID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build avatar response"})
		return
	}
	s.writeProtobufResponse(c, response, "avatar delete", "user_id", userID)
}

func (s *HTTPServer) uploadServerLogo(c *gin.Context) {
	ctx, actor, ok := s.authenticatedUploadRequest(c)
	if !ok {
		return
	}
	if !s.requireServerBrandingManager(c, ctx, actor.GetId()) {
		return
	}

	fileHeader, cleanup, ok := s.singleMultipartFile(c, serverLogoUploadFieldName, "logo")
	if !ok {
		return
	}
	defer cleanup()

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to open logo"})
		return
	}
	defer file.Close()

	asset, err := s.core.UploadServerLogo(ctx, file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.core.SetServerLogo(ctx, actor.GetId(), asset); err != nil {
		s.core.CleanupAsset(ctx, core.DeprecatedAssetFromAsset(asset))
		s.logger.Error("Failed to save uploaded server logo", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save logo"})
		return
	}

	response, err := s.serverBrandingAssetResponse(ctx)
	if err != nil {
		s.logger.Error("Failed to build server logo upload response", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build logo response"})
		return
	}
	s.writeProtobufResponse(c, response, "server logo upload")
}

func (s *HTTPServer) deleteServerLogo(c *gin.Context) {
	ctx, actor, ok := s.authenticatedUploadRequest(c)
	if !ok {
		return
	}
	if !s.requireServerBrandingManager(c, ctx, actor.GetId()) {
		return
	}

	if err := s.core.DeleteServerLogo(ctx, actor.GetId()); err != nil {
		s.logger.Error("Failed to delete server logo", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete logo"})
		return
	}

	response, err := s.serverBrandingAssetResponse(ctx)
	if err != nil {
		s.logger.Error("Failed to build server logo delete response", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build logo response"})
		return
	}
	s.writeProtobufResponse(c, response, "server logo delete")
}

func (s *HTTPServer) uploadServerBanner(c *gin.Context) {
	ctx, actor, ok := s.authenticatedUploadRequest(c)
	if !ok {
		return
	}
	if !s.requireServerBrandingManager(c, ctx, actor.GetId()) {
		return
	}

	fileHeader, cleanup, ok := s.singleMultipartFile(c, serverBannerUploadFieldName, "banner")
	if !ok {
		return
	}
	defer cleanup()

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to open banner"})
		return
	}
	defer file.Close()

	asset, err := s.core.UploadServerBanner(ctx, file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.core.SetServerBanner(ctx, actor.GetId(), asset); err != nil {
		s.core.CleanupAsset(ctx, core.DeprecatedAssetFromAsset(asset))
		s.logger.Error("Failed to save uploaded server banner", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save banner"})
		return
	}

	response, err := s.serverBrandingAssetResponse(ctx)
	if err != nil {
		s.logger.Error("Failed to build server banner upload response", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build banner response"})
		return
	}
	s.writeProtobufResponse(c, response, "server banner upload")
}

func (s *HTTPServer) deleteServerBanner(c *gin.Context) {
	ctx, actor, ok := s.authenticatedUploadRequest(c)
	if !ok {
		return
	}
	if !s.requireServerBrandingManager(c, ctx, actor.GetId()) {
		return
	}

	if err := s.core.DeleteServerBanner(ctx, actor.GetId()); err != nil {
		s.logger.Error("Failed to delete server banner", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete banner"})
		return
	}

	response, err := s.serverBrandingAssetResponse(ctx)
	if err != nil {
		s.logger.Error("Failed to build server banner delete response", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build banner response"})
		return
	}
	s.writeProtobufResponse(c, response, "server banner delete")
}

func readProtobufRequest(c *gin.Context, maxBytes int64) ([]byte, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	return io.ReadAll(c.Request.Body)
}

func (s *HTTPServer) authenticatedUploadRequest(c *gin.Context) (context.Context, *corev1.User, bool) {
	s.requestContextWithAuditMetadata(c)
	c.Request = s.injectUserIntoContext(c)
	ctx := c.Request.Context()
	user := auth.ForContext(ctx)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return ctx, nil, false
	}
	return ctx, user, true
}

func (s *HTTPServer) canManageUserProfileAsset(ctx context.Context, actorID, targetID string) (bool, error) {
	if actorID == targetID {
		return true, nil
	}
	return s.core.HasServerPermission(ctx, actorID, core.PermRoleAssign)
}

func (s *HTTPServer) requireServerBrandingManager(c *gin.Context, ctx context.Context, userID string) bool {
	canManage, err := s.core.CanManageServer(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to check server branding permission", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify permission"})
		return false
	}
	if !canManage {
		c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied"})
		return false
	}
	return true
}

func (s *HTTPServer) singleMultipartFile(c *gin.Context, fieldName, label string) (*multipart.FileHeader, func(), bool) {
	maxMemory, maxUploadSize := s.multipartUploadLimits()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)
	if err := c.Request.ParseMultipartForm(maxMemory); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid multipart upload"})
		return nil, nil, false
	}
	form := c.Request.MultipartForm
	if form == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid multipart upload"})
		return nil, nil, false
	}
	cleanup := func() {
		_ = form.RemoveAll()
	}

	files := form.File[fieldName]
	if len(files) == 0 {
		cleanup()
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("No %s provided", label)})
		return nil, nil, false
	}
	if len(files) > 1 {
		cleanup()
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Only one %s may be uploaded", label)})
		return nil, nil, false
	}
	return files[0], cleanup, true
}

func (s *HTTPServer) userAvatarAssetResponse(ctx context.Context, userID string) (*apiv1.UserAvatarAssetResponse, error) {
	user, err := s.core.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	avatarURL, err := s.core.GetUserAvatarURL(ctx, userID, nil, nil, "")
	if err != nil {
		return nil, err
	}
	return &apiv1.UserAvatarAssetResponse{User: user, AvatarUrl: avatarURL}, nil
}

func (s *HTTPServer) serverBrandingAssetResponse(ctx context.Context) (*apiv1.ServerBrandingAssetResponse, error) {
	profile, err := (&wireConn{server: s}).serverProfileView(ctx)
	if err != nil {
		return nil, err
	}
	return &apiv1.ServerBrandingAssetResponse{Profile: profile}, nil
}

func (s *HTTPServer) writeProtobufResponse(c *gin.Context, msg proto.Message, operation string, logArgs ...any) {
	data, err := proto.Marshal(msg)
	if err != nil {
		args := append([]any{"error", err}, logArgs...)
		s.logger.Error("Failed to marshal "+operation+" response", args...)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode response"})
		return
	}
	c.Data(http.StatusOK, protobufContentType, data)
}

func attachmentViewOptionsFromRefreshRequest(req *apiv1.RefreshMessageAttachmentUrlsRequest) (attachmentViewOptions, error) {
	opts := defaultAttachmentViewOptions()
	if req.GetThumbnailWidth() > 0 {
		opts.thumbnailWidth = int(req.GetThumbnailWidth())
	}
	if req.GetThumbnailHeight() > 0 {
		opts.thumbnailHeight = int(req.GetThumbnailHeight())
	}
	if opts.thumbnailWidth <= 0 || opts.thumbnailWidth > 2048 || opts.thumbnailHeight <= 0 || opts.thumbnailHeight > 2048 {
		return opts, fmt.Errorf("Thumbnail dimensions must be between 1 and 2048")
	}

	switch req.GetThumbnailFit() {
	case apiv1.AssetFitMode_ASSET_FIT_MODE_UNSPECIFIED, apiv1.AssetFitMode_ASSET_FIT_MODE_CONTAIN:
		opts.thumbnailFit = "contain"
	case apiv1.AssetFitMode_ASSET_FIT_MODE_COVER:
		opts.thumbnailFit = "cover"
	case apiv1.AssetFitMode_ASSET_FIT_MODE_EXACT:
		opts.thumbnailFit = "exact"
	default:
		return opts, fmt.Errorf("Invalid thumbnail fit mode")
	}
	return opts, nil
}

func (s *HTTPServer) canUploadAttachmentForMessage(ctx context.Context, userID string, kind core.RoomKind, roomID, threadRootEventID string) (bool, error) {
	if threadRootEventID != "" {
		return s.core.CanPostInThread(ctx, userID, kind, roomID)
	}
	return s.core.CanPostMessage(ctx, userID, kind, roomID)
}

func (s *HTTPServer) uploadOneRoomAttachment(ctx context.Context, userID, roomID string, fileHeader *multipart.FileHeader) (*corev1.Attachment, bool, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, false, fmt.Errorf("failed to open attachment")
	}
	defer file.Close()

	contentType := uploadContentType(fileHeader.Filename, fileHeader.Header.Get("Content-Type"))
	mediaType := normalizedMediaType(contentType)
	if !s.config.Video.Enabled && strings.HasPrefix(mediaType, "video/") {
		return nil, false, fmt.Errorf("video uploads are disabled on this server")
	}

	var reader io.Reader = file
	animatedGIF := false
	if s.config.Video.Enabled && mediaType == "image/gif" {
		data, err := io.ReadAll(file)
		if err != nil {
			return nil, false, fmt.Errorf("failed to read attachment")
		}
		animatedGIF = assets.IsAnimatedGIF(data)
		reader = bytes.NewReader(data)
	}

	attachment, err := s.core.UploadAttachment(ctx, userID, roomID, fileHeader.Filename, contentType, reader)
	if err != nil {
		return nil, false, fmt.Errorf("failed to upload attachment: %w", err)
	}
	needsProcessing := s.config.Video.Enabled && core.AttachmentNeedsVideoProcessing(attachment, animatedGIF)
	return attachment, needsProcessing, nil
}

func (s *HTTPServer) multipartUploadLimits() (int64, int64) {
	maxMemory := int64(s.config.Core.Assets.MaxUploadSize)
	if maxMemory == 0 {
		maxMemory = assets.DefaultMaxUploadSize
	}
	maxUploadSize := maxMemory
	if s.config.Video.Enabled {
		videoMax := int64(s.config.Video.MaxUploadSizeOrDefault())
		if videoMax > maxUploadSize {
			maxUploadSize = videoMax
		}
	}
	return maxMemory, maxUploadSize
}

func uploadContentType(filename string, headerContentType string) string {
	if contentType := strings.TrimSpace(headerContentType); contentType != "" {
		return contentType
	}
	if contentType := mime.TypeByExtension(filepath.Ext(filename)); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

func normalizedMediaType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}
	return strings.ToLower(mediaType)
}

func firstFormValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
