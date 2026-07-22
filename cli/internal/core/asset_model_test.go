package core

import "testing"

func TestNewAssetModelWiresCore(t *testing.T) {
	core := &ChattoCore{}

	service := NewAssetModel(core)

	if service.ChattoCore != core {
		t.Fatal("core facade was not wired")
	}
}
