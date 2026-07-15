package projectionsnapshot

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

const testSecret = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
const testStreamIdentity = "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const otherStreamIdentity = "evt-incarnation-v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const testCompatibilityID = "test-v1"

func testSaveInput(seq uint64, payload []byte) SaveInput {
	return SaveInput{ProjectionKey: "threads", CompatibilityID: testCompatibilityID, StreamName: "EVT", StreamIdentity: testStreamIdentity, CutoffSequence: seq, Payload: payload}
}

type memoryBlobStore struct {
	objects             map[string][]byte
	modified            map[string]time.Time
	pointers            map[string][]byte
	revisions           map[string]uint64
	nextRev             uint64
	failPut             func(string) bool
	failGet             func(string) bool
	failDelete          func(string) bool
	failWalk            func(int) error
	beforePointerUpdate func(string, uint64)
	walkHook            func(int, string)
	walkInfo            func(int, BlobInfo) BlobInfo
	walkCalls           int
	now                 func() time.Time
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

func newMemoryBlobStore() *memoryBlobStore {
	return &memoryBlobStore{
		objects:   make(map[string][]byte),
		modified:  make(map[string]time.Time),
		pointers:  make(map[string][]byte),
		revisions: make(map[string]uint64),
		nextRev:   1,
		now:       func() time.Time { return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC) },
	}
}

func (m *memoryBlobStore) GetPointer(_ context.Context, key string) ([]byte, uint64, error) {
	if m.failGet != nil && m.failGet(key) {
		return nil, 0, errors.New("injected pointer get failure")
	}
	value, ok := m.pointers[key]
	if !ok {
		return nil, 0, ErrPointerNotFound
	}
	return append([]byte(nil), value...), m.revisions[key], nil
}

func (m *memoryBlobStore) CreatePointer(_ context.Context, key string, value []byte) (uint64, error) {
	if m.failPut != nil && m.failPut(key) {
		return 0, errors.New("injected pointer create failure")
	}
	if _, ok := m.pointers[key]; ok {
		return 0, ErrPointerConflict
	}
	revision := m.nextRev
	m.nextRev++
	m.pointers[key] = append([]byte(nil), value...)
	m.revisions[key] = revision
	return revision, nil
}

func (m *memoryBlobStore) UpdatePointer(_ context.Context, key string, value []byte, expected uint64) (uint64, error) {
	if m.failPut != nil && m.failPut(key) {
		return 0, errors.New("injected pointer update failure")
	}
	if m.beforePointerUpdate != nil {
		m.beforePointerUpdate(key, expected)
	}
	if m.revisions[key] != expected || expected == 0 {
		return 0, ErrPointerConflict
	}
	revision := m.nextRev
	m.nextRev++
	m.pointers[key] = append([]byte(nil), value...)
	m.revisions[key] = revision
	return revision, nil
}
func (*memoryBlobStore) Backend() string { return "memory" }
func (m *memoryBlobStore) Put(_ context.Context, key string, data []byte, _ string) error {
	if m.failPut != nil && m.failPut(key) {
		return errors.New("injected put failure")
	}
	m.objects[key] = append([]byte(nil), data...)
	m.modified[key] = m.now()
	return nil
}
func (m *memoryBlobStore) Get(_ context.Context, key string, max int64) ([]byte, error) {
	if m.failGet != nil && m.failGet(key) {
		return nil, errors.New("injected get failure")
	}
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
	if m.failDelete != nil && m.failDelete(key) {
		return errors.New("injected delete failure")
	}
	if _, ok := m.objects[key]; !ok {
		return ErrBlobNotFound
	}
	delete(m.objects, key)
	delete(m.modified, key)
	return nil
}
func (m *memoryBlobStore) Walk(ctx context.Context, prefix string, visit func(BlobInfo) error) error {
	m.walkCalls++
	call := m.walkCalls
	if m.failWalk != nil {
		if err := m.failWalk(call); err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(m.objects))
	for key := range m.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		if m.walkHook != nil {
			m.walkHook(call, key)
		}
		data, ok := m.objects[key]
		if !ok {
			continue
		}
		info := BlobInfo{Key: key, Size: int64(len(data)), ModifiedAt: m.modified[key]}
		if m.walkInfo != nil {
			info = m.walkInfo(call, info)
		}
		if err := visit(info); err != nil {
			return err
		}
	}
	return nil
}

