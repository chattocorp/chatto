package core

import (
	"testing"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type testProjection interface {
	Apply(*corev1.Event, uint64) error
}

// applyAll feeds events into a projection in order with seq starting at 1.
func applyAll(t *testing.T, p testProjection, events []*corev1.Event) {
	t.Helper()
	for i, e := range events {
		if err := p.Apply(e, uint64(i+1)); err != nil {
			t.Fatalf("Apply event %d: %v", i+1, err)
		}
	}
}

func assertApplyDoesNotMutateEvent(t *testing.T, p testProjection, event *corev1.Event, seq uint64) {
	t.Helper()
	before := proto.Clone(event).(*corev1.Event)
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
		out[i] = e.Event.GetId()
	}
	return out
}
