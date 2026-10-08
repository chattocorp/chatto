package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// permRoomMemberBanLegacy is the 0.4 room-ban permission. It has no effect in
// 0.5 and is read only to carry its decisions over to PermRoomMemberRemove.
const permRoomMemberBanLegacy Permission = "room.ban-member"

// v05UpgradePermissions are the non-call permissions introduced in 0.5. Any
// historical decision for one of them, at any scope and for any subject, shows
// that the RBAC log was bootstrapped or reviewed by 0.5. The upgrade grants
// then stay unapplied.
func v05UpgradePermissions() []Permission {
	return []Permission{
		PermServerManageNeighbors,
		PermRoomMemberRemove,
		PermMessageRead,
		PermMessageReadInteractions,
		PermMessagePostInInteractions,
		PermUserInvite,
		PermBotCreate,
		PermBotManage,
	}
}

// seedUpgradePermissions upgrades old RBAC logs using ordinary permission
// facts. It runs after seedDefaultRBAC on every startup and has two gates:
//
//   - Each call permission is initialized for everyone at server scope unless
//     an everyone decision for it at server or Direct messages scope was ever
//     granted, denied, or cleared. New servers seed the Direct messages
//     decision, so they stay closed (ADR-116).
//   - The 0.5 grants apply as one set, only while no v05UpgradePermissions
//     decision exists anywhere in the log. They keep only capabilities that
//     0.4 servers had: everyone reads messages, and room.remove-member copies
//     each current room.ban-member decision. Capabilities that are new in 0.5,
//     such as bots and invite links, stay off until an operator grants them.
//
// Retries and restarts can never undo an operator's later clear or deny. The
// complete RBAC tail guards concurrent initializers.
func (c *ChattoCore) seedUpgradePermissions(ctx context.Context) error {
	for range maxRBACMutationRetries {
		state, err := c.scanRBACUpgradeState(ctx)
		if err != nil {
			return err
		}
		decisions := state.upgradeDecisions()
		if len(decisions) == 0 {
			return nil
		}
		entries := rbacSeedEntries(nil, nil, decisions)
		entries[0].Expect = events.ExpectFilterSeq(evtstream.RBACSubjectFilter(), state.seq)
		if _, err := c.EventPublisher.AppendBatch(ctx, entries); err != nil {
			if errors.Is(err, events.ErrConflict) {
				continue
			}
			return err
		}
		if state.v05Pending && c.logger != nil {
			c.logger.Info("Applied 0.5 RBAC upgrade grants", "decisions", len(decisions))
		}
		return nil
	}
	return fmt.Errorf("seed upgrade permissions: %w", events.ErrConflict)
}

// rbacUpgradeState is one complete or early-stopped scan of the RBAC log.
type rbacUpgradeState struct {
	seq         uint64
	callDecided map[Permission]bool // everyone call decisions at server or DM scope ever seen
	v05Pending  bool                // no 0.5 permission decision seen
	// replay holds current decisions for the 0.5 grants. It is complete only
	// when v05Pending is true, because the scan stops once the gate closes.
	replay *RBACProjection
}

func (s rbacUpgradeState) upgradeDecisions() []rbacSeedDecision {
	allow := func(role string, permission Permission) rbacSeedDecision {
		return rbacSeedDecision{scope: ScopeServer, subjectKind: evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, subject: role, permission: permission, decision: DecisionAllow}
	}
	var decisions []rbacSeedDecision
	for _, permission := range callPermissionIDs() {
		if !s.callDecided[permission] {
			decisions = append(decisions, allow(RoleEveryone, permission))
		}
	}
	if !s.v05Pending {
		return decisions
	}
	decisions = append(decisions, allow(RoleEveryone, PermMessageRead))
	for _, decision := range s.replay.Decisions() {
		if decision.permission != permRoomMemberBanLegacy || !PermissionAppliesAtScope(PermRoomMemberRemove, decision.scope) {
			continue
		}
		decision.permission = PermRoomMemberRemove
		decisions = append(decisions, decision)
	}
	return decisions
}

// scanRBACUpgradeState stops early once every gate is closed. This bounds
// startup work on logs that are already upgraded. Only a complete scan can
// authorize missing grants.
func (c *ChattoCore) scanRBACUpgradeState(ctx context.Context) (rbacUpgradeState, error) {
	state := rbacUpgradeState{callDecided: map[Permission]bool{}, v05Pending: true, replay: NewRBACProjection()}
	v05 := map[Permission]bool{}
	for _, permission := range v05UpgradePermissions() {
		v05[permission] = true
	}
	for {
		page, err := c.EventPublisher.SubjectRecordsAfterPage(ctx, evtstream.RBACSubjectFilter(), state.seq, 500, 1<<20)
		if err != nil {
			return rbacUpgradeState{}, err
		}
		for _, record := range page.Records {
			event := &evtv1.Event{}
			if err := proto.Unmarshal(record.Data, event); err != nil {
				return rbacUpgradeState{}, fmt.Errorf("decode RBAC record at sequence %d: %w", record.Sequence, err)
			}
			if state.v05Pending {
				if err := state.replay.Apply(event, record.Sequence); err != nil {
					return rbacUpgradeState{}, err
				}
			}
			permission, key, ok := rbacUpgradeDecision(event)
			if v05[Permission(permission)] {
				state.v05Pending = false
			}
			if ok && (key.scope == ScopeServer || key.scope == ScopeDM) && key.subjectKind == evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE && key.subject == RoleEveryone {
				for _, id := range callPermissionIDs() {
					if key.permission == id {
						state.callDecided[id] = true
					}
				}
			}
			if !state.v05Pending && len(state.callDecided) == len(callPermissionIDs()) {
				state.seq = page.LastSequence
				return state, nil
			}
		}
		state.seq = page.LastSequence
		if !page.More {
			return state, nil
		}
	}
}

