package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type assetUploadService struct {
	api *API
}

func (s *assetUploadService) CreateUpload(ctx context.Context, req *connect.Request[apiv1.CreateUploadRequest]) (*connect.Response[apiv1.CreateUploadResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	contentType := strings.TrimSpace(req.Msg.GetContentType())
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !s.api.config.Video.Enabled && strings.HasPrefix(contentType, "video/") {
		return nil, invalidArgument("video uploads are disabled on this server")
	}
	upload, err := s.api.core.AssetUploads().CreateUpload(ctx, core.AssetUploadCreateInput{
		ActorID:           caller.UserID,
		RoomID:            req.Msg.GetRoomId(),
		Filename:          req.Msg.GetFilename(),
		ContentType:       contentType,
		Size:              req.Msg.GetSize(),
		SHA256:            req.Msg.GetSha256(),
		ThreadRootEventID: req.Msg.GetThreadRootEventId(),
		AlsoSendToChannel: req.Msg.GetAlsoSendToChannel(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.CreateUploadResponse{Upload: apiAssetUpload(upload)}), nil
}

func (s *assetUploadService) UploadChunk(ctx context.Context, req *connect.Request[apiv1.UploadChunkRequest]) (*connect.Response[apiv1.UploadChunkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	upload, err := s.api.core.AssetUploads().UploadChunk(ctx, core.AssetUploadChunkInput{
		ActorID:     caller.UserID,
		UploadID:    req.Msg.GetUploadId(),
		Offset:      req.Msg.GetOffset(),
		Content:     req.Msg.GetContent(),
		ChunkSHA256: req.Msg.GetChunkSha256(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UploadChunkResponse{Upload: apiAssetUpload(upload)}), nil
}

func (s *assetUploadService) GetUpload(ctx context.Context, req *connect.Request[apiv1.GetUploadRequest]) (*connect.Response[apiv1.GetUploadResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	upload, err := s.api.core.AssetUploads().GetUpload(ctx, caller.UserID, req.Msg.GetUploadId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetUploadResponse{Upload: apiAssetUpload(upload)}), nil
}

func (s *assetUploadService) CompleteUpload(ctx context.Context, req *connect.Request[apiv1.CompleteUploadRequest]) (*connect.Response[apiv1.CompleteUploadResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	upload, attachment, err := s.api.core.AssetUploads().CompleteUpload(ctx, core.AssetUploadCompleteInput{
		ActorID:  caller.UserID,
		UploadID: req.Msg.GetUploadId(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.CompleteUploadResponse{
		Upload: apiAssetUpload(upload),
		Asset:  apiUploadedAttachmentAsset(attachment),
	}), nil
}

func (s *assetUploadService) CancelUpload(ctx context.Context, req *connect.Request[apiv1.CancelUploadRequest]) (*connect.Response[apiv1.CancelUploadResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	upload, err := s.api.core.AssetUploads().CancelUpload(ctx, core.AssetUploadCancelInput{
		ActorID:  caller.UserID,
		UploadID: req.Msg.GetUploadId(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.CancelUploadResponse{Upload: apiAssetUpload(upload)}), nil
}

func apiAssetUpload(upload *core.AssetUploadSession) *apiv1.AssetUpload {
	if upload == nil {
		return nil
	}
	return &apiv1.AssetUpload{
		UploadId:        upload.UploadID,
		RoomId:          upload.RoomID,
		Status:          apiAssetUploadStatus(upload.Status),
		CommittedOffset: upload.CommittedOffset,
		Size:            upload.Size,
		MaxChunkSize:    upload.MaxChunkSize,
		Sha256:          upload.SHA256,
		ExpiresAt:       timestamppb.New(upload.ExpiresAt),
		AssetId:         upload.AssetID,
	}
}

func apiAssetUploadStatus(status core.AssetUploadStatus) apiv1.AssetUploadStatus {
	switch status {
	case core.AssetUploadStatusOpen:
		return apiv1.AssetUploadStatus_ASSET_UPLOAD_STATUS_OPEN
	case core.AssetUploadStatusCompleted:
		return apiv1.AssetUploadStatus_ASSET_UPLOAD_STATUS_COMPLETED
	case core.AssetUploadStatusCancelled:
		return apiv1.AssetUploadStatus_ASSET_UPLOAD_STATUS_CANCELLED
	default:
		return apiv1.AssetUploadStatus_ASSET_UPLOAD_STATUS_UNSPECIFIED
	}
}

func apiUploadedAttachmentAsset(attachment *corev1.Attachment) *apiv1.UploadedAttachmentAsset {
	if attachment == nil {
		return nil
	}
	return &apiv1.UploadedAttachmentAsset{
		AssetId:     attachment.GetId(),
		Filename:    attachment.GetFilename(),
		ContentType: attachment.GetContentType(),
		Size:        attachment.GetSize(),
		Width:       attachment.GetWidth(),
		Height:      attachment.GetHeight(),
	}
}
