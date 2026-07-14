package core

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/projectionsnapshot"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/internal/testutil/fakes3"
)

func TestS3ProjectionSnapshotBlobStoreRoundTrip(t *testing.T) {
	server := fakes3.NewServer(t)
	useSSL := false
	pathStyle := true
	client, err := NewS3Client(config.S3Config{
		Endpoint: server.EndpointHost(), Bucket: "snapshots", PathPrefix: "tenant/chatto",
		AccessKeyID: "key", SecretAccessKey: "secret", UseSSL: &useSSL, PathStyle: &pathStyle,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	store := s3SnapshotBlobStore{client: client}
	key := "internal/projection-snapshots/v1/test-object"
	payload := bytes.Repeat([]byte("encrypted"), 20)
	if err := store.Put(ctx, key, payload, "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(ctx, key, int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded, payload) {
		t.Fatal("S3 snapshot blob changed")
	}
	if _, err := store.Get(ctx, key, int64(len(payload)-1)); err == nil {
		t.Fatal("S3 blob size limit was not enforced")
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, key, 1024); !errors.Is(err, projectionsnapshot.ErrBlobNotFound) {
		t.Fatalf("deleted blob error = %v", err)
	}
}

func TestS3ProjectionSnapshotBlobStoreWalksPaginatedLogicalPrefix(t *testing.T) {
	server := fakes3.NewServer(t)
	useSSL := false
	pathStyle := true
	client, err := NewS3Client(config.S3Config{
		Endpoint: server.EndpointHost(), Bucket: "snapshots", PathPrefix: "tenant/chatto",
		AccessKeyID: "key", SecretAccessKey: "secret", UseSSL: &useSSL, PathStyle: &pathStyle,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.listPageSize = 2
	ctx := context.Background()
	if err := client.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	store := s3SnapshotBlobStore{client: client}
	prefix := "internal/projection-snapshots/v1/objects/"
	want := []string{prefix + "a", prefix + "b", prefix + "c", prefix + "d", prefix + "e"}
	for _, key := range append(slices.Clone(want), "unrelated/object") {
		if err := store.Put(ctx, key, []byte(key), "application/octet-stream"); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	if err := store.Walk(ctx, prefix, func(info projectionsnapshot.BlobInfo) error {
		got = append(got, info.Key)
		if info.Size != int64(len(info.Key)) || info.ModifiedAt.IsZero() {
			t.Errorf("invalid S3 inventory metadata: %#v", info)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("walked keys = %v, want %v", got, want)
	}

	stopErr := errors.New("stop walking")
	visits := 0
	err = store.Walk(ctx, prefix, func(projectionsnapshot.BlobInfo) error {
		visits++
		return stopErr
	})
	if !errors.Is(err, stopErr) || visits != 1 {
		t.Fatalf("callback stop error/visits = %v/%d", err, visits)
	}
}

func TestNATSProjectionSnapshotBlobStoreWalksPrefixAndStops(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	objectStore, err := js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: "SNAPSHOT_WALK_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	store := natsSnapshotBlobStore{store: objectStore}
	prefix := "internal/projection-snapshots/v1/objects/"
	for _, key := range []string{prefix + "a", prefix + "b", "unrelated/object"} {
		if err := store.Put(ctx, key, []byte(key), "application/octet-stream"); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	if err := store.Walk(ctx, prefix, func(info projectionsnapshot.BlobInfo) error {
		got = append(got, info.Key)
		if info.Size != int64(len(info.Key)) || info.ModifiedAt.IsZero() || time.Since(info.ModifiedAt) > time.Minute {
			t.Errorf("invalid NATS inventory metadata: %#v", info)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{prefix + "a", prefix + "b"}) {
		t.Fatalf("walked keys = %v", got)
	}

	stopErr := errors.New("stop walking")
	visits := 0
	err = store.Walk(ctx, prefix, func(projectionsnapshot.BlobInfo) error {
		visits++
		return stopErr
	})
	if !errors.Is(err, stopErr) || visits != 1 {
		t.Fatalf("callback stop error/visits = %v/%d", err, visits)
	}
}

func TestNATSProjectionSnapshotPointerStoreEnforcesRevisions(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "SNAPSHOT_POINTER_CAS_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	store := natsSnapshotPointerStore{kv: kv}
	if _, _, err := store.GetPointer(ctx, "pointer.key"); !errors.Is(err, projectionsnapshot.ErrPointerNotFound) {
		t.Fatalf("missing pointer error = %v", err)
	}
	revision, err := store.CreatePointer(ctx, "pointer.key", []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreatePointer(ctx, "pointer.key", []byte("duplicate")); !errors.Is(err, projectionsnapshot.ErrPointerConflict) {
		t.Fatalf("duplicate create error = %v", err)
	}
	if _, err := store.UpdatePointer(ctx, "pointer.key", []byte("stale"), revision+1); !errors.Is(err, projectionsnapshot.ErrPointerConflict) {
		t.Fatalf("stale update error = %v", err)
	}
	value, currentRevision, err := store.GetPointer(ctx, "pointer.key")
	if err != nil || string(value) != "first" || currentRevision != revision {
		t.Fatalf("pointer changed after conflicts: value=%q revision=%d err=%v", value, currentRevision, err)
	}
	if _, err := store.UpdatePointer(ctx, "pointer.key", []byte("second"), revision); err != nil {
		t.Fatal(err)
	}
	value, _, err = store.GetPointer(ctx, "pointer.key")
	if err != nil || string(value) != "second" {
		t.Fatalf("updated pointer value=%q err=%v", value, err)
	}
}

func TestProjectionSnapshotSweepDeletesOldOrphanThroughStorageBackends(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		store func(*testing.T) (projectionsnapshot.BlobStore, projectionsnapshot.PointerStore)
	}{
		{
			name: "nats",
			store: func(t *testing.T) (projectionsnapshot.BlobStore, projectionsnapshot.PointerStore) {
				_, nc := testutil.StartNATS(t)
				js, err := jetstream.New(nc)
				if err != nil {
					t.Fatal(err)
				}
				objectStore, err := js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: "SNAPSHOT_SWEEP_TEST"})
				if err != nil {
					t.Fatal(err)
				}
				pointerKV, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "SNAPSHOT_POINTER_TEST"})
				if err != nil {
					t.Fatal(err)
				}
				return natsSnapshotBlobStore{store: objectStore}, natsSnapshotPointerStore{kv: pointerKV}
			},
		},
		{
			name: "s3",
			store: func(t *testing.T) (projectionsnapshot.BlobStore, projectionsnapshot.PointerStore) {
				server := fakes3.NewServer(t)
				useSSL := false
				pathStyle := true
				client, err := NewS3Client(config.S3Config{
					Endpoint: server.EndpointHost(), Bucket: "snapshots", PathPrefix: "tenant/chatto",
					AccessKeyID: "key", SecretAccessKey: "secret", UseSSL: &useSSL, PathStyle: &pathStyle,
				})
				if err != nil {
					t.Fatal(err)
				}
				client.listPageSize = 1
				if err := client.EnsureBucket(ctx); err != nil {
					t.Fatal(err)
				}
				_, nc := testutil.StartNATS(t)
				js, err := jetstream.New(nc)
				if err != nil {
					t.Fatal(err)
				}
				pointerKV, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "S3_SNAPSHOT_POINTER_TEST"})
				if err != nil {
					t.Fatal(err)
				}
				return s3SnapshotBlobStore{client: client}, natsSnapshotPointerStore{kv: pointerKV}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, pointers := test.store(t)
			now := time.Now().UTC().Add(48 * time.Hour)
			repository, err := projectionsnapshot.NewRepository(store, projectionsnapshot.RepositoryOptions{
				Pointers:  pointers,
				SecretHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
				Now:       func() time.Time { return now },
			})
			if err != nil {
				t.Fatal(err)
			}
			saved, err := repository.Save(ctx, projectionsnapshot.SaveInput{
				ProjectionKey: "threads", CompatibilityID: "threads-v1", StreamName: "EVT",
				StreamIdentity: "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CutoffSequence: 1, Payload: []byte("current"),
			})
			if err != nil {
				t.Fatal(err)
			}
			var savedKey string
			if err := store.Walk(ctx, "internal/projection-snapshots/v1/objects/", func(info projectionsnapshot.BlobInfo) error {
				savedKey = info.Key
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			orphanKey := strings.TrimSuffix(savedKey, saved.GenerationID) + strings.Repeat("f", 32)
			if err := store.Put(ctx, orphanKey, []byte("orphan"), "application/octet-stream"); err != nil {
				t.Fatal(err)
			}

			result, err := repository.Sweep(ctx, projectionsnapshot.SweepOptions{
				GracePeriod: 24 * time.Hour, MaxDeletes: 10, MaxDeleteBytes: 1024,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.DeletedObjects != 1 || result.ReferencedObjects != 1 {
				t.Fatalf("sweep result = %#v", result)
			}
			if _, err := store.Get(ctx, orphanKey, 1024); !errors.Is(err, projectionsnapshot.ErrBlobNotFound) {
				t.Fatalf("orphan Get error = %v", err)
			}
			if _, err := repository.Load(ctx, "threads", "threads-v1", "EVT", "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1); err != nil {
				t.Fatalf("referenced generation no longer loads: %v", err)
			}
		})
	}
}
