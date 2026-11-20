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

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
	"github.com/chattocorp/chatto/cli/internal/search"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/chattocorp/chatto/pkg/events"
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
	status := providerStatus(projector, nil)
	require.Equal(t, searchv1.ProviderState_PROVIDER_STATE_INDEXING, status.GetState())
	require.NotNil(t, status.GetRetryAfter())

	releaseProjection()
	require.Eventually(t, func() bool {
		status = providerStatus(projector, nil)
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

	require.Eventually(t, func() bool { return projector.Status().Err != nil }, 2*time.Second, 10*time.Millisecond)
	status := providerStatus(projector, nil)
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
	require.Eventually(t, func() bool { return projector.Status().Err != nil }, 2*time.Second, 10*time.Millisecond)

	status := providerStatus(projector, nil)
	require.Equal(t, searchv1.ProviderState_PROVIDER_STATE_DEGRADED, status.GetState())
}

func TestProviderReportsBlockedCommitAfterStartupAsDegraded(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "EVT", Subjects: []string{"evt.>"}, Storage: jetstream.MemoryStorage,
		Metadata: map[string]string{evtstream.IdentityMetadataKey: "evt-incarnation-v1:abababababababababababababababab"},
	})
	require.NoError(t, err)
	projection := &blockingStatusProjection{entered: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	releaseProjection := func() { releaseOnce.Do(func() { close(projection.release) }) }
	t.Cleanup(releaseProjection)
	projector, err := evtstream.NewProjector(js, stream, projection, events.ProjectorOptions{StallAfter: 100 * time.Millisecond})
	require.NoError(t, err)
	runCtx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	go func() { _ = projector.Run(runCtx) }()
	require.Eventually(t, func() bool { return projector.Status().StartupComplete }, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT, providerStatus(projector, nil).GetFreshness())

	publisher := evtstream.NewPublisher(js, stream, log.New(io.Discard))
	_, err = publisher.AppendEventually(ctx, evtstream.RoomAggregate("R1").Subject(evtstream.EventMessagePosted), &evtv1.Event{
		Id: "M1", ActorId: "U1",
		Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1"}},
	})
	require.NoError(t, err)
	<-projection.entered

	var status *searchv1.GetStatusResponse
	require.Eventually(t, func() bool {
		status = providerStatus(projector, nil)
		return status.GetState() == searchv1.ProviderState_PROVIDER_STATE_DEGRADED
	}, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, searchv1.IndexFreshness_INDEX_FRESHNESS_STALLED, status.GetFreshness())
	require.Equal(t, string(events.ProjectorApplying), status.GetCurrentOperation())
	require.Greater(t, status.GetObservedSequence(), status.GetCommittedSequence())

	releaseProjection()
	require.Eventually(t, func() bool {
		status = providerStatus(projector, nil)
		return status.GetState() == searchv1.ProviderState_PROVIDER_STATE_READY
	}, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT, status.GetFreshness())
	require.Nil(t, status.GetPendingDuration())
}

func TestProviderStatusSeparatesAvailabilityFromFreshness(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	for name, tc := range map[string]struct {
		status    events.ProjectorStatus
		state     searchv1.ProviderState
		freshness searchv1.IndexFreshness
	}{
		"not started": {
			status: events.ProjectorStatus{}, state: searchv1.ProviderState_PROVIDER_STATE_STARTING,
			freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_UNSPECIFIED,
		},
		"advancing replay": {
			status: events.ProjectorStatus{Started: true, LastSeq: 5, ObservedSeq: 10, PendingFor: time.Second},
			state:  searchv1.ProviderState_PROVIDER_STATE_INDEXING, freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_BEHIND,
		},
		"stalled replay": {
			status: events.ProjectorStatus{Started: true, LastSeq: 5, ObservedSeq: 10, Stalled: true},
			state:  searchv1.ProviderState_PROVIDER_STATE_INDEXING, freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_STALLED,
		},
		"idle and caught up": {
			status: events.ProjectorStatus{Started: true, StartupComplete: true, LastSeq: 10, ObservedSeq: 10},
			state:  searchv1.ProviderState_PROVIDER_STATE_READY, freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT,
		},
		"live and behind": {
			status: events.ProjectorStatus{Started: true, StartupComplete: true, LastSeq: 9, ObservedSeq: 10},
			state:  searchv1.ProviderState_PROVIDER_STATE_READY, freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_BEHIND,
		},
		"stalled after startup": {
			status: events.ProjectorStatus{Started: true, StartupComplete: true, LastSeq: 9, ObservedSeq: 10, Stalled: true},
			state:  searchv1.ProviderState_PROVIDER_STATE_DEGRADED, freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_STALLED,
		},
	} {
		t.Run(name, func(t *testing.T) {
			response := providerStatusFrom(tc.status, now)
			require.Equal(t, tc.state, response.GetState())
			require.Equal(t, tc.freshness, response.GetFreshness())
			require.Equal(t, now, response.GetObservedAt().AsTime())
			require.Equal(t, tc.status.LastSeq, response.GetCommittedSequence())
			require.Equal(t, tc.status.ObservedSeq, response.GetObservedSequence())
		})
	}
}
