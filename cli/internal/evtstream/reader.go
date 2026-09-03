package evtstream

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/pkg/events"
)

type encodedMessageReader interface {
	Message(context.Context, uint64) (events.EncodedSubjectRecord, error)
	Messages(context.Context, []uint64) ([]events.EncodedSubjectRecord, error)
}

// Reader loads complete Chatto events at exact EVT stream sequences. It is a
// read boundary for derived indexes that retain sequences instead of event
// payloads.
type Reader struct {
	messages *events.StreamMessageReader
	reader   encodedMessageReader
}

// NewReader constructs an exact-sequence reader for the EVT stream. Successful
// reads remain in a process-local cache until CacheIdleTTL elapses without an
// access.
func NewReader(stream jetstream.Stream, config events.StreamMessageReaderConfig) (*Reader, error) {
	messages, err := events.NewStreamMessageReader(stream, config)
	if err != nil {
		return nil, err
	}
	return &Reader{messages: messages, reader: messages}, nil
}

// Run maintains the process-local stream-message cache until ctx is canceled.
func (r *Reader) Run(ctx context.Context) error {
	if r == nil || r.messages == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	return r.messages.Run(ctx)
}

// Forget removes exact EVT sequences from the process-local cache.
func (r *Reader) Forget(sequences ...uint64) {
	if r != nil && r.messages != nil {
		r.messages.Forget(sequences...)
	}
}

// Clear removes all EVT records from the process-local cache.
func (r *Reader) Clear() {
	if r != nil && r.messages != nil {
		r.messages.Clear()
	}
}

// EventAt loads and decodes one EVT record through the configured exact stream
// read. The returned subject and sequence are authoritative broker metadata.
func (r *Reader) EventAt(ctx context.Context, sequence uint64) (*SubjectEvent, error) {
	if r == nil || r.reader == nil {
		return nil, fmt.Errorf("EVT reader is unavailable")
	}
	record, err := r.reader.Message(ctx, sequence)
	if err != nil {
		return nil, fmt.Errorf("read EVT sequence %d: %w", sequence, err)
	}
	return decodeSubjectEvent(record)
}

// EventsAt loads exact EVT sequences with bounded concurrency. Duplicate
// sequences are read once, while the returned slice preserves caller order and
// duplicate positions. The first failure cancels outstanding reads.
func (r *Reader) EventsAt(ctx context.Context, sequences []uint64) ([]*SubjectEvent, error) {
	if r == nil || r.reader == nil {
		return nil, fmt.Errorf("EVT reader is unavailable")
	}
	records, err := r.reader.Messages(ctx, sequences)
	if err != nil {
		return nil, fmt.Errorf("read EVT sequences: %w", err)
	}
	result := make([]*SubjectEvent, len(records))
	for i, record := range records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		event, err := decodeSubjectEvent(record)
		if err != nil {
			return nil, err
		}
		result[i] = event
	}
	return result, nil
}

func decodeSubjectEvent(record events.EncodedSubjectRecord) (*SubjectEvent, error) {
	event, err := decodeEventData(record.Data)
	if err != nil {
		return nil, fmt.Errorf("decode EVT sequence %d: %w", record.Sequence, err)
	}
	if err := validateEvent(event); err != nil {
		return nil, fmt.Errorf("validate EVT sequence %d: %w", record.Sequence, err)
	}
	return &SubjectEvent{Subject: record.Subject, Sequence: record.Sequence, Event: event}, nil
}
