package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const maxRBACMutationRetries = 5

var errRBACNoop = errors.New("rbac mutation is a no-op")

func rbacPermissionGrantedEvent(scope PermissionScope, scopeID, subject string, perm Permission) *corev1.RbacPermissionGrantedEvent {
	return &corev1.RbacPermissionGrantedEvent{
		Location:   rbacPermissionLocation(scope, scopeID),
		Subject:    subject,
		Permission: string(perm),
	}
}

func rbacPermissionDeniedEvent(scope PermissionScope, scopeID, subject string, perm Permission) *corev1.RbacPermissionDeniedEvent {
	return &corev1.RbacPermissionDeniedEvent{
		Location:   rbacPermissionLocation(scope, scopeID),
		Subject:    subject,
		Permission: string(perm),
	}
}

func rbacPermissionClearedEvent(scope PermissionScope, scopeID, subject string, perm Permission) *corev1.RbacPermissionClearedEvent {
	return &corev1.RbacPermissionClearedEvent{
		Location:   rbacPermissionLocation(scope, scopeID),
		Subject:    subject,
		Permission: string(perm),
	}
}

func rbacPermissionLocation(scope PermissionScope, scopeID string) string {
	if scope == ScopeServer {
		return string(ScopeServer)
	}
	return scopeID
}

func rbacSubjectForEvent(event *corev1.Event) string {
	return rbacAggregateForEvent(event).SubjectFor(event)
}

func rbacAggregateForEvent(event *corev1.Event) events.Aggregate {
	if event == nil {
		return events.RBACServerAggregate()
	}
	switch e := event.GetEvent().(type) {
	case *corev1.Event_RbacPermissionGranted:
		return rbacAggregateForPermissionLocation(e.RbacPermissionGranted.GetLocation())
	case *corev1.Event_RbacPermissionDenied:
		return rbacAggregateForPermissionLocation(e.RbacPermissionDenied.GetLocation())
	case *corev1.Event_RbacPermissionCleared:
		return rbacAggregateForPermissionLocation(e.RbacPermissionCleared.GetLocation())
	default:
		return events.RBACServerAggregate()
	}
}

func rbacAggregateForPermissionLocation(location string) events.Aggregate {
	if location == "" || location == string(ScopeServer) {
		return events.RBACServerAggregate()
	}
	return events.RBACScopedAggregate(location)
}

func (c *ChattoCore) appendRBACEvent(ctx context.Context, event *corev1.Event, check func() error) (uint64, error) {
	filter := events.RBACSubjectFilter()

	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read RBAC OCC filter seq: %w", err)
		}
		if err := c.RBACProjector.WaitForSeq(ctx, filterSeq); err != nil {
			return 0, fmt.Errorf("wait for RBAC projection: %w", err)
		}
		if check != nil {
			if err := check(); err != nil {
				return 0, err
			}
		}
		subject := rbacSubjectForEvent(event)

		seq, err := c.EventPublisher.AppendAtFilter(ctx, subject, event, filter, filterSeq)
		if err == nil {
			if err := c.RBACProjector.WaitForSeq(ctx, seq); err != nil {
				return 0, fmt.Errorf("wait for RBAC projection: %w", err)
			}
			return seq, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return 0, err
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return 0, fmt.Errorf("RBAC OCC retry exhausted after %d attempts: %w", maxRBACMutationRetries, events.ErrConflict)
}

func (c *ChattoCore) appendRBACBatch(ctx context.Context, entries []events.BatchEntry, check func() error) (uint64, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	filter := events.RBACSubjectFilter()

	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read RBAC OCC filter seq: %w", err)
		}
		if err := c.RBACProjector.WaitForSeq(ctx, filterSeq); err != nil {
			return 0, fmt.Errorf("wait for RBAC projection: %w", err)
		}
		if check != nil {
			if err := check(); err != nil {
				return 0, err
			}
		}

		chunk := append([]events.BatchEntry(nil), entries...)
		chunk[0].HasOCC = true
		chunk[0].ExpectedSeq = filterSeq
		chunk[0].FilterSubject = filter

		seqs, err := c.EventPublisher.AppendBatch(ctx, chunk)
		if err == nil {
			lastSeq := seqs[len(seqs)-1]
			if err := c.RBACProjector.WaitForSeq(ctx, lastSeq); err != nil {
				return 0, fmt.Errorf("wait for RBAC projection: %w", err)
			}
			return lastSeq, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return 0, err
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return 0, fmt.Errorf("RBAC batch OCC retry exhausted after %d attempts: %w", maxRBACMutationRetries, events.ErrConflict)
}
