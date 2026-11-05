package core

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/notificationstream"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/nats-io/nats.go/jetstream"
)

// missingStreamGate pauses a startup after its absent-stream read. A second
// startup can then persist the identity before the first startup continues.
type missingStreamGate struct {
	jetstream.JetStream
	name    string
	started chan struct{}
	resume  chan struct{}
	cleared *jetstream.StreamInfo
}

func (js *missingStreamGate) Stream(ctx context.Context, name string) (jetstream.Stream, error) {
	stream, err := js.JetStream.Stream(ctx, name)
	if name == js.name && errors.Is(err, jetstream.ErrStreamNotFound) {
		close(js.started)
		select {
		case <-js.resume:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return stream, err
}

func (js *missingStreamGate) CreateOrUpdateStream(ctx context.Context, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
	stream, err := js.JetStream.CreateOrUpdateStream(ctx, cfg)
	if err == nil && cfg.Name == js.name && len(cfg.Metadata) == 0 {
		// Capture the persisted state that another replica can read during startup.
		js.cleared, err = stream.Info(ctx)
	}
	return stream, err
}

func TestConcurrentStreamStartupPreservesPublishedIdentity(t *testing.T) {
	for _, resource := range []struct {
		cfg      jetstream.StreamConfig
		identity streamIdentity
	}{
		{evtStreamConfig(config.CoreConfig{}), evtStreamIdentity},
		{notificationsStreamConfig(config.CoreConfig{}), notificationStreamIdentity},
	} {
		t.Run(resource.cfg.Name, func(t *testing.T) {
			_, nc := testutil.StartNATS(t)
			js, err := jetstream.New(nc)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(testContext(t))
			gate := &missingStreamGate{JetStream: js, name: resource.cfg.Name, started: make(chan struct{}), resume: make(chan struct{})}
			done := make(chan error, 1)
			finished := make(chan struct{})
			t.Cleanup(func() {
				cancel()
				<-finished
			})
			go func() {
				defer close(finished)
				_, err := createIdentifiedStream(ctx, gate, resource.cfg, resource.identity)
				done <- err
			}()
			select {
			case <-gate.started:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			first, err := createIdentifiedStream(ctx, js, resource.cfg, resource.identity)
			if err != nil {
				t.Fatal(err)
			}
			want := maps.Clone(first.CachedInfo().Config.Metadata)
			want["application.extra"] = "preserved"
			cfg := first.CachedInfo().Config
			cfg.Metadata = maps.Clone(want)
			if _, err := js.UpdateStream(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			close(gate.resume)
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if gate.cleared != nil {
				t.Fatal("startup temporarily cleared the identity published by another replica")
			}
			info, err := first.Info(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(info.Config.Metadata, want) {
				t.Fatal("startup changed the identity or metadata published by another replica")
			}
		})
	}
}

// timeoutAfterStreamCommit models a lost reply after successful provisioning.
// The first EVT and NOTIFICATIONS create and update calls are affected. Other
// operations use the real client, including metadata reads and retries.
type timeoutAfterStreamCommit struct {
	jetstream.JetStream
	calls       map[string]int
	createCalls map[string]int
}

func (js *timeoutAfterStreamCommit) CreateStream(ctx context.Context, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
	stream, err := js.JetStream.CreateStream(ctx, cfg)
	if cfg.Name != "EVT" && cfg.Name != notificationstream.StreamName {
		return stream, err
	}
	js.createCalls[cfg.Name]++
	if err == nil && js.createCalls[cfg.Name] == 1 {
		return nil, context.DeadlineExceeded
	}
	return stream, err
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
			faults := &timeoutAfterStreamCommit{JetStream: js, calls: make(map[string]int), createCalls: make(map[string]int)}
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
				if !existing && faults.createCalls[info.Config.Name] < 2 {
					t.Fatalf("%s creation was not retried", info.Config.Name)
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
