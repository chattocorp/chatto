package events

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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

	// durableWorkerIdleFetchWait is the pull lifetime of an idle worker (see
	// Run). An idle worker sends one pull request for each interval instead of
	// one for each FetchMaxWait. Idle polling is the main source of idle
	// JetStream API traffic when many replicas run many workers.
	//
	// Trade-off: when a replica disappears without closing its connection, the
	// server keeps apparent interest in its open pull until the pull expires.
	// Work that arrives in that time goes to the dead pull, counts as a
	// delivery attempt, and reaches another replica only after the consumer's
	// AckWait. A longer idle pull makes that window longer (about 29 seconds
	// instead of about one second). A clean shutdown or a connection close that
	// the server detects removes the interest and the pull at once.
	durableWorkerIdleFetchWait = 30 * time.Second
	// durableWorkerMaxRequestExpiresRejection is the start of the description
	// that nats-server sends in a 409 status when a pull is longer than the
	// consumer's MaxRequestExpires. nats.go has no typed error for it.
	durableWorkerMaxRequestExpiresRejection = "Exceeded MaxRequestExpires of "
	// durableWorkerIdleFetchHeartbeats is the number of pull heartbeat
	// intervals in one pull of 10 seconds or more. nats.go uses a 5-second
	// heartbeat for such pulls by default; a longer interval sends fewer idle
	// messages. nats.go fails a pull that misses two heartbeats, so a lost
	// pull, for example after a broker restart, retries within two intervals.
	durableWorkerIdleFetchHeartbeats = 3
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
	MaxConcurrent int
	// FetchMaxWait is the lifetime of a normal pull. It also sets the
	// shutdown handoff delay. An idle worker uses a longer pull (30 seconds,
	// or the consumer's MaxRequestExpires when that is shorter, but never
	// shorter than FetchMaxWait). See Run.
	FetchMaxWait      time.Duration
	FetchRetryDelay   time.Duration
	RetryDelay        time.Duration
	AckTimeout        time.Duration
	HeartbeatInterval time.Duration
	Logger            *slog.Logger
}

