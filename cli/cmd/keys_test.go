package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestEncryptDecryptKeysRoundTrip(t *testing.T) {
	keys := []ExportedKey{
		{KeyRef: "kek.alice", Key: []byte("01234567890123456789012345678901")}, // 32 bytes
		{UserID: "user-bob", Key: []byte("abcdefghijklmnopqrstuvwxyz012345")},  // legacy v2 shape
	}

	passphrase := "test-passphrase-123"
	tempFile := filepath.Join(t.TempDir(), "keys.age")

	// Encrypt to file
	if err := encryptKeysToFile(keys, passphrase, tempFile); err != nil {
		t.Fatal("encryptKeysToFile failed:", err)
	}

	// Verify the file starts with age header
	data, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatal("Failed to read encrypted file:", err)
	}
	if !strings.HasPrefix(string(data), "age-encryption.org/v1\n") {
		t.Error("Encrypted file does not start with age header")
	}

	// Decrypt with correct passphrase
	decrypted, err := decryptKeysFromFile(tempFile, passphrase)
	if err != nil {
		t.Fatal("decryptKeysFromFile failed:", err)
	}

	if len(decrypted) != len(keys) {
		t.Fatalf("Decrypted %d keys, want %d", len(decrypted), len(keys))
	}

	for i, dk := range decrypted {
		if dk.KeyRef != keys[i].KeyRef {
			t.Errorf("Key %d: KeyRef = %q, want %q", i, dk.KeyRef, keys[i].KeyRef)
		}
		if dk.UserID != keys[i].UserID {
			t.Errorf("Key %d: UserID = %q, want %q", i, dk.UserID, keys[i].UserID)
		}
		if string(dk.Key) != string(keys[i].Key) {
			t.Errorf("Key %d: key content mismatch", i)
		}
	}

	// Decrypt with wrong passphrase should fail
	_, err = decryptKeysFromFile(tempFile, "wrong-passphrase")
	if err == nil {
		t.Error("Expected decryption to fail with wrong passphrase")
	}
}

func TestKeysExportImportRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --- Source server: create encryption keys ---

	_, _, srcJS := startTestNATS(t)

	srcKV, err := srcJS.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "ENCRYPTION_KEYS",
		Storage: jetstream.FileStorage,
	})
	if err != nil {
		t.Fatal("Failed to create ENCRYPTION_KEYS bucket:", err)
	}

	storedDEK, err := proto.Marshal(&corev1.StoredUserDEK{
		EncryptedContentKey: []byte("wrapped"),
		ContentKeyNonce:     []byte("nonce"),
		WrappingKeyRef:      "kek.DEKRef01",
	})
	if err != nil {
		t.Fatal("marshal stored DEK:", err)
	}

	testKeys := map[string][]byte{
		"user.alice":   []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"), // 32 bytes
		"user.bob":     []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"),
		"user.charlie": []byte("CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"),
		"kek.DEKRef01": []byte("DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD"),
		"dek.Ref01":    storedDEK,
	}

	for k, v := range testKeys {
		if _, err := srcKV.Put(ctx, k, v); err != nil {
			t.Fatalf("Failed to put key %s: %v", k, err)
		}
	}

	// --- Export keys ---

	exported, err := exportAllKeys(ctx, srcKV)
	if err != nil {
		t.Fatal("exportAllKeys failed:", err)
	}

	if len(exported) != len(testKeys) {
		t.Fatalf("Exported %d keys, want %d", len(exported), len(testKeys))
	}

	exportedByRef := make(map[string][]byte)
	exportedByUser := make(map[string][]byte)
	for _, ek := range exported {
		exportedByRef[ek.KeyRef] = ek.Key
		if ek.UserID != "" {
			exportedByUser[ek.UserID] = ek.Key
		}
	}

	for k, wantVal := range testKeys {
		got, ok := exportedByRef[k]
		if !ok {
			t.Errorf("Missing exported key ref %s", k)
			continue
		}
		if string(got) != string(wantVal) {
			t.Errorf("Key for %s: got %q, want %q", k, got, wantVal)
		}
	}
	if _, ok := exportedByUser["alice"]; !ok {
		t.Error("legacy user key should still populate user_id")
	}

	// --- Encrypt to file and decrypt ---

	passphrase := "test-passphrase"
	tempFile := filepath.Join(t.TempDir(), "keys.age")

	if err := encryptKeysToFile(exported, passphrase, tempFile); err != nil {
		t.Fatal("encryptKeysToFile failed:", err)
	}

	decrypted, err := decryptKeysFromFile(tempFile, passphrase)
	if err != nil {
		t.Fatal("decryptKeysFromFile failed:", err)
	}

	// --- Import into a fresh NATS server ---

	_, _, dstJS := startTestNATS(t)

	dstKV, err := dstJS.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "ENCRYPTION_KEYS",
		Storage: jetstream.FileStorage,
	})
	if err != nil {
		t.Fatal("Failed to create destination ENCRYPTION_KEYS bucket:", err)
	}

	imported, skipped := importKeys(ctx, dstKV, decrypted)

	if imported != len(testKeys) {
		t.Errorf("Imported %d keys, want %d", imported, len(testKeys))
	}
	if skipped != 0 {
		t.Errorf("Skipped %d keys, want 0", skipped)
	}

	// --- Verify: keys survived the round-trip ---

	for k, wantVal := range testKeys {
		entry, err := dstKV.Get(ctx, k)
		if err != nil {
			t.Fatalf("Failed to get key %s from destination: %v", k, err)
		}
		if string(entry.Value()) != string(wantVal) {
			t.Errorf("Key %s: got %q, want %q", k, string(entry.Value()), string(wantVal))
		}
	}

	// --- Verify: importing again skips existing keys ---

	for _, key := range decrypted {
		if _, err := dstKV.Create(ctx, keyRefForImport(key), key.Key); err == nil {
			t.Errorf("Expected key %s to already exist on second import", keyRefForImport(key))
		}
	}
}

func TestKeyRefForImport_LegacyV2UserID(t *testing.T) {
	got := keyRefForImport(ExportedKey{UserID: "alice"})
	if got != "user.alice" {
		t.Fatalf("keyRefForImport legacy = %q, want user.alice", got)
	}
}
