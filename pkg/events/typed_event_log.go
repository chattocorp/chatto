package events

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoTypedDecoder is returned when typed subject reads are requested from a
// TypedEventLog constructed without a decoder.
var ErrNoTypedDecoder = errors.New("typed event log has no decoder")

// TypedEventEncoder turns one application event into an opaque event-log
// record before publication. The application owns validation, stable record
// IDs, wire encoding, and every storage policy expressed by the record.
type TypedEventEncoder[E any] func(E) (EncodedRecord, error)

// TypedEventDecoder turns one opaque event-log record payload back into its
// application event for typed subject reads. It must reject records that the
// application cannot interpret rather than guessing.
type TypedEventDecoder[E any] func([]byte) (E, error)

// TypedSubjectRecord pairs one decoded application event with its matched
// durable subject and stream sequence.
type TypedSubjectRecord[E any] struct {
	Subject  string
	Sequence uint64
	Event    E
}

// TypedBatchEntry is one application event in an atomic publish batch. At
// least one entry must carry a per-subject, wildcard-filter, or whole-stream
// OCC guard; the underlying EncodedEventLog enforces that invariant.
type TypedBatchEntry[E any] struct {
	Subject           string
	Event             E
	ExpectedSeq       uint64
	FilterSubject     string
	HasOCC            bool
	ExpectedStreamSeq uint64
	HasStreamOCC      bool
}

// TypedMutationEntry is one typed application event selected by a mutation
// decision. The shared event framework applies OCC from the chosen boundary.
type TypedMutationEntry[E any] struct {
	Subject string
	Event   E
}

// TypedEventLog adapts the envelope-neutral byte-oriented event log to one
// application event type E through an application-supplied encoder. It owns
// only the mechanical encode, decode, and batch/mutation mapping; applications
// keep their subjects, semantic validation, envelope policy, and composition.
//
// Embedding an *EncodedEventLog means untyped log reads such as LastSubjectSeq,
// LastSubjectPosition, StreamUsage, and SubjectRecordsAfterPage remain
// available on the typed value.
type TypedEventLog[E any] struct {
	*EncodedEventLog

	encode TypedEventEncoder[E]
	decode TypedEventDecoder[E]
}

// NewTypedEventLog binds a typed adapter to an already constructed encoded
// event log. Encode must be non-nil; it is applied to every published event.
// Decode may be nil when the application only publishes through this log, but
// typed subject reads then return an error.
func NewTypedEventLog[E any](log *EncodedEventLog, encode TypedEventEncoder[E], decode TypedEventDecoder[E]) *TypedEventLog[E] {
	return &TypedEventLog[E]{EncodedEventLog: log, encode: encode, decode: decode}
}

// Append validates, encodes, and publishes an event using the subject's
// current tail as its OCC token.
func (t *TypedEventLog[E]) Append(ctx context.Context, subject string, event E) (uint64, error) {
	record, err := t.encode(event)
	if err != nil {
		return 0, err
	}
	return t.EncodedEventLog.Append(ctx, subject, record)
}

// AppendEventually retries OCC conflicts with the exact same encoded event.
// Use it only when the event's semantics are safe after an intervening write.
func (t *TypedEventLog[E]) AppendEventually(ctx context.Context, subject string, event E) (uint64, error) {
	record, err := t.encode(event)
	if err != nil {
		return 0, err
	}
	return t.EncodedEventLog.AppendEventually(ctx, subject, record)
}

// AppendAt publishes an event with a caller-supplied expected last sequence
// for subject.
func (t *TypedEventLog[E]) AppendAt(ctx context.Context, subject string, event E, expectedSeq uint64) (uint64, error) {
	record, err := t.encode(event)
	if err != nil {
		return 0, err
	}
	return t.EncodedEventLog.AppendAt(ctx, subject, record, expectedSeq)
}

