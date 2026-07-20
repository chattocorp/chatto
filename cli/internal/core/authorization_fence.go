package core

import (
	"context"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// authorizationFenceEvent is an operational durability fact: it advances the
// narrow OCC lane shared by authorization-changing writes and mutations whose
// authorization decision must remain valid until commit. Policy state stays in
// its owning events and projections.
func authorizationFenceEvent(actorID string) *corev1.Event {
	return newEvent(actorID, &corev1.Event{Event: &corev1.Event_AuthorizationFenceAdvanced{
		AuthorizationFenceAdvanced: &corev1.AuthorizationFenceAdvancedEvent{},
	}})
}

func (c *ChattoCore) authorizationFenceSeq(ctx context.Context) (uint64, error) {
	return c.EventPublisher.LastSubjectSeq(ctx, events.AuthorizationSubjectFilter())
}

// appendAuthorizationFencedBatch atomically commits the supplied domain facts
// and advances the authorization fence. Callers put their normal domain OCC on
// one of entries; the final fence entry independently verifies that no
// authorization-changing write committed since expectedAuthorizationSeq.
func (c *ChattoCore) appendAuthorizationFencedBatch(
	ctx context.Context,
	actorID string,
	entries []events.BatchEntry,
	expectedAuthorizationSeq uint64,
) ([]uint64, error) {
	chunk := append([]events.BatchEntry(nil), entries...)
	fence := authorizationFenceEvent(actorID)
	chunk = append(chunk, events.BatchEntry{
		Subject:       events.AuthorizationAggregate().SubjectFor(fence),
		Event:         fence,
		HasOCC:        true,
		ExpectedSeq:   expectedAuthorizationSeq,
		FilterSubject: events.AuthorizationSubjectFilter(),
	})
	return c.EventPublisher.AppendBatch(ctx, chunk)
}
