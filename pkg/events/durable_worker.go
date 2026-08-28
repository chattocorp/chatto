package events

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	defaultDurableWorkerFetchWait         = time.Second
	defaultDurableWorkerFetchRetryDelay   = time.Second
	defaultDurableWorkerRetryDelay        = 30 * time.Second
	defaultDurableWorkerAckTimeout        = 5 * time.Second
	defaultDurableWorkerHeartbeatInterval = 30 * time.Second
)

// ErrDurableWorkerAlreadyStarted is returned when Run is called more than once
// on the same worker. A worker owns one process-local execution lifecycle; make
// a new worker for a restarted consumer loop.
var ErrDurableWorkerAlreadyStarted = errors.New("durable worker already started")

// DurableDelivery is one opaque event delivered by a durable JetStream pull
// consumer. Subject is caller-owned operational metadata and may be logged;
// it must be an opaque, non-sensitive subject. Applications own decoding,
// validation, projection catch-up, and idempotency. Data is detached from the
// underlying JetStream message.
type DurableDelivery struct {
	Subject        string
	Data           []byte
	StreamSequence uint64
	PublishedAt    time.Time
	NumDelivered   uint64
}

// DurableDeliveryHandler performs one at-least-once piece of work. It must stop
// promptly when its context is cancelled. Returning nil acknowledges the
// delivery. Other errors retry after the worker's default delay unless wrapped
// with RetryDeliveryAfter or TerminateDelivery. Returned errors may be logged
// and must not contain secrets or personally identifiable information.
type DurableDeliveryHandler func(context.Context, DurableDelivery) error

// DurableWorkerOptions controls process-local execution. The application
// remains responsible for the durable consumer's stream, name, filters,
// acknowledgement policy, and rollout contract.
type DurableWorkerOptions struct {
	MaxConcurrent     int
	FetchMaxWait      time.Duration
	FetchRetryDelay   time.Duration
	RetryDelay        time.Duration
	AckTimeout        time.Duration
	HeartbeatInterval time.Duration
	Logger            Logger
}

// DurableWorker runs bounded, at-least-once work from an application-owned
// durable JetStream pull consumer.
type DurableWorker struct {
	consumer jetstream.Consumer
	handle   DurableDeliveryHandler
	opts     DurableWorkerOptions
	runMu    sync.Mutex
	started  bool
}

type retryDeliveryError struct {
	err   error
	delay time.Duration
}

func (e *retryDeliveryError) Error() string { return e.err.Error() }
func (e *retryDeliveryError) Unwrap() error { return e.err }

type terminateDeliveryError struct {
	err    error
	reason string
}

func (e *terminateDeliveryError) Error() string { return e.err.Error() }
func (e *terminateDeliveryError) Unwrap() error { return e.err }

// RetryDeliveryAfter overrides the worker's default retry delay for one
// handler failure. A non-positive delay asks JetStream to redeliver promptly.
func RetryDeliveryAfter(err error, delay time.Duration) error {
	if err == nil {
		err = errors.New("durable delivery retry requested")
	}
	return &retryDeliveryError{err: err, delay: delay}
}

// TerminateDelivery marks malformed or permanently unsupported input as
// non-retryable. The reason is recorded by JetStream and should not contain
// secrets or personally identifiable information.
func TerminateDelivery(reason string, err error) error {
	if err == nil {
		err = errors.New("durable delivery terminated")
	}
	return &terminateDeliveryError{err: err, reason: reason}
}

// Options returns the worker's effective, default-resolved options as fixed
// at construction. It exists so applications can assert their own
// option-mapping helpers pass delivery knobs through unchanged.
func (w *DurableWorker) Options() DurableWorkerOptions {
	return w.opts
}

