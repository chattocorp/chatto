package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func (s *messageService) FetchLinkPreview(ctx context.Context, req *connect.Request[apiv1.FetchLinkPreviewRequest]) (*connect.Response[apiv1.FetchLinkPreviewResponse], error) {
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
	tokenURL := preview.GetUrl()
	if tokenURL == "" {
		tokenURL = req.Msg.Url
	}
	token, err := s.api.core.CreateLinkPreviewToken(ctx, tokenURL)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&apiv1.FetchLinkPreviewResponse{
		Preview:      apiLinkPreview(s.api, preview),
		PreviewToken: token,
	}), nil
}

func apiLinkPreview(api *API, preview *corev1.LinkPreview) *apiv1.LinkPreview {
	if preview == nil {
		return nil
	}

	imageAssetID := preview.GetImageAssetId()
	imageAssetKey := imageAssetID
	if image := preview.GetImageAsset(); image != nil && image.GetId() != "" {
		imageAssetID = image.GetId()
		imageAssetKey = core.ServerAssetDeliveryKey(image)
	}

	imageURL := ""
	if imageAssetKey != "" {
		imageURL = api.core.GetTransformedServerAssetURL(imageAssetKey, 600, 314, "contain")
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
	if socialPost := preview.GetSocialPost(); socialPost != nil {
		out.SocialPost = apiSocialPostPreview(api, socialPost, 0)
	}
	return out
}

func apiSocialPostPreview(api *API, socialPost *corev1.SocialPostPreview, quoteDepth int) *apiv1.SocialPostPreview {
	if socialPost == nil {
		return nil
	}
	mapped := &apiv1.SocialPostPreview{
		Provider:       socialPost.GetProvider(),
		Text:           socialPost.GetText(),
		PublishedAt:    socialPost.GetPublishedAt(),
		ContentWarning: optionalString(socialPost.GetContentWarning()),
		Url:            socialPost.GetUrl(),
	}
	if author := socialPost.GetAuthor(); author != nil {
		mapped.Author = &apiv1.SocialPostAuthor{
			DisplayName: author.GetDisplayName(),
			Handle:      author.GetHandle(),
		}
		mapped.Author.AvatarUrl, mapped.Author.AvatarAssetId = linkPreviewAsset(api, author.GetAvatarAsset(), 96, 96, "cover")
	}
	if external := socialPost.GetExternalLink(); external != nil {
		mapped.ExternalLink = &apiv1.SocialPostExternalLink{
			Url:         external.GetUrl(),
			Title:       optionalString(external.GetTitle()),
			Description: optionalString(external.GetDescription()),
		}
		mapped.ExternalLink.ImageUrl, mapped.ExternalLink.ImageAssetId = linkPreviewAsset(api, external.GetImageAsset(), 600, 314, "contain")
	}
	for _, image := range socialPost.GetImages() {
		imageURL, assetID := linkPreviewAsset(api, image.GetAsset(), 600, 600, "contain")
		if imageURL == nil || assetID == nil {
			continue
		}
		mapped.Images = append(mapped.Images, &apiv1.SocialPostImage{
			Url: *imageURL, AssetId: *assetID, Alt: optionalString(image.GetAlt()),
			Width: optionalUint32(image.GetWidth()), Height: optionalUint32(image.GetHeight()),
		})
	}
	if quoteDepth == 0 {
		mapped.QuotedPost = apiSocialPostPreview(api, socialPost.GetQuotedPost(), quoteDepth+1)
	}
	return mapped
}

func linkPreviewAsset(api *API, asset *corev1.AssetRecord, width, height int, fit string) (*string, *string) {
	if asset == nil || asset.GetId() == "" {
		return nil, nil
	}
	assetID := asset.GetId()
	url := api.core.GetTransformedServerAssetURL(core.ServerAssetDeliveryKey(asset), width, height, fit)
	if url == "" {
		return nil, &assetID
	}
	return &url, &assetID
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalUint32(value uint32) *uint32 {
	if value == 0 {
		return nil
	}
	return &value
}
