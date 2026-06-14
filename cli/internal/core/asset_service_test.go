package core

import "testing"

func TestNewAssetServiceWiresCore(t *testing.T) {
	core := &ChattoCore{}

	service := NewAssetService(core)

	if service.ChattoCore != core {
		t.Fatal("core facade was not wired")
	}
}

func TestChattoCoreAssetLifecycleLazilyInitializesService(t *testing.T) {
	core := &ChattoCore{}

	first := core.assetLifecycle()
	second := core.assetLifecycle()

	if first == nil {
		t.Fatal("asset service was not initialized")
	}
	if first != second {
		t.Fatal("asset service was not reused")
	}
	if core.assetService != first {
		t.Fatal("asset service was not stored on core")
	}
	if first.ChattoCore != core {
		t.Fatal("asset service does not point at its core facade")
	}
}
