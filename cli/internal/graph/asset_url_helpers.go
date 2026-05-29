package graph

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/auth"
	"hmans.de/chatto/internal/graph/model"
)

func stableAssetURLModel(assetURL core.StableAssetURL) *model.AssetURL {
	return &model.AssetURL{
		URL:       assetURL.URL,
		ExpiresAt: timestamppb.New(assetURL.ExpiresAt),
	}
}

func callerID(ctx context.Context) string {
	user := auth.ForContext(ctx)
	if user == nil {
		return ""
	}
	return user.Id
}
