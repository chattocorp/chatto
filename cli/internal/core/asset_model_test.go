package core

import "testing"

func TestNewAssetModelWiresCore(t *testing.T) {
	core := &ChattoCore{}

	service := NewAssetModel(core)

	if service.ChattoCore != core {
		t.Fatal("core facade was not wired")
	}
}

func TestChattoCoreAssetLifecycleLazilyInitializesModel(t *testing.T) {
	core := &ChattoCore{}

	first := core.assetLifecycle()
	second := core.assetLifecycle()

	if first == nil {
		t.Fatal("asset model was not initialized")
	}
	if first != second {
		t.Fatal("asset model was not reused")
	}
	if core.assetModel != first {
		t.Fatal("asset model was not stored on core")
	}
	if first.ChattoCore != core {
		t.Fatal("asset model does not point at its core facade")
	}
}
