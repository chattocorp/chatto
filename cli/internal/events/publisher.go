// Package events is the internal event-sourcing framework for Chatto.
//
// It wraps the SERVER_EVT JetStream stream into a discipline:
//   - Every publish is OCC. There is no non-OCC publish primitive.
//   - Reads come from projections — in-memory Go structs that consume
//     events and update their state.
//   - Read-your-writes is opt-in via Projector.WaitForSeq.
//
// See docs/adr/ADR-033, ADR-034, ADR-035 for the broader design.
package events

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Logger is the small logging surface the framework uses.
// *log.Logger from github.com/charmbracelet/log satisfies it.
type Logger interface {
	Debug(msg interface{}, keyvals ...interface{})
	Info(msg interface{}, keyvals ...interface{})
	Warn(msg interface{}, keyvals ...interface{})
	Error(msg interface{}, keyvals ...interface{})
}

// ErrConflict is returned from AppendAt when the supplied expected sequence
// doesn't match the stream's current state for the subject. Callers in
// migration code use errors.Is(err, ErrConflict) to skip already-emitted
// subjects without inspecting raw NATS error codes.
var ErrConflict = errors.New("expected-last-subject-sequence mismatch")

// ErrInvalidEvent is returned when the event payload is nil or otherwise
// not well-formed before publish.
var ErrInvalidEvent = errors.New("invalid event")

// Publisher writes events to a JetStream stream with optimistic concurrency
// control. The stream is expected to be the SERVER_EVT stream; the Publisher
// itself doesn't enforce that — it operates on whatever stream is passed in,
// so the same primitive is reusable in tests against ad-hoc streams.
type Publisher struct {
	js     jetstream.JetStream
	stream jetstream.Stream
	logger Logger
}

// NewPublisher constructs a Publisher bound to a specific stream.
func NewPublisher(js jetstream.JetStream, stream jetstream.Stream, logger Logger) *Publisher {
	return &Publisher{js: js, stream: stream, logger: logger}
}

const (
	maxAppendRetries = 5
	flushTimeout     = 2 * time.Second
)

// Append publishes an event to a subject, automatically computing the
// expected last subject sequence from the stream and retrying on conflict.
// Returns the new stream sequence on success.
//
// This is the bread-and-butter mutation primitive: read state, decide the
// write, call Append, get a seq back. The OCC retry handles concurrent
// writers transparently. For deterministic-sequence callers (migrations),
// use AppendAt instead.
func (p *Publisher) Append(ctx context.Context, subject string, event *corev1.Event) (uint64, error) {
	if err := validateEvent(event); err != nil {
		return 0, err
	}

	data, err := proto.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("marshal event: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= maxAppendRetries; attempt++ {
		expectedSeq, err := p.lastSubjectSeq(ctx, subject)
		if err != nil {
			return 0, err
		}

		seq, err := p.publishAt(ctx, subject, data, expectedSeq)
		if err == nil {
			return seq, nil
		}

		if !errors.Is(err, ErrConflict) {
			return 0, err
		}

		p.logger.Debug("OCC conflict, retrying",
			"subject", subject,
			"expected_seq", expectedSeq,
			"attempt", attempt,
			"max_attempts", maxAppendRetries)
		lastErr = err

		// Exponential backoff with jitter (1, 2, 4, 8, 16 ms + 0-5ms).
		baseDelay := time.Duration(1<<(attempt-1)) * time.Millisecond
		jitter := time.Duration(rand.Int63n(int64(5 * time.Millisecond)))
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(baseDelay + jitter):
		}
	}

	return 0, fmt.Errorf("append after %d attempts: %w", maxAppendRetries, lastErr)
}

// AppendAt publishes an event with a caller-supplied expected last subject
// sequence. Returns ErrConflict (wrapped, retrievable via errors.Is) if the
// stream's current state for the subject doesn't match expectedSeq.
//
// IMPORTANT: expectedSeq is the *stream* sequence of the most recent
// message published to this subject — not a per-subject counter. A
// fresh subject expects 0 ("no prior message"). After a successful
// publish, the returned seq becomes the next call's expectedSeq.
//
// Used by migration code: for the first event on each subject, pass 0;
// thread the returned seq forward for subsequent events. A conflict on
// the first event means the subject is already migrated; the caller
// can skip the rest of the subject's events.
func (p *Publisher) AppendAt(ctx context.Context, subject string, event *corev1.Event, expectedSeq uint64) (uint64, error) {
	if err := validateEvent(event); err != nil {
		return 0, err
	}

	data, err := proto.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("marshal event: %w", err)
	}

	return p.publishAt(ctx, subject, data, expectedSeq)
}

// publishAt is the shared publish-with-expected-seq core used by both
// Append (which retries with re-read) and AppendAt (single shot).
// Translates the NATS sequence-mismatch error to ErrConflict.
func (p *Publisher) publishAt(ctx context.Context, subject string, data []byte, expectedSeq uint64) (uint64, error) {
	ack, err := p.js.Publish(ctx, subject, data,
		jetstream.WithExpectLastSequencePerSubject(expectedSeq))
	if err == nil {
		return ack.Sequence, nil
	}

	var apiErr *jetstream.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence {
		return 0, fmt.Errorf("subject %q at expected seq %d: %w", subject, expectedSeq, ErrConflict)
	}
	return 0, fmt.Errorf("publish: %w", err)
}

// lastSubjectSeq returns the current last sequence for a subject, or 0 if
// no messages exist for it.
func (p *Publisher) lastSubjectSeq(ctx context.Context, subject string) (uint64, error) {
	msg, err := p.stream.GetLastMsgForSubject(ctx, subject)
	if err == nil {
		return msg.Sequence, nil
	}
	if errors.Is(err, jetstream.ErrMsgNotFound) {
		return 0, nil
	}
	return 0, fmt.Errorf("last msg for subject %q: %w", subject, err)
}

func validateEvent(event *corev1.Event) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}
