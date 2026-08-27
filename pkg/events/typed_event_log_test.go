package events_test

import (
	"bytes"
	"errors"
	"testing"

	. "hmans.de/chatto/pkg/events"
)

// typedTestEvent is a minimal application event for exercising the typed
// adapter with a non-protobuf envelope.
type typedTestEvent struct {
	ID      string
	Payload []byte
}

func encodeTypedTestEvent(event typedTestEvent) (EncodedRecord, error) {
	if event.ID == "" {
		return EncodedRecord{}, errors.New("id required")
	}
	return EncodedRecord{ID: event.ID, Data: bytes.Clone(event.Payload)}, nil
}

func decodeTypedTestEvent(data []byte) (typedTestEvent, error) {
	return typedTestEvent{ID: "decoded", Payload: bytes.Clone(data)}, nil
}

func TestTypedEventLogEncodesAndDecodesThroughEncodedLog(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	subject := "evt.typed.record.created"

	eventLog := NewEncodedEventLog(js, stream, testLogger())
	typed := NewTypedEventLog(
		eventLog,
		encodeTypedTestEvent,
		func(data []byte) (typedTestEvent, error) {
			return typedTestEvent{ID: "typed-1", Payload: bytes.Clone(data)}, nil
		},
	)

	seq, err := typed.AppendAt(ctx, subject, typedTestEvent{ID: "typed-1", Payload: []byte{0x01, 0x02}}, 0)
	if err != nil {
		t.Fatalf("AppendAt: %v", err)
	}

	records, lastSeq, err := typed.SubjectEventsWithSubjectsAfter(ctx, subject, 0, 100, 0)
	if err != nil {
		t.Fatalf("SubjectEventsWithSubjectsAfter: %v", err)
	}
	if len(records) != 1 || lastSeq != seq {
		t.Fatalf("records=%d lastSeq=%d, want 1 and %d", len(records), lastSeq, seq)
	}
	if records[0].Subject != subject || records[0].Sequence != seq || records[0].Event.ID != "typed-1" || !bytes.Equal(records[0].Event.Payload, []byte{0x01, 0x02}) {
		t.Fatalf("record = %+v", records[0])
	}
}

var errBadTypedEvent = errors.New("typed encoder rejects empty payload")

func TestTypedEventLogWrapsBatchEntryEncodeErrors(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	rejectingEncoder := func(event typedTestEvent) (EncodedRecord, error) {
		if len(event.Payload) == 0 {
			return EncodedRecord{}, errBadTypedEvent
		}
		return encodeTypedTestEvent(event)
	}
	typed := NewTypedEventLog(NewEncodedEventLog(js, stream, testLogger()), rejectingEncoder, decodeTypedTestEvent)

	_, err := typed.AppendBatch(ctx, []TypedBatchEntry[typedTestEvent]{
		{Subject: "evt.typed.batch.first", Event: typedTestEvent{ID: "first", Payload: []byte{0x00}}, HasOCC: true},
		{Subject: "evt.typed.batch.second", Event: typedTestEvent{ID: "second"}},
	})
	if !errors.Is(err, errBadTypedEvent) {
		t.Fatalf("AppendBatch error = %v, want wrapped errBadTypedEvent", err)
	}
}

func TestTypedEventLogRequiresDecoderForSubjectReads(t *testing.T) {
	js, stream := setupTestStream(t)
	ctx := testContext(t)
	typed := NewTypedEventLog[typedTestEvent](NewEncodedEventLog(js, stream, testLogger()), encodeTypedTestEvent, nil)

	if _, _, err := typed.SubjectEventsWithSubjectsAfter(ctx, "evt.typed.none", 0, 10, 0); !errors.Is(err, ErrNoTypedDecoder) {
		t.Fatalf("read without decoder = %v, want ErrNoTypedDecoder", err)
	}
}