// NewDurableWorker validates and constructs a worker. It applies framework
// defaults for unset options but does not create or modify the supplied
// consumer.
func NewDurableWorker(
	consumer jetstream.Consumer,
	handle DurableDeliveryHandler,
	opts DurableWorkerOptions,
) (*DurableWorker, error) {
	if consumer == nil {
		return nil, fmt.Errorf("durable worker consumer is nil")
	}
	if handle == nil {
		return nil, fmt.Errorf("durable worker handler is nil")
	}
	if opts.MaxConcurrent <= 0 {
		return nil, fmt.Errorf("durable worker max concurrency must be positive")
	}
	if opts.FetchMaxWait <= 0 {
		opts.FetchMaxWait = defaultDurableWorkerFetchWait
	}
	if opts.FetchRetryDelay <= 0 {
		opts.FetchRetryDelay = defaultDurableWorkerFetchRetryDelay
	}
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = defaultDurableWorkerRetryDelay
	}
	if opts.AckTimeout <= 0 {
		opts.AckTimeout = defaultDurableWorkerAckTimeout
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = defaultDurableWorkerHeartbeatInterval
	}
	opts.Logger = normalizeLogger(opts.Logger)
	return &DurableWorker{consumer: consumer, handle: handle, opts: opts}, nil
}

// Run fetches and processes deliveries until the context is cancelled. It may
// be called only once per worker. Fetch failures are retried because durable
// workers must survive transient broker
// outages. Cancellation first stops outstanding fetches, then stops progress
// heartbeats and negatively acknowledges active deliveries before waiting for
// their handlers to stop. This ordering prevents the stopping worker from
// reclaiming its own handoffs through an outstanding pull.
func (w *DurableWorker) Run(ctx context.Context) error {
	if w == nil || w.consumer == nil || w.handle == nil {
		return fmt.Errorf("durable worker is not configured")
	}
	w.runMu.Lock()
	if w.started {
		w.runMu.Unlock()
		return ErrDurableWorkerAlreadyStarted
	}
	w.started = true
	w.runMu.Unlock()

	handlerCtx, cancelHandlers := context.WithCancel(context.WithoutCancel(ctx))
	var group sync.WaitGroup
	defer func() {
		cancelHandlers()
		group.Wait()
	}()

	active := make(chan struct{}, w.opts.MaxConcurrent)
	for ctx.Err() == nil {
		select {
		case active <- struct{}{}:
		case <-ctx.Done():
			return nil
		}

		fetchCtx, cancelFetch := context.WithTimeout(ctx, w.opts.FetchMaxWait)
		msg, err := w.consumer.Next(jetstream.FetchContext(fetchCtx))
		fetchCtxErr := fetchCtx.Err()
		cancelFetch()
		if err != nil {
			<-active
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(fetchCtxErr, context.DeadlineExceeded) || errors.Is(err, nats.ErrTimeout) {
				continue
			}
			// A configured consumer disappearing is an application-owned
			// lifecycle failure, not a transport interruption. Returning lets the
			// application recreate or deliberately retire the consumer instead of
			// leaving this worker in a permanent retry loop on a stale handle.
			if errors.Is(err, jetstream.ErrConsumerDeleted) || errors.Is(err, jetstream.ErrConsumerNotFound) {
				return fmt.Errorf("durable worker consumer is unavailable: %w", err)
			}
			w.logWarn("Durable work fetch failed; retrying", "error", err)
			if !waitForDurableWorkerRetry(ctx, w.opts.FetchRetryDelay) {
				return nil
			}
			continue
		}
		if ctx.Err() != nil {
			if err := msg.Nak(); err != nil {
				w.logWarn("Durable delivery handoff failed", "subject", msg.Subject(), "error", err)
			}
			<-active
			return nil
		}
		group.Add(1)
		go func(msg jetstream.Msg) {
			defer group.Done()
			defer func() { <-active }()
			w.process(handlerCtx, msg)
		}(msg)
	}
	return nil
}

func waitForDurableWorkerRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (w *DurableWorker) process(ctx context.Context, msg jetstream.Msg) {
	metadata, err := msg.Metadata()
	if err != nil {
		w.logError("Durable delivery metadata unavailable", "error", err)
		w.retry(msg, w.opts.RetryDelay)
		return
	}

	delivery := DurableDelivery{
		Subject:        msg.Subject(),
		Data:           bytes.Clone(msg.Data()),
		StreamSequence: metadata.Sequence.Stream,
		PublishedAt:    metadata.Timestamp,
		NumDelivered:   metadata.NumDelivered,
	}

	result := make(chan error, 1)
	go func() { result <- w.handle(ctx, delivery) }()

	heartbeat := time.NewTicker(w.opts.HeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case err = <-result:
			if ctx.Err() != nil {
				w.handoff(msg, delivery)
				return
			}
			w.finish(ctx, msg, delivery, err)
			return
		case <-ctx.Done():
			w.handoff(msg, delivery)
			// Retain ownership of the handler goroutine. Applications must honor
			// cancellation so their dependencies cannot outlive the worker.
			<-result
			return
		case <-heartbeat.C:
			if err := msg.InProgress(); err != nil {
				w.logWarn("Durable delivery heartbeat failed", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "error", err)
			}
		}
	}
}

func (w *DurableWorker) handoff(msg jetstream.Msg, delivery DurableDelivery) {
	delay := durableWorkerHandoffDelay(w.opts.FetchMaxWait)
	// Canceling a client-side pull does not guarantee that JetStream has already
	// expired the corresponding server-side request. Delay redelivery past the
	// maximum pull lifetime so an orphaned request from this stopping worker
	// cannot reclaim the handoff.
	if err := msg.NakWithDelay(delay); err != nil {
		w.logWarn("Durable delivery handoff failed", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "retry_delay", delay, "error", err)
	}
}

func durableWorkerHandoffDelay(fetchMaxWait time.Duration) time.Duration {
	margin := fetchMaxWait / 10
	if margin < time.Millisecond {
		margin = time.Millisecond
	}
	if margin > 100*time.Millisecond {
		margin = 100 * time.Millisecond
	}
	return fetchMaxWait + margin
}

func (w *DurableWorker) finish(ctx context.Context, msg jetstream.Msg, delivery DurableDelivery, err error) {
	var terminateErr *terminateDeliveryError
	if errors.As(err, &terminateErr) {
		if termErr := msg.TermWithReason(terminateErr.reason); termErr != nil {
			w.logWarn("Durable delivery termination failed", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "error", termErr)
		} else {
			w.logError("Durable delivery terminated", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "delivery_attempt", delivery.NumDelivered, "reason", terminateErr.reason, "error", terminateErr.err)
		}
		return
	}

	if err != nil {
		delay := w.opts.RetryDelay
		var retryErr *retryDeliveryError
		if errors.As(err, &retryErr) {
			delay = retryErr.delay
		}
		if shouldLogDurableDeliveryAttempt(delivery.NumDelivered) {
			w.logWarn("Durable delivery failed; retrying", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "delivery_attempt", delivery.NumDelivered, "retry_delay", delay, "error", err)
		}
		w.retry(msg, delay)
		return
	}

	ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.opts.AckTimeout)
	defer cancel()
	if err := msg.DoubleAck(ackCtx); err != nil {
		w.logWarn("Durable delivery acknowledgement was not confirmed", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "error", err)
	}
}

// shouldLogDurableDeliveryAttempt keeps persistent failures observable without
// emitting one log line on every unlimited redelivery. The first attempt and
// powers of two provide exponentially sparse progress samples.
func shouldLogDurableDeliveryAttempt(attempt uint64) bool {
	return attempt <= 1 || attempt&(attempt-1) == 0
}

func (w *DurableWorker) retry(msg jetstream.Msg, delay time.Duration) {
	var err error
	if delay > 0 {
		err = msg.NakWithDelay(delay)
	} else {
		err = msg.Nak()
	}
	if err != nil {
		w.logWarn("Durable delivery retry request failed", "subject", msg.Subject(), "error", err)
	}
}

func (w *DurableWorker) logWarn(message interface{}, keyvals ...interface{}) {
	if w.opts.Logger != nil {
		w.opts.Logger.Warn(message, keyvals...)
	}
}

func (w *DurableWorker) logError(message interface{}, keyvals ...interface{}) {
	if w.opts.Logger != nil {
		w.opts.Logger.Error(message, keyvals...)
	}
}
