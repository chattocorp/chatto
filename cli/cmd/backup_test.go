package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jsm.go"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/testutil"
)

func TestSkipReason(t *testing.T) {
	tests := []struct {
		name        string
		includeKeys bool
		wantSkip    bool
		wantReason  string
	}{
		// Should be skipped (default: includeKeys=false)
		{"KV_MEMORY_CACHE", false, true, "ephemeral (memory storage)"},
		{"KV_USER_PRESENCE", false, true, "ephemeral (memory storage)"},
		{"KV_CALL_STATE", false, true, "ephemeral (memory storage)"},
		{"KV_ENCRYPTION_KEYS", false, true, "security (keys excluded from backups; pass --include-keys to override)"},
		{"KV_LINK_PREVIEW_CACHE", false, true, "cache (regeneratable)"},
		{"KV_AUTH_TOKENS", false, true, "security (prevents token leakage)"},
		{"OBJ_ASSET_CACHE", false, true, "cache (regeneratable)"},

		// With --include-keys, KV_ENCRYPTION_KEYS is backed up; others stay skipped.
		{"KV_ENCRYPTION_KEYS", true, false, ""},
		{"KV_MEMORY_CACHE", true, true, "ephemeral (memory storage)"},
		{"KV_USER_PRESENCE", true, true, "ephemeral (memory storage)"},
		{"KV_AUTH_TOKENS", true, true, "security (prevents token leakage)"},

		// Should NOT be skipped regardless of includeKeys. OBJ_INSTANCE_ASSETS
		// is legacy pre-0.1 data, but 0.0.x backups must preserve it so 0.1
		// boot can copy those objects into SERVER_ASSETS.
		{"KV_INSTANCE", false, false, ""},
		{"KV_INSTANCE_RBAC", false, false, ""},
		{"KV_INSTANCE_CONFIG", false, false, ""},
		{"KV_RUNTIME_STATE", false, false, ""},
		{"OBJ_INSTANCE_ASSETS", false, false, ""},
		{"SPACE_abc123_EVENTS", false, false, ""},
		{"KV_SPACE_abc123_CONFIG", false, false, ""},
		{"KV_SPACE_abc123_RBAC", false, false, ""},
		{"KV_SPACE_abc123_RUNTIME", false, false, ""},
		{"KV_SPACE_abc123_BODIES", false, false, ""},
		{"KV_SPACE_abc123_REACTIONS", false, false, ""},
		{"KV_SPACE_abc123_THREADS", false, false, ""},
		{"OBJ_SPACE_abc123_ASSETS", false, false, ""},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/includeKeys=%v", tt.name, tt.includeKeys), func(t *testing.T) {
			reason := skipReason(tt.name, tt.includeKeys)
			if tt.wantSkip {
				if reason == "" {
					t.Errorf("expected stream %q to be skipped, but got no reason", tt.name)
				}
				if reason != tt.wantReason {
					t.Errorf("expected reason %q, got %q", tt.wantReason, reason)
				}
			} else {
				if reason != "" {
					t.Errorf("expected stream %q to NOT be skipped, but got reason %q", tt.name, reason)
				}
			}
		})
	}
}

