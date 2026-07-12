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

func TestValidateBackupManifest(t *testing.T) {
	tests := []struct {
		name    string
		streams []StreamBackupInfo
		wantErr bool
	}{
		{name: "valid", streams: []StreamBackupInfo{{Name: "EVT"}, {Name: "KV_RUNTIME_STATE"}}},
		{name: "empty", streams: []StreamBackupInfo{{Name: ""}}, wantErr: true},
		{name: "parent traversal", streams: []StreamBackupInfo{{Name: "../../outside"}}, wantErr: true},
		{name: "absolute", streams: []StreamBackupInfo{{Name: "/tmp/outside"}}, wantErr: true},
		{name: "windows separator", streams: []StreamBackupInfo{{Name: `..\outside`}}, wantErr: true},
		{name: "invalid NATS punctuation", streams: []StreamBackupInfo{{Name: "bad.name"}}, wantErr: true},
		{name: "duplicate", streams: []StreamBackupInfo{{Name: "EVT"}, {Name: "EVT"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBackupManifest(BackupManifest{Streams: tt.streams})
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateBackupManifest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOpenRestoreArchive(t *testing.T) {
	file := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := os.WriteFile(file, []byte("archive"), 0600); err != nil {
		t.Fatal(err)
	}
	opened, err := openRestoreArchive(file)
	if err != nil {
		t.Fatalf("valid archive rejected: %v", err)
	}
	opened.Close()
	if opened, err := openRestoreArchive(t.TempDir()); err == nil {
		opened.Close()
		t.Fatal("directory accepted as restore archive")
	}
	if opened, err := openRestoreArchive(filepath.Join(t.TempDir(), "missing")); err == nil {
		opened.Close()
		t.Fatal("missing archive accepted")
	}
	symlink := filepath.Join(t.TempDir(), "backup-link")
	if err := os.Symlink(file, symlink); err != nil {
		t.Fatal(err)
	}
	if opened, err := openRestoreArchive(symlink); err == nil {
		opened.Close()
		t.Fatal("symlink accepted as restore archive")
	}
	large := filepath.Join(t.TempDir(), "too-large.tar.gz")
	largeFile, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := largeFile.Truncate(maxRestoreArchiveCompressedBytes + 1); err != nil {
		largeFile.Close()
		t.Fatal(err)
	}
	if err := largeFile.Close(); err != nil {
		t.Fatal(err)
	}
	if opened, err := openRestoreArchive(large); err == nil {
		opened.Close()
		t.Fatal("oversized compressed archive accepted")
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
