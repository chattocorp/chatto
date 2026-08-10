// Package evtstream adapts Chatto's durable EVT contract to the reusable
// event-sourcing mechanics in pkg/events.
//
// It owns the application-specific parts of that contract:
//   - the corev1.Event protobuf envelope and codec;
//   - stable aggregate subjects and event tokens;
//   - the EVT stream incarnation metadata; and
//   - typed publishing and projection construction.
//
// The underlying event log retains the framework discipline:
//   - Every publish is OCC. There is no non-OCC publish primitive.
//   - Reads come from projections — in-memory Go structs that consume events.
//   - Read-your-writes is opt-in via Projector.WaitFor.
//
// See docs/adr/ADR-033, ADR-034, ADR-035, and ADR-056.
package evtstream

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

// ErrInvalidEvent is returned when a Chatto event is nil or otherwise not
// well-formed before encoding.
var ErrInvalidEvent = errors.New("invalid event")

// Publisher is Chatto's typed adapter over the byte-oriented event log. It
// validates and protobuf-encodes core events while EncodedEventLog owns NATS
// OCC, atomic publication, stream positions, and encoded reads.
type Publisher struct {
	log *events.EncodedEventLog
}

// NewPublisher constructs a Chatto event publisher bound to a stream.
func NewPublisher(js jetstream.JetStream, stream jetstream.Stream, logger events.Logger) *Publisher {
	return &Publisher{log: events.NewEncodedEventLog(js, stream, logger)}
}

// StreamUsage returns the current message and byte totals for the bound stream.
func (p *Publisher) StreamUsage(ctx context.Context) (messages, bytes uint64, err error) {
	return p.log.StreamUsage(ctx)
}

// Append validates, encodes, and publishes an event using the subject's current
// tail as its OCC token.
func (p *Publisher) Append(ctx context.Context, subject string, event *corev1.Event) (uint64, error) {
	record, err := encodeEvent(event)
	if err != nil {
		return 0, err
	}
	return p.log.Append(ctx, subject, record)
}

// AppendEventually retries OCC conflicts with the exact same encoded event.
// Use it only when the event's semantics are safe after an intervening write.
func (p *Publisher) AppendEventually(
	ctx context.Context,
	subject string,
	event *corev1.Event,
) (uint64, error) {
	record, err := encodeEvent(event)
	if err != nil {
		return 0, err
	}
	return p.log.AppendEventually(ctx, subject, record)
}

// AppendAt publishes an event with a caller-supplied expected last sequence
// for subject.
func (p *Publisher) AppendAt(
	ctx context.Context,
	subject string,
	event *corev1.Event,
	expectedSeq uint64,
) (uint64, error) {
	record, err := encodeEvent(event)
	if err != nil {
		return 0, err
	}
	return p.log.AppendAt(ctx, subject, record, expectedSeq)
}

// AppendAtFilter publishes to subject with OCC against a possibly wildcarded
// subject filter.
func (p *Publisher) AppendAtFilter(
	ctx context.Context,
	subject string,
	event *corev1.Event,
	filter string,
	expectedFilterSeq uint64,
) (uint64, error) {
	record, err := encodeEvent(event)
	if err != nil {
		return 0, err
	}
	return p.log.AppendAtFilter(ctx, subject, record, filter, expectedFilterSeq)
}

// BatchEntry is one Chatto event in an atomic publish batch. At least one entry
// must carry a per-subject, wildcard-filter, or whole-stream OCC guard.
type BatchEntry struct {
	Subject           string
	Event             *corev1.Event
	ExpectedSeq       uint64
	FilterSubject     string
	HasOCC            bool
	ExpectedStreamSeq uint64
	HasStreamOCC      bool
}

// AppendBatch validates and encodes every event before atomically publishing
// the resulting opaque records. Either all records land adjacently or none do.
func (p *Publisher) AppendBatch(ctx context.Context, entries []BatchEntry) ([]uint64, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	hasOCC := false
	for i, entry := range entries {
		if err := validateEvent(entry.Event); err != nil {
			return nil, fmt.Errorf("batch entry %d: %w", i, err)
		}
		hasOCC = hasOCC || entry.HasOCC || entry.HasStreamOCC
	}
	if !hasOCC {
		return nil, events.ErrMissingOCC
	}

	encoded := make([]events.EncodedBatchEntry, len(entries))
	for i, entry := range entries {
		record, err := encodeEvent(entry.Event)
		if err != nil {
			return nil, fmt.Errorf("batch entry %d: %w", i, err)
		}
		encoded[i] = events.EncodedBatchEntry{
			Subject:           entry.Subject,
			Record:            record,
			ExpectedSeq:       entry.ExpectedSeq,
			FilterSubject:     entry.FilterSubject,
			HasOCC:            entry.HasOCC,
			ExpectedStreamSeq: entry.ExpectedStreamSeq,
			HasStreamOCC:      entry.HasStreamOCC,
		}
	}
	return p.log.AppendBatch(ctx, encoded)
}

