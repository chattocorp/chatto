package projectionsnapshot

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	// Namespace membership is immutable for mixed-version cleanup safety. The
	// v1 namespace contains only the Threads projection. Adding another
	// projection requires a new namespace version so an older replica cannot
	// mistake the newer projection's opaque pointer and generations for orphans.
	objectPrefix         = "internal/projection-snapshots/v1/"
	maxPayloadSize       = 64 << 20
	maxEncryptedSize     = 80 << 20
	maxDecompressedSize  = 72 << 20
	contentType          = "application/vnd.chatto.projection-snapshot"
	streamIdentityPrefix = "evt-incarnation-v1:"
)

// ProjectionV1ThreadsKey is the sole projection key in snapshot namespace v1.
const ProjectionV1ThreadsKey = "threads"

// projectionV1Keys is the immutable membership of the v1 snapshot namespace.
// Adding a projection requires a new namespace so mixed-version sweepers cannot
// mistake its generations for orphans.
var projectionV1Keys = [...]string{ProjectionV1ThreadsKey}

var (
	ErrBlobNotFound     = errors.New("projection snapshot blob not found")
	ErrPointerNotFound  = errors.New("projection snapshot pointer not found")
	ErrPointerConflict  = errors.New("projection snapshot pointer revision conflict")
	ErrSnapshotNotFound = errors.New("projection snapshot not found")
	ErrIncompatible     = errors.New("incompatible projection snapshot")
	errInvalidPointer   = errors.New("invalid projection snapshot pointer")
)

type Logger interface {
	Debug(msg interface{}, keyvals ...interface{})
	Info(msg interface{}, keyvals ...interface{})
	Warn(msg interface{}, keyvals ...interface{})
	Error(msg interface{}, keyvals ...interface{})
}

// BlobStore is the private binary-storage boundary used by snapshots. Keys are
// internal and must never be exposed through asset APIs or lifecycle events.
type BlobStore interface {
	Backend() string
	Put(context.Context, string, []byte, string) error
	Get(context.Context, string, int64) ([]byte, error)
	Delete(context.Context, string) error
	Walk(context.Context, string, func(BlobInfo) error) error
}

// PointerStore provides durable optimistic concurrency for the encrypted
// latest-generation pointer. Payload blobs may live in NATS or S3, but pointer
// publication must be revisioned so a stale writer cannot regress history.
type PointerStore interface {
	GetPointer(context.Context, string) ([]byte, uint64, error)
	CreatePointer(context.Context, string, []byte) (uint64, error)
	UpdatePointer(context.Context, string, []byte, uint64) (uint64, error)
}

// BlobInfo is the storage metadata required for conservative snapshot cleanup.
// Keys are private logical locators and must not be logged or exposed through
// operator APIs.
type BlobInfo struct {
	Key        string
	Size       int64
	ModifiedAt time.Time
}

type RepositoryOptions struct {
	Pointers        PointerStore
	SecretHex       string
	ProducerVersion string
	Logger          Logger
	Rand            io.Reader
	Now             func() time.Time
}

type Repository struct {
	blobs           BlobStore
	pointers        PointerStore
	codec           *envelopeCodec
	secret          []byte
	producerVersion string
	logger          Logger
	rand            io.Reader
	now             func() time.Time
	maxPayloadSize  int
}

type SaveInput struct {
	ProjectionKey   string
	CompatibilityID string
	StreamName      string
	StreamIdentity  string
	CutoffSequence  uint64
	Payload         []byte
}

type LoadedSnapshot struct {
	GenerationID    string
	CutoffSequence  uint64
	StreamIdentity  string
	Payload         []byte
	CreatedAt       time.Time
	ProducerVersion string
}

