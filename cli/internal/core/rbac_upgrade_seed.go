package core

import (
	"context"
	"errors"
	"fmt"

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
//     that server/everyone decision was ever granted, denied, or cleared.
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
	callDecided map[Permission]bool // server/everyone call decisions ever seen
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
			if ok && key.scope == ScopeServer && key.subjectKind == evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE && key.subject == RoleEveryone {
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

// warnIgnoredRoleDenies logs how many stored role denies have no effect.
// Roles only grant permissions since 0.5 (ADR-116), so a deny on a named role,
// for example from an earlier suspension recipe, is ignored. Call it after the
// projections are current. The log names only a count.
func (c *ChattoCore) warnIgnoredRoleDenies() {
	if count := c.ignoredRoleDenyCount(); count > 0 && c.logger != nil {
		c.logger.Warn("Role denies have no effect: roles only grant permissions. Set denies for everyone or for single users instead.", "ignored_role_denies", count)
	}
}

// ignoredRoleDenyCount counts the stored denies of named roles.
func (c *ChattoCore) ignoredRoleDenyCount() int {
	count := 0
	for _, role := range c.rbacModel.roles() {
		if role.GetName() == RoleEveryone {
			continue
		}
		for _, decision := range c.rbacModel.rolePermissionDecisions(role.GetName()) {
			if decision.Decision == DecisionDeny {
				count++
			}
		}
	}
	return count
}
