package core

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/projectionsnapshot"
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
