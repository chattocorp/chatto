package events

import (
	"bytes"
	"errors"
	"strconv"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
)

func TestEncodedEventLogPreservesOpaqueRecord(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	subject := "evt.compatibility.record.created"
	data := []byte{0x00, 0xff, 0x10, 0x80, 0x01}

	seq, err := eventLog.AppendAt(ctx, subject, EncodedRecord{ID: "opaque-1", Data: data}, 0)
	if err != nil {
		t.Fatalf("AppendAt: %v", err)
	}
	stored, err := stream.GetMsg(ctx, seq)
	if err != nil {
		t.Fatalf("GetMsg: %v", err)
	}
	if !bytes.Equal(stored.Data, data) {
		t.Fatalf("stored data = %x, want %x", stored.Data, data)
	}
	if got := stored.Header.Get(jetstream.MsgIDHeader); got != "opaque-1" {
		t.Fatalf("Nats-Msg-Id = %q, want opaque-1", got)
	}

	records, lastSeq, err := eventLog.SubjectRecordsAfter(ctx, subject, 0)
	if err != nil {
		t.Fatalf("SubjectRecordsAfter: %v", err)
	}
	if len(records) != 1 || lastSeq != seq {
		t.Fatalf("records=%d lastSeq=%d, want 1 and %d", len(records), lastSeq, seq)
	}
	if records[0].Subject != subject || records[0].Sequence != seq || records[0].ID != "opaque-1" || !bytes.Equal(records[0].Data, data) {
		t.Fatalf("record = %+v, want subject=%q sequence=%d data=%x", records[0], subject, seq, data)
	}
	records[0].Data[0] ^= 0xff
	storedAgain, err := stream.GetMsg(ctx, seq)
	if err != nil {
		t.Fatalf("GetMsg after caller mutation: %v", err)
	}
	if !bytes.Equal(storedAgain.Data, data) {
		t.Fatal("mutating returned record changed durable data")
	}
}

func TestPublisherStoresCanonicalCoreEventBytes(t *testing.T) {
	js, stream := setupTestStream(t)
	publisher := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	event := makeEvent("R-compatible", "U1")
	want, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	seq, err := publisher.AppendAt(
		ctx,
		RoomAggregate("R-compatible").Subject(EventUserJoinedRoom),
		event,
		0,
	)
	if err != nil {
		t.Fatalf("AppendAt: %v", err)
	}
	stored, err := stream.GetMsg(ctx, seq)
	if err != nil {
		t.Fatalf("GetMsg: %v", err)
	}
	if !bytes.Equal(stored.Data, want) {
		t.Fatalf("stored protobuf bytes changed:\n got %x\nwant %x", stored.Data, want)
	}
	if got := stored.Header.Get(jetstream.MsgIDHeader); got != event.GetId() {
		t.Fatalf("Nats-Msg-Id = %q, want %q", got, event.GetId())
	}
	if got := stored.Header.Get(jetstream.ExpectedLastSubjSeqHeader); got != "0" {
		t.Fatalf("expected-last-subject-sequence = %q, want 0", got)
	}
}

func TestPublisherReadsExistingCoreEventBytes(t *testing.T) {
	js, stream := setupTestStream(t)
	publisher := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	subject := RoomAggregate("R-existing").Subject(EventUserJoinedRoom)
	event := makeEvent("R-existing", "U1")
	data, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	ack, err := js.Publish(
		ctx,
		subject,
		data,
		jetstream.WithExpectLastSequencePerSubject(0),
		jetstream.WithMsgID(event.GetId()),
	)
	if err != nil {
		t.Fatalf("seed existing EVT record: %v", err)
	}

	events, lastSeq, err := publisher.SubjectEvents(ctx, subject)
	if err != nil {
		t.Fatalf("SubjectEvents: %v", err)
	}
	if len(events) != 1 || lastSeq != ack.Sequence || !proto.Equal(events[0], event) {
		t.Fatalf("decoded events=%v lastSeq=%d, want existing event at %d", events, lastSeq, ack.Sequence)
	}
}

