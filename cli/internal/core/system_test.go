package core

import (
	"testing"
)

func TestChattoCore_GetConnectionInfo(t *testing.T) {
	core, _ := setupTestCore(t)

	t.Run("returns connection info", func(t *testing.T) {
		info := core.GetConnectionInfo()
		if info == nil {
			t.Fatal("expected non-nil connection info")
		}

		// Should be connected in tests
		if !info.Connected {
			t.Error("expected to be connected")
		}
	})

	t.Run("has valid server info when connected", func(t *testing.T) {
		info := core.GetConnectionInfo()
		if !info.Connected {
			t.Skip("not connected, skipping server info tests")
		}

		// Server ID should be non-empty
		if info.ServerID == "" {
			t.Error("expected non-empty ServerID when connected")
		}

		// Version should be non-empty
		if info.Version == "" {
			t.Error("expected non-empty Version when connected")
		}

		// MaxPayload should be > 0
		if info.MaxPayload <= 0 {
			t.Errorf("expected positive MaxPayload, got %d", info.MaxPayload)
		}
	})
}

func TestChattoCore_GetAccountInfo(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns account info", func(t *testing.T) {
		info, err := core.GetAccountInfo(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if info == nil {
			t.Fatal("expected non-nil account info")
		}

		// StreamsUsed should be >= 0
		if info.StreamsUsed < 0 {
			t.Errorf("expected non-negative StreamsUsed, got %d", info.StreamsUsed)
		}
	})

	t.Run("returns positive numbers for storage usage", func(t *testing.T) {
		// Current storage is deployment-wide and created at boot. Creating a
		// Space record (or rooms within it) doesn't add streams; what's there
		// at boot is what's there. We just check the numbers look sensible.
		info, err := core.GetAccountInfo(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.StreamsUsed <= 0 {
			t.Errorf("expected positive StreamsUsed, got %d", info.StreamsUsed)
		}
	})
}

func TestChattoCore_GetJetStreamStats(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	stats, err := core.GetJetStreamStats(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
	if len(stats.Streams) == 0 {
		t.Fatal("expected at least one stream")
	}

	var foundEVT bool
	for _, stream := range stats.Streams {
		if stream.Name == "EVT" {
			foundEVT = true
		}
		if stream.Name == "" {
			t.Error("stream should expose a name")
		}
		if stream.Storage == "" {
			t.Errorf("stream %q should expose storage", stream.Name)
		}
	}
	if !foundEVT {
		t.Error("expected EVT stream in stats")
	}
}

func TestConnectionInfo_Fields(t *testing.T) {
	t.Run("ConnectionInfo has all expected fields", func(t *testing.T) {
		info := ConnectionInfo{
			Connected:  true,
			ServerID:   "server-123",
			ServerName: "test-server",
			Version:    "2.10.0",
			MaxPayload: 1048576,
			RTT:        "1ms",
		}

		if !info.Connected {
			t.Error("expected Connected true")
		}
		if info.ServerID != "server-123" {
			t.Errorf("expected ServerID 'server-123', got '%s'", info.ServerID)
		}
		if info.ServerName != "test-server" {
			t.Errorf("expected ServerName 'test-server', got '%s'", info.ServerName)
		}
		if info.Version != "2.10.0" {
			t.Errorf("expected Version '2.10.0', got '%s'", info.Version)
		}
		if info.MaxPayload != 1048576 {
			t.Errorf("expected MaxPayload 1048576, got %d", info.MaxPayload)
		}
		if info.RTT != "1ms" {
			t.Errorf("expected RTT '1ms', got '%s'", info.RTT)
		}
	})
}

func TestAccountInfo_Fields(t *testing.T) {
	t.Run("AccountInfo has all expected fields", func(t *testing.T) {
		info := AccountInfo{
			Memory:        1073741824,
			MemoryUsed:    536870912,
			Storage:       10737418240,
			StorageUsed:   5368709120,
			Streams:       100,
			StreamsUsed:   50,
			Consumers:     1000,
			ConsumersUsed: 250,
		}

		if info.Memory != 1073741824 {
			t.Errorf("expected Memory 1073741824, got %d", info.Memory)
		}
		if info.MemoryUsed != 536870912 {
			t.Errorf("expected MemoryUsed 536870912, got %d", info.MemoryUsed)
		}
		if info.Storage != 10737418240 {
			t.Errorf("expected Storage 10737418240, got %d", info.Storage)
		}
		if info.StorageUsed != 5368709120 {
			t.Errorf("expected StorageUsed 5368709120, got %d", info.StorageUsed)
		}
		if info.Streams != 100 {
			t.Errorf("expected Streams 100, got %d", info.Streams)
		}
		if info.StreamsUsed != 50 {
			t.Errorf("expected StreamsUsed 50, got %d", info.StreamsUsed)
		}
		if info.Consumers != 1000 {
			t.Errorf("expected Consumers 1000, got %d", info.Consumers)
		}
		if info.ConsumersUsed != 250 {
			t.Errorf("expected ConsumersUsed 250, got %d", info.ConsumersUsed)
		}
	})
}
