package evtstream

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

type exactReaderStub struct {
	mu      sync.Mutex
	reads   map[uint64]int
	msgs    map[uint64]*jetstream.RawStreamMsg
	err     error
	started chan struct{}
}

type boundedExactReaderStub struct {
	mu      sync.Mutex
	active  int
	maximum int
	started chan struct{}
	release chan struct{}
	data    []byte
}

func (s *boundedExactReaderStub) GetMsg(ctx context.Context, sequence uint64, _ ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error) {
	s.mu.Lock()
	s.active++
	s.maximum = max(s.maximum, s.active)
	s.mu.Unlock()
	s.started <- struct{}{}
	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	s.mu.Lock()
	s.active--
	s.mu.Unlock()
	return &jetstream.RawStreamMsg{Subject: "evt.room.room.user_joined", Sequence: sequence, Data: s.data}, nil
}

func (s *exactReaderStub) GetMsg(ctx context.Context, sequence uint64, _ ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error) {
	s.mu.Lock()
	s.reads[sequence]++
	msg := s.msgs[sequence]
	err := s.err
	s.mu.Unlock()
	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, jetstream.ErrMsgNotFound
	}
	return msg, nil
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

func TestReaderEventsAtDeduplicatesAndPreservesOrder(t *testing.T) {
	stub := &exactReaderStub{
		reads: make(map[uint64]int),
		msgs: map[uint64]*jetstream.RawStreamMsg{
			4: {Subject: "evt.room.room.user_joined", Sequence: 4, Data: encodedReaderEvent(t, "four")},
			9: {Subject: "evt.room.room.user_joined", Sequence: 9, Data: encodedReaderEvent(t, "nine")},
		},
	}
	reader := &Reader{stream: stub}

	records, err := reader.EventsAt(context.Background(), []uint64{9, 4, 9})
	if err != nil {
		t.Fatalf("EventsAt: %v", err)
	}
	if got := []string{records[0].Event.GetId(), records[1].Event.GetId(), records[2].Event.GetId()}; got[0] != "nine" || got[1] != "four" || got[2] != "nine" {
		t.Fatalf("event order = %v, want [nine four nine]", got)
	}
	if stub.reads[9] != 1 || stub.reads[4] != 1 {
		t.Fatalf("reads = %v, want one read per unique sequence", stub.reads)
	}
}

func TestReaderEventAtRejectsCorruptEvent(t *testing.T) {
	reader := &Reader{stream: &exactReaderStub{
		reads: make(map[uint64]int),
		msgs:  map[uint64]*jetstream.RawStreamMsg{7: {Subject: "evt.room.room.user_joined", Sequence: 7, Data: []byte{0xff}}},
	}}
	if _, err := reader.EventAt(context.Background(), 7); err == nil {
		t.Fatal("EventAt error = nil, want corrupt protobuf error")
	}
}

func TestReaderEventAtRejectsMissingRecord(t *testing.T) {
	reader := &Reader{stream: &exactReaderStub{
		reads: make(map[uint64]int),
		msgs:  make(map[uint64]*jetstream.RawStreamMsg),
	}}
	if _, err := reader.EventAt(context.Background(), 7); !errors.Is(err, jetstream.ErrMsgNotFound) {
		t.Fatalf("EventAt error = %v, want jetstream.ErrMsgNotFound", err)
	}
}

func TestReaderEventAtRejectsInvalidEvent(t *testing.T) {
	data, err := proto.Marshal(&evtv1.Event{Id: "event"})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	reader := &Reader{stream: &exactReaderStub{
		reads: make(map[uint64]int),
		msgs:  map[uint64]*jetstream.RawStreamMsg{7: {Subject: "evt.room.room.user_joined", Sequence: 7, Data: data}},
	}}
	if _, err := reader.EventAt(context.Background(), 7); err == nil {
		t.Fatal("EventAt error = nil, want invalid event error")
	}
}

func TestReaderEventAtRejectsUnexpectedBrokerSequence(t *testing.T) {
	reader := &Reader{stream: &exactReaderStub{
		reads: make(map[uint64]int),
		msgs: map[uint64]*jetstream.RawStreamMsg{
			7: {Subject: "evt.room.room.user_joined", Sequence: 8, Data: encodedReaderEvent(t, "event")},
		},
	}}
	if _, err := reader.EventAt(context.Background(), 7); err == nil {
		t.Fatal("EventAt error = nil, want unexpected sequence error")
	}
}

func TestReaderEventsAtHonorsCancellation(t *testing.T) {
	started := make(chan struct{}, 1)
	reader := &Reader{stream: &exactReaderStub{reads: make(map[uint64]int), started: started}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := reader.EventsAt(ctx, []uint64{1, 2})
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("EventsAt error = %v, want context.Canceled", err)
	}
}

func TestReaderEventsAtBoundsConcurrentReads(t *testing.T) {
	stub := &boundedExactReaderStub{
		started: make(chan struct{}, exactReadConcurrency+1),
		release: make(chan struct{}),
		data:    encodedReaderEvent(t, "event"),
	}
	reader := &Reader{stream: stub}
	sequences := make([]uint64, exactReadConcurrency*2)
	for i := range sequences {
		sequences[i] = uint64(i + 1)
	}
	done := make(chan error, 1)
	go func() {
		_, err := reader.EventsAt(context.Background(), sequences)
		done <- err
	}()
	for range exactReadConcurrency {
		<-stub.started
	}
	select {
	case <-stub.started:
		t.Fatal("more than the configured number of reads started concurrently")
	default:
	}
	close(stub.release)
	if err := <-done; err != nil {
		t.Fatalf("EventsAt: %v", err)
	}
	stub.mu.Lock()
	maximum := stub.maximum
	stub.mu.Unlock()
	if maximum != exactReadConcurrency {
		t.Fatalf("maximum concurrent reads = %d, want %d", maximum, exactReadConcurrency)
	}
}
