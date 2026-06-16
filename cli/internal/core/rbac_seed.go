package core

import (
	"context"
	"errors"
	"sort"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type rbacSeedDecision struct {
	scope       PermissionScope
	scopeID     string
	subjectKind corev1.RbacPermissionSubjectKind
	subject     string
	permission  Permission
	decision    DecisionKind
}

type rbacSeedAssignment struct {
	userID   string
	roleName string
}

func (c *ChattoCore) seedDefaultRBAC(ctx context.Context) error {
	entries := rbacSeedEntries(defaultRBACRoles(), nil, defaultRBACDecisions())
	if len(entries) == 0 {
		return nil
	}
	entries[0].HasOCC = true
	entries[0].ExpectedSeq = 0
	entries[0].FilterSubject = events.RBACSubjectFilter()

	if _, err := c.EventPublisher.AppendBatch(ctx, entries); err != nil {
		if errors.Is(err, events.ErrConflict) {
			return nil
		}
		return err
	}
	c.logger.Info("Seeded default RBAC roles and permissions", "events", len(entries))
	return nil
}

func defaultRBACRoles() map[string]*corev1.Role {
	return map[string]*corev1.Role{
		RoleOwner: {
			Name:        RoleOwner,
			DisplayName: "Owner",
			Description: "Full server control",
			Position:    PositionOwner,
			Pingable:    false,
		},
		RoleAdmin: {
			Name:        RoleAdmin,
			DisplayName: "Admin",
			Description: "Full administrative access to the server",
			Position:    PositionAdmin,
			Pingable:    false,
		},
		RoleModerator: {
			Name:        RoleModerator,
			Description: "View access to admin panels without management permissions",
			DisplayName: "Moderator",
			Position:    PositionModerator,
			Pingable:    true,
		},
		RoleEveryone: {
			Name:        RoleEveryone,
			DisplayName: "Everyone",
			Description: "All authenticated users",
			Position:    PositionEveryone,
			Pingable:    false,
		},
	}
}

func defaultRBACDecisions() []rbacSeedDecision {
	roleDefaults := []struct {
		role  string
		perms []Permission
	}{
		{RoleAdmin, DefaultAdminPermissions()},
		{RoleModerator, DefaultModeratorPermissions()},
		{RoleEveryone, DefaultEveryonePermissions()},
	}
	var decisions []rbacSeedDecision
	for _, spec := range roleDefaults {
		for _, perm := range spec.perms {
			if PermissionAppliesAtScope(perm, ScopeServer) {
				decisions = append(decisions, rbacSeedDecision{
					scope:       ScopeServer,
					subjectKind: corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE,
					subject:     spec.role,
					permission:  perm,
					decision:    DecisionAllow,
				})
			}
		}
	}
	return decisions
}

func rbacSeedEntries(roles map[string]*corev1.Role, assignments []rbacSeedAssignment, decisions []rbacSeedDecision) []events.BatchEntry {
	createdAt := timestamppb.Now()
	var entries []events.BatchEntry

	roleNames := make([]string, 0, len(roles))
	for name := range roles {
		roleNames = append(roleNames, name)
	}
	sort.Strings(roleNames)
	for _, name := range roleNames {
		role := roles[name]
		event := newEvent(SystemActorID, &corev1.Event{CreatedAt: createdAt, Event: &corev1.Event_RbacRoleCreated{
			RbacRoleCreated: &corev1.RbacRoleCreatedEvent{
				RoleName:    role.GetName(),
				DisplayName: role.GetDisplayName(),
				Description: role.GetDescription(),
				Rank:        role.GetPosition(),
				Pingable:    role.GetPingable(),
			},
		}})
		entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
	}

	sort.Slice(assignments, func(i, j int) bool {
		if assignments[i].userID != assignments[j].userID {
			return assignments[i].userID < assignments[j].userID
		}
		return assignments[i].roleName < assignments[j].roleName
	})
	for _, assignment := range assignments {
		event := newEvent(SystemActorID, &corev1.Event{CreatedAt: createdAt, Event: &corev1.Event_RbacRoleAssigned{
			RbacRoleAssigned: &corev1.RbacRoleAssignedEvent{UserId: assignment.userID, RoleName: assignment.roleName},
		}})
		entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
	}

	sort.Slice(decisions, func(i, j int) bool {
		a, b := decisions[i], decisions[j]
		if a.scope != b.scope {
			return a.scope < b.scope
		}
		if a.scopeID != b.scopeID {
			return a.scopeID < b.scopeID
		}
		if a.subject != b.subject {
			return a.subject < b.subject
		}
		if a.permission != b.permission {
			return a.permission < b.permission
		}
		return a.decision < b.decision
	})
	for _, decision := range decisions {
		var event *corev1.Event
		subjectKind := decision.subjectKind
		if subjectKind == corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_UNSPECIFIED {
			subjectKind = rbacPermissionSubjectKindForID(decision.subject)
		}
		if decision.decision == DecisionDeny {
			event = newEvent(SystemActorID, &corev1.Event{CreatedAt: createdAt, Event: &corev1.Event_RbacPermissionDenied{
				RbacPermissionDenied: rbacPermissionDeniedEvent(decision.scope, decision.scopeID, subjectKind, decision.subject, decision.permission),
			}})
		} else {
			event = newEvent(SystemActorID, &corev1.Event{CreatedAt: createdAt, Event: &corev1.Event_RbacPermissionGranted{
				RbacPermissionGranted: rbacPermissionGrantedEvent(decision.scope, decision.scopeID, subjectKind, decision.subject, decision.permission),
			}})
		}
		entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
	}

	return entries
}
