package core

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/notificationstream"
	"hmans.de/chatto/internal/testutil"
)

// timeoutAfterStreamCommit models a lost reply after successful provisioning.
// Only the first EVT and NOTIFICATIONS call is affected; other operations use
// the real client, including metadata reads and subsequent identity updates.
type timeoutAfterStreamCommit struct {
	jetstream.JetStream
	calls map[string]int
}

func (js *timeoutAfterStreamCommit) CreateOrUpdateStream(ctx context.Context, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
	stream, err := js.JetStream.CreateOrUpdateStream(ctx, cfg)
	if cfg.Name != "EVT" && cfg.Name != notificationstream.StreamName {
		return stream, err
	}
	js.calls[cfg.Name]++
	if err == nil && js.calls[cfg.Name] == 1 {
		return nil, context.DeadlineExceeded
	}
	return stream, err
}

func TestStorageRetryPreservesStreamMetadata(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "fresh"
		if existing {
			name = "existing"
		}
		t.Run(name, func(t *testing.T) {
			_, nc := testutil.StartNATS(t)
			js, err := jetstream.New(nc)
			if err != nil {
				t.Fatal(err)
			}
			ctx := testContext(t)
			cfg := config.CoreConfig{}
			want := make(map[string]map[string]string)
			if existing {
				initial, err := newStorage(js, ctx, cfg)
				if err != nil {
					t.Fatal(err)
				}
				// Use valid identities derived from a different creation time,
				// as restored streams can retain older incarnation metadata.
				oldCreated := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
				for _, resource := range []struct {
					stream   jetstream.Stream
					key      string
					identity func(time.Time) (string, error)
				}{
					{initial.serverEvtStream, evtstream.IdentityMetadataKey, evtstream.NewIdentity},
					{initial.notificationStream, notificationstream.IdentityMetadataKey, notificationstream.NewIdentity},
				} {
					info, err := resource.stream.Info(ctx)
					if err != nil {
						t.Fatal(err)
					}
					identity, err := resource.identity(oldCreated)
					if err != nil {
						t.Fatal(err)
					}
					info.Config.Metadata[resource.key] = identity
					info.Config.Metadata["application.extra"] = "preserved"
					want[info.Config.Name] = maps.Clone(info.Config.Metadata)
					if _, err := js.UpdateStream(ctx, info.Config); err != nil {
						t.Fatal(err)
					}
				}
			}
			faults := &timeoutAfterStreamCommit{JetStream: js, calls: make(map[string]int)}
			storage, err := newStorage(faults, ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			for _, stream := range []jetstream.Stream{storage.serverEvtStream, storage.notificationStream} {
				info, err := stream.Info(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if faults.calls[info.Config.Name] < 2 {
					t.Fatalf("%s was not retried", info.Config.Name)
				}
				if existing && !maps.Equal(info.Config.Metadata, want[info.Config.Name]) {
					t.Fatalf("%s metadata changed: %v", info.Config.Name, info.Config.Metadata)
				}
				want[info.Config.Name] = maps.Clone(info.Config.Metadata)
			}
			if _, err := evtstream.Identity(storage.serverEvtStream); err != nil {
				t.Fatal(err)
			}
			if _, err := notificationstream.IdentityFromInfo(storage.notificationStream.CachedInfo()); err != nil {
				t.Fatal(err)
			}
			// A later startup must attach to the same identities.
			next, err := newStorage(js, ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			for _, stream := range []jetstream.Stream{next.serverEvtStream, next.notificationStream} {
				info := stream.CachedInfo()
				if !maps.Equal(info.Config.Metadata, want[info.Config.Name]) {
					t.Fatalf("later startup changed %s metadata", info.Config.Name)
				}
			}
		})
	}
}