// MutationEntry is one typed Chatto event selected by a mutation decision.
// The shared event framework applies OCC from the chosen boundary.
type MutationEntry struct {
	Subject string
	Event   *corev1.Event
}

// ExecuteMutation captures the selected boundary, reruns decide after OCC
// conflicts, and atomically publishes the returned events. Returning no
// entries is a successful no-op.
func (p *Publisher) ExecuteMutation(
	ctx context.Context,
	boundary events.MutationBoundary,
	decide func(context.Context, events.MutationAttempt) ([]MutationEntry, error),
) (events.MutationResult, error) {
	if decide == nil {
		return events.MutationResult{}, events.ErrInvalidMutationDecision
	}
	return p.log.ExecuteMutation(ctx, boundary, func(ctx context.Context, attempt events.MutationAttempt) ([]events.EncodedMutationEntry, error) {
		entries, err := decide(ctx, attempt)
		if err != nil {
			return nil, err
		}
		encoded := make([]events.EncodedMutationEntry, len(entries))
		for i, entry := range entries {
			record, err := encodeEvent(entry.Event)
			if err != nil {
				return nil, fmt.Errorf("mutation entry %d: %w", i, err)
			}
			encoded[i] = events.EncodedMutationEntry{Subject: entry.Subject, Record: record}
		}
		return encoded, nil
	})
}

// LastStreamSeq returns the current last sequence of EVT.
func (p *Publisher) LastStreamSeq(ctx context.Context) (uint64, error) {
	return p.log.LastStreamSeq(ctx)
}

// LastSubjectSeq returns the stream's current last sequence for an exact
// subject or wildcard subject filter.
func (p *Publisher) LastSubjectSeq(ctx context.Context, subjectOrFilter string) (uint64, error) {
	return p.log.LastSubjectSeq(ctx, subjectOrFilter)
}

// LastSubjectPosition returns the stream's current position for an exact
// subject or wildcard subject filter.
func (p *Publisher) LastSubjectPosition(
	ctx context.Context,
	subjectOrFilter string,
) (events.StreamPosition, error) {
	return p.log.LastSubjectPosition(ctx, subjectOrFilter)
}

// SubjectEvents returns decoded events on a subject in stream order.
func (p *Publisher) SubjectEvents(
	ctx context.Context,
	subject string,
) ([]*corev1.Event, uint64, error) {
	return p.SubjectEventsAfter(ctx, subject, 0)
}

// SubjectEventsAfter returns decoded events after a stream sequence.
func (p *Publisher) SubjectEventsAfter(
	ctx context.Context,
	subject string,
	afterSeq uint64,
) ([]*corev1.Event, uint64, error) {
	subjectEvents, lastSeq, err := p.SubjectEventsWithSubjectsAfter(ctx, subject, afterSeq)
	if err != nil {
		return nil, lastSeq, err
	}
	events := make([]*corev1.Event, 0, len(subjectEvents))
	for _, subjectEvent := range subjectEvents {
		events = append(events, subjectEvent.Event)
	}
	return events, lastSeq, nil
}

// SubjectEvent preserves the durable subject alongside a decoded event.
type SubjectEvent struct {
	Subject string
	Event   *corev1.Event
}

// SubjectEventsWithSubjectsAfter decodes opaque records while preserving their
// matched durable subjects.
func (p *Publisher) SubjectEventsWithSubjectsAfter(
	ctx context.Context,
	subject string,
	afterSeq uint64,
) ([]*SubjectEvent, uint64, error) {
	records, lastSeq, err := p.log.SubjectRecordsAfter(ctx, subject, afterSeq)
	if err != nil {
		return nil, lastSeq, err
	}
	events := make([]*SubjectEvent, 0, len(records))
	for _, record := range records {
		var event corev1.Event
		if err := proto.Unmarshal(record.Data, &event); err != nil {
			return nil, 0, fmt.Errorf("unmarshal event at seq %d: %w", record.Sequence, err)
		}
		events = append(events, &SubjectEvent{Subject: record.Subject, Event: &event})
	}
	return events, lastSeq, nil
}

// SubjectEventIDs returns envelope IDs on a subject in stream order.
func (p *Publisher) SubjectEventIDs(
	ctx context.Context,
	subject string,
) ([]string, uint64, error) {
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

func encodeEvent(event *corev1.Event) (events.EncodedRecord, error) {
	if err := validateEvent(event); err != nil {
		return events.EncodedRecord{}, err
	}
	data, err := proto.Marshal(event)
	if err != nil {
		return events.EncodedRecord{}, fmt.Errorf("marshal event: %w", err)
	}
	return events.EncodedRecord{ID: event.GetId(), Data: data}, nil
}

func validateEvent(event *corev1.Event) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	if event.GetId() == "" {
		return fmt.Errorf("%w: event id is empty", ErrInvalidEvent)
	}
	if EventTypeOf(event) == "" {
		return fmt.Errorf("%w: %T is not a durable EVT event type", ErrInvalidEvent, event.GetEvent())
	}
	return nil
}
