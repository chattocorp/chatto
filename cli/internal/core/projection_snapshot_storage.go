package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/jetstreamutil"
	"hmans.de/chatto/internal/projectionsnapshot"
)

type projectionSnapshotSource struct {
	repository *projectionsnapshot.Repository
}

type natsSnapshotPointerStore struct {
	kv jetstream.KeyValue
}

func (n natsSnapshotPointerStore) GetPointer(ctx context.Context, key string) ([]byte, uint64, error) {
	entry, err := n.kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil, 0, projectionsnapshot.ErrPointerNotFound
		}
		return nil, 0, fmt.Errorf("get projection snapshot pointer: %w", err)
	}
	return append([]byte(nil), entry.Value()...), entry.Revision(), nil
}

func (n natsSnapshotPointerStore) CreatePointer(ctx context.Context, key string, value []byte) (uint64, error) {
	revision, err := n.kv.Create(ctx, key, value)
	if err != nil {
		if jetstreamutil.IsSequenceConflict(err) {
			return 0, projectionsnapshot.ErrPointerConflict
		}
		return 0, fmt.Errorf("create projection snapshot pointer: %w", err)
	}
	return revision, nil
}

func (n natsSnapshotPointerStore) UpdatePointer(ctx context.Context, key string, value []byte, expected uint64) (uint64, error) {
	revision, err := n.kv.Update(ctx, key, value, expected)
	if err != nil {
		if jetstreamutil.IsSequenceConflict(err) || errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return 0, projectionsnapshot.ErrPointerConflict
		}
		return 0, fmt.Errorf("update projection snapshot pointer: %w", err)
	}
	return revision, nil
}

func (s projectionSnapshotSource) LoadProjectionSnapshot(ctx context.Context, request events.ProjectionSnapshotLoadRequest) (events.ProjectionSnapshot, error) {
	loaded, err := s.repository.Load(ctx, request.ProjectionKey, request.CompatibilityID, request.StreamName, request.StreamIdentity, request.MaxCutoff)
	if err != nil {
		return events.ProjectionSnapshot{}, err
	}
	return events.ProjectionSnapshot{
		GenerationID:   loaded.GenerationID,
		CutoffSequence: loaded.CutoffSequence,
		Payload:        loaded.Payload,
	}, nil
}

type natsSnapshotBlobStore struct {
	store jetstream.ObjectStore
}

func (n natsSnapshotBlobStore) Backend() string { return "nats" }

func (n natsSnapshotBlobStore) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := n.store.Put(ctx, jetstream.ObjectMeta{
		Name:        key,
		Description: "Encrypted ephemeral projection snapshot",
		Headers:     natsHeaders(contentType),
	}, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("put NATS snapshot object: %w", err)
	}
	return nil
}

func (n natsSnapshotBlobStore) Get(ctx context.Context, key string, maxBytes int64) ([]byte, error) {
	object, err := n.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrObjectNotFound) {
			return nil, projectionsnapshot.ErrBlobNotFound
		}
		return nil, fmt.Errorf("get NATS snapshot object: %w", err)
	}
	defer object.Close()
	return readSnapshotBlob(object, maxBytes)
}

func (n natsSnapshotBlobStore) Delete(ctx context.Context, key string) error {
	if err := n.store.Delete(ctx, key); err != nil {
		if errors.Is(err, jetstream.ErrObjectNotFound) {
			return projectionsnapshot.ErrBlobNotFound
		}
		return fmt.Errorf("delete NATS snapshot object: %w", err)
	}
	return nil
}

func (n natsSnapshotBlobStore) Walk(ctx context.Context, prefix string, visit func(projectionsnapshot.BlobInfo) error) error {
	watcher, err := n.store.Watch(ctx, jetstream.IgnoreDeletes())
	if err != nil {
		return fmt.Errorf("watch NATS snapshot objects: %w", err)
	}
	defer watcher.Stop()
	for {
		select {
		case info, ok := <-watcher.Updates():
			if !ok {
				if err := ctx.Err(); err != nil {
					return err
				}
				return fmt.Errorf("NATS snapshot object inventory ended before completion")
			}
			if info == nil {
				return nil
			}
			if !strings.HasPrefix(info.Name, prefix) {
				continue
			}
			if info.Size > math.MaxInt64 {
				return fmt.Errorf("NATS snapshot object size exceeds int64")
			}
			if err := visit(projectionsnapshot.BlobInfo{Key: info.Name, Size: int64(info.Size), ModifiedAt: info.ModTime}); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

type s3SnapshotBlobStore struct {
	client *S3Client
}

func (s s3SnapshotBlobStore) Backend() string { return "s3" }

func (s s3SnapshotBlobStore) Put(ctx context.Context, key string, data []byte, contentType string) error {
	if _, err := s.client.PutObjectFromBytes(ctx, key, data, contentType); err != nil {
		return fmt.Errorf("put S3 snapshot object: %w", err)
	}
	return nil
}

func (s s3SnapshotBlobStore) Get(ctx context.Context, key string, maxBytes int64) ([]byte, error) {
	reader, info, err := s.client.GetObject(ctx, key)
	if err != nil {
		if IsNoSuchKeyError(err) {
			return nil, projectionsnapshot.ErrBlobNotFound
		}
		return nil, fmt.Errorf("get S3 snapshot object: %w", err)
	}
	defer reader.Close()
	if info.Size > maxBytes {
		return nil, fmt.Errorf("S3 snapshot object exceeds %d bytes", maxBytes)
	}
	return readSnapshotBlob(reader, maxBytes)
}

func (s s3SnapshotBlobStore) Delete(ctx context.Context, key string) error {
	if err := s.client.DeleteObject(ctx, key); err != nil {
		return fmt.Errorf("delete S3 snapshot object: %w", err)
	}
	return nil
}

func (s s3SnapshotBlobStore) Walk(ctx context.Context, prefix string, visit func(projectionsnapshot.BlobInfo) error) error {
	return s.client.WalkObjects(ctx, prefix, func(info S3ObjectInfo) error {
		return visit(projectionsnapshot.BlobInfo{Key: info.Key, Size: info.Size, ModifiedAt: info.ModifiedAt})
	})
}

func readSnapshotBlob(reader io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(reader, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read snapshot object: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("snapshot object exceeds %d bytes", maxBytes)
	}
	return data, nil
}

func natsHeaders(contentType string) map[string][]string {
	if contentType == "" {
		return nil
	}
	return map[string][]string{"Content-Type": {contentType}}
}
