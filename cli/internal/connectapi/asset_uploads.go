package connectapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1/apiv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type assetUploadService struct {
	api *API
}

// maxConcurrentUploadChunks limits the UploadChunk requests that one process
// reads at the same time. Connect reads a complete request into memory before
// the handler runs, and storage makes more copies. Thus, one request can use
// several times core.DefaultAssetUploadChunkSize. Memory is per process, so
// the limit is per process too.
const maxConcurrentUploadChunks = 4

// limitUploadChunks makes UploadChunk requests wait for one of limit slots
// before next reads their body. Other procedures pass through. A request that
// ends while it waits gets an Unavailable error, and the client can retry the
// chunk.
func limitUploadChunks(next http.Handler, limit int, options ...connect.HandlerOption) http.Handler {
	slots := make(chan struct{}, limit)
	errorWriter := connect.NewErrorWriter(options...)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != apiv1connect.AssetUploadServiceUploadChunkProcedure {
			next.ServeHTTP(w, r)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		case <-r.Context().Done():
			_ = errorWriter.Write(w, r, connect.NewError(connect.CodeUnavailable, errors.New("upload chunk was not accepted before the request ended")))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *assetUploadService) CreateUpload(ctx context.Context, req *connect.Request[apiv1.CreateUploadRequest]) (*connect.Response[apiv1.CreateUploadResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	// Canonicalize before the video check so that a type such as "Video/MP4"
	// cannot bypass it.
	contentType, _ := core.CanonicalContentType(req.Msg.GetContentType())
	if !s.api.config.Video.Enabled && strings.HasPrefix(contentType, "video/") {
		return nil, invalidArgument("video uploads are disabled on this server")
	}
	upload, err := s.api.core.AssetUploads().CreateUpload(ctx, core.AssetUploadCreateInput{
		ActorID:     caller.UserID,
		RoomID:      req.Msg.GetRoomId(),
		Filename:    req.Msg.GetFilename(),
		ContentType: contentType,
		Size:        req.Msg.GetSize(),
		SHA256:      req.Msg.GetSha256(),
	})
	if err != nil {
		return nil, err
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
		return nil, err
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
		return nil, err
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
		return nil, err
	}
	return connect.NewResponse(&apiv1.CompleteUploadResponse{
		Upload: apiAssetUpload(upload),
		Asset:  apiAsset(ctx, s.api, attachment, caller.UserID, assetThumbnailOptions(nil)),
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
		return nil, err
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
