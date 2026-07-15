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
	stat, err := store.Stat(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if stat.ContentType != "application/octet-stream" || stat.Purpose != projectionsnapshot.ObjectPurpose || stat.ModifiedAt.IsZero() {
		t.Fatalf("S3 snapshot marker metadata = %#v", stat)
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

func TestS3ProjectionSnapshotExpiryDeletesOnlyMarkedGenerationObjects(t *testing.T) {
	ctx := context.Background()
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
	store := s3SnapshotBlobStore{client: client}
	now := time.Now().UTC().Add(48 * time.Hour)
	repository, err := projectionsnapshot.NewRepository(store, projectionsnapshot.RepositoryOptions{
		Pointers: natsSnapshotPointerStore{kv: pointerKV}, SecretHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Save(ctx, projectionsnapshot.SaveInput{
		ProjectionKey: "threads", ContractID: "v1", StreamName: "EVT",
		StreamIdentity: "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CutoffSequence: 1, Payload: []byte("current"),
	}); err != nil {
		t.Fatal(err)
	}
	lookalike := "internal/projection-snapshots/threads/v1/objects/0123456789abcdef/" + strings.Repeat("f", 32)
	if _, err := client.PutObjectFromBytes(ctx, lookalike, []byte("not a snapshot"), projectionsnapshot.ObjectContentType); err != nil {
		t.Fatal(err)
	}

	result, err := repository.Expire(ctx, projectionsnapshot.ExpireOptions{
		Retention: 24 * time.Hour, MaxDeletes: 10, MaxDeleteBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedObjects != 1 || result.IgnoredObjects != 1 {
		t.Fatalf("expiry result = %#v", result)
	}
	if _, err := client.StatObject(ctx, lookalike); err != nil {
		t.Fatalf("unmarked lookalike was deleted: %v", err)
	}
	if _, err := repository.Load(ctx, "threads", "v1", "EVT", "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1); err == nil {
		t.Fatal("expired current generation still loaded")
	}
}

func TestNATSProjectionSnapshotObjectStoreTTLExpiresObjects(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: "SNAPSHOT_TTL_TEST", TTL: 250 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	blobs := natsSnapshotBlobStore{store: store}
	if err := blobs.Put(ctx, "internal/projection-snapshots/threads/v1/objects/0123456789abcdef/"+strings.Repeat("a", 32), []byte("snapshot"), projectionsnapshot.ObjectContentType); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, err := blobs.Stat(ctx, "internal/projection-snapshots/threads/v1/objects/0123456789abcdef/"+strings.Repeat("a", 32))
		if errors.Is(err, projectionsnapshot.ErrBlobNotFound) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("NATS snapshot object did not expire within timeout")
}