func newTestRepository(t *testing.T, blobs *memoryBlobStore, secret string) *Repository {
	t.Helper()
	r, err := NewRepository(blobs, RepositoryOptions{
		Pointers:        blobs,
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
	loaded, err := repository.Load(ctx, "threads", testCompatibilityID, "EVT", testStreamIdentity, 42)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GenerationID != saved.GenerationID || loaded.CutoffSequence != 42 || loaded.StreamIdentity != testStreamIdentity || !bytes.Equal(loaded.Payload, payload) {
		t.Fatalf("loaded snapshot = %#v", loaded)
	}
	for key, data := range blobs.objects {
		if strings.Contains(key, "threads") || bytes.Contains(data, []byte("threads")) || bytes.Contains(data, payload) {
			t.Fatalf("snapshot metadata leaked through object %q", key)
		}
	}
}

func TestRepositoryRejectsStalePointerPublication(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	first, err := repository.Save(ctx, testSaveInput(1, []byte("first")))
	if err != nil {
		t.Fatal(err)
	}

	var newest LoadedSnapshot
	blobs.beforePointerUpdate = func(_ string, _ uint64) {
		blobs.beforePointerUpdate = nil
		if _, err := repository.Save(ctx, testSaveInput(2, []byte("second"))); err != nil {
			t.Fatal(err)
		}
		blobs.failDelete = func(key string) bool { return key == repository.generationObjectKey(first.GenerationID) }
		newest, err = repository.Save(ctx, testSaveInput(3, []byte("third")))
		blobs.failDelete = nil
		if err != nil {
			t.Fatal(err)
		}
	}

	if _, err := repository.Save(ctx, testSaveInput(4, []byte("stale"))); !errors.Is(err, ErrPointerConflict) {
		t.Fatalf("stale Save error = %v, want pointer conflict", err)
	}
	loaded, err := repository.Load(ctx, "threads", testCompatibilityID, "EVT", testStreamIdentity, 4)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GenerationID != newest.GenerationID || loaded.CutoffSequence != 3 {
		t.Fatalf("stale writer regressed pointer: loaded=%#v newest=%#v", loaded, newest)
	}
	if len(blobs.objects) != 3 {
		t.Fatalf("stale publication left unexpected generation count: %d", len(blobs.objects))
	}
}

func TestRepositoryRejectsSnapshotCapturedBeforeCurrentPublication(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	newest, err := repository.Save(ctx, testSaveInput(200, []byte("newest")))
	if err != nil {
		t.Fatal(err)
	}

	for _, cutoff := range []uint64{100, 200} {
		if _, err := repository.Save(ctx, testSaveInput(cutoff, []byte("captured earlier"))); !errors.Is(err, ErrSnapshotNotAdvanced) {
			t.Fatalf("Save cutoff %d error = %v, want snapshot-not-advanced", cutoff, err)
		}
	}
	loaded, err := repository.Load(ctx, ProjectionV1ThreadsKey, testCompatibilityID, "EVT", testStreamIdentity, 200)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GenerationID != newest.GenerationID || loaded.CutoffSequence != 200 || len(blobs.objects) != 1 {
		t.Fatalf("non-advancing save changed repository: loaded=%#v objects=%d", loaded, len(blobs.objects))
	}
}

func TestRepositoryAllowsNewHistoryOrCompatibilityAtSameCutoff(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	if _, err := repository.Save(ctx, testSaveInput(200, []byte("original"))); err != nil {
		t.Fatal(err)
	}

	newCompatibility := testSaveInput(200, []byte("new compatibility"))
	newCompatibility.CompatibilityID = "test-v2"
	if _, err := repository.Save(ctx, newCompatibility); err != nil {
		t.Fatalf("new compatibility Save: %v", err)
	}
	newHistory := newCompatibility
	newHistory.CutoffSequence = 1
	newHistory.StreamIdentity = otherStreamIdentity
	if _, err := repository.Save(ctx, newHistory); err != nil {
		t.Fatalf("new EVT history Save: %v", err)
	}
}

func TestRepositoryRejectsProjectionOutsideFrozenV1Namespace(t *testing.T) {
	ctx := context.Background()
	repository := newTestRepository(t, newMemoryBlobStore(), testSecret)
	input := testSaveInput(1, []byte("state"))
	input.ProjectionKey = "typo"
	if _, err := repository.Save(ctx, input); err == nil || !strings.Contains(err.Error(), "not a member of namespace v1") {
		t.Fatalf("Save error = %v", err)
	}
	if _, err := repository.Load(ctx, "typo", testCompatibilityID, "EVT", testStreamIdentity, 1); err == nil || !strings.Contains(err.Error(), "not a member of namespace v1") {
		t.Fatalf("Load error = %v", err)
	}
}

func TestRepositoryV2AcceptsOnlyFrozenCoreMembership(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository, err := NewRepository(blobs, RepositoryOptions{
		Namespace: NamespaceV2Core,
		Pointers:  blobs,
		SecretHex: testSecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range NamespaceV2Core.projectionKeys {
		input := testSaveInput(1, []byte(key))
		input.ProjectionKey = key
		if _, err := repository.Save(context.Background(), input); err != nil {
			t.Fatalf("Save %q: %v", key, err)
		}
	}
	for _, key := range []string{ProjectionV1ThreadsKey, "future_projection"} {
		input := testSaveInput(1, []byte(key))
		input.ProjectionKey = key
		if _, err := repository.Save(context.Background(), input); err == nil || !strings.Contains(err.Error(), "not a member of namespace v2") {
			t.Fatalf("Save %q error = %v", key, err)
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
	blobs.objects[repository.generationObjectKey(second.GenerationID)][envelopeHeaderSize] ^= 1

	loaded, err := repository.Load(ctx, "threads", testCompatibilityID, "EVT", testStreamIdentity, 20)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GenerationID != first.GenerationID || string(loaded.Payload) != "first" {
		t.Fatalf("fallback loaded = %#v", loaded)
	}
}

func TestRepositoryFallsBackWhenCurrentGenerationHasDifferentStreamIdentity(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	ctx := context.Background()
	previous, err := repository.Save(ctx, testSaveInput(10, []byte("previous")))
	if err != nil {
		t.Fatal(err)
	}
	currentInput := testSaveInput(20, []byte("current"))
	currentInput.StreamIdentity = otherStreamIdentity
	if _, err := repository.Save(ctx, currentInput); err != nil {
		t.Fatal(err)
	}

	loaded, err := repository.Load(ctx, "threads", testCompatibilityID, "EVT", testStreamIdentity, 20)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GenerationID != previous.GenerationID || string(loaded.Payload) != "previous" {
		t.Fatalf("loaded snapshot = %#v", loaded)
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
	if _, ok := blobs.objects[repository.generationObjectKey(generations[0].GenerationID)]; ok {
		t.Fatal("oldest generation was not deleted")
	}
	for _, generation := range generations[1:] {
		if _, ok := blobs.objects[repository.generationObjectKey(generation.GenerationID)]; !ok {
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
	if _, err := wrongKey.Load(ctx, "threads", testCompatibilityID, "EVT", testStreamIdentity, 10); err == nil {
		t.Fatal("wrong key loaded snapshot")
	}
	for _, test := range []struct {
		compatibility, stream, identity string
		max                             uint64
	}{
		{"test-v2", "EVT", testStreamIdentity, 10},
		{testCompatibilityID, "OTHER", testStreamIdentity, 10},
		{testCompatibilityID, "EVT", otherStreamIdentity, 10},
		{testCompatibilityID, "EVT", testStreamIdentity, 9},
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

func TestRepositoryDoesNotUploadGenerationAfterTransientPointerReadFailure(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	first, err := repository.Save(ctx, testSaveInput(1, []byte("first")))
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.Save(ctx, testSaveInput(2, []byte("second")))
	if err != nil {
		t.Fatal(err)
	}
	objectCount := len(blobs.objects)
	pointerKey := repository.pointerKey("threads")
	blobs.failGet = func(key string) bool { return key == pointerKey }

	_, err = repository.Save(ctx, testSaveInput(3, []byte("third")))
	if err == nil || !strings.Contains(err.Error(), "read snapshot pointer") {
		t.Fatalf("Save error = %v, want pointer read failure", err)
	}
	if len(blobs.objects) != objectCount {
		t.Fatalf("pointer read failure changed object count from %d to %d", objectCount, len(blobs.objects))
	}

	blobs.failGet = nil
	loaded, err := repository.Load(ctx, "threads", testCompatibilityID, "EVT", testStreamIdentity, 3)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GenerationID != second.GenerationID || string(loaded.Payload) != "second" {
		t.Fatalf("pointer changed after read failure: %#v", loaded)
	}

	blobs.objects[repository.generationObjectKey(second.GenerationID)][envelopeHeaderSize] ^= 1
	loaded, err = repository.Load(ctx, "threads", testCompatibilityID, "EVT", testStreamIdentity, 3)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GenerationID != first.GenerationID || string(loaded.Payload) != "first" {
		t.Fatalf("previous fallback changed after read failure: %#v", loaded)
	}
}

func TestRepositoryReplacesDefinitivelyInvalidPointer(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	logger := &captureLogger{}
	repository, err := NewRepository(blobs, RepositoryOptions{Pointers: blobs, SecretHex: testSecret, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Save(ctx, testSaveInput(1, []byte("first"))); err != nil {
		t.Fatal(err)
	}
	blobs.pointers[repository.pointerKey("threads")][envelopeHeaderSize] ^= 1

	second, err := repository.Save(ctx, testSaveInput(2, []byte("second")))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.Load(ctx, "threads", testCompatibilityID, "EVT", testStreamIdentity, 2)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GenerationID != second.GenerationID || string(loaded.Payload) != "second" {
		t.Fatalf("loaded snapshot = %#v", loaded)
	}
	foundWarning := false
	for _, record := range logger.logs {
		if record.message == "Projection snapshot pointer invalid; replacing it" && errors.Is(record.fields["error"].(error), errInvalidPointer) {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatal("invalid pointer replacement was not logged")
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
		if _, err := repository.Load(ctx, "threads", testCompatibilityID, "EVT", testStreamIdentity, 1); err == nil || !strings.Contains(err.Error(), "payload exceeds") {
			t.Fatalf("oversized Load error = %v", err)
		}
	})
}

func TestRepositoryDeletesGenerationAfterPointerWriteFailure(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	blobs.failPut = func(key string) bool { return strings.HasPrefix(key, "projection_snapshot_pointer.") }
	repository := newTestRepository(t, blobs, testSecret)
	_, err := repository.Save(ctx, testSaveInput(1, []byte("state")))
	if err == nil {
		t.Fatal("Save succeeded despite pointer failure")
	}
	if len(blobs.objects) != 0 {
		t.Fatalf("pointer failure left %d orphan objects", len(blobs.objects))
	}
}

func TestRepositoryLogsFailedGenerationCleanupAfterPointerWriteFailure(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	blobs.failPut = func(key string) bool { return strings.HasPrefix(key, "projection_snapshot_pointer.") }
	blobs.failDelete = func(key string) bool { return strings.Contains(key, "/objects/") }
	logger := &captureLogger{}
	repository, err := NewRepository(blobs, RepositoryOptions{Pointers: blobs, SecretHex: testSecret, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.Save(ctx, testSaveInput(1, []byte("state")))
	if err == nil || !strings.Contains(err.Error(), "publish snapshot pointer") {
		t.Fatalf("Save error = %v, want pointer publication failure", err)
	}
	if len(blobs.objects) != 1 {
		t.Fatalf("failed cleanup object count = %d, want 1 orphan", len(blobs.objects))
	}
	foundWarning := false
	for _, record := range logger.logs {
		if record.message == "Unpublished projection snapshot cleanup failed" && record.fields["stage"] == "publish_rollback" {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatal("failed unpublished generation cleanup was not logged")
	}
}

func TestRepositoryLogsOperationalSnapshotContext(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	logger := &captureLogger{}
	repository, err := NewRepository(blobs, RepositoryOptions{Pointers: blobs, SecretHex: testSecret, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := repository.Save(ctx, testSaveInput(12, []byte("state")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Load(ctx, "threads", testCompatibilityID, "EVT", testStreamIdentity, 12); err != nil {
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
