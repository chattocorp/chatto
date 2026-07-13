package projectionsnapshot

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const testSecret = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
const testStreamIdentity = "2026-07-13T12:00:00Z"

func testSaveInput(seq uint64, payload []byte) SaveInput {
	return SaveInput{ProjectionKey: "threads", CompatibilityID: "threads-v1", StreamName: "EVT", StreamIdentity: testStreamIdentity, CutoffSequence: seq, Payload: payload}
}

type memoryBlobStore struct {
	objects map[string][]byte
	failPut func(string) bool
}

type capturedLog struct {
	level   string
	message string
	fields  map[string]interface{}
}
type captureLogger struct{ logs []capturedLog }

func (l *captureLogger) add(level string, message interface{}, keyvals ...interface{}) {
	fields := make(map[string]interface{})
	for i := 0; i+1 < len(keyvals); i += 2 {
		fields[keyvals[i].(string)] = keyvals[i+1]
	}
	l.logs = append(l.logs, capturedLog{level: level, message: message.(string), fields: fields})
}
func (l *captureLogger) Debug(message interface{}, keyvals ...interface{}) {
	l.add("debug", message, keyvals...)
}
func (l *captureLogger) Info(message interface{}, keyvals ...interface{}) {
	l.add("info", message, keyvals...)
}
func (l *captureLogger) Warn(message interface{}, keyvals ...interface{}) {
	l.add("warn", message, keyvals...)
}
func (l *captureLogger) Error(message interface{}, keyvals ...interface{}) {
	l.add("error", message, keyvals...)
}

func newMemoryBlobStore() *memoryBlobStore { return &memoryBlobStore{objects: make(map[string][]byte)} }
func (*memoryBlobStore) Backend() string   { return "memory" }
func (m *memoryBlobStore) Put(_ context.Context, key string, data []byte, _ string) error {
	if m.failPut != nil && m.failPut(key) {
		return errors.New("injected put failure")
	}
	m.objects[key] = append([]byte(nil), data...)
	return nil
}
func (m *memoryBlobStore) Get(_ context.Context, key string, max int64) ([]byte, error) {
	data, ok := m.objects[key]
	if !ok {
		return nil, ErrBlobNotFound
	}
	if int64(len(data)) > max {
		return nil, errors.New("too large")
	}
	return append([]byte(nil), data...), nil
}
func (m *memoryBlobStore) Delete(_ context.Context, key string) error {
	if _, ok := m.objects[key]; !ok {
		return ErrBlobNotFound
	}
	delete(m.objects, key)
	return nil
}

