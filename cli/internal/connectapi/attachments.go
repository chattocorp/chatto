package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

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
	attachments := make([]*apiv1.RoomAttachmentListItem, 0, len(result.Items))
	for _, item := range result.Items {
		if item == nil {
			continue
		}
		attachments = append(attachments, &apiv1.RoomAttachmentListItem{
			Attachment:        apiAsset(s.api, item.Attachment, caller.UserID, thumbnail),
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
	return connect.NewResponse(&apiv1.GetAssetResponse{
		Asset: apiAsset(s.api, asset, caller.UserID, assetThumbnailOptions(req.Msg.Thumbnail)),
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
	out := make([]*apiv1.Asset, 0, len(assets))
	for _, asset := range assets {
		out = append(out, apiAsset(s.api, asset, caller.UserID, thumbnail))
	}
	return connect.NewResponse(&apiv1.BatchGetAssetsResponse{Assets: out}), nil
}

func apiAsset(api *API, attachment *corev1.Attachment, viewerID string, thumbnail attachmentThumbnailRequest) *apiv1.Asset {
	if attachment == nil {
		return nil
	}
	return &apiv1.Asset{
		Id:                attachment.Id,
		Filename:          attachment.Filename,
		ContentType:       attachment.ContentType,
		Size:              attachment.Size,
		Width:             attachment.Width,
		Height:            attachment.Height,
		AssetUrl:          assetURLView(api.core.GetStableAttachmentAssetURL(attachment.Id, viewerID)),
		ThumbnailAssetUrl: assetURLView(api.core.GetStableTransformedAttachmentAssetURL(attachment.Id, viewerID, thumbnail.width, thumbnail.height, thumbnail.fit)),
		VideoProcessing:   apiVideoProcessing(api, viewerID, attachment),
	}
}

func apiVideoProcessing(api *API, viewerID string, attachment *corev1.Attachment) *apiv1.MessageVideoProcessing {
	if attachment == nil || (!strings.HasPrefix(attachment.GetContentType(), "video/") && attachment.GetContentType() != "image/gif") {
		return nil
	}

	manifest, ok := api.core.Assets.VideoAttachmentManifest(attachment.GetId())
	if !ok || manifest == nil {
		return nil
	}

	if succeeded := manifest.Succeeded; succeeded != nil {
		video := succeeded.GetVideo()
		if video == nil {
			return nil
		}
		result := &apiv1.MessageVideoProcessing{
			Status:          apiv1.MessageVideoProcessingStatus_MESSAGE_VIDEO_PROCESSING_STATUS_COMPLETED,
			DurationMs:      video.GetDurationMs(),
			Width:           video.GetWidth(),
			Height:          video.GetHeight(),
			SourceAvailable: assetSourceAvailable(api, attachment.GetId(), true),
		}
		if thumbnailID := video.GetThumbnailAssetId(); thumbnailID != "" {
			result.ThumbnailAssetUrl = assetURLView(api.core.GetStableAttachmentAssetURL(thumbnailID, viewerID))
		}
		for _, variant := range video.GetVariants() {
			if variant == nil {
				continue
			}
			var width, height int32
			var size int64
			if created, ok := api.core.Assets.AssetCreation(variant.GetAssetId()); ok {
				asset := created.GetAsset()
				if asset != nil {
					width = asset.GetWidth()
					height = asset.GetHeight()
					size = asset.GetSize()
				}
			}
			result.Variants = append(result.Variants, &apiv1.MessageVideoVariant{
				Quality:  variant.GetQuality(),
				Width:    width,
				Height:   height,
				Size:     size,
				AssetUrl: assetURLView(api.core.GetStableAttachmentAssetURL(variant.GetAssetId(), viewerID)),
			})
		}
		if hls := video.GetHls(); hls != nil && len(hls.GetRenditions()) > 0 {
			result.Hls = &apiv1.MessageVideoHLS{
				MasterPlaylistUrl: assetURLView(api.core.GetStableHLSMasterPlaylistAssetURL(attachment.GetId(), viewerID)),
			}
		}
		return result
	}

	if failed := manifest.Failed; failed != nil {
		reasonCode := assetProcessingFailureReasonCode(failed.GetFailureCode())
		return &apiv1.MessageVideoProcessing{
			Status:          apiv1.MessageVideoProcessingStatus_MESSAGE_VIDEO_PROCESSING_STATUS_FAILED,
			SourceAvailable: reasonCode != "original_missing" && assetSourceAvailable(api, attachment.GetId(), true),
			ReasonCode:      reasonCode,
		}
	}

	if manifest.Started != nil {
		return &apiv1.MessageVideoProcessing{
			Status:          apiv1.MessageVideoProcessingStatus_MESSAGE_VIDEO_PROCESSING_STATUS_PROCESSING,
			SourceAvailable: assetSourceAvailable(api, attachment.GetId(), true),
		}
	}

	return nil
}

func assetSourceAvailable(api *API, assetID string, fallback bool) bool {
	created, ok := api.core.Assets.AssetCreation(assetID)
	if !ok || created == nil {
		return fallback
	}
	return created.GetOriginalBinaryAvailable()
}

func assetProcessingFailureReasonCode(code corev1.AssetProcessingFailureCode) string {
	switch code {
	case corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING:
		return "original_missing"
	case corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED:
		return "processing_failed"
	default:
		return "processing_failed"
	}
}

func assetThumbnailOptions(options *apiv1.ImageTransformOptions) attachmentThumbnailRequest {
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
		case apiv1.ImageFitMode_IMAGE_FIT_MODE_CONTAIN:
			fit = "contain"
		case apiv1.ImageFitMode_IMAGE_FIT_MODE_COVER:
			fit = "cover"
		}
	}
	return attachmentThumbnailRequest{width: width, height: height, fit: fit}
}
