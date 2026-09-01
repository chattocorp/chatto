package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const maxRBACMutationRetries = 5

var errRBACNoop = errors.New("rbac mutation is a no-op")

func rbacPermissionGrantedEvent(scope PermissionScope, scopeID string, subjectKind evtv1.RbacPermissionSubjectKind, subjectID string, perm Permission) *evtv1.RbacPermissionGrantedEvent {
	return &evtv1.RbacPermissionGrantedEvent{
		Scope:      rbacPermissionScope(scope, scopeID),
		Subject:    rbacPermissionSubject(subjectKind, subjectID),
		Permission: string(perm),
	}
}

func rbacPermissionDeniedEvent(scope PermissionScope, scopeID string, subjectKind evtv1.RbacPermissionSubjectKind, subjectID string, perm Permission) *evtv1.RbacPermissionDeniedEvent {
	return &evtv1.RbacPermissionDeniedEvent{
		Scope:      rbacPermissionScope(scope, scopeID),
		Subject:    rbacPermissionSubject(subjectKind, subjectID),
		Permission: string(perm),
	}
}

func rbacPermissionClearedEvent(scope PermissionScope, scopeID string, subjectKind evtv1.RbacPermissionSubjectKind, subjectID string, perm Permission) *evtv1.RbacPermissionClearedEvent {
	return &evtv1.RbacPermissionClearedEvent{
		Scope:      rbacPermissionScope(scope, scopeID),
		Subject:    rbacPermissionSubject(subjectKind, subjectID),
		Permission: string(perm),
	}
}

func rbacRolePermissionGrantedEvent(scope PermissionScope, scopeID, roleName string, perm Permission) *evtv1.RbacPermissionGrantedEvent {
	return rbacPermissionGrantedEvent(scope, scopeID, evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, roleName, perm)
}

func rbacRolePermissionDeniedEvent(scope PermissionScope, scopeID, roleName string, perm Permission) *evtv1.RbacPermissionDeniedEvent {
	return rbacPermissionDeniedEvent(scope, scopeID, evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, roleName, perm)
}

func rbacRolePermissionClearedEvent(scope PermissionScope, scopeID, roleName string, perm Permission) *evtv1.RbacPermissionClearedEvent {
	return rbacPermissionClearedEvent(scope, scopeID, evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, roleName, perm)
}

func rbacUserPermissionGrantedEvent(scope PermissionScope, scopeID, userID string, perm Permission) *evtv1.RbacPermissionGrantedEvent {
	return rbacPermissionGrantedEvent(scope, scopeID, evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER, userID, perm)
}

func rbacUserPermissionDeniedEvent(scope PermissionScope, scopeID, userID string, perm Permission) *evtv1.RbacPermissionDeniedEvent {
	return rbacPermissionDeniedEvent(scope, scopeID, evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER, userID, perm)
}

func rbacUserPermissionClearedEvent(scope PermissionScope, scopeID, userID string, perm Permission) *evtv1.RbacPermissionClearedEvent {
	return rbacPermissionClearedEvent(scope, scopeID, evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER, userID, perm)
}

func rbacPermissionScope(scope PermissionScope, scopeID string) *evtv1.RbacPermissionScope {
	kind := evtv1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_UNSPECIFIED
	switch scope {
	case ScopeServer:
		kind = evtv1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_SERVER
		scopeID = ""
	case ScopeGroup:
		kind = evtv1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_GROUP
	case ScopeRoom:
		kind = evtv1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_ROOM
	}
	return &evtv1.RbacPermissionScope{Kind: kind, Id: scopeID}
}

func rbacPermissionSubject(kind evtv1.RbacPermissionSubjectKind, id string) *evtv1.RbacPermissionSubject {
	return &evtv1.RbacPermissionSubject{Kind: kind, Id: id}
}

func rbacPermissionSubjectKindForID(subject string) evtv1.RbacPermissionSubjectKind {
	if IsUserSubject(subject) {
		return evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER
	}
	return evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE
}

func rbacSubjectForEvent(event *evtv1.Event) string {
	return rbacAggregateForEvent(event).SubjectFor(event)
}

func rbacAggregateForEvent(event *evtv1.Event) evtstream.Aggregate {
	if event == nil {
		return evtstream.RBACServerAggregate()
	}
	switch e := event.GetEvent().(type) {
	case *evtv1.Event_RbacPermissionGranted:
		return rbacAggregateForPermissionScope(e.RbacPermissionGranted.GetScope())
	case *evtv1.Event_RbacPermissionDenied:
		return rbacAggregateForPermissionScope(e.RbacPermissionDenied.GetScope())
	case *evtv1.Event_RbacPermissionCleared:
		return rbacAggregateForPermissionScope(e.RbacPermissionCleared.GetScope())
	default:
		return evtstream.RBACServerAggregate()
	}
}

