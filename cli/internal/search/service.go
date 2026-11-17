package search

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"google.golang.org/protobuf/proto"

	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
)

// Provider implements the normalized search contract. It is trusted with
// indexed message content but does not own end-user authorization.
type Provider interface {
	Query(context.Context, *searchv1.QueryRequest) (*searchv1.QueryResponse, error)
	GetStatus(context.Context, *searchv1.GetStatusRequest) (*searchv1.GetStatusResponse, error)
}

// ServiceOptions configures NATS micro monitoring metadata.
type ServiceOptions struct {
	ImplementationVersion string
	ErrorHandler          micro.ErrHandler
}

// AddService registers a queue-grouped search provider on the well-known v1
// subjects. The caller owns stopping the returned service during shutdown.
func AddService(ctx context.Context, nc *nats.Conn, provider Provider, options ServiceOptions) (micro.Service, error) {
	service, err := addProviderService(ctx, nc, provider, options)
	if err != nil {
		return nil, err
	}
	if err := AddQueryEndpoint(ctx, service, provider); err != nil {
		_ = service.Stop()
		return nil, err
	}
	if err := AddStatusEndpoint(ctx, service, provider); err != nil {
		_ = service.Stop()
		return nil, err
	}
	return service, nil
}

// AddStartupStatusService reports startup progress without joining the ready
// status or query queues. A provider that can answer queries also registers a
// separate service with AddService, and stops that service when it can no
// longer answer queries. The startup status service stays registered.
func AddStartupStatusService(ctx context.Context, nc *nats.Conn, provider Provider, options ServiceOptions) (micro.Service, error) {
	service, err := addProviderService(ctx, nc, provider, options)
	if err != nil {
		return nil, err
	}
	if err := service.AddEndpoint("startup-status", micro.ContextHandler(ctx, func(ctx context.Context, request micro.Request) {
		handleStatus(ctx, provider, request)
	}), micro.WithEndpointSubject(StartupStatusSubject)); err != nil {
		_ = service.Stop()
		return nil, err
	}
	return service, nil
}

func addProviderService(ctx context.Context, nc *nats.Conn, provider Provider, options ServiceOptions) (micro.Service, error) {
	if ctx == nil {
		return nil, fmt.Errorf("search service context is required")
	}
	if nc == nil {
		return nil, fmt.Errorf("search service NATS connection is required")
	}
	if provider == nil {
		return nil, fmt.Errorf("search service provider is required")
	}
	metadata := map[string]string{}
	if options.ImplementationVersion != "" {
		metadata["implementation_version"] = options.ImplementationVersion
	}
	service, err := micro.AddService(nc, micro.Config{
		Name:         ServiceName,
		Version:      ServiceVersion,
		Description:  "Chatto message search provider contract v1",
		Metadata:     metadata,
		QueueGroup:   QueueGroup,
		ErrorHandler: options.ErrorHandler,
	})
	if err != nil {
		return nil, err
	}
	return service, nil
}

// AddStatusEndpoint joins an existing provider service to the ready status
// queue. Callers must register it only when the provider can answer queries.
func AddStatusEndpoint(ctx context.Context, service micro.Service, provider Provider) error {
	if err := validateServiceEndpoint(ctx, service, provider); err != nil {
		return err
	}
	return service.AddEndpoint("status", micro.ContextHandler(ctx, func(ctx context.Context, request micro.Request) {
		handleStatus(ctx, provider, request)
	}), micro.WithEndpointSubject(StatusSubject))
}

const (
	// maxConcurrentQueries bounds the query handlers that one query endpoint
	// runs at a time. When all handlers are busy, for example because index
	// work blocks them, the endpoint rejects new queries at once with a
	// retryable error. Blocked work thus cannot accumulate without bound.
	maxConcurrentQueries = 16
	// queryHandlerTimeout bounds one query. It is shorter than the client's
	// request timeout, so the client receives a provider error, not a timeout,
	// when the provider honors cancellation.
	queryHandlerTimeout = 8 * time.Second
)

// AddQueryEndpoint joins an existing provider service to the shared query
// queue. Callers must not register it until the provider can answer queries.
//
// Queries run concurrently, at most maxConcurrentQueries at a time, and each
// query context expires after queryHandlerTimeout.
func AddQueryEndpoint(ctx context.Context, service micro.Service, provider Provider) error {
	if err := validateServiceEndpoint(ctx, service, provider); err != nil {
		return err
	}
	slots := make(chan struct{}, maxConcurrentQueries)
	return service.AddEndpoint("query", micro.ContextHandler(ctx, func(ctx context.Context, request micro.Request) {
		select {
		case slots <- struct{}{}:
		default:
			respondError(request, ErrorCodeUnavailable, "search provider is busy")
			return
		}
		go func() {
			defer func() { <-slots }()
			queryContext, cancel := context.WithTimeout(ctx, queryHandlerTimeout)
			defer cancel()
			handleQuery(queryContext, provider, request)
		}()
	}), micro.WithEndpointSubject(QuerySubject))
}

func validateServiceEndpoint(ctx context.Context, service micro.Service, provider Provider) error {
	if ctx == nil {
		return fmt.Errorf("search service context is required")
	}
	if service == nil {
		return fmt.Errorf("search service is required")
	}
	if provider == nil {
		return fmt.Errorf("search service provider is required")
	}
	return nil
}

func handleQuery(ctx context.Context, provider Provider, request micro.Request) {
	query := &searchv1.QueryRequest{}
	if err := proto.Unmarshal(request.Data(), query); err != nil {
		respondError(request, ErrorCodeInvalidArgument, "invalid search query payload")
		return
	}
	if err := validateQueryRequest(query); err != nil {
		respondError(request, ErrorCodeInvalidArgument, "invalid search query")
		return
	}
	response, err := provider.Query(ctx, query)
	if err != nil {
		respondProviderError(request, err)
		return
	}
	if err := validateQueryResponse(response, query.GetPageSize()); err != nil {
		respondError(request, ErrorCodeInternal, "search provider returned an invalid response")
		return
	}
	respondProto(request, response)
}

func handleStatus(ctx context.Context, provider Provider, request micro.Request) {
	statusRequest := &searchv1.GetStatusRequest{}
	if err := proto.Unmarshal(request.Data(), statusRequest); err != nil {
		respondError(request, ErrorCodeInvalidArgument, "invalid search status payload")
		return
	}
	response, err := provider.GetStatus(ctx, statusRequest)
	if err != nil {
		respondProviderError(request, err)
		return
	}
	if err := validateStatusResponse(response); err != nil {
		respondError(request, ErrorCodeInternal, "search provider returned an invalid response")
		return
	}
	respondProto(request, response)
}

func respondProviderError(request micro.Request, err error) {
	if errors.Is(err, ErrProviderNotReady) || errors.Is(err, ErrUnavailable) {
		respondError(request, ErrorCodeUnavailable, "search provider is not ready")
		return
	}
	var serviceError *ServiceError
	if errors.As(err, &serviceError) && serviceError.Code != "" && serviceError.Description != "" {
		_ = request.Error(serviceError.Code, serviceError.Description, serviceError.Details)
		return
	}
	respondError(request, ErrorCodeInternal, "search provider failed")
}

func respondProto(request micro.Request, response proto.Message) {
	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
	if err != nil {
		respondError(request, ErrorCodeInternal, "search provider response encoding failed")
		return
	}
	_ = request.Respond(payload)
}

func respondError(request micro.Request, code, description string) {
	_ = request.Error(code, description, nil)
}
