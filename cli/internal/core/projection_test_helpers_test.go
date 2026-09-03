package core

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

type testProjection interface {
	Apply(*evtv1.Event, uint64) error
}

// applyAll feeds events into a projection in order with seq starting at 1.
func applyAll(t *testing.T, p testProjection, events []*evtv1.Event) {
	t.Helper()
	for i, e := range events {
		if err := p.Apply(e, uint64(i+1)); err != nil {
			t.Fatalf("Apply event %d: %v", i+1, err)
		}
	}
}

func assertApplyDoesNotMutateEvent(t *testing.T, p testProjection, event *evtv1.Event, seq uint64) {
	t.Helper()
	before := proto.Clone(event).(*evtv1.Event)
	if err := p.Apply(event, seq); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !proto.Equal(event, before) {
		t.Fatalf("Apply mutated input event\nafter:  %v\nbefore: %v", event, before)
	}
}

func timelineEventIDs(entries []*TimelineEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.EventID
	}
	return out
}

func eventIDOrEmpty(entry *TimelineEntry) string {
	if entry == nil {
		return ""
	}
	return entry.EventID
}

type timelineEventReaderStub struct {
	mu      sync.Mutex
	records map[uint64]*evtstream.SubjectEvent
	reads   [][]uint64
	hook    func()
}

type recordingTimelineEventReader struct {
	mu       sync.Mutex
	delegate timelineEventReader
	reads    [][]uint64
}

func (r *recordingTimelineEventReader) EventsAt(ctx context.Context, sequences []uint64) ([]*evtstream.SubjectEvent, error) {
	r.mu.Lock()
	r.reads = append(r.reads, append([]uint64(nil), sequences...))
	r.mu.Unlock()
	return r.delegate.EventsAt(ctx, sequences)
}

func (s *timelineEventReaderStub) EventsAt(ctx context.Context, sequences []uint64) ([]*evtstream.SubjectEvent, error) {
	s.mu.Lock()
	s.reads = append(s.reads, append([]uint64(nil), sequences...))
	hook := s.hook
	s.hook = nil
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	result := make([]*evtstream.SubjectEvent, len(sequences))
	for i, sequence := range sequences {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		record := s.records[sequence]
		if record == nil {
			return nil, fmt.Errorf("missing test EVT sequence %d", sequence)
		}
		result[i] = record
	}
	return result, nil
}

func testTimelineEventReader(events []*evtv1.Event) *timelineEventReaderStub {
	reader := &timelineEventReaderStub{records: make(map[uint64]*evtstream.SubjectEvent, len(events))}
	for i, event := range events {
		sequence := uint64(i + 1)
		reader.records[sequence] = &evtstream.SubjectEvent{
			Subject:  evtstream.RoomAggregate(roomIDOfEvent(event)).Subject(evtstream.EventTypeOf(event)),
			Sequence: sequence,
			Event:    event,
		}
	}
	return reader
}
