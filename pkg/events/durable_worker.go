package events

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	defaultDurableWorkerFetchWait         = time.Second
	defaultDurableWorkerRetryDelay        = 30 * time.Second
	defaultDurableWorkerAckTimeout        = 5 * time.Second
	defaultDurableWorkerHeartbeatInterval = 30 * time.Second
)

// DurableDelivery is one opaque event delivered by a durable JetStream pull
// consumer. Applications own decoding, validation, projection catch-up, and
// idempotency. Data is detached from the underlying JetStream message.
type DurableDelivery struct {
	Subject        string
	Data           []byte
	StreamSequence uint64
	PublishedAt    time.Time
	NumDelivered   uint64
}

// DurableDeliveryHandler performs one at-least-once piece of work. Returning
// nil acknowledges the delivery. Other errors retry after the worker's default
// delay unless wrapped with RetryDeliveryAfter or TerminateDelivery.
type DurableDeliveryHandler func(context.Context, DurableDelivery) error

// DurableWorkerOptions controls process-local execution. The application
// remains responsible for the durable consumer's stream, name, filters,
// acknowledgement policy, and rollout contract.
type DurableWorkerOptions struct {
	MaxConcurrent     int
	FetchMaxWait      time.Duration
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

// NewDurableWorker validates and constructs a worker. It does not create or
// modify the supplied consumer.
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
	if opts.RetryDelay <= 0 {
		opts.RetryDelay = defaultDurableWorkerRetryDelay
	}
	if opts.AckTimeout <= 0 {
		opts.AckTimeout = defaultDurableWorkerAckTimeout
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = defaultDurableWorkerHeartbeatInterval
	}
	return &DurableWorker{consumer: consumer, handle: handle, opts: opts}, nil
}

// Run fetches and processes deliveries until the context is cancelled or a
// consumer fetch fails. Cancellation leaves active work unacknowledged so it
// can be handed to another worker.
func (w *DurableWorker) Run(ctx context.Context) error {
	if w == nil || w.consumer == nil || w.handle == nil {
		return fmt.Errorf("durable worker is not configured")
	}
	for ctx.Err() == nil {
		batch, err := w.consumer.Fetch(
			w.opts.MaxConcurrent,
			jetstream.FetchMaxWait(w.opts.FetchMaxWait),
		)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("fetch durable work: %w", err)
		}

		var group sync.WaitGroup
		for msg := range batch.Messages() {
			group.Add(1)
			go func() {
				defer group.Done()
				w.process(ctx, msg)
			}()
		}
		group.Wait()
		if err := batch.Error(); err != nil && ctx.Err() == nil {
			return fmt.Errorf("receive durable work: %w", err)
		}
	}
	return nil
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

	heartbeatDone := make(chan struct{})
	go w.heartbeat(msg, delivery, heartbeatDone)
	err = w.handle(ctx, delivery)
	close(heartbeatDone)

	if ctx.Err() != nil {
		if nakErr := msg.Nak(); nakErr != nil {
			w.logWarn("Durable delivery handoff failed", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "error", nakErr)
		}
		return
	}

	var terminateErr *terminateDeliveryError
	if errors.As(err, &terminateErr) {
		if termErr := msg.TermWithReason(terminateErr.reason); termErr != nil {
			w.logWarn("Durable delivery termination failed", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "error", termErr)
		}
		return
	}

	if err != nil {
		delay := w.opts.RetryDelay
		var retryErr *retryDeliveryError
		if errors.As(err, &retryErr) {
			delay = retryErr.delay
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

func (w *DurableWorker) heartbeat(msg jetstream.Msg, delivery DurableDelivery, done <-chan struct{}) {
	ticker := time.NewTicker(w.opts.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := msg.InProgress(); err != nil {
				w.logWarn("Durable delivery heartbeat failed", "subject", delivery.Subject, "stream_sequence", delivery.StreamSequence, "error", err)
			}
		}
	}
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