// AppendAtFilter publishes to subject with OCC against a possibly wildcarded
// subject filter.
func (t *TypedEventLog[E]) AppendAtFilter(
	ctx context.Context,
	subject string,
	event E,
	filter string,
	expectedFilterSeq uint64,
) (uint64, error) {
	record, err := t.encode(event)
	if err != nil {
		return 0, err
	}
	return t.EncodedEventLog.AppendAtFilter(ctx, subject, record, filter, expectedFilterSeq)
}

// AppendBatch encodes every event before atomically publishing the resulting
// opaque records. Either all records land adjacently or none do.
func (t *TypedEventLog[E]) AppendBatch(ctx context.Context, entries []TypedBatchEntry[E]) ([]uint64, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	encoded := make([]EncodedBatchEntry, len(entries))
	for i, entry := range entries {
		record, err := t.encode(entry.Event)
		if err != nil {
			return nil, fmt.Errorf("batch entry %d: %w", i, err)
		}
		encoded[i] = EncodedBatchEntry{
			Subject:           entry.Subject,
			Record:            record,
			ExpectedSeq:       entry.ExpectedSeq,
			FilterSubject:     entry.FilterSubject,
			HasOCC:            entry.HasOCC,
			ExpectedStreamSeq: entry.ExpectedStreamSeq,
			HasStreamOCC:      entry.HasStreamOCC,
		}
	}
	return t.EncodedEventLog.AppendBatch(ctx, encoded)
}

// ExecuteMutation captures the selected boundary, reruns decide after OCC
// conflicts, and atomically publishes the returned events. Returning no
// entries is a successful no-op.
func (t *TypedEventLog[E]) ExecuteMutation(
	ctx context.Context,
	boundary MutationBoundary,
	decide func(context.Context, MutationAttempt) ([]TypedMutationEntry[E], error),
) (MutationResult, error) {
	if decide == nil {
		return MutationResult{}, ErrInvalidMutationDecision
	}
	return t.EncodedEventLog.ExecuteMutation(ctx, boundary, func(ctx context.Context, attempt MutationAttempt) ([]EncodedMutationEntry, error) {
		entries, err := decide(ctx, attempt)
		if err != nil {
			return nil, err
		}
		encoded := make([]EncodedMutationEntry, len(entries))
		for i, entry := range entries {
			record, err := t.encode(entry.Event)
			if err != nil {
				return nil, fmt.Errorf("mutation entry %d: %w", i, err)
			}
			encoded[i] = EncodedMutationEntry{Subject: entry.Subject, Record: record}
		}
		return encoded, nil
	})
}

// SubjectEventsWithSubjectsAfter decodes all records matching subject with a
// stream sequence greater than afterSeq while preserving each record's matched
// durable subject and sequence. maxRecords bounds every page;
// maxBytes bounds page payload bytes when positive, mirroring
// SubjectRecordsAfterPage.
func (t *TypedEventLog[E]) SubjectEventsWithSubjectsAfter(
	ctx context.Context,
	subject string,
	afterSeq uint64,
	maxRecords int,
	maxBytes int,
) ([]TypedSubjectRecord[E], uint64, error) {
	if t.decode == nil {
		return nil, 0, ErrNoTypedDecoder
	}
	var decodedEvents []TypedSubjectRecord[E]
	var lastSeq uint64
	for {
		page, err := t.EncodedEventLog.SubjectRecordsAfterPage(ctx, subject, afterSeq, maxRecords, maxBytes)
		if err != nil {
			return nil, lastSeq, err
		}
		for _, record := range page.Records {
			event, err := t.decode(record.Data)
			if err != nil {
				return nil, 0, fmt.Errorf("decode record at seq %d: %w", record.Sequence, err)
			}
			decodedEvents = append(decodedEvents, TypedSubjectRecord[E]{Subject: record.Subject, Sequence: record.Sequence, Event: event})
		}
		if page.LastSequence > lastSeq {
			lastSeq = page.LastSequence
		}
		if !page.More || len(page.Records) == 0 {
			break
		}
		afterSeq = page.LastSequence
	}
	return decodedEvents, lastSeq, nil
}
