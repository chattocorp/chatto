package core

import (
	"testing"
	"time"

	"hmans.de/chatto/internal/config"
)

func TestAssetCacheConfigUsesSingleReplica(t *testing.T) {
	cfg := config.CoreConfig{}
	cfg.Replicas = 5
	cfg.Assets.Cache.TTL = config.Duration(3 * time.Hour)

	got := assetCacheConfig(cfg)
	if got.Bucket != "ASSET_CACHE" {
		t.Fatalf("bucket = %q, want ASSET_CACHE", got.Bucket)
	}
	if got.Replicas != 1 {
		t.Fatalf("replicas = %d, want 1", got.Replicas)
	}
	if got.TTL != 3*time.Hour {
		t.Fatalf("TTL = %s, want 3h", got.TTL)
	}
}
