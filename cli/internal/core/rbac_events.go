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

func rbacPermissionGrantedEvent(scope PermissionScope, scopeID string, subjectKind corev1.RbacPermissionSubjectKind, subjectID string, perm Permission) *corev1.RbacPermissionGrantedEvent {
	return &corev1.RbacPermissionGrantedEvent{
		Scope:      rbacPermissionScope(scope, scopeID),
		Subject:    rbacPermissionSubject(subjectKind, subjectID),
		Permission: string(perm),
	}
}

func rbacPermissionDeniedEvent(scope PermissionScope, scopeID string, subjectKind corev1.RbacPermissionSubjectKind, subjectID string, perm Permission) *corev1.RbacPermissionDeniedEvent {
	return &corev1.RbacPermissionDeniedEvent{
		Scope:      rbacPermissionScope(scope, scopeID),
		Subject:    rbacPermissionSubject(subjectKind, subjectID),
		Permission: string(perm),
	}
}

func rbacPermissionClearedEvent(scope PermissionScope, scopeID string, subjectKind corev1.RbacPermissionSubjectKind, subjectID string, perm Permission) *corev1.RbacPermissionClearedEvent {
	return &corev1.RbacPermissionClearedEvent{
		Scope:      rbacPermissionScope(scope, scopeID),
		Subject:    rbacPermissionSubject(subjectKind, subjectID),
		Permission: string(perm),
	}
}

func rbacRolePermissionGrantedEvent(scope PermissionScope, scopeID, roleName string, perm Permission) *corev1.RbacPermissionGrantedEvent {
	return rbacPermissionGrantedEvent(scope, scopeID, corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, roleName, perm)
}

func rbacRolePermissionDeniedEvent(scope PermissionScope, scopeID, roleName string, perm Permission) *corev1.RbacPermissionDeniedEvent {
	return rbacPermissionDeniedEvent(scope, scopeID, corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, roleName, perm)
}

func rbacRolePermissionClearedEvent(scope PermissionScope, scopeID, roleName string, perm Permission) *corev1.RbacPermissionClearedEvent {
	return rbacPermissionClearedEvent(scope, scopeID, corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, roleName, perm)
}

func rbacUserPermissionGrantedEvent(scope PermissionScope, scopeID, userID string, perm Permission) *corev1.RbacPermissionGrantedEvent {
	return rbacPermissionGrantedEvent(scope, scopeID, corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER, userID, perm)
}

func rbacUserPermissionDeniedEvent(scope PermissionScope, scopeID, userID string, perm Permission) *corev1.RbacPermissionDeniedEvent {
	return rbacPermissionDeniedEvent(scope, scopeID, corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER, userID, perm)
}

func rbacUserPermissionClearedEvent(scope PermissionScope, scopeID, userID string, perm Permission) *corev1.RbacPermissionClearedEvent {
	return rbacPermissionClearedEvent(scope, scopeID, corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER, userID, perm)
}

func rbacPermissionScope(scope PermissionScope, scopeID string) *corev1.RbacPermissionScope {
	kind := corev1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_UNSPECIFIED
	switch scope {
	case ScopeServer:
		kind = corev1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_SERVER
		scopeID = ""
	case ScopeGroup:
		kind = corev1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_GROUP
	case ScopeRoom:
		kind = corev1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_ROOM
	}
	return &corev1.RbacPermissionScope{Kind: kind, Id: scopeID}
}

func rbacPermissionSubject(kind corev1.RbacPermissionSubjectKind, id string) *corev1.RbacPermissionSubject {
	return &corev1.RbacPermissionSubject{Kind: kind, Id: id}
}

func rbacPermissionSubjectKindForID(subject string) corev1.RbacPermissionSubjectKind {
	if IsUserSubject(subject) {
		return corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER
	}
	return corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE
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
		return rbacAggregateForPermissionScope(e.RbacPermissionGranted.GetScope())
	case *corev1.Event_RbacPermissionDenied:
		return rbacAggregateForPermissionScope(e.RbacPermissionDenied.GetScope())
	case *corev1.Event_RbacPermissionCleared:
		return rbacAggregateForPermissionScope(e.RbacPermissionCleared.GetScope())
	default:
		return events.RBACServerAggregate()
	}
}

func rbacAggregateForPermissionScope(scope *corev1.RbacPermissionScope) events.Aggregate {
	if scope == nil || scope.GetKind() == corev1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_SERVER {
		return events.RBACServerAggregate()
	}
	return events.RBACScopedAggregate(scope.GetId())
}