func NewRepository(blobs BlobStore, opts RepositoryOptions) (*Repository, error) {
	if blobs == nil {
		return nil, fmt.Errorf("snapshot blob store is nil")
	}
	if opts.Pointers == nil {
		return nil, fmt.Errorf("snapshot pointer store is nil")
	}
	secret, err := hex.DecodeString(opts.SecretHex)
	if err != nil || len(secret) != 32 {
		return nil, fmt.Errorf("decode core.secret_key for snapshots: expected 32-byte hex secret")
	}
	random := opts.Rand
	if random == nil {
		random = rand.Reader
	}
	codec, err := newEnvelopeCodec(secret, random)
	if err != nil {
		return nil, err
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	version := opts.ProducerVersion
	if version == "" {
		version = "unknown"
	}
	return &Repository{
		blobs: blobs, pointers: opts.Pointers, codec: codec, secret: secret, producerVersion: version,
		logger: opts.Logger, rand: random, now: now, maxPayloadSize: maxPayloadSize,
	}, nil
}

func (r *Repository) Backend() string { return r.blobs.Backend() }

func (r *Repository) Save(ctx context.Context, input SaveInput) (LoadedSnapshot, error) {
	if input.ProjectionKey == "" || input.CompatibilityID == "" || input.StreamName == "" {
		return LoadedSnapshot{}, fmt.Errorf("snapshot projection key, compatibility id, and stream name are required")
	}
	if !validStreamIdentity(input.StreamIdentity) {
		return LoadedSnapshot{}, fmt.Errorf("snapshot EVT cutoff identity is invalid")
	}
	if len(input.Payload) > r.maxPayloadSize {
		return LoadedSnapshot{}, fmt.Errorf("snapshot payload exceeds %d bytes", r.maxPayloadSize)
	}
	pointer, pointerRevision, err := r.loadPointerAtRevision(ctx, input.ProjectionKey)
	switch {
	case err == nil:
	case errors.Is(err, ErrSnapshotNotFound):
		pointer = &corev1.ProjectionSnapshotPointer{}
	case errors.Is(err, errInvalidPointer):
		r.logWarn("Projection snapshot pointer invalid; replacing it", input.ProjectionKey, "pointer_read", err)
		pointer = &corev1.ProjectionSnapshotPointer{}
	default:
		return LoadedSnapshot{}, fmt.Errorf("read snapshot pointer: %w", err)
	}
	if pointer == nil {
		pointer = &corev1.ProjectionSnapshotPointer{}
	}

	started := time.Now()
	var generationID [generationIDSize]byte
	if _, err := io.ReadFull(r.rand, generationID[:]); err != nil {
		return LoadedSnapshot{}, fmt.Errorf("generate snapshot id: %w", err)
	}
	generationIDText := generationIDString(generationID)
	payloadHash := sha256.Sum256(input.Payload)
	generation := &corev1.ProjectionSnapshotGeneration{
		GenerationId:    generationIDText,
		StreamName:      input.StreamName,
		CutoffSequence:  input.CutoffSequence,
		ProjectionKey:   input.ProjectionKey,
		CompatibilityId: input.CompatibilityID,
		ProducerVersion: r.producerVersion,
		CreatedAt:       timestamppb.New(r.now().UTC()),
		Payload:         input.Payload,
		PayloadSize:     uint64(len(input.Payload)),
		PayloadSha256:   payloadHash[:],
		StreamIdentity:  input.StreamIdentity,
	}
	plain, err := proto.MarshalOptions{Deterministic: true}.Marshal(generation)
	if err != nil {
		return LoadedSnapshot{}, fmt.Errorf("marshal snapshot generation: %w", err)
	}
	compressed, err := compress(plain)
	if err != nil {
		return LoadedSnapshot{}, err
	}
	sealed, err := r.codec.seal(generationID, compressed)
	if err != nil {
		return LoadedSnapshot{}, fmt.Errorf("encrypt snapshot generation: %w", err)
	}
	if len(sealed) > maxEncryptedSize {
		return LoadedSnapshot{}, fmt.Errorf("encrypted snapshot exceeds %d bytes", maxEncryptedSize)
	}
	objectKey := r.generationObjectKey(generationIDText)
	if err := r.blobs.Put(ctx, objectKey, sealed, contentType); err != nil {
		return LoadedSnapshot{}, fmt.Errorf("write snapshot generation: %w", err)
	}

	dropped := pointer.GetPreviousGenerationId()
	pointer.PreviousGenerationId = pointer.GetCurrentGenerationId()
	pointer.CurrentGenerationId = generationIDText
	if err := r.savePointer(ctx, input.ProjectionKey, pointer, pointerRevision); err != nil {
		if cleanupErr := r.blobs.Delete(ctx, objectKey); cleanupErr != nil && !errors.Is(cleanupErr, ErrBlobNotFound) {
			r.logWarn("Unpublished projection snapshot cleanup failed", input.ProjectionKey, "publish_rollback", cleanupErr, "generation_id", generationIDText)
		}
		return LoadedSnapshot{}, fmt.Errorf("publish snapshot pointer: %w", err)
	}
	if dropped != "" && dropped != pointer.GetPreviousGenerationId() {
		if err := r.blobs.Delete(ctx, r.generationObjectKey(dropped)); err != nil && !errors.Is(err, ErrBlobNotFound) {
			r.logWarn("Projection snapshot cleanup failed", input.ProjectionKey, "cleanup", err)
		}
	}
	r.logInfo("Projection snapshot published", input.ProjectionKey,
		"publish", nil,
		"generation_id", generationIDText,
		"cutoff_seq", input.CutoffSequence,
		"payload_bytes", len(input.Payload),
		"stored_bytes", len(sealed),
		"producer_version", r.producerVersion,
		"duration", time.Since(started))
	return LoadedSnapshot{GenerationID: generationIDText, CutoffSequence: input.CutoffSequence, StreamIdentity: input.StreamIdentity, Payload: input.Payload, CreatedAt: generation.GetCreatedAt().AsTime(), ProducerVersion: r.producerVersion}, nil
}

func (r *Repository) Load(ctx context.Context, projectionKey, compatibilityID, streamName, streamIdentity string, maxCutoff uint64) (LoadedSnapshot, error) {
	started := time.Now()
	pointer, err := r.loadPointer(ctx, projectionKey)
	if err != nil {
		if errors.Is(err, ErrSnapshotNotFound) {
			r.logDebug("Projection snapshot not found", projectionKey, "pointer_read", nil)
		}
		return LoadedSnapshot{}, err
	}
	ids := []string{pointer.GetCurrentGenerationId(), pointer.GetPreviousGenerationId()}
	var failures []error
	for index, id := range ids {
		if id == "" {
			continue
		}
		loaded, err := r.loadGeneration(ctx, id, projectionKey, compatibilityID, streamName, streamIdentity, maxCutoff)
		if err == nil {
			r.logInfo("Projection snapshot loaded", projectionKey,
				"restore", nil,
				"generation_id", id,
				"cutoff_seq", loaded.CutoffSequence,
				"pointer_slot", index,
				"payload_bytes", len(loaded.Payload),
				"producer_version", loaded.ProducerVersion,
				"duration", time.Since(started))
			return loaded, nil
		}
		failures = append(failures, fmt.Errorf("generation %s: %w", id, err))
		r.logWarn("Projection snapshot generation rejected", projectionKey, "generation_read", err, "generation_id", id, "pointer_slot", index)
	}
	if len(failures) == 0 {
		return LoadedSnapshot{}, ErrSnapshotNotFound
	}
	return LoadedSnapshot{}, errors.Join(failures...)
}

func (r *Repository) loadGeneration(ctx context.Context, id, projectionKey, compatibilityID, streamName, streamIdentity string, maxCutoff uint64) (LoadedSnapshot, error) {
	expectedID, err := parseGenerationID(id)
	if err != nil {
		return LoadedSnapshot{}, err
	}
	sealed, err := r.blobs.Get(ctx, r.generationObjectKey(id), maxEncryptedSize)
	if err != nil {
		return LoadedSnapshot{}, err
	}
	envelopeID, compressed, err := r.codec.open(sealed)
	if err != nil {
		return LoadedSnapshot{}, err
	}
	if envelopeID != expectedID {
		return LoadedSnapshot{}, fmt.Errorf("%w: envelope generation id mismatch", ErrInvalidEnvelope)
	}
	plain, err := decompress(compressed)
	if err != nil {
		return LoadedSnapshot{}, err
	}
	var generation corev1.ProjectionSnapshotGeneration
	if err := proto.Unmarshal(plain, &generation); err != nil {
		return LoadedSnapshot{}, fmt.Errorf("unmarshal snapshot generation: %w", err)
	}
	if generation.GetGenerationId() != id {
		return LoadedSnapshot{}, fmt.Errorf("%w: manifest generation id mismatch", ErrIncompatible)
	}
	if generation.GetProjectionKey() != projectionKey {
		return LoadedSnapshot{}, fmt.Errorf("%w: projection key mismatch", ErrIncompatible)
	}
	if generation.GetCompatibilityId() != compatibilityID {
		return LoadedSnapshot{}, fmt.Errorf("%w: compatibility id %q does not match %q", ErrIncompatible, generation.GetCompatibilityId(), compatibilityID)
	}
	if generation.GetStreamName() != streamName {
		return LoadedSnapshot{}, fmt.Errorf("%w: stream name %q does not match %q", ErrIncompatible, generation.GetStreamName(), streamName)
	}
	if !validStreamIdentity(generation.GetStreamIdentity()) {
		return LoadedSnapshot{}, fmt.Errorf("%w: EVT stream identity is invalid", ErrIncompatible)
	}
	if generation.GetStreamIdentity() != streamIdentity {
		return LoadedSnapshot{}, fmt.Errorf("%w: EVT stream identity changed", ErrIncompatible)
	}
	if generation.GetCutoffSequence() > maxCutoff {
		return LoadedSnapshot{}, fmt.Errorf("%w: cutoff %d exceeds stream target %d", ErrIncompatible, generation.GetCutoffSequence(), maxCutoff)
	}
	if generation.GetPayloadSize() != uint64(len(generation.GetPayload())) {
		return LoadedSnapshot{}, fmt.Errorf("snapshot payload size mismatch")
	}
	if len(generation.GetPayload()) > r.maxPayloadSize {
		return LoadedSnapshot{}, fmt.Errorf("snapshot payload exceeds %d bytes", r.maxPayloadSize)
	}
	hash := sha256.Sum256(generation.GetPayload())
	if !bytes.Equal(hash[:], generation.GetPayloadSha256()) {
		return LoadedSnapshot{}, fmt.Errorf("snapshot payload checksum mismatch")
	}
	if err := generation.GetCreatedAt().CheckValid(); err != nil {
		return LoadedSnapshot{}, fmt.Errorf("snapshot creation time: %w", err)
	}
	return LoadedSnapshot{GenerationID: id, CutoffSequence: generation.GetCutoffSequence(), StreamIdentity: generation.GetStreamIdentity(), Payload: generation.GetPayload(), CreatedAt: generation.GetCreatedAt().AsTime(), ProducerVersion: generation.GetProducerVersion()}, nil
}

func validStreamIdentity(identity string) bool {
	if len(identity) != len(streamIdentityPrefix)+32 || !strings.HasPrefix(identity, streamIdentityPrefix) {
		return false
	}
	_, err := hex.DecodeString(identity[len(streamIdentityPrefix):])
	return err == nil
}

func (r *Repository) loadPointer(ctx context.Context, projectionKey string) (*corev1.ProjectionSnapshotPointer, error) {
	pointer, _, err := r.loadPointerAtRevision(ctx, projectionKey)
	return pointer, err
}

func (r *Repository) loadPointerAtRevision(ctx context.Context, projectionKey string) (*corev1.ProjectionSnapshotPointer, uint64, error) {
	sealed, revision, err := r.pointers.GetPointer(ctx, r.pointerKey(projectionKey))
	if err != nil {
		if errors.Is(err, ErrPointerNotFound) {
			return nil, 0, ErrSnapshotNotFound
		}
		return nil, 0, err
	}
	if int64(len(sealed)) > maxEncryptedSize {
		return nil, revision, fmt.Errorf("%w: pointer exceeds %d bytes", errInvalidPointer, maxEncryptedSize)
	}
	envelopeID, plain, err := r.codec.open(sealed)
	if err != nil {
		return nil, revision, fmt.Errorf("%w: %v", errInvalidPointer, err)
	}
	if envelopeID != [generationIDSize]byte{} {
		return nil, revision, fmt.Errorf("%w: envelope generation id is not empty", errInvalidPointer)
	}
	var pointer corev1.ProjectionSnapshotPointer
	if err := proto.Unmarshal(plain, &pointer); err != nil {
		return nil, revision, fmt.Errorf("%w: unmarshal: %v", errInvalidPointer, err)
	}
	for name, id := range map[string]string{
		"current generation id":  pointer.GetCurrentGenerationId(),
		"previous generation id": pointer.GetPreviousGenerationId(),
	} {
		if id == "" {
			continue
		}
		if _, err := parseGenerationID(id); err != nil {
			return nil, revision, fmt.Errorf("%w: %s: %v", errInvalidPointer, name, err)
		}
	}
	return &pointer, revision, nil
}

func (r *Repository) savePointer(ctx context.Context, projectionKey string, pointer *corev1.ProjectionSnapshotPointer, revision uint64) error {
	plain, err := proto.MarshalOptions{Deterministic: true}.Marshal(pointer)
	if err != nil {
		return err
	}
	sealed, err := r.codec.seal([generationIDSize]byte{}, plain)
	if err != nil {
		return err
	}
	if revision == 0 {
		_, err = r.pointers.CreatePointer(ctx, r.pointerKey(projectionKey), sealed)
	} else {
		_, err = r.pointers.UpdatePointer(ctx, r.pointerKey(projectionKey), sealed, revision)
	}
	return err
}

func (r *Repository) pointerKey(projectionKey string) string {
	return "projection_snapshot_pointer." + opaqueLocator(r.secret, "v1:"+projectionKey)
}

func (r *Repository) generationObjectPrefix() string {
	return objectPrefix + "objects/" + opaqueLocator(r.secret, "generation-key-epoch-v1") + "/"
}

func (r *Repository) generationObjectKey(generationID string) string {
	return r.generationObjectPrefix() + generationID
}

func compress(plain []byte) ([]byte, error) {
	var out bytes.Buffer
	w, err := gzip.NewWriterLevel(&out, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	w.Header.ModTime = time.Unix(0, 0)
	w.Header.OS = 255
	if _, err := w.Write(plain); err != nil {
		return nil, fmt.Errorf("compress snapshot: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("finish snapshot compression: %w", err)
	}
	return out.Bytes(), nil
}

func decompress(compressed []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("open snapshot compression: %w", err)
	}
	defer r.Close()
	limited := io.LimitReader(r, maxDecompressedSize+1)
	plain, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("decompress snapshot: %w", err)
	}
	if len(plain) > maxDecompressedSize {
		return nil, fmt.Errorf("decompressed snapshot exceeds %d bytes", maxDecompressedSize)
	}
	return plain, nil
}

func (r *Repository) logDebug(message, projection, stage string, err error, extra ...interface{}) {
	if r.logger == nil {
		return
	}
	r.logger.Debug(message, append([]interface{}{"projection", projection, "backend", r.Backend(), "stage", stage, "error", err}, extra...)...)
}

func (r *Repository) logInfo(message, projection, stage string, err error, extra ...interface{}) {
	if r.logger == nil {
		return
	}
	r.logger.Info(message, append([]interface{}{"projection", projection, "backend", r.Backend(), "stage", stage, "error", err}, extra...)...)
}

func (r *Repository) logWarn(message, projection, stage string, err error, extra ...interface{}) {
	if r.logger == nil {
		return
	}
	r.logger.Warn(message, append([]interface{}{"projection", projection, "backend", r.Backend(), "stage", stage, "error", err}, extra...)...)
}
