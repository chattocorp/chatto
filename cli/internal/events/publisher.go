// Package events is the internal event-sourcing framework for Chatto.
//
// It wraps the EVT JetStream stream into a discipline:
//   - Every publish is OCC. There is no non-OCC publish primitive.
//   - Reads come from projections — in-memory Go structs that consume
//     events and update their state.
//   - Read-your-writes is opt-in via Projector.WaitForSeq.
//
// See docs/adr/ADR-033, ADR-034, ADR-035 for the broader design.
package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	mrand "math/rand"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
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

// ErrMissingOCC is returned when an atomic batch contains no optimistic
// concurrency guard. Every batch needs at least one guard so there is no
// accidental "publish without OCC" path through the framework.
var ErrMissingOCC = errors.New("missing optimistic concurrency guard")

// Publisher writes events to a JetStream stream with optimistic concurrency
// control. The stream is expected to be the EVT stream; the Publisher
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

const maxAppendRetries = 5

// Append publishes an event to a subject, automatically computing the
// expected last subject sequence from the stream. Returns the new stream
// sequence on success.
//
// This is the bread-and-butter mutation primitive: read state, decide the
// write, call Append, get a seq back. Conflicts are returned to the caller
// so state-replacement mutations can re-read and re-compose before retrying.
// For deterministic-sequence callers (migrations), use AppendAt instead.
func (p *Publisher) Append(ctx context.Context, subject string, event *corev1.Event) (uint64, error) {
	if err := validateEvent(event); err != nil {
		return 0, err
	}

	data, err := proto.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("marshal event: %w", err)
	}

	expectedSeq, err := p.lastSubjectSeq(ctx, subject)
	if err != nil {
		return 0, err
	}
	return p.publishAt(ctx, subject, data, expectedSeq, "")
}

// AppendEventually is the append-only variant of Append: it retries OCC
// conflicts with the same event payload. Use it only for event types where
// retrying the exact same fact is safe after an intervening write, such as
// message posts, membership joins/leaves, and tombstone-style lifecycle
// events. State-replacement events should use Append or AppendAt and
// re-compose from the latest projection state on conflict.
func (p *Publisher) AppendEventually(ctx context.Context, subject string, event *corev1.Event) (uint64, error) {
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

		seq, err := p.publishAt(ctx, subject, data, expectedSeq, "")
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
		jitter := time.Duration(mrand.Int63n(int64(5 * time.Millisecond)))
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

	return p.publishAt(ctx, subject, data, expectedSeq, "")
}

// AppendAtFilter publishes with OCC against a wildcard subject filter.
// The publish lands on `subject`; the OCC check is "the last stream
// message matching `filter` is at sequence `expectedFilterSeq`."
//
// Use this when an invariant spans multiple per-aggregate subjects —
// e.g. unique room names across every evt.room.{R} subject, where the
// per-subject OCC of the target subject can't detect another aggregate
// claiming the same name. The cluster-global filter check, backed by
// JetStream's filtered-state lookup, gives multi-process safety.
//
// Single-shot: returns ErrConflict on mismatch. The caller drives the
// retry loop because the pre-publish projection check (e.g. "is this
// name available?") must be re-evaluated on each attempt. Typical
// `expectedFilterSeq` source is the corresponding projector's
// LastSeq(); on conflict, backoff briefly and retry — the local
// projector consumer will catch up to the foreign publish within a few
// milliseconds.
func (p *Publisher) AppendAtFilter(ctx context.Context, subject string, event *corev1.Event, filter string, expectedFilterSeq uint64) (uint64, error) {
	if err := validateEvent(event); err != nil {
		return 0, err
	}
	data, err := proto.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("marshal event: %w", err)
	}
	return p.publishAt(ctx, subject, data, expectedFilterSeq, filter)
}

// publishAt is the shared publish-with-expected-seq core used by
// Append, AppendAt, and AppendAtFilter. When filter is empty, the
// expected-seq check applies to `subject` itself; when filter is set
// (typically a wildcard like "evt.room.>"), the check applies to the
// last stream message matching the filter — see
// `Nats-Expected-Last-Subject-Sequence-Subject` in the JetStream
// protocol. Translates the NATS sequence-mismatch error to ErrConflict.
func (p *Publisher) publishAt(ctx context.Context, subject string, data []byte, expectedSeq uint64, filter string) (uint64, error) {
	var opt jetstream.PublishOpt
	if filter == "" {
		opt = jetstream.WithExpectLastSequencePerSubject(expectedSeq)
	} else {
		opt = jetstream.WithExpectLastSequenceForSubject(expectedSeq, filter)
	}
	ack, err := p.js.Publish(ctx, subject, data, opt)
	if err == nil {
		return ack.Sequence, nil
	}

	var apiErr *jetstream.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence {
		target := subject
		if filter != "" {
			target = "filter " + filter
		}
		return 0, fmt.Errorf("%s at expected seq %d: %w", target, expectedSeq, ErrConflict)
	}
	return 0, fmt.Errorf("publish: %w", err)
}

