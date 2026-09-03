package evtstream

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

type encodedReaderStub struct {
	records map[uint64]events.EncodedSubjectRecord
	err     error
}

func (s *encodedReaderStub) Message(_ context.Context, sequence uint64) (events.EncodedSubjectRecord, error) {
	if s.err != nil {
		return events.EncodedSubjectRecord{}, s.err
	}
	record, ok := s.records[sequence]
	if !ok {
		return events.EncodedSubjectRecord{}, jetstream.ErrMsgNotFound
	}
	return record, nil
}

func (s *encodedReaderStub) Messages(ctx context.Context, sequences []uint64) ([]events.EncodedSubjectRecord, error) {
	result := make([]events.EncodedSubjectRecord, len(sequences))
	for i, sequence := range sequences {
		record, err := s.Message(ctx, sequence)
		if err != nil {
			return nil, err
		}
		result[i] = record
	}
	return result, nil
}

func encodedReaderEvent(t *testing.T, id string) []byte {
	t.Helper()
	data, err := proto.Marshal(&evtv1.Event{
		Id: id,
		Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{
			RoomId: "room",
		}},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return data
}

func TestReaderEventsAtDecodesAndPreservesOrder(t *testing.T) {
	reader := &Reader{reader: &encodedReaderStub{records: map[uint64]events.EncodedSubjectRecord{
		4: {Subject: "evt.room.room.user_joined", Sequence: 4, Data: encodedReaderEvent(t, "four")},
		9: {Subject: "evt.room.room.user_joined", Sequence: 9, Data: encodedReaderEvent(t, "nine")},
	}}}

	records, err := reader.EventsAt(context.Background(), []uint64{9, 4, 9})
	if err != nil {
		t.Fatalf("EventsAt: %v", err)
	}
	if got := []string{records[0].Event.GetId(), records[1].Event.GetId(), records[2].Event.GetId()}; got[0] != "nine" || got[1] != "four" || got[2] != "nine" {
		t.Fatalf("event order = %v, want [nine four nine]", got)
	}
}

func TestReaderEventAtRejectsCorruptEvent(t *testing.T) {
	reader := &Reader{reader: &encodedReaderStub{records: map[uint64]events.EncodedSubjectRecord{
		7: {Subject: "evt.room.room.user_joined", Sequence: 7, Data: []byte{0xff}},
	}}}
	if _, err := reader.EventAt(context.Background(), 7); err == nil {
		t.Fatal("EventAt error = nil, want corrupt protobuf error")
	}
}

func TestReaderEventAtRejectsMissingRecord(t *testing.T) {
	reader := &Reader{reader: &encodedReaderStub{records: make(map[uint64]events.EncodedSubjectRecord)}}
	if _, err := reader.EventAt(context.Background(), 7); !errors.Is(err, jetstream.ErrMsgNotFound) {
		t.Fatalf("EventAt error = %v, want jetstream.ErrMsgNotFound", err)
	}
}

func TestReaderEventAtRejectsInvalidEvent(t *testing.T) {
	data, err := proto.Marshal(&evtv1.Event{Id: "event"})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	reader := &Reader{reader: &encodedReaderStub{records: map[uint64]events.EncodedSubjectRecord{
		7: {Subject: "evt.room.room.user_joined", Sequence: 7, Data: data},
	}}}
	if _, err := reader.EventAt(context.Background(), 7); err == nil {
		t.Fatal("EventAt error = nil, want invalid event error")
	}
}

func TestReaderEventsAtPropagatesReadFailure(t *testing.T) {
	reader := &Reader{reader: &encodedReaderStub{err: context.Canceled}}
	if _, err := reader.EventsAt(context.Background(), []uint64{1, 2}); !errors.Is(err, context.Canceled) {
		t.Fatalf("EventsAt error = %v, want context.Canceled", err)
	}
}
