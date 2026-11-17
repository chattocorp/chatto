package bleve

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
	"github.com/chattocorp/chatto/cli/internal/search"
	"github.com/chattocorp/chatto/pkg/events"
)

// Provider exposes a projection through the provider-neutral NATS contract.
type Provider struct {
	projection events.ProjectionHandle[*Projection]
	// probe runs the synthetic query through Query. It is nil until the unit
	// creates it.
	probe *queryProbe
	// serving is true while this provider is subscribed to the shared query
	// and ready status queues.
	serving atomic.Bool
}

func newProvider(projection events.ProjectionHandle[*Projection]) *Provider {
	p := &Provider{projection: projection}
	p.probe = newQueryProbe(func(ctx context.Context) error {
		_, err := p.Query(ctx, queryProbeRequest())
		return err
	})
	return p
}

func (p *Provider) Query(ctx context.Context, request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
	if p == nil || p.projection.Projection() == nil || p.projection.Projector() == nil {
		return nil, search.ErrProviderNotReady
	}
	status := p.projection.Projector().Status()
	if !status.StartupComplete {
		return nil, search.ErrProviderNotReady
	}
	response, err := p.projection.Projection().query(ctx, request)
	if errors.Is(err, errInvalidCursor) {
		return nil, &search.ServiceError{Code: search.ErrorCodeInvalidArgument, Description: "invalid search cursor"}
	}
	return response, err
}

func (p *Provider) GetStatus(context.Context, *searchv1.GetStatusRequest) (*searchv1.GetStatusResponse, error) {
	if p == nil {
		return providerStatus(nil, nil), nil
	}
	return providerStatus(p.projection.Projector(), p.probe), nil
}

func providerStatus(projector *events.Projector, probe *queryProbe) *searchv1.GetStatusResponse {
	if projector == nil {
		return &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_STARTING}
	}
	now := time.Now()
	response := providerStatusFrom(projector.Status(), now)
	applyQueryProbe(response, probe, now)
	return response
}

// applyQueryProbe adds the query probe result. A failed or blocked probe
// means that this provider cannot answer queries now, although its index
// exists, so a READY provider becomes DEGRADED.
func applyQueryProbe(response *searchv1.GetStatusResponse, probe *queryProbe, now time.Time) {
	snapshot, started := probe.Snapshot()
	if !started {
		return
	}
	response.QueryProbe = snapshot.proto()
	if response.GetState() == searchv1.ProviderState_PROVIDER_STATE_READY && snapshot.Failing(now) {
		response.State = searchv1.ProviderState_PROVIDER_STATE_DEGRADED
	}
}

// providerStatusFrom maps projector status to provider status. Query
// availability comes from startup and failure state; freshness comes from
// projector progress. A stalled index after startup is DEGRADED: it still
// answers queries, but results can miss recent messages.
func providerStatusFrom(status events.ProjectorStatus, now time.Time) *searchv1.GetStatusResponse {
	indexed := status.StartupMessages
	committed := status.LastSeq
	observed := status.ObservedSeq
	response := &searchv1.GetStatusResponse{
		State:             searchv1.ProviderState_PROVIDER_STATE_STARTING,
		IndexedEventCount: &indexed,
		Freshness:         indexFreshness(status),
		CommittedSequence: &committed,
		ObservedSequence:  &observed,
		CurrentOperation:  string(status.CurrentOperation),
		ObservedAt:        timestamppb.New(now),
	}
	if !status.LastCommitAt.IsZero() {
		response.LastCommitTime = timestamppb.New(status.LastCommitAt)
	}
	if status.PendingFor > 0 {
		response.PendingDuration = durationpb.New(status.PendingFor)
	}
	if status.CurrentOperation != events.ProjectorIdle {
		response.CurrentOperationDuration = durationpb.New(status.CurrentOperationDuration)
	}
	switch {
	case status.Err != nil && !status.StartupComplete:
		response.State = searchv1.ProviderState_PROVIDER_STATE_UNAVAILABLE
	case status.Err != nil:
		response.State = searchv1.ProviderState_PROVIDER_STATE_DEGRADED
	case status.StartupComplete && status.Stalled:
		response.State = searchv1.ProviderState_PROVIDER_STATE_DEGRADED
	case status.StartupComplete:
		response.State = searchv1.ProviderState_PROVIDER_STATE_READY
	case status.Started:
		response.State = searchv1.ProviderState_PROVIDER_STATE_INDEXING
		response.RetryAfter = durationpb.New(time.Second)
	}
	return response
}

func indexFreshness(status events.ProjectorStatus) searchv1.IndexFreshness {
	switch {
	case !status.Started:
		return searchv1.IndexFreshness_INDEX_FRESHNESS_UNSPECIFIED
	case status.Stalled:
		return searchv1.IndexFreshness_INDEX_FRESHNESS_STALLED
	case status.ObservedSeq > status.LastSeq:
		return searchv1.IndexFreshness_INDEX_FRESHNESS_BEHIND
	default:
		return searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT
	}
}