func TestClassifyStream(t *testing.T) {
	tests := []struct {
		name     string
		wantType string
	}{
		{"KV_INSTANCE", "kv"},
		{"KV_SPACE_abc_CONFIG", "kv"},
		{"OBJ_INSTANCE_ASSETS", "object_store"},
		{"OBJ_SPACE_abc_ASSETS", "object_store"},
		{"SPACE_abc_EVENTS", "stream"},
		{"SOME_OTHER_STREAM", "stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyStream(tt.name)
			if got != tt.wantType {
				t.Errorf("classifyStream(%q) = %q, want %q", tt.name, got, tt.wantType)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestCreateAndExtractTarGz(t *testing.T) {
	// Create a temp source directory with test files
	srcDir := t.TempDir()
	subDir := filepath.Join(srcDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write test files
	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create archive
	archivePath := filepath.Join(t.TempDir(), "test.tar.gz")
	if err := createTarGz(srcDir, archivePath); err != nil {
		t.Fatal("createTarGz failed:", err)
	}

	// Verify archive exists
	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatal("archive not created:", err)
	}
	if info.Size() == 0 {
		t.Fatal("archive is empty")
	}

	// Extract archive
	extractDir := t.TempDir()
	if err := extractTarGz(archivePath, extractDir); err != nil {
		t.Fatal("extractTarGz failed:", err)
	}

	// The archive should contain a directory named after the source
	baseName := filepath.Base(srcDir)
	extractedBase := filepath.Join(extractDir, baseName)

	// Verify extracted files
	content1, err := os.ReadFile(filepath.Join(extractedBase, "file1.txt"))
	if err != nil {
		t.Fatal("failed to read extracted file1.txt:", err)
	}
	if string(content1) != "hello" {
		t.Errorf("file1.txt content = %q, want %q", string(content1), "hello")
	}

	content2, err := os.ReadFile(filepath.Join(extractedBase, "subdir", "file2.txt"))
	if err != nil {
		t.Fatal("failed to read extracted subdir/file2.txt:", err)
	}
	if string(content2) != "world" {
		t.Errorf("file2.txt content = %q, want %q", string(content2), "world")
	}
}

// startTestNATS starts an embedded NATS server for testing and returns the
// server, a connected client, and a JetStream context. Cleanup is automatic.
func startTestNATS(t *testing.T) (*server.Server, *nats.Conn, jetstream.JetStream) {
	t.Helper()

	ns, nc := testutil.StartNATS(t)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("Failed to create JetStream context: %v", err)
	}

	return ns, nc, js
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --- Source server: create test data ---

	_, srcNC, srcJS := startTestNATS(t)

	// Create a KV bucket with some entries
	kv, err := srcJS.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "TEST_DATA",
		Storage: jetstream.FileStorage,
	})
	if err != nil {
		t.Fatal("Failed to create KV bucket:", err)
	}

	testEntries := map[string]string{
		"user.alice": `{"name":"Alice"}`,
		"user.bob":   `{"name":"Bob"}`,
		"config.app": `{"theme":"dark"}`,
	}
	for k, v := range testEntries {
		if _, err := kv.PutString(ctx, k, v); err != nil {
			t.Fatalf("Failed to put KV entry %s: %v", k, err)
		}
	}

	// Create a regular stream with messages
	_, err = srcJS.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "TEST_EVENTS",
		Subjects: []string{"events.>"},
		Storage:  jetstream.FileStorage,
	})
	if err != nil {
		t.Fatal("Failed to create stream:", err)
	}

	testMessages := []string{
		"events.room1.msg1",
		"events.room1.msg2",
		"events.room2.msg1",
	}
	for _, subj := range testMessages {
		if _, err := srcJS.Publish(ctx, subj, []byte("payload:"+subj)); err != nil {
			t.Fatalf("Failed to publish to %s: %v", subj, err)
		}
	}

	// Create a memory-only stream (should be skipped)
	_, err = srcJS.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "MEMORY_CACHE",
		Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatal("Failed to create memory KV:", err)
	}

	// --- Backup: snapshot all streams ---

	srcMgr, err := jsm.New(srcNC)
	if err != nil {
		t.Fatal("Failed to create JSM manager:", err)
	}

	streamNames, err := enumerateStreams(ctx, srcJS)
	if err != nil {
		t.Fatal("Failed to enumerate streams:", err)
	}

	backupDir := filepath.Join(t.TempDir(), "backup")
	streamsDir := filepath.Join(backupDir, "streams")
	if err := os.MkdirAll(streamsDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifest := BackupManifest{
		Version:   1,
		CreatedAt: time.Now().UTC(),
		Streams:   make([]StreamBackupInfo, 0),
	}

	for i, name := range streamNames {
		info := backupStream(ctx, srcMgr, name, streamsDir, i+1, len(streamNames), false)
		manifest.Streams = append(manifest.Streams, info)

		if info.Error != "" {
			manifest.Stats.Failed++
		} else if info.Type == "skipped" {
			manifest.Stats.Skipped++
		} else {
			manifest.Stats.TotalBytes += info.Bytes
		}
	}
	manifest.Stats.TotalStreams = len(streamNames) - manifest.Stats.Skipped - manifest.Stats.Failed

	// Verify the backup skipped MEMORY_CACHE
	var skippedMemoryCache bool
	for _, s := range manifest.Streams {
		if s.Name == "KV_MEMORY_CACHE" && s.Type == "skipped" {
			skippedMemoryCache = true
		}
	}
	if !skippedMemoryCache {
		t.Error("Expected KV_MEMORY_CACHE to be skipped in backup")
	}

	// Write manifest
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "manifest.json"), manifestData, 0644); err != nil {
		t.Fatal(err)
	}

	// Create tar.gz archive
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := createTarGz(backupDir, archivePath); err != nil {
		t.Fatal("Failed to create archive:", err)
	}

	// --- Restore: extract archive and restore to a fresh server ---

	_, dstNC, dstJS := startTestNATS(t)

	// Extract archive
	extractDir := t.TempDir()
	if err := extractTarGz(archivePath, extractDir); err != nil {
		t.Fatal("Failed to extract archive:", err)
	}

	// Find the backup dir inside the extract
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatal("Unexpected archive structure")
	}
	restoredBackupDir := filepath.Join(extractDir, entries[0].Name())

	// Read manifest from extracted archive
	restoredManifestData, err := os.ReadFile(filepath.Join(restoredBackupDir, "manifest.json"))
	if err != nil {
		t.Fatal("Failed to read restored manifest:", err)
	}

	var restoredManifest BackupManifest
	if err := json.Unmarshal(restoredManifestData, &restoredManifest); err != nil {
		t.Fatal("Failed to parse restored manifest:", err)
	}

	dstMgr, err := jsm.New(dstNC)
	if err != nil {
		t.Fatal("Failed to create dst JSM manager:", err)
	}

	restoredStreamsDir := filepath.Join(restoredBackupDir, "streams")

	var restoredCount int
	for _, streamInfo := range restoredManifest.Streams {
		if streamInfo.Type == "skipped" || streamInfo.Error != "" {
			continue
		}

		streamDir := filepath.Join(restoredStreamsDir, streamInfo.Name)
		_, _, err := dstMgr.RestoreSnapshotFromDirectory(ctx, streamInfo.Name, streamDir)
		if err != nil {
			t.Fatalf("Failed to restore stream %s: %v", streamInfo.Name, err)
		}
		restoredCount++
	}

	if restoredCount != manifest.Stats.TotalStreams {
		t.Errorf("Restored %d streams, expected %d", restoredCount, manifest.Stats.TotalStreams)
	}

	// --- Verify: check all data survived the round-trip ---

	// Verify KV data
	restoredKV, err := dstJS.KeyValue(ctx, "TEST_DATA")
	if err != nil {
		t.Fatal("Failed to open restored KV bucket:", err)
	}

	for k, want := range testEntries {
		entry, err := restoredKV.Get(ctx, k)
		if err != nil {
			t.Fatalf("Failed to get restored KV entry %s: %v", k, err)
		}
		if string(entry.Value()) != want {
			t.Errorf("KV entry %s = %q, want %q", k, string(entry.Value()), want)
		}
	}

	// Verify stream messages
	restoredStream, err := dstJS.Stream(ctx, "TEST_EVENTS")
	if err != nil {
		t.Fatal("Failed to open restored stream:", err)
	}

	info, err := restoredStream.Info(ctx)
	if err != nil {
		t.Fatal("Failed to get stream info:", err)
	}
	if info.State.Msgs != uint64(len(testMessages)) {
		t.Errorf("Stream has %d messages, want %d", info.State.Msgs, len(testMessages))
	}

	// Read back each message and verify payload
	cons, err := restoredStream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatal("Failed to create consumer:", err)
	}

	for i := range len(testMessages) {
		msg, err := cons.Next()
		if err != nil {
			t.Fatalf("Failed to fetch message %d: %v", i, err)
		}
		wantPayload := "payload:" + msg.Subject()
		if string(msg.Data()) != wantPayload {
			t.Errorf("Message %d: payload = %q, want %q", i, string(msg.Data()), wantPayload)
		}
	}

	// Verify MEMORY_CACHE was NOT restored (it was skipped)
	_, err = dstJS.KeyValue(ctx, "MEMORY_CACHE")
	if err == nil {
		t.Error("MEMORY_CACHE should not have been restored (was skipped during backup)")
	}
}

