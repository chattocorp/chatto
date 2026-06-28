package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type attachmentService struct {
	api *API
}

type attachmentThumbnailRequest struct {
	width  int
	height int
	fit    string
}

func (s *attachmentService) ListRoomAttachments(ctx context.Context, req *connect.Request[apiv1.ListRoomAttachmentsRequest]) (*connect.Response[apiv1.ListRoomAttachmentsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.api.core.ListRoomAttachments(ctx, core.ListRoomAttachmentsInput{
		ActorID: caller.UserID,
		RoomID:  req.Msg.RoomId,
		Limit:   int(req.Msg.Limit),
		Offset:  int(req.Msg.Offset),
	})
	if err != nil {
		return nil, connectError(err)
	}

	thumbnail := attachmentThumbnailOptions(req.Msg.Thumbnail)
	items := make([]*apiv1.RoomAttachmentListItem, 0, len(result.Items))
	for _, item := range result.Items {
		if item == nil {
			continue
		}
		items = append(items, &apiv1.RoomAttachmentListItem{
			Attachment:        s.attachment(item.Attachment, caller.UserID, thumbnail),
			MessageEventId:    item.MessageEventID,
			ThreadRootEventId: item.ThreadRootEventID,
			CreatedAt:         item.CreatedAt,
		})
	}

	return connect.NewResponse(&apiv1.ListRoomAttachmentsResponse{
		Items:      items,
		TotalCount: int32(result.TotalCount),
		HasMore:    result.HasMore,
	}), nil
}

func (s *attachmentService) RefreshMessageAttachmentUrls(ctx context.Context, req *connect.Request[apiv1.RefreshMessageAttachmentUrlsRequest]) (*connect.Response[apiv1.RefreshMessageAttachmentUrlsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	attachments, err := s.api.core.MessageAttachments(ctx, core.MessageAttachmentsInput{
		ActorID: caller.UserID,
		RoomID:  req.Msg.RoomId,
		EventID: req.Msg.EventId,
	})
	if err != nil {
		return nil, connectError(err)
	}

	thumbnail := attachmentThumbnailOptions(req.Msg.Thumbnail)
	items := make([]*apiv1.RefreshedAttachmentUrls, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		video := (&timelineHydrator{
			api:      s.api,
			viewerID: caller.UserID,
		}).videoProcessing(attachment)
		items = append(items, &apiv1.RefreshedAttachmentUrls{
			AttachmentId:           attachment.Id,
			AssetUrl:               assetURLView(s.api.core.GetStableAttachmentAssetURL(attachment.Id, caller.UserID)),
			ThumbnailAssetUrl:      assetURLView(s.api.core.GetStableTransformedAttachmentAssetURL(attachment.Id, caller.UserID, thumbnail.width, thumbnail.height, thumbnail.fit)),
			VideoThumbnailAssetUrl: s.videoThumbnailAssetURL(attachment, caller.UserID),
			Variants:               refreshedVideoVariants(video),
		})
	}

	return connect.NewResponse(&apiv1.RefreshMessageAttachmentUrlsResponse{
		Attachments: items,
	}), nil
}

func refreshedVideoVariants(processing *apiv1.RoomTimelineVideoProcessing) []*apiv1.RoomTimelineVideoVariant {
	if processing == nil {
		return nil
	}
	return processing.GetVariants()
}

func (s *attachmentService) attachment(attachment *corev1.Attachment, viewerID string, thumbnail attachmentThumbnailRequest) *apiv1.RoomTimelineAttachment {
	if attachment == nil {
		return nil
	}
	h := &timelineHydrator{
		api:      s.api,
		viewerID: viewerID,
	}
	return &apiv1.RoomTimelineAttachment{
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

func (s *attachmentService) videoThumbnailAssetURL(attachment *corev1.Attachment, viewerID string) *apiv1.RoomTimelineAssetUrl {
	if attachment == nil || (!strings.HasPrefix(attachment.GetContentType(), "video/") && attachment.GetContentType() != "image/gif") {
		return nil
	}
	manifest, ok := s.api.core.Assets.VideoAttachmentManifest(attachment.GetId())
	if !ok || manifest == nil || manifest.Succeeded == nil || manifest.Succeeded.GetVideo() == nil {
		return nil
	}
	thumbnailID := manifest.Succeeded.GetVideo().GetThumbnailAssetId()
	if thumbnailID == "" {
		return nil
	}
	return assetURLView(s.api.core.GetStableAttachmentAssetURL(thumbnailID, viewerID))
}

func attachmentThumbnailOptions(options *apiv1.AttachmentThumbnailOptions) attachmentThumbnailRequest {
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
		case apiv1.AttachmentFitMode_ATTACHMENT_FIT_MODE_CONTAIN:
			fit = "contain"
		case apiv1.AttachmentFitMode_ATTACHMENT_FIT_MODE_COVER:
			fit = "cover"
		}
	}
	return attachmentThumbnailRequest{width: width, height: height, fit: fit}
}
