package evtstream_test

import (
	"bytes"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	. "hmans.de/chatto/internal/evtstream"
	. "hmans.de/chatto/pkg/events"
)

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