// DurableWorker runs bounded, at-least-once work from an application-owned
// durable JetStream pull consumer.
type DurableWorker struct {
	consumer jetstream.Consumer
	handle   DurableDeliveryHandler
	opts     DurableWorkerOptions
	// idleFetchWait is the lifetime of a long pull. Only the Run goroutine
	// changes it after construction. See Run and adaptIdleFetchWait.
	idleFetchWait time.Duration
	runMu         sync.Mutex
	started       bool
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

// TerminationReason reports whether err was created by TerminateDelivery and
// returns its reason. Applications that run a DurableDeliveryHandler outside a
// DurableWorker use it to keep the same non-retryable classification.
func TerminationReason(err error) (string, bool) {
	if terminateErr, ok := errors.AsType[*terminateDeliveryError](err); ok {
		return terminateErr.reason, true
	}
	return "", false
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
	if consumer == nil || handle == nil {
		panic("events: durable worker requires a consumer and a handler")
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
	var maxRequestExpires time.Duration
	if info := consumer.CachedInfo(); info != nil {
		maxRequestExpires = info.Config.MaxRequestExpires
	}
	return &DurableWorker{
		consumer:      consumer,
		handle:        handle,
		opts:          opts,
		idleFetchWait: idleFetchWait(opts.FetchMaxWait, maxRequestExpires),
	}, nil
}

// Run fetches and processes deliveries until the context is cancelled. It may
// be called only once per worker. Fetch failures are retried because durable
// workers must survive transient broker
// outages. Cancellation first stops outstanding fetches, then stops progress
// heartbeats and negatively acknowledges active deliveries before waiting for
// their handlers to stop. This ordering prevents the stopping worker from
// reclaiming its own handoffs through an outstanding pull.
//
// The worker has at most one outstanding pull. A pull is short (FetchMaxWait)
// unless both of these are true:
//
//   - The previous pull ended empty because it expired.
//   - No delivery is active.
//
// Then the pull is long (durableWorkerIdleFetchWait, limited by the consumer's
// MaxRequestExpires), so an idle worker sends few pull requests. After a
// delivery or a failed pull, the next pull is short. Thus a backlog uses only
// short pulls, and a rejected long pull falls back to a short pull. The
// consumer's MaxRequestExpires can become smaller while the worker runs. When
// JetStream rejects a long pull for that reason, the worker takes the new limit
// from the rejection. It does not read consumer information, because nats.go
// does not synchronize the cached information of a shared consumer handle.
//
// Only a pull can start a delivery, so no delivery is active while a long
// pull is outstanding. Thus, when cancellation hands off active deliveries,
// any orphaned pull of this worker is a short pull, and the handoff delay of
// FetchMaxWait plus a margin still outlives it.
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
	// idle is true when the previous pull expired without a delivery.
	idle := false
	for ctx.Err() == nil {
		select {
		case active <- struct{}{}:
		case <-ctx.Done():
			return nil
		}

		// This loop holds one slot for the pull itself. One occupied slot thus
		// means that no handler is active. A handler can release its slot
		// concurrently; that only makes this check choose a short pull.
		long := idle && len(active) == 1
		fetchWait := w.opts.FetchMaxWait
		if long {
			fetchWait = w.idleFetchWait
		}
		fetchCtx, cancelFetch := context.WithTimeout(ctx, fetchWait)
		msg, err := w.consumer.Next(fetchOptions(fetchCtx)...)
		cancelFetch()
		if err != nil {
			<-active
			if ctx.Err() != nil {
				return nil
			}
			// Classify the returned error, not the fetch context: a pull can fail
			// with a server or heartbeat error just before its deadline.
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, nats.ErrTimeout) {
				idle = true
				continue
			}
			idle = false
			// A configured consumer disappearing is an application-owned
			// lifecycle failure, not a transport interruption. Returning lets the
			// application recreate or deliberately retire the consumer instead of
			// leaving this worker in a permanent retry loop on a stale handle.
			if errors.Is(err, jetstream.ErrConsumerDeleted) || errors.Is(err, jetstream.ErrConsumerNotFound) {
				return fmt.Errorf("durable worker consumer is unavailable: %w", err)
			}
			w.opts.Logger.Warn("Durable work fetch failed; retrying", "error", err)
			if long {
				w.adaptIdleFetchWait(err)
			}
			if !waitForDurableWorkerRetry(ctx, w.opts.FetchRetryDelay) {
				return nil
			}
			continue
		}
		idle = false
		if ctx.Err() != nil {
			if err := msg.Nak(); err != nil {
				w.opts.Logger.Warn("Durable delivery handoff failed", "subject", msg.Subject(), "error", err)
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

// idleFetchWait returns the lifetime of a pull that starts while no delivery
// is active. The consumer's MaxRequestExpires, when set, limits it, because
// JetStream rejects a pull with a longer expiry. It is never shorter than
// fetchMaxWait, which an application chooses for its consumer.
func idleFetchWait(fetchMaxWait, maxRequestExpires time.Duration) time.Duration {
	wait := durableWorkerIdleFetchWait
	if maxRequestExpires > 0 {
		wait = min(wait, maxRequestExpires)
	}
	return max(wait, fetchMaxWait)
}

// adaptIdleFetchWait shortens long pulls after JetStream rejected a long pull
// because it is longer than the consumer's MaxRequestExpires. It takes the
// limit from the rejection. If the rejection has no readable limit, it halves
// the long pull, so that repeated rejections reach an accepted length. Other
// failures, for example a missed heartbeat, do not change the long pull. The
// worker does not lengthen the long pull again if the limit later increases.
func (w *DurableWorker) adaptIdleFetchWait(err error) {
	_, rest, found := strings.Cut(err.Error(), durableWorkerMaxRequestExpiresRejection)
	if !found {
		return
	}
	limit := w.idleFetchWait / 2
	if fields := strings.Fields(rest); len(fields) > 0 {
		if parsed, err := time.ParseDuration(fields[0]); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	w.idleFetchWait = idleFetchWait(w.opts.FetchMaxWait, min(limit, w.idleFetchWait))
}

// fetchOptions returns the pull options for one fetch that ends with ctx. A
// parent deadline can make the pull shorter than the requested wait, so the
// idle heartbeat comes from the remaining lifetime. Below 10 seconds, nats.go
// sends no heartbeat by default, and the worker keeps that default.
//
// nats.go v1.54.0 does not stop the heartbeat timer of a completed fetch; the
// timer callback runs two heartbeat intervals later and then releases the
// fetch state. With a FetchMaxWait below 10 seconds, only long pulls have a
// heartbeat, and a long pull starts only after an empty short pull. A worker
// thus starts at most one such timer for each short pull that expires empty
// (about 0.9 times FetchMaxWait), also under load. A FetchMaxWait of 10 seconds
// or more gives short pulls a heartbeat too.
func fetchOptions(ctx context.Context) []jetstream.FetchOpt {
	opts := []jetstream.FetchOpt{jetstream.FetchContext(ctx)}
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining >= 10*time.Second {
			opts = append(opts, jetstream.FetchHeartbeat(remaining/durableWorkerIdleFetchHeartbeats))
		}
	}
	return opts
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
		w.opts.Logger.Error("Durable delivery metadata unavailable", "error", err)
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
				w.opts.Logger.Warn("Durable delivery heartbeat failed", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "error", err)
			}
		}
	}
}

func (w *DurableWorker) handoff(msg jetstream.Msg, delivery DurableDelivery) {
	delay := durableWorkerHandoffDelay(w.opts.FetchMaxWait)
	// Canceling a client-side pull does not guarantee that JetStream has already
	// expired the corresponding server-side request: nats.go removes the inbox
	// interest asynchronously after Next returns. Delay redelivery past the
	// maximum pull lifetime so an orphaned request from this stopping worker
	// cannot reclaim the handoff. A worker with an active delivery starts only
	// short pulls (see Run), so FetchMaxWait bounds that lifetime.
	if err := msg.NakWithDelay(delay); err != nil {
		w.opts.Logger.Warn("Durable delivery handoff failed", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "retry_delay", delay, "error", err)
	}
}

func durableWorkerHandoffDelay(fetchMaxWait time.Duration) time.Duration {
	margin := min(max(fetchMaxWait/10, time.Millisecond), 100*time.Millisecond)
	return fetchMaxWait + margin
}

func (w *DurableWorker) finish(ctx context.Context, msg jetstream.Msg, delivery DurableDelivery, err error) {
	if terminateErr, ok := errors.AsType[*terminateDeliveryError](err); ok {
		if termErr := msg.TermWithReason(terminateErr.reason); termErr != nil {
			w.opts.Logger.Warn("Durable delivery termination failed", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "error", termErr)
		} else {
			w.opts.Logger.Error("Durable delivery terminated", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "delivery_attempt", delivery.NumDelivered, "reason", terminateErr.reason, "error", terminateErr.err)
		}
		return
	}

	if err != nil {
		delay := w.opts.RetryDelay
		if retryErr, ok := errors.AsType[*retryDeliveryError](err); ok {
			delay = retryErr.delay
		}
		if shouldLogDurableDeliveryAttempt(delivery.NumDelivered) {
			w.opts.Logger.Warn("Durable delivery failed; retrying", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "delivery_attempt", delivery.NumDelivered, "retry_delay", delay, "error", err)
		}
		w.retry(msg, delay)
		return
	}

	ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.opts.AckTimeout)
	defer cancel()
	if err := msg.DoubleAck(ackCtx); err != nil {
		w.opts.Logger.Warn("Durable delivery acknowledgement was not confirmed", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "error", err)
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
		w.opts.Logger.Warn("Durable delivery retry request failed", "subject", msg.Subject(), "error", err)
	}
}
