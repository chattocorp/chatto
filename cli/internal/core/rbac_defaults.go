package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	serverRBACDefaultsVersion uint32 = 1
	roomRBACDefaultsVersion   uint32 = 1
)

// ensureRBACDefaultsInitialized atomically writes any selected defaults and a
// durable version marker under the deployment-wide RBAC OCC filter. The
// decision check is repeated after every conflict so a concurrent operator or
// replica write is preserved instead of being overwritten by stale bootstrap
// intent.
func (c *ChattoCore) ensureRBACDefaultsInitialized(
	ctx context.Context,
	scope PermissionScope,
	scopeID string,
	version uint32,
	seedWhenEmpty bool,
	defaults []rbacSeedDecision,
) error {
	filter := events.RBACSubjectFilter()

	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return fmt.Errorf("read RBAC defaults OCC filter seq: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return fmt.Errorf("wait for RBAC defaults projection: %w", err)
		}
		currentVersion := c.RBAC.DefaultsVersion(scope, scopeID)
		if currentVersion >= version {
			return nil
		}

		entries := make([]events.BatchEntry, 0, len(defaults)+1)
		if currentVersion == 0 && seedWhenEmpty && !c.hasPermissionDecisionsForDefaults(scope, scopeID) {
			entries = append(entries, rbacSeedEntries(nil, nil, append([]rbacSeedDecision(nil), defaults...))...)
		}
		entries = append(entries, rbacDefaultsInitializedEntry(scope, scopeID, version))
		entries[0].HasOCC = true
		entries[0].ExpectedSeq = filterSeq
		entries[0].FilterSubject = filter

		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			last := len(entries) - 1
			if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(entries[last].Subject, seqs[last])); err != nil {
				return fmt.Errorf("wait for RBAC defaults marker: %w", err)
			}
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return fmt.Errorf("append RBAC defaults batch: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}

	return fmt.Errorf("RBAC defaults OCC retry exhausted after %d attempts: %w", maxRBACMutationRetries, events.ErrConflict)
}

func (c *ChattoCore) hasPermissionDecisionsForDefaults(scope PermissionScope, scopeID string) bool {
	if scope == ScopeServer {
		return c.RBAC.HasAnyPermissionDecisions()
	}
	return c.RBAC.HasPermissionDecisions(scope, scopeID)
}

func rbacDefaultsInitializedEntry(scope PermissionScope, scopeID string, version uint32) events.BatchEntry {
	marker := &corev1.RbacDefaultsInitializedEvent{Version: version}
	switch scope {
	case ScopeServer:
		marker.Scope = &corev1.RbacDefaultsInitializedEvent_Server{Server: &corev1.RbacDefaultsInitializedEvent_ServerScope{}}
	case ScopeRoom:
		marker.Scope = &corev1.RbacDefaultsInitializedEvent_RoomId{RoomId: scopeID}
	default:
		panic(fmt.Sprintf("unsupported RBAC defaults scope %q", scope))
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacDefaultsInitialized{
		RbacDefaultsInitialized: marker,
	}})
	return events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event}
}

func defaultChannelRoomDecisions(roomID, roomName string) []rbacSeedDecision {
	var decisions []rbacSeedDecision
	appendRoleDecisions := func(roleName string, permissions []Permission, decision DecisionKind) {
		for _, permission := range permissions {
			decisions = append(decisions, rbacSeedDecision{
				scope:       ScopeRoom,
				scopeID:     roomID,
				subjectKind: corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE,
				subject:     roleName,
				permission:  permission,
				decision:    decision,
			})
		}
	}

	if strings.EqualFold(roomName, AnnouncementsRoomName) {
		appendRoleDecisions(RoleEveryone, DefaultAnnouncementsEveryonePermissions(), DecisionAllow)
		appendRoleDecisions(RoleEveryone, DefaultAnnouncementsEveryoneDenials(), DecisionDeny)
	} else {
		appendRoleDecisions(RoleEveryone, DefaultRoomEveryonePermissions(), DecisionAllow)
	}
	appendRoleDecisions(RoleModerator, DefaultRoomModeratorPermissions(), DecisionAllow)
	appendRoleDecisions(RoleAdmin, DefaultRoomAdminPermissions(), DecisionAllow)
	for _, roleName := range []string{RoleModerator, RoleAdmin} {
		appendRoleDecisions(roleName, DefaultAnnouncementsPosterPermissions(), DecisionAllow)
	}
	return decisions
}
