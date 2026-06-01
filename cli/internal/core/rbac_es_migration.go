package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type rbacSeedDecision struct {
	scope      PermissionScope
	scopeID    string
	subject    string
	permission Permission
	decision   DecisionKind
}

type rbacSeedAssignment struct {
	userID   string
	roleName string
}

func (c *ChattoCore) migrateRBACToES(ctx context.Context) error {
	entries, legacyKeys, err := c.buildRBACMigrationEntries(ctx)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}

	entries[0].HasOCC = true
	entries[0].ExpectedSeq = 0
	entries[0].FilterSubject = events.RBACSubjectFilter()

	startedAt := time.Now()
	if _, err := c.EventPublisher.AppendBatch(ctx, entries); err != nil {
		if errors.Is(err, events.ErrConflict) {
			c.logger.Info("RBAC ES migration: EVT already seeded, skipping", "legacy_keys", legacyKeys)
			return nil
		}
		return fmt.Errorf("publish RBAC migration events: %w", err)
	}

	c.logger.Info(
		"RBAC ES migration: seeded events",
		"events_imported", len(entries),
		"legacy_keys", legacyKeys,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
	return nil
}

func (c *ChattoCore) buildRBACMigrationEntries(ctx context.Context) ([]events.BatchEntry, int, error) {
	var keys []string
	if c.storage.serverRBACKV != nil {
		var err error
		keys, err = listSortedKeysFromKV(ctx, c.storage.serverRBACKV)
		if err != nil {
			return nil, 0, fmt.Errorf("list SERVER_RBAC keys: %w", err)
		}
	}

	roles := defaultRBACRoles()
	assignments := []rbacSeedAssignment{}
	decisions := []rbacSeedDecision{}

	legacyKeys := 0
	for _, key := range keys {
		if key == rbacDefaultsSentinel {
			continue
		}
		legacyKeys++
		switch {
		case strings.HasPrefix(key, RoleKeyPrefix):
			entry, err := c.storage.serverRBACKV.Get(ctx, key)
			if err != nil {
				if errors.Is(err, jetstream.ErrKeyNotFound) {
					continue
				}
				return nil, 0, fmt.Errorf("get %s: %w", key, err)
			}
			var role corev1.Role
			if err := proto.Unmarshal(entry.Value(), &role); err != nil {
				c.logger.Warn("RBAC ES migration: skipping unmarshalable role", "key", key, "error", err)
				continue
			}
			if role.GetName() != "" {
				roles[role.GetName()] = proto.Clone(&role).(*corev1.Role)
			}
		case strings.HasPrefix(key, MemberKeyPrefix):
			roleName, userID := ParseMemberKey(key)
			if roleName != "" && userID != "" && roleName != RoleEveryone {
				assignments = append(assignments, rbacSeedAssignment{userID: userID, roleName: roleName})
			}
		case strings.HasPrefix(key, AllowKeyPrefix):
			if decision, ok := seedDecisionFromAllowKey(key); ok {
				decisions = append(decisions, decision)
			}
		case strings.HasPrefix(key, DenyKeyPrefix):
			if decision, ok := seedDecisionFromDenyKey(key); ok {
				decisions = append(decisions, decision)
			}
		case strings.HasPrefix(key, GroupAllowKeyPrefix):
			if decision, ok := seedDecisionFromScopedKey(key, ScopeGroup, DecisionAllow); ok {
				decisions = append(decisions, decision)
			}
		case strings.HasPrefix(key, GroupDenyKeyPrefix):
			if decision, ok := seedDecisionFromScopedKey(key, ScopeGroup, DecisionDeny); ok {
				decisions = append(decisions, decision)
			}
		case strings.HasPrefix(key, RoomAllowKeyPrefix):
			if decision, ok := seedDecisionFromScopedKey(key, ScopeRoom, DecisionAllow); ok {
				decisions = append(decisions, decision)
			}
		case strings.HasPrefix(key, RoomDenyKeyPrefix):
			if decision, ok := seedDecisionFromScopedKey(key, ScopeRoom, DecisionDeny); ok {
				decisions = append(decisions, decision)
			}
		}
	}

	if legacyKeys == 0 {
		decisions = append(decisions, defaultRBACDecisions()...)
	}

	return rbacSeedEntries(roles, assignments, decisions), legacyKeys, nil
}

