package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"google.golang.org/protobuf/proto"
)

// splitInvitationPermissions copies the old administrative permission's current
// allow/deny decisions once. Any manage-invites decision (including a clear)
// closes the upgrade gate, so later operator changes survive restart. Fresh
// bootstrap already contains that permission. The whole RBAC tail guards the
// atomic copy against other replicas and concurrent permission edits.
func (c *ChattoCore) splitInvitationPermissions(ctx context.Context) error {
	for range maxRBACMutationRetries {
		replay := NewRBACProjection()
		var seq uint64
		for {
			page, err := c.EventPublisher.SubjectRecordsAfterPage(ctx, evtstream.RBACSubjectFilter(), seq, 500, 1<<20)
			if err != nil {
				return err
			}
			for _, record := range page.Records {
				event := &evtv1.Event{}
				if err := proto.Unmarshal(record.Data, event); err != nil {
					return fmt.Errorf("decode invitation permission history: %w", err)
				}
				permission, _, _ := rbacUpgradeDecision(event)
				if Permission(permission) == PermUserManageInvites {
					return nil
				}
				if err := replay.Apply(event, record.Sequence); err != nil {
					return err
				}
			}
			seq = page.LastSequence
			if !page.More {
				break
			}
		}
		var decisions []rbacSeedDecision
		for _, decision := range replay.Decisions() {
			if decision.permission == PermUserInvite && decision.scope == ScopeServer {
				decision.permission = PermUserManageInvites
				decisions = append(decisions, decision)
			}
		}
		entries := rbacSeedEntries(nil, nil, decisions)
		if len(entries) == 0 {
			// Record that everyone has no management grant. Even a server with
			// no old invite decisions must close the gate before future grants.
			event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionCleared{
				RbacPermissionCleared: rbacPermissionClearedEvent(ScopeServer, "", evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, RoleEveryone, PermUserManageInvites),
			}})
			entries = []evtstream.BatchEntry{{Subject: rbacSubjectForEvent(event), Event: event}}
		}
		entries[0].Expect = events.ExpectFilterSeq(evtstream.RBACSubjectFilter(), seq)
		if _, err := c.EventPublisher.AppendBatch(ctx, entries); err != nil {
			if errors.Is(err, events.ErrConflict) {
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("split invitation permissions: %w", events.ErrConflict)
}
