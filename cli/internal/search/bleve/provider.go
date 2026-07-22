package bleve

import (
	"context"
	"errors"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"hmans.de/chatto/internal/events"
	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
	"hmans.de/chatto/internal/search"
)

// Provider exposes a projection through the provider-neutral NATS contract.
type Provider struct {
	Projection *Projection
	Projector  *events.Projector
}

func (p *Provider) Query(ctx context.Context, request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
	if p == nil || p.Projection == nil || p.Projector == nil {
		return nil, search.ErrProviderNotReady
	}
	status := p.Projector.Status()
	if !status.StartupComplete {
		return nil, search.ErrProviderNotReady
	}
	response, err := p.Projection.query(ctx, request)
	if errors.Is(err, errInvalidCursor) {
		return nil, &search.ServiceError{Code: search.ErrorCodeInvalidArgument, Description: "invalid search cursor"}
	}
	return response, err
}

func (p *Provider) GetStatus(context.Context, *searchv1.GetStatusRequest) (*searchv1.GetStatusResponse, error) {
	state := searchv1.ProviderState_PROVIDER_STATE_STARTING
	response := &searchv1.GetStatusResponse{State: state}
	if p == nil || p.Projector == nil {
		return response, nil
	}
	status := p.Projector.Status()
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
	return response, nil
}
