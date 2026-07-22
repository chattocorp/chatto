package search

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"google.golang.org/protobuf/proto"

	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
)

const defaultProviderRequestTimeout = 10 * time.Second

// Client calls one compatible search provider through NATS request/reply.
type Client struct {
	nc             *nats.Conn
	requestTimeout time.Duration
}

// NewClient returns a provider client using nc.
func NewClient(nc *nats.Conn) *Client {
	return &Client{nc: nc, requestTimeout: defaultProviderRequestTimeout}
}

// Query requests one ordered page of thin provider hits.
func (c *Client) Query(ctx context.Context, request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
	if err := validateQueryRequest(request); err != nil {
		return nil, fmt.Errorf("validate search query: %w", err)
	}
	response := &searchv1.QueryResponse{}
	if err := c.request(ctx, QuerySubject, request, response); err != nil {
		return nil, err
	}
	if err := validateQueryResponse(response, request.GetPageSize()); err != nil {
		return nil, err
	}
	return response, nil
}

// GetStatus requests the provider's current readiness state.
func (c *Client) GetStatus(ctx context.Context) (*searchv1.GetStatusResponse, error) {
	response := &searchv1.GetStatusResponse{}
	if err := c.request(ctx, StatusSubject, &searchv1.GetStatusRequest{}, response); err != nil {
		if !errors.Is(err, ErrUnavailable) {
			return nil, err
		}
		response.Reset()
		if err := c.request(ctx, StartupStatusSubject, &searchv1.GetStatusRequest{}, response); err != nil {
			return nil, err
		}
	}
	if err := validateStatusResponse(response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) request(ctx context.Context, subject string, request, response proto.Message) error {
	if c == nil || c.nc == nil {
		return fmt.Errorf("%w: NATS connection is required", ErrUnavailable)
	}
	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal search provider request: %w", err)
	}
	requestContext, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	message, err := c.nc.RequestMsgWithContext(requestContext, &nats.Msg{Subject: subject, Data: payload})
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, nats.ErrTimeout) {
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return fmt.Errorf("request search provider: %w", err)
	}
	if description := message.Header.Get(micro.ErrorHeader); description != "" {
		return &ServiceError{
			Code:        message.Header.Get(micro.ErrorCodeHeader),
			Description: description,
			Details:     append([]byte(nil), message.Data...),
		}
	}
	if err := proto.Unmarshal(message.Data, response); err != nil {
		return fmt.Errorf("%w: decode response: %v", ErrInvalidResponse, err)
	}
	return nil
}
