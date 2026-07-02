package connectapi

import (
	"context"

	"connectrpc.com/connect"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type linkPreviewService struct {
	api *API
}

func (s *linkPreviewService) FetchLinkPreview(ctx context.Context, req *connect.Request[apiv1.FetchLinkPreviewRequest]) (*connect.Response[apiv1.FetchLinkPreviewResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	preview, err := s.api.core.GetLinkPreview(ctx, req.Msg.Url)
	if err != nil {
		return nil, connectError(err)
	}
	if preview == nil {
		return connect.NewResponse(&apiv1.FetchLinkPreviewResponse{}), nil
	}

	return connect.NewResponse(&apiv1.FetchLinkPreviewResponse{
		Preview: apiLinkPreview(s.api, preview),
	}), nil
}

func apiLinkPreview(api *API, preview *corev1.LinkPreview) *apiv1.LinkPreview {
	if preview == nil {
		return nil
	}

	imageAssetID := preview.GetImageAssetId()
	if image := preview.GetImageAsset(); image != nil && image.GetId() != "" {
		imageAssetID = image.GetId()
	}

	imageURL := ""
	if imageAssetID != "" {
		imageURL = api.core.GetTransformedServerAssetURL(imageAssetID, 600, 314, "contain")
	}

	out := &apiv1.LinkPreview{
		Url: preview.GetUrl(),
	}
	if title := preview.GetTitle(); title != "" {
		out.Title = stringPtr(title)
	}
	if description := preview.GetDescription(); description != "" {
		out.Description = stringPtr(description)
	}
	if imageURL != "" {
		out.ImageUrl = stringPtr(imageURL)
	}
	if imageAssetID != "" {
		out.ImageAssetId = stringPtr(imageAssetID)
	}
	if siteName := preview.GetSiteName(); siteName != "" {
		out.SiteName = stringPtr(siteName)
	}
	if embedType := preview.GetEmbedType(); embedType != "" {
		out.EmbedType = stringPtr(embedType)
	}
	if embedID := preview.GetEmbedId(); embedID != "" {
		out.EmbedId = stringPtr(embedID)
	}
	return out
}
