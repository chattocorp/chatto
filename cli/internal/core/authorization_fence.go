package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
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

// prepareAuthorizationFencedMutation captures the authorization fence. When a
// check is supplied, it waits for the RBAC projection used by the decision and
// then repeats that check. The caller must use the returned sequence in
// appendAuthorizationFencedBatch so a concurrent authorization change rejects
// the complete domain batch.
func (c *ChattoCore) prepareAuthorizationFencedMutation(ctx context.Context, check func() error) (uint64, error) {
	authorizationSeq, err := c.authorizationFenceSeq(ctx)
	if err != nil {
		return 0, fmt.Errorf("read authorization fence seq: %w", err)
	}
	if check == nil {
		return authorizationSeq, nil
	}
	rbacSeq, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.RBACSubjectFilter())
	if err != nil {
		return 0, fmt.Errorf("read RBAC OCC filter seq: %w", err)
	}
	if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(evtstream.RBACSubjectFilter(), rbacSeq)); err != nil {
		return 0, fmt.Errorf("wait for RBAC projection: %w", err)
	}
	if err := check(); err != nil {
		return 0, err
	}
	return authorizationSeq, nil
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