// rbacUpgradeDecision returns the permission and key of a grant, deny, or
// clear event. The key accepts the typed and legacy wire shapes, like
// RBACProjection. The permission is set even when the key is unreadable.
func rbacUpgradeDecision(event *evtv1.Event) (string, rbacDecisionKey, bool) {
	var scope *evtv1.RbacPermissionScope
	var subject *evtv1.RbacPermissionSubject
	var permission string
	var legacy proto.Message
	switch e := event.Event.(type) {
	case *evtv1.Event_RbacPermissionGranted:
		scope, subject, permission, legacy = e.RbacPermissionGranted.Scope, e.RbacPermissionGranted.Subject, e.RbacPermissionGranted.Permission, e.RbacPermissionGranted
	case *evtv1.Event_RbacPermissionDenied:
		scope, subject, permission, legacy = e.RbacPermissionDenied.Scope, e.RbacPermissionDenied.Subject, e.RbacPermissionDenied.Permission, e.RbacPermissionDenied
	case *evtv1.Event_RbacPermissionCleared:
		scope, subject, permission, legacy = e.RbacPermissionCleared.Scope, e.RbacPermissionCleared.Subject, e.RbacPermissionCleared.Permission, e.RbacPermissionCleared
	default:
		return "", rbacDecisionKey{}, false
	}
	if key, ok := rbacDecisionKeyFromFields(scope, subject, permission); ok {
		return permission, key, true
	}
	key, ok := legacyRBACDecisionKeyFromUnknown(legacy, permission)
	return permission, key, ok
}

// ignoredRoleDenyLogLimit caps the room and room-group IDs that the startup
// warning lists.
const ignoredRoleDenyLogLimit = 50

// warnIgnoredRoleDenies logs the stored role denies that have no effect.
// Roles, everyone included, only grant permissions since 0.5 (ADR-116), so a
// stored role deny, for example from an earlier suspension recipe or a
// restricted room, is ignored. Call it after the projections are current. The
// log names only a count, role names, permissions, scopes, and opaque room and
// room-group IDs.
func (c *ChattoCore) warnIgnoredRoleDenies() {
	denies := c.ignoredRoleDenies()
	if denies.count == 0 || c.logger == nil {
		return
	}
	roomIDs, moreRooms := capIDs(denies.roomIDs, ignoredRoleDenyLogLimit)
	groupIDs, moreGroups := capIDs(denies.groupIDs, ignoredRoleDenyLogLimit)
	entries, moreEntries := capIDs(denies.entries, ignoredRoleDenyLogLimit)
	c.logger.Warn("Role denies have no effect: roles only grant permissions. Review the affected rooms and room groups, and set denies on single users instead.",
		"ignored_role_denies", denies.count,
		"affected_room_ids", roomIDs,
		"more_affected_rooms", moreRooms,
		"affected_group_ids", groupIDs,
		"more_affected_groups", moreGroups,
		"ignored_denies", entries,
		"more_ignored_denies", moreEntries,
	)
}

// ignoredRoleDenySummary describes the stored role denies.
type ignoredRoleDenySummary struct {
	// count is the number of stored role denies at all scopes.
	count int
	// roomIDs and groupIDs list, sorted and without duplicates, the rooms and
	// room groups that have a stored role deny.
	roomIDs, groupIDs []string
	// entries describes each deny as role:permission@scope or
	// role:permission@scope:id, sorted, so operators can find it.
	entries []string
}

// ignoredRoleDenies summarizes the stored denies of all roles, everyone
// included.
func (c *ChattoCore) ignoredRoleDenies() ignoredRoleDenySummary {
	var out ignoredRoleDenySummary
	rooms := make(map[string]struct{})
	groups := make(map[string]struct{})
	for _, role := range c.rbacModel.roles() {
		for _, decision := range c.rbacModel.rolePermissionDecisions(role.GetName()) {
			if decision.Decision != DecisionDeny {
				continue
			}
			out.count++
			entry := role.GetName() + ":" + string(decision.Permission) + "@" + string(decision.Scope)
			if decision.ScopeID != "" {
				entry += ":" + decision.ScopeID
			}
			out.entries = append(out.entries, entry)
			switch decision.Scope {
			case ScopeRoom:
				rooms[decision.ScopeID] = struct{}{}
			case ScopeGroup:
				groups[decision.ScopeID] = struct{}{}
			}
		}
	}
	out.roomIDs = slices.Sorted(maps.Keys(rooms))
	out.groupIDs = slices.Sorted(maps.Keys(groups))
	slices.Sort(out.entries)
	return out
}

// capIDs returns at most limit values and the number of values left out.
func capIDs(ids []string, limit int) ([]string, int) {
	if len(ids) <= limit {
		return ids, 0
	}
	return ids[:limit], len(ids) - limit
}
