package core

import (
	"context"
	"testing"

	"hmans.de/chatto/internal/events"
)

func TestNewAssetModelWiresCore(t *testing.T) {
	core := &ChattoCore{}
	projection := NewAssetProjection()

	service := newTestAssetModel(t, core, projection, nil)

	if service.ChattoCore != core {
		t.Fatal("core facade was not wired")
	}
	if service.assets.Projection() != projection || service.assets.Projector() == nil {
		t.Fatal("asset projection dependencies were not wired")
	}
}

func TestAssetModelMissingProjectionFailsClosed(t *testing.T) {
	model := newTestAssetModel(t, &ChattoCore{}, nil, nil)

	if got := model.AssetState(NewAssetID()); got != (AssetState{}) {
		t.Fatalf("AssetState = %#v, want zero state", got)
	}
	if err := model.waitForAssets(context.Background(), events.StreamPosition{}); err == nil {
		t.Fatal("waitForAssets returned nil without a projector")
	}
	if err := model.waitForAssetsCurrent(context.Background()); err == nil {
		t.Fatal("waitForAssetsCurrent returned nil without a projector")
	}
}

func TestChattoCoreAssetBoundaryFailsClosedBeforeInitialization(t *testing.T) {
	core := &ChattoCore{}

	if got := core.GetAssetState(NewAssetID()); got != (AssetState{}) {
		t.Fatalf("GetAssetState = %#v, want zero state", got)
	}
	if _, ok := core.ResolvePublicServerAsset(context.Background(), NewAssetID()); ok {
		t.Fatal("ResolvePublicServerAsset accepted an asset without an initialized model")
	}
	if _, _, ok := core.AssetEventTimelineTarget(nil); ok {
		t.Fatal("AssetEventTimelineTarget resolved without an initialized model")
	}

	core.assetModel = newTestAssetModel(t, core, nil, nil)
	if _, ok := core.ResolvePublicServerAsset(context.Background(), NewAssetID()); ok {
		t.Fatal("ResolvePublicServerAsset accepted an asset without an initialized projection")
	}
}
