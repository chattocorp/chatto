package bleve

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
	"hmans.de/chatto/internal/search"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/pkg/events"
)

type blockingStatusProjection struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

type failingStatusProjection struct{}

func (p *failingStatusProjection) Subjects() []string { return []string{"evt.>"} }

func (p *failingStatusProjection) Apply(*evtv1.Event, uint64) error {
	return errors.New("index write failed")
}

func (p *blockingStatusProjection) Subjects() []string { return []string{"evt.>"} }

func (p *blockingStatusProjection) Apply(*evtv1.Event, uint64) error {
	p.once.Do(func() { close(p.entered) })
	<-p.release
	return nil
}

func TestNewProviderKeepsProjectionRuntimeTogether(t *testing.T) {
	projection := &Projection{}
	// The projector never runs, so it needs no NATS connection.
	handle, err := evtstream.NewProjectionHandle(struct{ jetstream.JetStream }{}, struct{ jetstream.Stream }{}, projection, events.ProjectorOptions{})
	require.NoError(t, err)
	provider := newProvider(handle)

	require.Same(t, projection, provider.projection.Projection())
	require.Same(t, handle.Projector(), provider.projection.Projector())
}

func TestProviderQueryWithoutProjectionRuntimeIsNotReady(t *testing.T) {
	response, err := (&Provider{}).Query(context.Background(), &searchv1.QueryRequest{})

	require.Nil(t, response)
	require.ErrorIs(t, err, search.ErrProviderNotReady)
}

func TestProviderStatusTransitionsFromIndexingToReady(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "EVT", Subjects: []string{"evt.>"}, Storage: jetstream.MemoryStorage,
		Metadata: map[string]string{evtstream.IdentityMetadataKey: "evt-incarnation-v1:dddddddddddddddddddddddddddddddd"},
	})
	require.NoError(t, err)
	publisher := evtstream.NewPublisher(js, stream, log.New(io.Discard))
	_, err = publisher.AppendEventually(ctx, evtstream.RoomAggregate("R1").Subject(evtstream.EventMessagePosted), &evtv1.Event{
		Id: "M1", ActorId: "U1",
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1"}},
	})
	require.NoError(t, err)

	projection := &blockingStatusProjection{entered: make(chan struct{}), release: make(chan struct{})}
	releaseProjection := func() {
		select {
		case <-projection.release:
		default:
			close(projection.release)
		}
	}
	t.Cleanup(releaseProjection)
	projector, err := evtstream.NewProjector(js, stream, projection, events.ProjectorOptions{})
	require.NoError(t, err)
	runCtx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	go func() { _ = projector.Run(runCtx) }()

	select {
	case <-projection.entered:
	case <-ctx.Done():
		t.Fatal("projection replay did not start")
	}
	status := providerStatus(projector)
	require.Equal(t, searchv1.ProviderState_PROVIDER_STATE_INDEXING, status.GetState())
	require.NotNil(t, status.GetRetryAfter())

	releaseProjection()
	require.Eventually(t, func() bool {
		status = providerStatus(projector)
		return status.GetState() == searchv1.ProviderState_PROVIDER_STATE_READY
	}, 2*time.Second, 10*time.Millisecond)
	require.Nil(t, status.GetRetryAfter())
}

func TestProviderReportsFailedInitialReplayAsUnavailable(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "EVT", Subjects: []string{"evt.>"}, Storage: jetstream.MemoryStorage,
		Metadata: map[string]string{evtstream.IdentityMetadataKey: "evt-incarnation-v1:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},
	})
	require.NoError(t, err)
	publisher := evtstream.NewPublisher(js, stream, log.New(io.Discard))
	_, err = publisher.AppendEventually(ctx, evtstream.RoomAggregate("R1").Subject(evtstream.EventMessagePosted), &evtv1.Event{
		Id: "M1", ActorId: "U1",
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1"}},
	})
	require.NoError(t, err)

	projector, err := evtstream.NewProjector(js, stream, &failingStatusProjection{}, events.ProjectorOptions{})
	require.NoError(t, err)
	go func() { _ = projector.Run(ctx) }()

	require.Eventually(t, func() bool { return projector.Status().Failed }, 2*time.Second, 10*time.Millisecond)
	status := providerStatus(projector)
	require.Equal(t, searchv1.ProviderState_PROVIDER_STATE_UNAVAILABLE, status.GetState())
	require.Nil(t, status.GetRetryAfter())
}

func TestProviderReportsFailureAfterStartupAsDegraded(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "EVT", Subjects: []string{"evt.>"}, Storage: jetstream.MemoryStorage,
		Metadata: map[string]string{evtstream.IdentityMetadataKey: "evt-incarnation-v1:ffffffffffffffffffffffffffffffff"},
	})
	require.NoError(t, err)
	projector, err := evtstream.NewProjector(js, stream, &failingStatusProjection{}, events.ProjectorOptions{})
	require.NoError(t, err)
	go func() { _ = projector.Run(ctx) }()
	require.Eventually(t, func() bool { return projector.Status().StartupComplete }, 2*time.Second, 10*time.Millisecond)

	publisher := evtstream.NewPublisher(js, stream, log.New(io.Discard))
	_, err = publisher.AppendEventually(ctx, evtstream.RoomAggregate("R1").Subject(evtstream.EventMessagePosted), &evtv1.Event{
		Id: "M1", ActorId: "U1",
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1"}},
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return projector.Status().Failed }, 2*time.Second, 10*time.Millisecond)

	status := providerStatus(projector)
	require.Equal(t, searchv1.ProviderState_PROVIDER_STATE_DEGRADED, status.GetState())
}