func TestEncryptedTarGzRoundTrip(t *testing.T) {
	// Create a temp source directory with test files
	srcDir := t.TempDir()
	subDir := filepath.Join(srcDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("hello encrypted"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("world encrypted"), 0644); err != nil {
		t.Fatal(err)
	}

	passphrase := "test-backup-passphrase"
	archivePath := filepath.Join(t.TempDir(), "test.tar.gz.age")

	// Create encrypted archive
	if err := createEncryptedTarGz(srcDir, archivePath, passphrase); err != nil {
		t.Fatal("createEncryptedTarGz failed:", err)
	}

	// Verify archive starts with age header
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal("Failed to read archive:", err)
	}
	if !strings.HasPrefix(string(data), "age-encryption.org/v1\n") {
		t.Error("Encrypted archive does not start with age header")
	}

	// Extract with correct passphrase
	extractDir := t.TempDir()
	if err := extractEncryptedTarGz(archivePath, extractDir, passphrase); err != nil {
		t.Fatal("extractEncryptedTarGz failed:", err)
	}

	baseName := filepath.Base(srcDir)
	extractedBase := filepath.Join(extractDir, baseName)

	content1, err := os.ReadFile(filepath.Join(extractedBase, "file1.txt"))
	if err != nil {
		t.Fatal("failed to read extracted file1.txt:", err)
	}
	if string(content1) != "hello encrypted" {
		t.Errorf("file1.txt content = %q, want %q", string(content1), "hello encrypted")
	}

	content2, err := os.ReadFile(filepath.Join(extractedBase, "subdir", "file2.txt"))
	if err != nil {
		t.Fatal("failed to read extracted subdir/file2.txt:", err)
	}
	if string(content2) != "world encrypted" {
		t.Errorf("file2.txt content = %q, want %q", string(content2), "world encrypted")
	}

	// Extract with wrong passphrase should fail
	failDir := t.TempDir()
	if err := extractEncryptedTarGz(archivePath, failDir, "wrong-passphrase"); err == nil {
		t.Error("Expected extraction to fail with wrong passphrase")
	}
}

func TestIsAgeEncrypted(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an age-encrypted file
	encFile := filepath.Join(tmpDir, "encrypted.age")
	if err := os.WriteFile(encFile, []byte("age-encryption.org/v1\nsome data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a plain file
	plainFile := filepath.Join(tmpDir, "plain.tar.gz")
	if err := os.WriteFile(plainFile, []byte{0x1f, 0x8b, 0x08}, 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"age encrypted file", encFile, true},
		{"plain tar.gz file", plainFile, false},
		{"nonexistent file", filepath.Join(tmpDir, "nope"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := isAgeEncrypted(tt.path)
			if got != tt.want {
				t.Errorf("isAgeEncrypted(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
