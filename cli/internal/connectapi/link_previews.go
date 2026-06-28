package connectapi

import (
	"context"

	"connectrpc.com/connect"
	appv1 "hmans.de/chatto/internal/pb/chatto/app/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type linkPreviewService struct {
	api *API
}

func (s *linkPreviewService) FetchLinkPreview(ctx context.Context, req *connect.Request[appv1.FetchLinkPreviewRequest]) (*connect.Response[appv1.FetchLinkPreviewResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}

	preview, err := s.api.core.GetLinkPreview(ctx, req.Msg.Url)
	if err != nil || preview == nil {
		return connect.NewResponse(&appv1.FetchLinkPreviewResponse{}), nil
	}

	return connect.NewResponse(&appv1.FetchLinkPreviewResponse{
		Preview: apiFetchedLinkPreview(s.api, preview),
	}), nil
}

func apiFetchedLinkPreview(api *API, preview *corev1.LinkPreview) *appv1.FetchedLinkPreview {
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

	return &appv1.FetchedLinkPreview{
		Url:          preview.GetUrl(),
		Title:        preview.GetTitle(),
		Description:  preview.GetDescription(),
		ImageUrl:     imageURL,
		ImageAssetId: imageAssetID,
		SiteName:     preview.GetSiteName(),
		EmbedType:    preview.GetEmbedType(),
		EmbedId:      preview.GetEmbedId(),
	}
}
