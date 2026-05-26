package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nats-io/jsm.go/api"
)

func TestManifestIncludesEncryptionKeys(t *testing.T) {
	tests := []struct {
		name    string
		streams []StreamBackupInfo
		want    bool
	}{
		{
			name: "keys included as kv",
			streams: []StreamBackupInfo{
				{Name: "KV_INSTANCE", Type: "kv", Messages: 10},
				{Name: "KV_ENCRYPTION_KEYS", Type: "kv", Messages: 4, Bytes: 1401},
			},
			want: true,
		},
		{
			name: "keys explicitly skipped",
			streams: []StreamBackupInfo{
				{Name: "KV_INSTANCE", Type: "kv", Messages: 10},
				{Name: "KV_ENCRYPTION_KEYS", Type: "skipped"},
			},
			want: false,
		},
		{
			name: "keys present but errored",
			streams: []StreamBackupInfo{
				{Name: "KV_ENCRYPTION_KEYS", Type: "kv", Error: "snapshot failed"},
			},
			want: false,
		},
		{
			name: "keys absent from manifest",
			streams: []StreamBackupInfo{
				{Name: "KV_INSTANCE", Type: "kv"},
				{Name: "KV_USER_PRESENCE", Type: "skipped"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manifestIncludesEncryptionKeys(BackupManifest{Streams: tt.streams})
			if got != tt.want {
				t.Errorf("manifestIncludesEncryptionKeys() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRestoreConfigForTargetOverridesReplicas(t *testing.T) {
	streamDir := t.TempDir()
	writeRestoreMetadata(t, streamDir, api.JSApiStreamRestoreRequest{
		Config: api.StreamConfig{
			Name:     "KV_INSTANCE",
			Replicas: 3,
		},
	})

	override, err := restoreConfigForTarget(streamDir, "KV_INSTANCE", 1)
	if err != nil {
		t.Fatal(err)
	}
	if override == nil {
		t.Fatal("override = nil, want replica override")
	}
	if override.backupReplicas != 3 {
		t.Errorf("backupReplicas = %d, want 3", override.backupReplicas)
	}
	if override.config.Replicas != 1 {
		t.Errorf("config.Replicas = %d, want 1", override.config.Replicas)
	}
}

func TestRestoreConfigForTargetKeepsMatchingReplicas(t *testing.T) {
	streamDir := t.TempDir()
	writeRestoreMetadata(t, streamDir, api.JSApiStreamRestoreRequest{
		Config: api.StreamConfig{
			Name:     "KV_INSTANCE",
			Replicas: 1,
		},
	})

	override, err := restoreConfigForTarget(streamDir, "KV_INSTANCE", 1)
	if err != nil {
		t.Fatal(err)
	}
	if override != nil {
		t.Fatalf("override = %#v, want nil", override)
	}
}

func writeRestoreMetadata(t *testing.T, streamDir string, req api.JSApiStreamRestoreRequest) {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(streamDir, "backup.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}
