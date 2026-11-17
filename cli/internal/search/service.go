package search

import (
	"context"
	"encoding/json"
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

// ServiceOptions configures NATS micro monitoring metadata and the provider
// identity in status replies.
type ServiceOptions struct {
	ImplementationVersion string
	// InstanceID identifies the provider process in status replies and service
	// metadata. Use the same value for every service of one process.
	InstanceID string
	// Host is the host name of the provider process, for example a pod name.
	Host         string
	ErrorHandler micro.ErrHandler
}

// identify adds the provider identity to a status reply.
func (o ServiceOptions) identify(response *searchv1.GetStatusResponse) {
	response.InstanceId = o.InstanceID
	response.Host = o.Host
	response.ImplementationVersion = o.ImplementationVersion
}

// AddService registers a queue-grouped search provider on the well-known v1
// subjects. The caller owns stopping the returned service during shutdown.
func AddService(ctx context.Context, nc *nats.Conn, provider Provider, options ServiceOptions) (micro.Service, error) {
	service, err := addProviderService(ctx, nc, provider, options)
	if err != nil {
		return nil, err
	}
	if err := AddQueryEndpoint(ctx, nc, service, provider); err != nil {
		_ = service.Stop()
		return nil, err
	}
	if err := addStatusEndpoint(ctx, service, provider, options); err != nil {
		_ = service.Stop()
		return nil, err
	}
	return service, nil
}

// AddStartupStatusService reports startup progress without joining the ready
// status or query queues. It also answers AllStatusSubject without a queue
// group, so a caller can collect the status of every provider process. A
// provider that can answer queries also registers a separate service with
// AddService, and stops that service when it can no longer answer queries.
// The startup status service stays registered.
func AddStartupStatusService(ctx context.Context, nc *nats.Conn, provider Provider, options ServiceOptions) (micro.Service, error) {
	service, err := addProviderService(ctx, nc, provider, options)
	if err != nil {
		return nil, err
	}
	if err := service.AddEndpoint("startup-status", micro.ContextHandler(ctx, func(ctx context.Context, request micro.Request) {
		handleStatus(ctx, provider, options, request)
	}), micro.WithEndpointSubject(StartupStatusSubject)); err != nil {
		_ = service.Stop()
		return nil, err
	}
	if err := service.AddEndpoint("all-status", micro.ContextHandler(ctx, func(ctx context.Context, request micro.Request) {
		handleStatus(ctx, provider, options, request)
	}), micro.WithEndpointSubject(AllStatusSubject), micro.WithEndpointQueueGroupDisabled()); err != nil {
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
	if options.InstanceID != "" {
		metadata["instance_id"] = options.InstanceID
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

// addStatusEndpoint joins an existing provider service to the ready status
// queue. Callers must register it only when the provider can answer queries.
func addStatusEndpoint(ctx context.Context, service micro.Service, provider Provider, options ServiceOptions) error {
	if err := validateServiceEndpoint(ctx, service, provider); err != nil {
		return err
	}
	return service.AddEndpoint("status", micro.ContextHandler(ctx, func(ctx context.Context, request micro.Request) {
		handleStatus(ctx, provider, options, request)
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
// query context expires after queryHandlerTimeout. A NATS micro request must
// not be used after its handler returns, so a query goroutine replies through
// nc with a detached copy of the request. Micro service statistics therefore
// count queries but not their processing time or provider errors.
func AddQueryEndpoint(ctx context.Context, nc *nats.Conn, service micro.Service, provider Provider) error {
	if err := validateServiceEndpoint(ctx, service, provider); err != nil {
		return err
	}
	if nc == nil {
		return fmt.Errorf("search service NATS connection is required")
	}
	slots := make(chan struct{}, maxConcurrentQueries)
	return service.AddEndpoint("query", micro.ContextHandler(ctx, func(ctx context.Context, request micro.Request) {
		select {
		case slots <- struct{}{}:
		default:
			respondError(request, ErrorCodeUnavailable, "search provider is busy")
			return
		}
		detached := newDetachedRequest(nc, request)
		go func() {
			defer func() { <-slots }()
			queryContext, cancel := context.WithTimeout(ctx, queryHandlerTimeout)
			defer cancel()
			handleQuery(queryContext, provider, detached)
		}()
	}), micro.WithEndpointSubject(QuerySubject))
}

// detachedRequest is a copy of a micro request that stays valid after the
// micro handler returns. It replies by publishing to the reply subject, with
// the same error headers as micro.Request.Error.
type detachedRequest struct {
	nc      *nats.Conn
	subject string
	reply   string
	data    []byte
	headers micro.Headers
}

func newDetachedRequest(nc *nats.Conn, request micro.Request) *detachedRequest {
	return &detachedRequest{
		nc:      nc,
		subject: request.Subject(),
		reply:   request.Reply(),
		data:    request.Data(),
		headers: request.Headers(),
	}
}

func (r *detachedRequest) Respond(data []byte, opts ...micro.RespondOpt) error {
	return r.publish(&nats.Msg{Data: data}, opts)
}

func (r *detachedRequest) RespondJSON(value any, opts ...micro.RespondOpt) error {
	data, err := json.Marshal(value)
	if err != nil {
		return micro.ErrMarshalResponse
	}
	return r.Respond(data, opts...)
}

func (r *detachedRequest) Error(code, description string, data []byte, opts ...micro.RespondOpt) error {
	if code == "" || description == "" {
		return micro.ErrArgRequired
	}
	return r.publish(&nats.Msg{
		Header: nats.Header{micro.ErrorHeader: []string{description}, micro.ErrorCodeHeader: []string{code}},
		Data:   data,
	}, opts)
}

func (r *detachedRequest) publish(message *nats.Msg, opts []micro.RespondOpt) error {
	for _, opt := range opts {
		opt(message)
	}
	if r.reply == "" {
		return nats.ErrMsgNoReply
	}
	message.Subject = r.reply
	if err := r.nc.PublishMsg(message); err != nil {
		return fmt.Errorf("%w: %s", micro.ErrRespond, err)
	}
	return nil
}

func (r *detachedRequest) Data() []byte           { return r.data }
func (r *detachedRequest) Headers() micro.Headers { return r.headers }
func (r *detachedRequest) Subject() string        { return r.subject }
func (r *detachedRequest) Reply() string          { return r.reply }

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

func handleStatus(ctx context.Context, provider Provider, options ServiceOptions, request micro.Request) {
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
	// Providers may return a shared status value, so identify a copy.
	response = proto.Clone(response).(*searchv1.GetStatusResponse)
	options.identify(response)
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