func newTestRepository(t *testing.T, blobs BlobStore, secret string) *Repository {
	t.Helper()
	r, err := NewRepository(blobs, RepositoryOptions{
		SecretHex:       secret,
		ProducerVersion: "test-version",
		Now:             func() time.Time { return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRepositoryRoundTripKeepsMetadataOpaque(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	payload := []byte("sensitive-user-id-and-thread-state")
	saved, err := repository.Save(ctx, testSaveInput(42, payload))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.Load(ctx, "threads", "threads-v1", "EVT", testStreamIdentity, 42)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GenerationID != saved.GenerationID || loaded.CutoffSequence != 42 || !bytes.Equal(loaded.Payload, payload) {
		t.Fatalf("loaded snapshot = %#v", loaded)
	}
	for key, data := range blobs.objects {
		if strings.Contains(key, "threads") || bytes.Contains(data, []byte("threads")) || bytes.Contains(data, payload) {
			t.Fatalf("snapshot metadata leaked through object %q", key)
		}
	}
}

func TestRepositoryFallsBackToPreviousGeneration(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	first, err := repository.Save(ctx, testSaveInput(10, []byte("first")))
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.Save(ctx, testSaveInput(20, []byte("second")))
	if err != nil {
		t.Fatal(err)
	}
	blobs.objects[generationObjectKey(second.GenerationID)][envelopeHeaderSize] ^= 1

	loaded, err := repository.Load(ctx, "threads", "threads-v1", "EVT", testStreamIdentity, 20)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GenerationID != first.GenerationID || string(loaded.Payload) != "first" {
		t.Fatalf("fallback loaded = %#v", loaded)
	}
}

func TestRepositoryRetainsCurrentAndPrevious(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	var generations []LoadedSnapshot
	for seq := uint64(1); seq <= 3; seq++ {
		generation, err := repository.Save(ctx, testSaveInput(seq, []byte{byte(seq)}))
		if err != nil {
			t.Fatal(err)
		}
		generations = append(generations, generation)
	}
	if _, ok := blobs.objects[generationObjectKey(generations[0].GenerationID)]; ok {
		t.Fatal("oldest generation was not deleted")
	}
	for _, generation := range generations[1:] {
		if _, ok := blobs.objects[generationObjectKey(generation.GenerationID)]; !ok {
			t.Fatalf("retained generation %s is missing", generation.GenerationID)
		}
	}
}

func TestRepositoryRejectsWrongKeyCompatibilityAndFutureCutoff(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	_, err := repository.Save(ctx, testSaveInput(10, []byte("state")))
	if err != nil {
		t.Fatal(err)
	}

	wrongKey := newTestRepository(t, blobs, strings.Repeat("11", 32))
	if _, err := wrongKey.Load(ctx, "threads", "threads-v1", "EVT", testStreamIdentity, 10); err == nil {
		t.Fatal("wrong key loaded snapshot")
	}
	for _, test := range []struct {
		compatibility, stream, identity string
		max                             uint64
	}{
		{"threads-v2", "EVT", testStreamIdentity, 10},
		{"threads-v1", "OTHER", testStreamIdentity, 10},
		{"threads-v1", "EVT", "different-stream", 10},
		{"threads-v1", "EVT", testStreamIdentity, 9},
	} {
		if _, err := repository.Load(ctx, "threads", test.compatibility, test.stream, test.identity, test.max); err == nil {
			t.Fatalf("invalid constraints loaded snapshot: %#v", test)
		}
	}
}

func TestRepositoryDoesNotPublishPointerAfterGenerationWriteFailure(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	blobs.failPut = func(key string) bool { return strings.Contains(key, "/objects/") }
	repository := newTestRepository(t, blobs, testSecret)
	_, err := repository.Save(ctx, testSaveInput(1, []byte("state")))
	if err == nil {
		t.Fatal("Save succeeded despite generation failure")
	}
	for key := range blobs.objects {
		if strings.Contains(key, "/pointers/") {
			t.Fatalf("pointer %q published after generation failure", key)
		}
	}
}

func TestRepositoryRejectsOversizedPayloadOnSaveAndLoad(t *testing.T) {
	ctx := context.Background()
	t.Run("save", func(t *testing.T) {
		blobs := newMemoryBlobStore()
		repository := newTestRepository(t, blobs, testSecret)
		repository.maxPayloadSize = 4
		if _, err := repository.Save(ctx, testSaveInput(1, []byte("large"))); err == nil || !strings.Contains(err.Error(), "payload exceeds") {
			t.Fatalf("oversized Save error = %v", err)
		}
		if len(blobs.objects) != 0 {
			t.Fatalf("oversized Save wrote %d objects", len(blobs.objects))
		}
	})

	t.Run("load", func(t *testing.T) {
		blobs := newMemoryBlobStore()
		repository := newTestRepository(t, blobs, testSecret)
		if _, err := repository.Save(ctx, testSaveInput(1, []byte("large"))); err != nil {
			t.Fatal(err)
		}
		repository.maxPayloadSize = 4
		if _, err := repository.Load(ctx, "threads", "threads-v1", "EVT", testStreamIdentity, 1); err == nil || !strings.Contains(err.Error(), "payload exceeds") {
			t.Fatalf("oversized Load error = %v", err)
		}
	})
}

func TestRepositoryDeletesGenerationAfterPointerWriteFailure(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	blobs.failPut = func(key string) bool { return strings.Contains(key, "/pointers/") }
	repository := newTestRepository(t, blobs, testSecret)
	_, err := repository.Save(ctx, testSaveInput(1, []byte("state")))
	if err == nil {
		t.Fatal("Save succeeded despite pointer failure")
	}
	if len(blobs.objects) != 0 {
		t.Fatalf("pointer failure left %d orphan objects", len(blobs.objects))
	}
}

func TestRepositoryLogsOperationalSnapshotContext(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	logger := &captureLogger{}
	repository, err := NewRepository(blobs, RepositoryOptions{SecretHex: testSecret, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := repository.Save(ctx, testSaveInput(12, []byte("state")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Load(ctx, "threads", "threads-v1", "EVT", testStreamIdentity, 12); err != nil {
		t.Fatal(err)
	}

	for _, message := range []string{"Projection snapshot published", "Projection snapshot loaded"} {
		found := false
		for _, record := range logger.logs {
			if record.message != message {
				continue
			}
			found = true
			for _, field := range []string{"projection", "backend", "stage", "generation_id", "cutoff_seq", "payload_bytes", "producer_version", "duration"} {
				if _, ok := record.fields[field]; !ok {
					t.Errorf("%q log missing %q", message, field)
				}
			}
			if record.fields["generation_id"] != saved.GenerationID {
				t.Errorf("%q generation id = %v", message, record.fields["generation_id"])
			}
		}
		if !found {
			t.Errorf("missing %q log", message)
		}
	}
}
