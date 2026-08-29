// Package evtstream adapts Chatto's durable EVT contract to the reusable
// event-sourcing mechanics in pkg/events.
//
// It owns the application-specific parts of that contract:
//   - the evtv1.Event protobuf envelope and codec;
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

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// ErrInvalidEvent is returned when a Chatto event is nil or otherwise not
// well-formed before encoding.
var ErrInvalidEvent = errors.New("invalid event")

const (
	subjectEventsPageSize     = 500
	subjectEventsPageMaxBytes = 16 * 1024 * 1024
)

// Publisher is Chatto's typed adapter over the byte-oriented event log. It
// validates and protobuf-encodes core events while EncodedEventLog owns NATS
// OCC, atomic publication, stream positions, and encoded reads.
//
// The embedded framework typed log supplies the mechanical encode/decode,
// batch, mutation, and paged-read mapping plus untyped log reads; this type
// adds only Chatto's envelope codec and EVT-specific convenience reads.
type Publisher struct {
	events.TypedEventLog[*evtv1.Event]
}

// NewPublisher constructs a Chatto event publisher bound to a stream.
func NewPublisher(js jetstream.JetStream, stream jetstream.Stream, logger events.Logger) *Publisher {
	log := events.NewEncodedEventLog(js, stream, logger)
	return &Publisher{
		TypedEventLog: *events.NewTypedEventLog(log, encodeEvent, decodeEventData),
	}
}

// BatchEntry is one Chatto event in an atomic publish batch. At least one entry
// must carry a per-subject, wildcard-filter, or whole-stream OCC guard.
type BatchEntry = events.TypedBatchEntry[*evtv1.Event]

// MutationEntry is one typed Chatto event selected by a mutation decision.
// The shared event framework applies OCC from the chosen boundary.
type MutationEntry = events.TypedMutationEntry[*evtv1.Event]

// SubjectEvents returns decoded events on a subject in stream order.
func (p *Publisher) SubjectEvents(
	ctx context.Context,
	subject string,
) ([]*evtv1.Event, uint64, error) {
	return p.SubjectEventsAfter(ctx, subject, 0)
}

// SubjectEventsAfter returns decoded events after a stream sequence.
func (p *Publisher) SubjectEventsAfter(
	ctx context.Context,
	subject string,
	afterSeq uint64,
) ([]*evtv1.Event, uint64, error) {
	subjectEvents, lastSeq, err := p.SubjectEventsWithSubjectsAfter(ctx, subject, afterSeq)
	if err != nil {
		return nil, lastSeq, err
	}
	decoded := make([]*evtv1.Event, 0, len(subjectEvents))
	for _, subjectEvent := range subjectEvents {
		decoded = append(decoded, subjectEvent.Event)
	}
	return decoded, lastSeq, nil
}

// SubjectEvent preserves the durable subject and stream sequence alongside a
// decoded event.
type SubjectEvent struct {
	Subject  string
	Sequence uint64
	Event    *evtv1.Event
}

// SubjectEventsWithSubjectsAfter decodes opaque records while preserving their
// matched durable subjects and stream sequences.
func (p *Publisher) SubjectEventsWithSubjectsAfter(
	ctx context.Context,
	subject string,
	afterSeq uint64,
) ([]*SubjectEvent, uint64, error) {
	records, lastSeq, err := p.TypedEventLog.SubjectEventsWithSubjectsAfter(ctx, subject, afterSeq, subjectEventsPageSize, subjectEventsPageMaxBytes)
	if err != nil {
		return nil, lastSeq, err
	}
	events := make([]*SubjectEvent, 0, len(records))
	for _, record := range records {
		events = append(events, &SubjectEvent{Subject: record.Subject, Sequence: record.Sequence, Event: record.Event})
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

func encodeEvent(event *evtv1.Event) (events.EncodedRecord, error) {
	if err := validateEvent(event); err != nil {
		return events.EncodedRecord{}, err
	}
	data, err := proto.Marshal(event)
	if err != nil {
		return events.EncodedRecord{}, fmt.Errorf("marshal event: %w", err)
	}
	return events.EncodedRecord{ID: event.GetId(), Data: data}, nil
}

func decodeEventData(data []byte) (*evtv1.Event, error) {
	var event evtv1.Event
	if err := proto.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

func validateEvent(event *evtv1.Event) error {
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