func TestEncodedEventLogWriteRemainsReadableByChattoPublisher(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	publisher := NewPublisher(js, stream, testLogger())
	ctx := testContext(t)
	subject := RoomAggregate("R-rollback").Subject(EventUserJoinedRoom)
	event := makeEvent("R-rollback", "U1")
	data, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	seq, err := eventLog.AppendAt(ctx, subject, EncodedRecord{ID: event.GetId(), Data: data}, 0)
	if err != nil {
		t.Fatalf("AppendAt: %v", err)
	}
	events, lastSeq, err := publisher.SubjectEvents(ctx, subject)
	if err != nil {
		t.Fatalf("SubjectEvents: %v", err)
	}
	if len(events) != 1 || lastSeq != seq || !proto.Equal(events[0], event) {
		t.Fatalf("decoded events=%v lastSeq=%d, want original event at %d", events, lastSeq, seq)
	}
}

func TestEncodedEventLogAppendAtFilterUsesWildcardTail(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	filter := "evt.compatibility.*"

	firstSeq, err := eventLog.AppendAtFilter(
		ctx,
		"evt.compatibility.first",
		EncodedRecord{ID: "first", Data: []byte("first")},
		filter,
		0,
	)
	if err != nil {
		t.Fatalf("first AppendAtFilter: %v", err)
	}
	if _, err := eventLog.AppendAtFilter(
		ctx,
		"evt.compatibility.second",
		EncodedRecord{ID: "second", Data: []byte("second")},
		filter,
		0,
	); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale wildcard append error = %v, want ErrConflict", err)
	}
	secondSeq, err := eventLog.AppendAtFilter(
		ctx,
		"evt.compatibility.second",
		EncodedRecord{ID: "second", Data: []byte("second")},
		filter,
		firstSeq,
	)
	if err != nil {
		t.Fatalf("current wildcard AppendAtFilter: %v", err)
	}
	stored, err := stream.GetMsg(ctx, secondSeq)
	if err != nil {
		t.Fatalf("GetMsg: %v", err)
	}
	if got, want := stored.Header.Get(jetstream.ExpectedLastSubjSeqHeader), strconv.FormatUint(firstSeq, 10); got != want {
		t.Fatalf("expected-last-subject-sequence = %q, want %q", got, want)
	}
	if got := stored.Header.Get(jetstream.ExpectedLastSubjSeqSubjHeader); got != filter {
		t.Fatalf("expected-last-subject filter = %q, want %q", got, filter)
	}
}

func TestEncodedEventLogAtomicBatchPreservesBytesAndOrder(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)
	entries := []EncodedBatchEntry{
		{
			Subject:     "evt.compatibility.batch.first",
			Record:      EncodedRecord{ID: "batch-first", Data: []byte{0x00, 0x01}},
			HasOCC:      true,
			ExpectedSeq: 0,
		},
		{
			Subject: "evt.compatibility.batch.second",
			Record:  EncodedRecord{ID: "batch-second", Data: []byte{0xfe, 0xff}},
		},
	}

	seqs, err := eventLog.AppendBatch(ctx, entries)
	if err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}
	if len(seqs) != len(entries) || seqs[1] != seqs[0]+1 {
		t.Fatalf("batch sequences = %v, want two contiguous entries", seqs)
	}
	for i, seq := range seqs {
		stored, err := stream.GetMsg(ctx, seq)
		if err != nil {
			t.Fatalf("GetMsg(%d): %v", seq, err)
		}
		if stored.Subject != entries[i].Subject || !bytes.Equal(stored.Data, entries[i].Record.Data) {
			t.Fatalf("stored batch entry %d = subject %q data %x", i, stored.Subject, stored.Data)
		}
		if got := stored.Header.Get(jetstream.MsgIDHeader); got != entries[i].Record.ID {
			t.Fatalf("entry %d Nats-Msg-Id = %q, want %q", i, got, entries[i].Record.ID)
		}
	}
}

func TestEncodedEventLogRejectsMissingRecordIDAndUnguardedBatch(t *testing.T) {
	js, stream := setupTestStream(t)
	eventLog := NewEncodedEventLog(js, stream, testLogger())
	ctx := testContext(t)

	if _, err := eventLog.AppendAt(ctx, "evt.compatibility.invalid", EncodedRecord{Data: []byte("data")}, 0); !errors.Is(err, ErrInvalidEncodedRecord) {
		t.Fatalf("missing ID error = %v, want ErrInvalidEncodedRecord", err)
	}
	if _, err := eventLog.AppendBatch(ctx, []EncodedBatchEntry{{
		Subject: "evt.compatibility.unguarded",
		Record:  EncodedRecord{ID: "unguarded", Data: []byte("data")},
	}}); !errors.Is(err, ErrMissingOCC) {
		t.Fatalf("unguarded batch error = %v, want ErrMissingOCC", err)
	}
}
