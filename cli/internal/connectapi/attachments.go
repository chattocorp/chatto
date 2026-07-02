package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type attachmentMapper struct {
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

	thumbnail := attachmentThumbnailOptions(req.Msg.Thumbnail)
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

func (s *messageService) RefreshMessageAttachmentUrls(ctx context.Context, req *connect.Request[apiv1.RefreshMessageAttachmentUrlsRequest]) (*connect.Response[apiv1.RefreshMessageAttachmentUrlsResponse], error) {
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
	mapper := attachmentMapper{api: s.api}
	items := make([]*apiv1.RefreshedAttachmentUrls, 0, len(attachments))
	for _, attachment := range attachments {
		if refreshed := mapper.refreshedAttachmentUrls(attachment, caller.UserID, thumbnail); refreshed != nil {
			items = append(items, refreshed)
		}
	}

	return connect.NewResponse(&apiv1.RefreshMessageAttachmentUrlsResponse{
		Attachments: items,
	}), nil
}

func (s *messageService) BatchRefreshMessageAttachmentUrls(ctx context.Context, req *connect.Request[apiv1.BatchRefreshMessageAttachmentUrlsRequest]) (*connect.Response[apiv1.BatchRefreshMessageAttachmentUrlsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	sets, err := s.api.core.BatchMessageAttachments(ctx, core.BatchMessageAttachmentsInput{
		ActorID:  caller.UserID,
		RoomID:   req.Msg.RoomId,
		EventIDs: req.Msg.GetEventIds(),
	})
	if err != nil {
		return nil, connectError(err)
	}

	thumbnail := attachmentThumbnailOptions(req.Msg.Thumbnail)
	mapper := attachmentMapper{api: s.api}
	messages := make([]*apiv1.RefreshedMessageAttachmentUrls, 0, len(sets))
	for _, set := range sets {
		if set == nil {
			continue
		}
		attachments := make([]*apiv1.RefreshedAttachmentUrls, 0, len(set.Attachments))
		for _, attachment := range set.Attachments {
			if refreshed := mapper.refreshedAttachmentUrls(attachment, caller.UserID, thumbnail); refreshed != nil {
				attachments = append(attachments, refreshed)
			}
		}
		messages = append(messages, &apiv1.RefreshedMessageAttachmentUrls{
			EventId:     set.EventID,
			Attachments: attachments,
		})
	}

	return connect.NewResponse(&apiv1.BatchRefreshMessageAttachmentUrlsResponse{
		Messages: messages,
	}), nil
}

func refreshedVideoVariants(processing *apiv1.RoomTimelineVideoProcessing) []*apiv1.RoomTimelineVideoVariant {
	if processing == nil {
		return nil
	}
	return processing.GetVariants()
}

func (s *attachmentMapper) refreshedAttachmentUrls(attachment *corev1.Attachment, viewerID string, thumbnail attachmentThumbnailRequest) *apiv1.RefreshedAttachmentUrls {
	if attachment == nil {
		return nil
	}
	video := (&timelineHydrator{
		api:      s.api,
		viewerID: viewerID,
	}).videoProcessing(attachment)
	return &apiv1.RefreshedAttachmentUrls{
		AttachmentId:           attachment.Id,
		AssetUrl:               assetURLView(s.api.core.GetStableAttachmentAssetURL(attachment.Id, viewerID)),
		ThumbnailAssetUrl:      assetURLView(s.api.core.GetStableTransformedAttachmentAssetURL(attachment.Id, viewerID, thumbnail.width, thumbnail.height, thumbnail.fit)),
		VideoThumbnailAssetUrl: s.videoThumbnailAssetURL(attachment, viewerID),
		Variants:               refreshedVideoVariants(video),
	}
}

func (s *attachmentMapper) attachment(attachment *corev1.Attachment, viewerID string, thumbnail attachmentThumbnailRequest) *apiv1.RoomTimelineAttachment {
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

func (s *attachmentMapper) videoThumbnailAssetURL(attachment *corev1.Attachment, viewerID string) *apiv1.RoomTimelineAssetUrl {
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