func listSortedKeysFromKV(ctx context.Context, kv jetstream.KeyValue) ([]string, error) {
	lister, err := kv.ListKeys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, err
	}
	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func defaultRBACRoles() map[string]*corev1.Role {
	return map[string]*corev1.Role{
		RoleOwner: {
			Name:        RoleOwner,
			DisplayName: "Owner",
			Description: "Full server control",
			Position:    PositionOwner,
		},
		RoleAdmin: {
			Name:        RoleAdmin,
			DisplayName: "Admin",
			Description: "Full administrative access to the server",
			Position:    PositionAdmin,
		},
		RoleModerator: {
			Name:        RoleModerator,
			DisplayName: "Moderator",
			Description: "View access to admin panels without management permissions",
			Position:    PositionModerator,
		},
		RoleEveryone: {
			Name:        RoleEveryone,
			DisplayName: "Everyone",
			Description: "All authenticated users",
			Position:    PositionEveryone,
		},
	}
}

func defaultRBACDecisions() []rbacSeedDecision {
	roleDefaults := []struct {
		role  string
		perms []Permission
	}{
		{RoleOwner, DefaultOwnerPermissions()},
		{RoleAdmin, DefaultAdminPermissions()},
		{RoleModerator, DefaultModeratorPermissions()},
		{RoleEveryone, DefaultEveryonePermissions()},
	}
	var decisions []rbacSeedDecision
	for _, spec := range roleDefaults {
		for _, perm := range spec.perms {
			if PermissionAppliesAtScope(perm, ScopeServer) {
				decisions = append(decisions, rbacSeedDecision{
					scope:      ScopeServer,
					subject:    spec.role,
					permission: perm,
					decision:   DecisionAllow,
				})
			}
		}
	}
	return decisions
}

func seedDecisionFromAllowKey(key string) (rbacSeedDecision, bool) {
	parts := ParseAllowKey(key)
	perm := ReconstructPermission(parts.Verb, parts.ObjectType)
	if parts.Subject == "" || perm == "" {
		return rbacSeedDecision{}, false
	}
	return rbacSeedDecision{scope: ScopeServer, subject: parts.Subject, permission: perm, decision: DecisionAllow}, true
}

func seedDecisionFromDenyKey(key string) (rbacSeedDecision, bool) {
	parts := ParseDenyKey(key)
	perm := ReconstructPermission(parts.Verb, parts.ObjectType)
	if parts.Subject == "" || perm == "" {
		return rbacSeedDecision{}, false
	}
	return rbacSeedDecision{scope: ScopeServer, subject: parts.Subject, permission: perm, decision: DecisionDeny}, true
}

func seedDecisionFromScopedKey(key string, scope PermissionScope, decision DecisionKind) (rbacSeedDecision, bool) {
	var parts ScopedRBACKeyParts
	switch {
	case scope == ScopeGroup && decision == DecisionAllow:
		parts = ParseSetAllowKey(key)
	case scope == ScopeGroup && decision == DecisionDeny:
		parts = ParseSetDenyKey(key)
	case scope == ScopeRoom && decision == DecisionAllow:
		parts = ParseRoomAllowKey(key)
	case scope == ScopeRoom && decision == DecisionDeny:
		parts = ParseRoomDenyKey(key)
	}
	perm := ReconstructPermission(parts.Verb, parts.ObjectType)
	if parts.ScopeID == "" || parts.Subject == "" || perm == "" {
		return rbacSeedDecision{}, false
	}
	return rbacSeedDecision{
		scope:      scope,
		scopeID:    parts.ScopeID,
		subject:    parts.Subject,
		permission: perm,
		decision:   decision,
	}, true
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
		event := newEvent("system:migration", &corev1.Event{CreatedAt: createdAt, Event: &corev1.Event_RbacRoleCreated{
			RbacRoleCreated: &corev1.RbacRoleCreatedEvent{
				RoleName:    role.GetName(),
				DisplayName: role.GetDisplayName(),
				Description: role.GetDescription(),
				Rank:        role.GetPosition(),
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
		event := newEvent("system:migration", &corev1.Event{CreatedAt: createdAt, Event: &corev1.Event_RbacRoleAssigned{
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
		if decision.decision == DecisionDeny {
			event = newEvent("system:migration", &corev1.Event{CreatedAt: createdAt, Event: &corev1.Event_RbacPermissionDenied{
				RbacPermissionDenied: rbacPermissionDeniedEvent(decision.scope, decision.scopeID, decision.subject, decision.permission),
			}})
		} else {
			event = newEvent("system:migration", &corev1.Event{CreatedAt: createdAt, Event: &corev1.Event_RbacPermissionGranted{
				RbacPermissionGranted: rbacPermissionGrantedEvent(decision.scope, decision.scopeID, decision.subject, decision.permission),
			}})
		}
		entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
	}

	return entries
}
