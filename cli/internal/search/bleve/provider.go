package bleve

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

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
	state := searchv1.ProviderState_PROVIDER_STATE_STARTING
	response := &searchv1.GetStatusResponse{State: state}
	if projector == nil {
		return response
	}
	status := projector.Status()
	indexed := status.StartupMessages
	response.IndexedEventCount = &indexed
	probeSnapshot, probeStarted := probe.Snapshot()
	if probeStarted {
		response.QueryProbe = probeSnapshot.proto()
	}
	switch {
	case status.Err != nil && !status.StartupComplete:
		response.State = searchv1.ProviderState_PROVIDER_STATE_UNAVAILABLE
	case status.Err != nil:
		response.State = searchv1.ProviderState_PROVIDER_STATE_DEGRADED
	// A failed or blocked query probe means this provider cannot answer
	// queries now, although its index exists.
	case status.StartupComplete && probeSnapshot.Failing(time.Now()):
		response.State = searchv1.ProviderState_PROVIDER_STATE_DEGRADED
	case status.StartupComplete:
		response.State = searchv1.ProviderState_PROVIDER_STATE_READY
	case status.Started:
		response.State = searchv1.ProviderState_PROVIDER_STATE_INDEXING
		response.RetryAfter = durationpb.New(time.Second)
	}
	return response
}
