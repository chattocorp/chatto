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

// seedCallPermissions upgrades old RBAC logs using ordinary permission facts.
// Any historical server/everyone decision, including a clear, makes that
// permission initialized. Retries and restarts can never undo an operator's
// later clear or deny. The complete RBAC tail guards concurrent initializers.
func (c *ChattoCore) seedCallPermissions(ctx context.Context) error {
	for attempt := 0; attempt < maxRBACMutationRetries; attempt++ {
		seen, seq, err := c.initializedCallPermissions(ctx)
		if err != nil {
			return err
		}
		var decisions []rbacSeedDecision
		for _, permission := range callPermissionIDs() {
			if !seen[permission] {
				decisions = append(decisions, rbacSeedDecision{scope: ScopeServer, subjectKind: evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, subject: RoleEveryone, permission: permission, decision: DecisionAllow})
			}
		}
		if len(decisions) == 0 {
			return nil
		}
		entries := rbacSeedEntries(nil, nil, decisions)
		entries[0].HasOCC, entries[0].ExpectedSeq, entries[0].FilterSubject = true, seq, evtstream.RBACSubjectFilter()
		if _, err := c.EventPublisher.AppendBatch(ctx, entries); err != nil {
			if errors.Is(err, events.ErrConflict) {
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("seed call permissions: %w", events.ErrConflict)
}

// initializedCallPermissions bounds startup memory and stops once all defaults
// have historical decisions. Only a complete scan can authorize missing grants.
func (c *ChattoCore) initializedCallPermissions(ctx context.Context) (map[Permission]bool, uint64, error) {
	seen := map[Permission]bool{}
	var after uint64
	for {
		page, err := c.EventPublisher.SubjectRecordsAfterPage(ctx, evtstream.RBACSubjectFilter(), after, 500, 1<<20)
		if err != nil {
			return nil, 0, err
		}
		for _, record := range page.Records {
			event := &evtv1.Event{}
			if err := proto.Unmarshal(record.Data, event); err != nil {
				return nil, 0, fmt.Errorf("decode RBAC record at sequence %d: %w", record.Sequence, err)
			}
			var scope *evtv1.RbacPermissionScope
			var subject *evtv1.RbacPermissionSubject
			var permission string
			switch e := event.Event.(type) {
			case *evtv1.Event_RbacPermissionGranted:
				scope, subject, permission = e.RbacPermissionGranted.Scope, e.RbacPermissionGranted.Subject, e.RbacPermissionGranted.Permission
			case *evtv1.Event_RbacPermissionDenied:
				scope, subject, permission = e.RbacPermissionDenied.Scope, e.RbacPermissionDenied.Subject, e.RbacPermissionDenied.Permission
			case *evtv1.Event_RbacPermissionCleared:
				scope, subject, permission = e.RbacPermissionCleared.Scope, e.RbacPermissionCleared.Subject, e.RbacPermissionCleared.Permission
			}
			if scope.GetKind() != evtv1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_SERVER || subject.GetKind() != evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE || subject.GetId() != RoleEveryone {
				continue
			}
			for _, id := range callPermissionIDs() {
				if permission == string(id) {
					seen[id] = true
				}
			}
			if len(seen) == len(callPermissionIDs()) {
				return seen, page.LastSequence, nil
			}
		}
		after = page.LastSequence
		if !page.More {
			return seen, after, nil
		}
	}
}
