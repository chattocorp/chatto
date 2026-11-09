package jetstreamutil

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// JetStreamResourceRetryPolicy bounds retries of repeatable resource provisioning.
// Applications choose the retry budget; the framework does not own resource policy.
type JetStreamResourceRetryPolicy struct {
	// MaxAttempts includes the initial call and must be positive.
	MaxAttempts int
	// RetryDelay is the initial exponential delay when MaxRetryDelay is positive.
	// Otherwise it is multiplied by the failed attempt number. It must be
	// non-negative and the largest linear delay must fit in time.Duration.
	RetryDelay time.Duration
	// MaxRetryDelay selects exponential backoff with equal jitter when positive.
	// Each wait is between half and all of the exponential delay, capped here.
	// Zero preserves linear backoff. It must be at least RetryDelay.
	MaxRetryDelay time.Duration
	// RequestTimeout bounds each callback when positive, independently of the
	// overall startup context. Zero leaves request deadlines to the caller.
	RequestTimeout time.Duration
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
	if policy.MaxAttempts < 1 || policy.RetryDelay < 0 || policy.MaxRetryDelay < 0 || policy.RequestTimeout < 0 ||
		(policy.MaxRetryDelay > 0 && (policy.RetryDelay <= 0 || policy.MaxRetryDelay < policy.RetryDelay)) ||
		(policy.MaxRetryDelay == 0 && policy.MaxAttempts > 1 && policy.RetryDelay > time.Duration(1<<63-1)/time.Duration(policy.MaxAttempts-1)) {
		return zero, fmt.Errorf("invalid JetStream resource retry policy")
	}
	delay := policy.RetryDelay
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		attemptCtx := ctx
		cancel := func() {}
		if policy.RequestTimeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, policy.RequestTimeout)
		}
		resource, err := create(attemptCtx)
		cancel()
		if parentErr := ctx.Err(); parentErr != nil {
			return zero, parentErr
		}
		if err == nil {
			return resource, nil
		}
		if attempt == policy.MaxAttempts || !isTransientJetStreamResourceError(err) {
			return zero, err
		}
		wait := time.Duration(attempt) * policy.RetryDelay
		if policy.MaxRetryDelay > 0 {
			wait = delay/2 + time.Duration(rand.Int64N(int64(delay-delay/2)+1))
			if delay > policy.MaxRetryDelay/2 {
				delay = policy.MaxRetryDelay
			} else {
				delay *= 2
			}
		}
		timer := time.NewTimer(wait)
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
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, nats.ErrTimeout) || errors.Is(err, nats.ErrNoResponders) {
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
	// 10008 means the cluster cannot serve the request temporarily. Do not
	// retry all 503 errors: disabled JetStream accounts are configuration errors.
	return apiErr.ErrorCode == 10008 || (apiErr.ErrorCode == 10049 && strings.Contains(apiErr.Description, "error creating store for stream")) ||
		(apiErr.ErrorCode == 10058 && strings.Contains(apiErr.Description, "stream name already in use"))
}
