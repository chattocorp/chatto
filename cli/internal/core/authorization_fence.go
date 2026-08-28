package core

import (
	"context"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// authorizationFenceEvent is an operational durability fact: it advances the
// narrow OCC lane shared by authorization-changing writes and mutations whose
// authorization decision must remain valid until commit. Policy state stays in
// its owning events and projections.
func authorizationFenceEvent(actorID string) *evtv1.Event {
	return newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_AuthorizationFenceAdvanced{
		AuthorizationFenceAdvanced: &evtv1.AuthorizationFenceAdvancedEvent{},
	}})
}

func (c *ChattoCore) authorizationFenceSeq(ctx context.Context) (uint64, error) {
	return c.EventPublisher.LastSubjectSeq(ctx, evtstream.AuthorizationSubjectFilter())
}

// appendAuthorizationFencedBatch atomically commits the supplied domain facts
// and advances the authorization fence. Callers put their normal domain OCC on
// one of entries; the final fence entry independently verifies that no
// authorization-changing write committed since expectedAuthorizationSeq.
func (c *ChattoCore) appendAuthorizationFencedBatch(
	ctx context.Context,
	actorID string,
	entries []evtstream.BatchEntry,
	expectedAuthorizationSeq uint64,
) ([]uint64, error) {
	chunk := append([]evtstream.BatchEntry(nil), entries...)
	fence := authorizationFenceEvent(actorID)
	chunk = append(chunk, evtstream.BatchEntry{
		Subject:       evtstream.AuthorizationAggregate().SubjectFor(fence),
		Event:         fence,
		HasOCC:        true,
		ExpectedSeq:   expectedAuthorizationSeq,
		FilterSubject: evtstream.AuthorizationSubjectFilter(),
	})
	return c.EventPublisher.AppendBatch(ctx, chunk)
}