func (c *ChattoCore) appendRBACEvent(ctx context.Context, event *corev1.Event, check func() error) (uint64, error) {
	filter := events.RBACSubjectFilter()

	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return 0, fmt.Errorf("read authorization fence seq: %w", err)
		}
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read RBAC OCC filter seq: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return 0, fmt.Errorf("wait for RBAC projection: %w", err)
		}
		if check != nil {
			if err := check(); err != nil {
				return 0, err
			}
		}
		subject := rbacSubjectForEvent(event)
		entries := []events.BatchEntry{{
			Subject:       subject,
			Event:         event,
			HasOCC:        true,
			ExpectedSeq:   filterSeq,
			FilterSubject: filter,
		}}

		seqs, err := c.appendAuthorizationFencedBatch(ctx, event.GetActorId(), entries, authorizationSeq)
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

// appendRoleAssignmentEvent fences every projection used by role-assignment
// authorization against the narrow authorization boundary. Unrelated chat
// traffic does not advance that lane or force retries.
func (c *ChattoCore) appendRoleAssignmentEvent(ctx context.Context, userID string, requireExistingUser bool, event *corev1.Event, check func() error) (uint64, error) {
	filter := events.RBACSubjectFilter()
	userFilter := events.UserAggregate(userID).AllEventsFilter()
	actorID := event.GetActorId()

	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return 0, fmt.Errorf("read authorization fence seq: %w", err)
		}

		groupPos, err := c.EventPublisher.LastSubjectPosition(ctx, events.GroupSubjectFilter())
		if err != nil {
			return 0, fmt.Errorf("read room-group projection position: %w", err)
		}
		if err := c.roomModel.waitForGroupLayout(ctx, groupPos); err != nil {
			return 0, fmt.Errorf("wait for room-group projection: %w", err)
		}

		roomPos, err := c.EventPublisher.LastSubjectPosition(ctx, events.RoomSubjectFilter())
		if err != nil {
			return 0, fmt.Errorf("read room directory projection position: %w", err)
		}
		if err := c.roomModel.waitForDirectory(ctx, roomPos); err != nil {
			return 0, fmt.Errorf("wait for room directory projection: %w", err)
		}

		if actorID != "" && actorID != SystemActorID {
			actorFilter := events.UserAggregate(actorID).AllEventsFilter()
			if err := c.userModel.waitForUsersCurrent(ctx, "role assignment actor", actorFilter); err != nil {
				return 0, err
			}
		}
		if requireExistingUser {
			if err := c.userModel.waitForUsersCurrent(ctx, "role target user", userFilter); err != nil {
				return 0, err
			}
		}

		rbacSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read RBAC OCC filter seq: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(filter, rbacSeq)); err != nil {
			return 0, fmt.Errorf("wait for RBAC projection: %w", err)
		}

		if requireExistingUser {
			if _, err := c.GetUser(ctx, userID); err != nil {
				return 0, err
			}
		}
		if check != nil {
			if err := check(); err != nil {
				return 0, err
			}
		}
		subject := rbacSubjectForEvent(event)
		entries := []events.BatchEntry{{
			Subject:       subject,
			Event:         event,
			HasOCC:        true,
			ExpectedSeq:   rbacSeq,
			FilterSubject: filter,
		}}

		seqs, err := c.appendAuthorizationFencedBatch(ctx, actorID, entries, authorizationSeq)
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

func (c *ChattoCore) appendRBACEventWithMentionableCheck(ctx context.Context, event *corev1.Event, check func() error) (uint64, error) {
	filter := events.EventSubjectFilter()

	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return 0, fmt.Errorf("read authorization fence seq: %w", err)
		}
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read mentionable OCC filter seq: %w", err)
		}
		if err := c.mentionables.waitFor(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return 0, fmt.Errorf("wait for mentionables projection: %w", err)
		}

		rbacSeq, err := c.EventPublisher.LastSubjectSeq(ctx, events.RBACSubjectFilter())
		if err != nil {
			return 0, fmt.Errorf("read RBAC OCC filter seq: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(events.RBACSubjectFilter(), rbacSeq)); err != nil {
			return 0, fmt.Errorf("wait for RBAC projection: %w", err)
		}

		if check != nil {
			if err := check(); err != nil {
				return 0, err
			}
		}
		subject := rbacSubjectForEvent(event)
		entries := []events.BatchEntry{{
			Subject:       subject,
			Event:         event,
			HasOCC:        true,
			ExpectedSeq:   filterSeq,
			FilterSubject: filter,
		}}

		seqs, err := c.appendAuthorizationFencedBatch(ctx, event.GetActorId(), entries, authorizationSeq)
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

func (c *ChattoCore) appendRBACBatch(ctx context.Context, entries []events.BatchEntry, check func() error) (uint64, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	filter := events.RBACSubjectFilter()

	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return 0, fmt.Errorf("read authorization fence seq: %w", err)
		}
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read RBAC OCC filter seq: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
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

		actorID := chunk[0].Event.GetActorId()
		seqs, err := c.appendAuthorizationFencedBatch(ctx, actorID, chunk, authorizationSeq)
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
