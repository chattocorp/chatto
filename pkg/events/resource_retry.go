package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// JetStreamResourceRetryPolicy bounds retries of repeatable resource provisioning.
// Applications choose the retry budget; the framework does not own resource policy.
type JetStreamResourceRetryPolicy struct {
	// MaxAttempts includes the initial call and must be positive.
	MaxAttempts int
	// RetryDelay is multiplied by the failed attempt number before the next call.
	// It must be non-negative and the largest delay must fit in time.Duration.
	RetryDelay time.Duration
}

// CreateJetStreamResourceWithRetry retries transient JetStream provisioning errors.
// create must honor ctx and be safe to repeat after a remotely successful operation
// whose response was lost. Use it for resource provisioning, not event publishing.
// Resource names, configuration, metadata, and request timeouts remain caller-owned.
// A request deadline is retryable only while the parent ctx remains active. Parent
// cancellation or expiry takes precedence over the callback result. Exhaustion
// returns the last operation error, and every failure returns the zero value of T.
func CreateJetStreamResourceWithRetry[T any](ctx context.Context, policy JetStreamResourceRetryPolicy, create func(context.Context) (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if policy.MaxAttempts < 1 || policy.RetryDelay < 0 ||
		(policy.MaxAttempts > 1 && policy.RetryDelay > time.Duration(1<<63-1)/time.Duration(policy.MaxAttempts-1)) {
		return zero, fmt.Errorf("invalid JetStream resource retry policy")
	}
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		resource, err := create(ctx)
		if parentErr := ctx.Err(); parentErr != nil {
			return zero, parentErr
		}
		if err == nil {
			return resource, nil
		}
		if attempt == policy.MaxAttempts || !isTransientJetStreamResourceError(err) {
			return zero, err
		}
		timer := time.NewTimer(time.Duration(attempt) * policy.RetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}
}

func isTransientJetStreamResourceError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	// nats.go's context-based management requests return ctx.Err(), including
	// when its default request timeout expires under an active parent context.
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	type apiErrorProvider interface {
		APIError() *jetstream.APIError
	}
	var provider apiErrorProvider
	if !errors.As(err, &provider) {
		return false
	}
	apiErr := provider.APIError()
	if apiErr == nil {
		return false
	}
	return (apiErr.ErrorCode == 10049 && strings.Contains(apiErr.Description, "error creating store for stream")) ||
		(apiErr.ErrorCode == 10058 && strings.Contains(apiErr.Description, "stream name already in use"))
}