func rbacAggregateForPermissionScope(scope *evtv1.RbacPermissionScope) evtstream.Aggregate {
	if scope == nil || scope.GetKind() == evtv1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_SERVER {
		return evtstream.RBACServerAggregate()
	}
	return evtstream.RBACScopedAggregate(scope.GetId())
}

func (c *ChattoCore) appendRBACEvent(ctx context.Context, event *evtv1.Event, check func() error) (uint64, error) {
	filter := evtstream.RBACSubjectFilter()

	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read RBAC OCC filter seq: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return 0, fmt.Errorf("wait for RBAC projection: %w", err)
		}
		if err := c.authorizeAtStableInputs(ctx, check); err != nil {
			return 0, err
		}
		subject := rbacSubjectForEvent(event)
		entries := []evtstream.BatchEntry{{
			Subject:       subject,
			Event:         event,
			HasOCC:        true,
			ExpectedSeq:   filterSeq,
			FilterSubject: filter,
		}}

		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			seq := seqs[0]
			if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(subject, seq)); err != nil {
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

// appendRoleAssignmentEvent waits for every projection used by role-assignment
// authorization and validates the cross-aggregate inputs before appending with
// RBAC OCC. Unrelated chat traffic does not affect the RBAC commit boundary.
func (c *ChattoCore) appendRoleAssignmentEvent(ctx context.Context, userID string, requireExistingUser bool, event *evtv1.Event, check func() error) (uint64, error) {
	filter := evtstream.RBACSubjectFilter()

	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		rbacSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read RBAC OCC filter seq: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(filter, rbacSeq)); err != nil {
			return 0, fmt.Errorf("wait for RBAC projection: %w", err)
		}

		if err := c.authorizeAtStableRoomInputs(ctx, func() error {
			if requireExistingUser {
				if _, err := c.GetUser(ctx, userID); err != nil {
					return err
				}
			}
			if check != nil {
				return check()
			}
			return nil
		}); err != nil {
			return 0, err
		}
		subject := rbacSubjectForEvent(event)
		entries := []evtstream.BatchEntry{{
			Subject:       subject,
			Event:         event,
			HasOCC:        true,
			ExpectedSeq:   rbacSeq,
			FilterSubject: filter,
		}}

		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			seq := seqs[0]
			if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(subject, seq)); err != nil {
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
	return 0, fmt.Errorf("role assignment OCC retry exhausted after %d attempts: %w", maxRBACMutationRetries, events.ErrConflict)
}

func (c *ChattoCore) appendRBACEventWithMentionableCheck(ctx context.Context, event *evtv1.Event, check func() error) (uint64, error) {
	filter := evtstream.EventSubjectFilter()

	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read mentionable OCC filter seq: %w", err)
		}
		if err := c.mentionables.waitFor(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return 0, fmt.Errorf("wait for mentionables projection: %w", err)
		}

		rbacSeq, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.RBACSubjectFilter())
		if err != nil {
			return 0, fmt.Errorf("read RBAC OCC filter seq: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(evtstream.RBACSubjectFilter(), rbacSeq)); err != nil {
			return 0, fmt.Errorf("wait for RBAC projection: %w", err)
		}

		if err := c.authorizeAtStableInputs(ctx, check); err != nil {
			return 0, err
		}
		subject := rbacSubjectForEvent(event)
		entries := []evtstream.BatchEntry{{
			Subject:       subject,
			Event:         event,
			HasOCC:        true,
			ExpectedSeq:   filterSeq,
			FilterSubject: filter,
		}}

		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			seq := seqs[0]
			if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(subject, seq)); err != nil {
				return 0, fmt.Errorf("wait for RBAC projection: %w", err)
			}
			if err := c.mentionables.waitFor(ctx, events.SubjectPosition(subject, seq)); err != nil {
				return 0, fmt.Errorf("wait for mentionables projection: %w", err)
			}
			return seq, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return 0, err
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(mentionableRetryDelay(attempt)):
		}
	}
	return 0, fmt.Errorf("mentionable RBAC OCC retry exhausted after %d attempts: %w", maxRBACMutationRetries, events.ErrConflict)
}

func (c *ChattoCore) appendRBACBatch(ctx context.Context, entries []evtstream.BatchEntry, check func() error) (uint64, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	filter := evtstream.RBACSubjectFilter()

	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read RBAC OCC filter seq: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return 0, fmt.Errorf("wait for RBAC projection: %w", err)
		}
		if err := c.authorizeAtStableInputs(ctx, check); err != nil {
			return 0, err
		}

		chunk := append([]evtstream.BatchEntry(nil), entries...)
		chunk[0].HasOCC = true
		chunk[0].ExpectedSeq = filterSeq
		chunk[0].FilterSubject = filter

		seqs, err := c.EventPublisher.AppendBatch(ctx, chunk)
		if err == nil {
			lastDomainIndex := len(chunk) - 1
			lastSeq := seqs[lastDomainIndex]
			lastSubject := chunk[lastDomainIndex].Subject
			if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
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
