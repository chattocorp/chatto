package cmd

import "testing"

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
