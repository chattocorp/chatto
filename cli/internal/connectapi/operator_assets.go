package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

type operatorAssetService struct {
	api *API
}

func (s *operatorAssetService) CreateUpload(ctx context.Context, req *connect.Request[operatorv1.CreateUploadRequest]) (*connect.Response[operatorv1.CreateUploadResponse], error) {
	contentType := strings.TrimSpace(req.Msg.GetContentType())
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !s.api.config.Video.Enabled && strings.HasPrefix(contentType, "video/") {
		return nil, invalidArgument("video uploads are disabled on this server")
	}
	upload, err := s.api.core.AssetUploads().CreateUpload(ctx, core.AssetUploadCreateInput{
		ActorID:     req.Msg.GetAuthorId(),
		Operator:    true,
		RoomID:      req.Msg.GetRoomId(),
		Filename:    req.Msg.GetFilename(),
		ContentType: contentType,
		Size:        req.Msg.GetSize(),
		SHA256:      req.Msg.GetSha256(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&operatorv1.CreateUploadResponse{Upload: apiAssetUpload(upload)}), nil
}

func (s *operatorAssetService) UploadChunk(ctx context.Context, req *connect.Request[operatorv1.UploadChunkRequest]) (*connect.Response[operatorv1.UploadChunkResponse], error) {
	upload, err := s.api.core.AssetUploads().UploadChunk(ctx, core.AssetUploadChunkInput{
		ActorID:     core.SystemActorID,
		Operator:    true,
		UploadID:    req.Msg.GetUploadId(),
		Offset:      req.Msg.GetOffset(),
		Content:     req.Msg.GetContent(),
		ChunkSHA256: req.Msg.GetChunkSha256(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&operatorv1.UploadChunkResponse{Upload: apiAssetUpload(upload)}), nil
}

func (s *operatorAssetService) CompleteUpload(ctx context.Context, req *connect.Request[operatorv1.CompleteUploadRequest]) (*connect.Response[operatorv1.CompleteUploadResponse], error) {
	_, attachment, err := s.api.core.AssetUploads().CompleteUpload(ctx, core.AssetUploadCompleteInput{
		ActorID: core.SystemActorID, Operator: true, UploadID: req.Msg.GetUploadId(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&operatorv1.CompleteUploadResponse{AssetId: attachment.GetId()}), nil
}

func (s *operatorAssetService) CancelUpload(ctx context.Context, req *connect.Request[operatorv1.CancelUploadRequest]) (*connect.Response[operatorv1.CancelUploadResponse], error) {
	upload, err := s.api.core.AssetUploads().CancelUpload(ctx, core.AssetUploadCancelInput{
		ActorID: core.SystemActorID, Operator: true, UploadID: req.Msg.GetUploadId(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&operatorv1.CancelUploadResponse{Upload: apiAssetUpload(upload)}), nil
}
