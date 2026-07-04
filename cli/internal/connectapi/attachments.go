package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type attachmentMapper struct {
	api *API
}

type assetService struct {
	api *API
}

const (
	defaultAttachmentListLimit = 50
	maxAttachmentListLimit     = 100
)

type attachmentThumbnailRequest struct {
	width  int
	height int
	fit    string
}

func (s *roomService) ListRoomAttachments(ctx context.Context, req *connect.Request[apiv1.ListRoomAttachmentsRequest]) (*connect.Response[apiv1.ListRoomAttachmentsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultAttachmentListLimit, maxAttachmentListLimit)
	result, err := s.api.core.ListRoomAttachments(ctx, core.ListRoomAttachmentsInput{
		ActorID: caller.UserID,
		RoomID:  req.Msg.RoomId,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return nil, connectError(err)
	}

	thumbnail := assetThumbnailOptions(req.Msg.Thumbnail)
	mapper := attachmentMapper{api: s.api}
	attachments := make([]*apiv1.RoomAttachmentListItem, 0, len(result.Items))
	for _, item := range result.Items {
		if item == nil {
			continue
		}
		attachments = append(attachments, &apiv1.RoomAttachmentListItem{
			Attachment:        mapper.attachment(item.Attachment, caller.UserID, thumbnail),
			MessageEventId:    item.MessageEventID,
			ThreadRootEventId: item.ThreadRootEventID,
			CreatedAt:         item.CreatedAt,
		})
	}

	return connect.NewResponse(&apiv1.ListRoomAttachmentsResponse{
		Attachments: attachments,
		Page:        apiPageInfo(result.TotalCount, result.HasMore),
	}), nil
}

func (s *assetService) GetAsset(ctx context.Context, req *connect.Request[apiv1.GetAssetRequest]) (*connect.Response[apiv1.GetAssetResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	asset, err := s.api.core.GetRoomAsset(ctx, core.RoomAssetInput{
		ActorID: caller.UserID,
		RoomID:  req.Msg.RoomId,
		AssetID: req.Msg.AssetId,
	})
	if err != nil {
		return nil, connectError(err)
	}
	mapper := attachmentMapper{api: s.api}
	return connect.NewResponse(&apiv1.GetAssetResponse{
		Asset: mapper.attachment(asset, caller.UserID, assetThumbnailOptions(req.Msg.Thumbnail)),
	}), nil
}

func (s *assetService) BatchGetAssets(ctx context.Context, req *connect.Request[apiv1.BatchGetAssetsRequest]) (*connect.Response[apiv1.BatchGetAssetsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	assets, err := s.api.core.BatchGetRoomAssets(ctx, core.BatchRoomAssetsInput{
		ActorID:  caller.UserID,
		RoomID:   req.Msg.RoomId,
		AssetIDs: req.Msg.GetAssetIds(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	thumbnail := assetThumbnailOptions(req.Msg.Thumbnail)
	mapper := attachmentMapper{api: s.api}
	out := make([]*apiv1.MessageAttachment, 0, len(assets))
	for _, asset := range assets {
		out = append(out, mapper.attachment(asset, caller.UserID, thumbnail))
	}
	return connect.NewResponse(&apiv1.BatchGetAssetsResponse{Assets: out}), nil
}

func (s *attachmentMapper) attachment(attachment *corev1.Attachment, viewerID string, thumbnail attachmentThumbnailRequest) *apiv1.MessageAttachment {
	if attachment == nil {
		return nil
	}
	h := &timelineHydrator{
		api:      s.api,
		viewerID: viewerID,
	}
	return &apiv1.MessageAttachment{
		Id:                attachment.Id,
		Filename:          attachment.Filename,
		ContentType:       attachment.ContentType,
		Width:             attachment.Width,
		Height:            attachment.Height,
		AssetUrl:          assetURLView(s.api.core.GetStableAttachmentAssetURL(attachment.Id, viewerID)),
		ThumbnailAssetUrl: assetURLView(s.api.core.GetStableTransformedAttachmentAssetURL(attachment.Id, viewerID, thumbnail.width, thumbnail.height, thumbnail.fit)),
		VideoProcessing:   h.videoProcessing(attachment),
	}
}

func assetThumbnailOptions(options *apiv1.AssetThumbnailOptions) attachmentThumbnailRequest {
	width, height := 120, 120
	fit := "cover"
	if options != nil {
		if options.GetWidth() > 0 {
			width = int(options.GetWidth())
		}
		if options.GetHeight() > 0 {
			height = int(options.GetHeight())
		}
		switch options.GetFit() {
		case apiv1.AssetFitMode_ASSET_FIT_MODE_CONTAIN:
			fit = "contain"
		case apiv1.AssetFitMode_ASSET_FIT_MODE_COVER:
			fit = "cover"
		}
	}
	return attachmentThumbnailRequest{width: width, height: height, fit: fit}
}
