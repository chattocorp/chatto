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
	if len(request.GetThreadRootIds()) > 0 && !response.GetThreadScopeApplied() {
		return nil, fmt.Errorf("%w: provider does not support thread scope", ErrUnavailable)
	}
	if len(request.GetExcludedThreadRootIds()) > 0 && !response.GetThreadExclusionsApplied() {
		return nil, fmt.Errorf("%w: provider does not support thread exclusions", ErrUnavailable)
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

const (
	// allStatusTimeout bounds how long ListStatuses waits for replies.
	allStatusTimeout = time.Second
	// allStatusQuietPeriod ends ListStatuses early when no further reply
	// arrives this long after the latest one.
	allStatusQuietPeriod = 100 * time.Millisecond
)

// ListStatuses requests the status of every provider process on
// AllStatusSubject. It returns the valid replies that arrive within
// allStatusTimeout, and stops waiting allStatusQuietPeriod after the latest
// reply. It skips error replies and invalid statuses. Providers that do not
// answer AllStatusSubject, such as older versions, are not included.
func (c *Client) ListStatuses(ctx context.Context) ([]*searchv1.GetStatusResponse, error) {
	if c == nil || c.nc == nil {
		return nil, fmt.Errorf("%w: NATS connection is required", ErrUnavailable)
	}
	payload, err := proto.Marshal(&searchv1.GetStatusRequest{})
	if err != nil {
		return nil, fmt.Errorf("marshal search provider request: %w", err)
	}
	inbox := c.nc.NewInbox()
	subscription, err := c.nc.SubscribeSync(inbox)
	if err != nil {
		return nil, fmt.Errorf("subscribe to search provider replies: %w", err)
	}
	defer func() { _ = subscription.Unsubscribe() }()
	if err := c.nc.PublishRequest(AllStatusSubject, inbox, payload); err != nil {
		return nil, fmt.Errorf("request search provider statuses: %w", err)
	}

	collectContext, cancel := context.WithTimeout(ctx, allStatusTimeout)
	defer cancel()
	var statuses []*searchv1.GetStatusResponse
	for {
		waitContext := collectContext
		var cancelWait context.CancelFunc = func() {}
		if len(statuses) > 0 {
			waitContext, cancelWait = context.WithTimeout(collectContext, allStatusQuietPeriod)
		}
		message, err := subscription.NextMsgWithContext(waitContext)
		cancelWait()
		if err != nil {
			// The deadline or the quiet period ended the collection.
			return statuses, nil
		}
		if message.Header.Get(micro.ErrorHeader) != "" {
			continue
		}
		status := &searchv1.GetStatusResponse{}
		if proto.Unmarshal(message.Data, status) != nil || validateStatusResponse(status) != nil {
			continue
		}
		statuses = append(statuses, status)
	}
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