// BatchEntry is one event in an atomic publish batch (AppendBatch).
// Each entry can carry its own optional OCC token — either a
// per-subject expected-last-sequence (set ExpectedSeq, leave
// FilterSubject empty) or a wildcard-filter expected-last-sequence
// (set both). Setting neither skips OCC for this entry.
//
// Within a batch, OCC is evaluated per-entry against the stream's
// committed state at batch-acceptance time; the server doesn't
// extrapolate "what the prior entries in this batch would commit
// to" when checking the next entry's OCC. Avoid same-subject
// dependent OCC within a single batch.
type BatchEntry struct {
	Subject       string
	Event         *corev1.Event
	ExpectedSeq   uint64 // 0 = no OCC (or "must be empty subject" when FilterSubject is set)
	FilterSubject string // when non-empty, ExpectedSeq is evaluated against this wildcard filter
	HasOCC        bool   // set true when ExpectedSeq is meaningful (distinguishes "no OCC" from "expect seq 0")
}

// AppendBatch publishes a sequence of events atomically: either all
// land adjacently in stream order or none do. The stream's
// AllowAtomicPublish must be enabled (see EVT stream config).
//
// Use this for multi-aggregate cascades where projections must
// never observe an intermediate state that breaks a cross-aggregate
// invariant — most notably MoveRoomToGroup, where the source's
// RoomRemovedFromGroup and the target's RoomAddedToGroup land
// together so the "every room belongs to exactly one group"
// invariant is preserved at every observable sequence.
//
// On success returns the stream sequences of each entry in
// publication order (contiguous; the last entry's seq is the commit
// ack's sequence). On per-entry OCC failure or any commit-time
// error, returns the wrapped error and zero entries land. Caller
// drives any retry — same shape as AppendAt / AppendAtFilter.
//
// Empty `entries` is a no-op returning a nil slice.
func (p *Publisher) AppendBatch(ctx context.Context, entries []BatchEntry) ([]uint64, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	for i, e := range entries {
		if err := validateEvent(e.Event); err != nil {
			return nil, fmt.Errorf("batch entry %d: %w", i, err)
		}
	}
	hasOCC := false
	for _, e := range entries {
		if e.HasOCC {
			hasOCC = true
			break
		}
	}
	if !hasOCC {
		return nil, ErrMissingOCC
	}

	batchID, err := newBatchID()
	if err != nil {
		return nil, fmt.Errorf("generate batch id: %w", err)
	}

	for i, e := range entries[:len(entries)-1] {
		if _, err := p.publishBatchEntry(ctx, e, batchID, uint64(i+1), false); err != nil {
			return nil, fmt.Errorf("batch entry %d: %w", i, err)
		}
	}

	// Final entry carries the commit marker; its ack is the batch ack.
	final := entries[len(entries)-1]
	commitSeq, err := p.publishBatchEntry(ctx, final, batchID, uint64(len(entries)), true)
	if err != nil {
		return nil, fmt.Errorf("batch commit: %w", err)
	}

	// Batch entries land contiguously in stream order, so we can
	// derive every entry's seq from the commit ack's seq.
	seqs := make([]uint64, len(entries))
	for i := range entries {
		seqs[i] = commitSeq - uint64(len(entries)-1-i)
	}
	return seqs, nil
}

// publishBatchEntry publishes a batch member via raw NATS request-
// reply and returns the server's stream sequence (0 for non-commit
// entries — the server doesn't include it in the empty intermediate
// ack). The high-level jetstream.PublishMsg wrapper can't be used
// because it rejects the empty-bodied intermediate acks as invalid;
// OCC failures still surface through the JSON error envelope.
func (p *Publisher) publishBatchEntry(ctx context.Context, e BatchEntry, batchID string, batchSeq uint64, commit bool) (uint64, error) {
	msg, err := p.buildBatchMsg(e, batchID, batchSeq, commit)
	if err != nil {
		return 0, err
	}
	resp, err := p.js.Conn().RequestMsgWithContext(ctx, msg)
	if err != nil {
		return 0, fmt.Errorf("publish: %w", err)
	}
	return decodeBatchAck(resp, e)
}

// pubAckEnvelope is the minimal shape of the JetStream JSON
// publish-ack response. Either an Error is set or the (Stream, Sequence)
// pair is populated. Matches the server's JSPubAckResponse struct.
type pubAckEnvelope struct {
	Error *struct {
		Code        int    `json:"code"`
		ErrCode     uint16 `json:"err_code"`
		Description string `json:"description"`
	} `json:"error,omitempty"`
	Stream    string `json:"stream,omitempty"`
	Sequence  uint64 `json:"seq,omitempty"`
	Duplicate bool   `json:"duplicate,omitempty"`
}

