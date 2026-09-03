package core

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/charmbracelet/log"

	"hmans.de/chatto/internal/config"
)

func TestEVTReadCacheConfigLogsEffectiveDefaults(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output)
	logger.SetFormatter(log.JSONFormatter)

	readerConfig := evtReadCacheConfig(config.CoreConfig{}, logger)
	if readerConfig.CacheIdleTTL != 15*time.Minute {
		t.Fatalf("cache idle TTL = %s, want 15m", readerConfig.CacheIdleTTL)
	}
	if readerConfig.CacheMaxBytes != 256<<20 {
		t.Fatalf("cache maximum bytes = %d, want 256 MiB", readerConfig.CacheMaxBytes)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode startup log: %v", err)
	}
	if entry["msg"] != "EVT read cache configured" || entry["idle_ttl"] != "15m0s" || entry["max_bytes"] != float64(256<<20) {
		t.Fatalf("startup log = %v", entry)
	}
}

func TestEVTReadCacheConfigMapsUnlimitedBytesToFrameworkZero(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output)
	logger.SetFormatter(log.JSONFormatter)
	limit := config.ByteSizeLimit(-1)
	readerConfig := evtReadCacheConfig(config.CoreConfig{EVTReadCacheMaxBytes: &limit}, logger)
	if readerConfig.CacheMaxBytes != 0 {
		t.Fatalf("framework cache maximum bytes = %d, want 0", readerConfig.CacheMaxBytes)
	}
	var entry map[string]interface{}
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode startup log: %v", err)
	}
	if entry["max_bytes"] != float64(-1) {
		t.Fatalf("startup maximum bytes = %v, want -1", entry["max_bytes"])
	}
}
