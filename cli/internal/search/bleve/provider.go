package bleve

import (
	"context"
	"errors"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
	"hmans.de/chatto/internal/search"
	"hmans.de/chatto/pkg/events"
)

// Provider exposes a projection through the provider-neutral NATS contract.
type Provider struct {
	projection events.ProjectionHandle[*Projection]
}

func newProvider(projection events.ProjectionHandle[*Projection]) *Provider {
	return &Provider{projection: projection}
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
		return providerStatus(nil), nil
	}
	return providerStatus(p.projection.Projector()), nil
}

func providerStatus(projector *events.Projector) *searchv1.GetStatusResponse {
	state := searchv1.ProviderState_PROVIDER_STATE_STARTING
	response := &searchv1.GetStatusResponse{State: state}
	if projector == nil {
		return response
	}
	status := projector.Status()
	indexed := status.StartupMessages
	response.IndexedEventCount = &indexed
	switch {
	case status.Failed && !status.StartupComplete:
		response.State = searchv1.ProviderState_PROVIDER_STATE_UNAVAILABLE
	case status.Failed:
		response.State = searchv1.ProviderState_PROVIDER_STATE_DEGRADED
	case status.StartupComplete:
		response.State = searchv1.ProviderState_PROVIDER_STATE_READY
	case status.Started:
		response.State = searchv1.ProviderState_PROVIDER_STATE_INDEXING
		response.RetryAfter = durationpb.New(time.Second)
	}
	return response
}
