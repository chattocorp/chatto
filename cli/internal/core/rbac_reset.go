package core

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ResetRBAC resets the RBAC event-sourced aggregate, re-seeds the system roles plus
// default permissions from code, and assigns the `owner` role to every user
// whose verified email matches `owners.emails` in the supplied config.
//
// This is the operator escape hatch for misconfigured / drifted RBAC state
// and the upgrade tool for moving an existing deployment onto the unified
// Phase-5 server-RBAC layout. Idempotent: running it twice produces the
// same result.
//
// Resetting is intentionally aggressive: custom roles, assignments, and
// explicit permission overrides are removed from the projection by appended
// reset events. Rebuild those after the reset.
func (c *ChattoCore) ResetRBAC(ctx context.Context, ownersCfg config.OwnersConfig) error {
	// Auto-promote config owners. Every user whose verified email matches
	// owners.emails in chatto.toml gets the `owner` role.
	users, err := c.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	promotions := []rbacSeedAssignment{}
	for _, user := range users {
		emails, err := c.GetVerifiedEmails(ctx, user.Id)
		if err != nil {
			c.logger.Warn("Failed to read verified emails for user during RBAC reset",
				"user_id", user.Id, "error", err)
			continue
		}
		matched := false
		for _, ve := range emails {
			if ownersCfg.IsServerOwnerEmail(ve.Email) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		promotions = append(promotions, rbacSeedAssignment{userID: user.Id, roleName: RoleOwner})
	}

	entries := c.rbacResetEntries(promotions)
	if _, err := c.appendRBACBatch(ctx, entries, nil); err != nil {
		return fmt.Errorf("publish RBAC reset events: %w", err)
	}

	for _, promotion := range promotions {
		c.logger.Info("Promoted config owner to owner role", "user_id", promotion.userID)
	}
	c.logger.Info("RBAC reset complete",
		"events_published", len(entries),
		"owners_promoted", len(promotions))
	return nil
}

func (c *ChattoCore) rbacResetEntries(promotions []rbacSeedAssignment) []events.BatchEntry {
	createdAt := timestamppb.Now()
	var entries []events.BatchEntry

	roles := c.RBAC.ListRoles()
	for _, role := range roles {
		if role.GetName() == "" || IsSystemRole(role.GetName()) {
			continue
		}
		event := newEvent(SystemActorID, &corev1.Event{CreatedAt: createdAt, Event: &corev1.Event_RbacRoleDeleted{
			RbacRoleDeleted: &corev1.RbacRoleDeletedEvent{RoleName: role.GetName()},
		}})
		entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
	}

	revokedUsers := make(map[string]struct{})
	for _, assignment := range c.RBAC.Assignments() {
		key := assignment.userID + "|" + assignment.roleName
		if _, ok := revokedUsers[key]; ok {
			continue
		}
		revokedUsers[key] = struct{}{}
		event := newEvent(SystemActorID, &corev1.Event{CreatedAt: createdAt, Event: &corev1.Event_RbacRoleRevoked{
			RbacRoleRevoked: &corev1.RbacRoleRevokedEvent{UserId: assignment.userID, RoleName: assignment.roleName},
		}})
		entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
	}

	for _, decision := range c.RBAC.Decisions() {
		subjectKind := decision.subjectKind
		if subjectKind == corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_UNSPECIFIED {
			subjectKind = rbacPermissionSubjectKindForID(decision.subject)
		}
		event := newEvent(SystemActorID, &corev1.Event{CreatedAt: createdAt, Event: &corev1.Event_RbacPermissionCleared{
			RbacPermissionCleared: rbacPermissionClearedEvent(decision.scope, decision.scopeID, subjectKind, decision.subject, decision.permission),
		}})
		entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
	}

	entries = append(entries, rbacSeedEntries(defaultRBACRoles(), promotions, defaultRBACDecisions())...)
	return entries
}