// decodeBatchAck distinguishes the three response shapes from the
// server: (a) empty body — success on a non-commit entry, returns
// (0, nil); (b) JSON error — OCC or other server-side rejection,
// returns (0, ErrConflict|wrapped); (c) JSON pub-ack — success on
// the commit entry, returns (seq, nil).
func decodeBatchAck(resp *nats.Msg, e BatchEntry) (uint64, error) {
	if len(resp.Data) == 0 {
		return 0, nil
	}
	var env pubAckEnvelope
	if err := json.Unmarshal(resp.Data, &env); err != nil {
		return 0, fmt.Errorf("decode ack: %w", err)
	}
	if env.Error != nil {
		if env.Error.ErrCode == uint16(jetstream.JSErrCodeStreamWrongLastSequence) {
			target := e.Subject
			if e.FilterSubject != "" {
				target = "filter " + e.FilterSubject
			}
			return 0, fmt.Errorf("%s at expected seq %d: %w", target, e.ExpectedSeq, ErrConflict)
		}
		return 0, fmt.Errorf("server: %s (err_code=%d)", env.Error.Description, env.Error.ErrCode)
	}
	return env.Sequence, nil
}

// buildBatchMsg assembles a *nats.Msg with the batch headers
// (Nats-Batch-Id / Nats-Batch-Sequence / optional Nats-Batch-Commit)
// and the per-entry OCC headers (Nats-Expected-Last-Subject-Sequence
// and optionally Nats-Expected-Last-Subject-Sequence-Subject).
func (p *Publisher) buildBatchMsg(e BatchEntry, batchID string, batchSeq uint64, commit bool) (*nats.Msg, error) {
	data, err := proto.Marshal(e.Event)
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}
	hdr := nats.Header{}
	hdr.Set("Nats-Batch-Id", batchID)
	hdr.Set("Nats-Batch-Sequence", strconv.FormatUint(batchSeq, 10))
	if commit {
		hdr.Set("Nats-Batch-Commit", "1")
	}
	if e.HasOCC {
		hdr.Set("Nats-Expected-Last-Subject-Sequence", strconv.FormatUint(e.ExpectedSeq, 10))
		if e.FilterSubject != "" {
			hdr.Set("Nats-Expected-Last-Subject-Sequence-Subject", e.FilterSubject)
		}
	}
	return &nats.Msg{Subject: e.Subject, Header: hdr, Data: data}, nil
}

// newBatchID returns a fresh batch identifier — 16 hex chars of
// crypto/rand. Used to group an atomic batch's publishes server-side.
func newBatchID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// LastSubjectSeq returns the stream's current last sequence for a
// subject (or wildcard subject filter), or 0 if no matching messages
// exist. Use this to source the expected-seq for wildcard-filter OCC
// (`Nats-Expected-Last-Subject-Sequence-Subject`) — reading directly
// from the stream avoids the trap of relying on a projector's LastSeq
// whose filter scope is narrower than the OCC filter (the projector
// wouldn't advance on events outside its filter, so its LastSeq would
// fall permanently behind the stream's actual filter-tail seq, and
// every OCC retry would conflict on the same stale value).
//
// Backed by `GetLastMsgForSubject`, which accepts NATS wildcards.
func (p *Publisher) LastSubjectSeq(ctx context.Context, subjectOrFilter string) (uint64, error) {
	return p.lastSubjectSeq(ctx, subjectOrFilter)
}

// SubjectEvents returns events currently published on a subject, in stream
// order, plus the stream sequence of the last matching event. Migration code
// uses this to resume imports without duplicating events after a crash or
// failed boot.
func (p *Publisher) SubjectEvents(ctx context.Context, subject string) ([]*corev1.Event, uint64, error) {
	consumer, err := p.stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubjects:    []string{subject},
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		return nil, 0, err
	}
	defer p.stream.DeleteConsumer(context.Background(), consumer.CachedInfo().Name)

	info, err := consumer.Info(ctx)
	if err != nil {
		return nil, 0, err
	}

	remaining := int(info.NumPending)
	events := make([]*corev1.Event, 0, remaining)
	var lastSeq uint64
	for remaining > 0 {
		batchSize := remaining
		if batchSize > 500 {
			batchSize = 500
		}
		msgs, err := consumer.Fetch(batchSize, jetstream.FetchMaxWait(10*time.Second))
		if err != nil {
			if errors.Is(err, jetstream.ErrNoMessages) {
				break
			}
			return nil, 0, err
		}

		fetched := 0
		for msg := range msgs.Messages() {
			fetched++
			meta, err := msg.Metadata()
			if err != nil {
				return nil, 0, fmt.Errorf("message metadata: %w", err)
			}
			lastSeq = meta.Sequence.Stream

			var event corev1.Event
			if err := proto.Unmarshal(msg.Data(), &event); err != nil {
				return nil, 0, fmt.Errorf("unmarshal event at seq %d: %w", lastSeq, err)
			}
			events = append(events, &event)
		}
		if fetched == 0 {
			break
		}
		remaining -= fetched
	}
	return events, lastSeq, nil
}

// SubjectEventIDs returns the envelope IDs currently published on a subject,
// in stream order, plus the stream sequence of the last matching event.
func (p *Publisher) SubjectEventIDs(ctx context.Context, subject string) ([]string, uint64, error) {
	events, lastSeq, err := p.SubjectEvents(ctx, subject)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.GetId())
	}
	return ids, lastSeq, nil
}

// lastSubjectSeq returns the current last sequence for a subject (or
// wildcard subject filter), or 0 if no matching messages exist.
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
