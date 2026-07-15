package core

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const rbacSnapshotCompatibilityID = "v1"

func (*RBACProjection) SnapshotCompatibilityID() string { return rbacSnapshotCompatibilityID }

func (p *RBACProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	snapshot := &corev1.RBACProjectionSnapshot{ReplayGuard: snapshotReplayGuard(p.replayGuard)}
	for _, roleName := range sortedMapKeys(p.roles) {
		snapshot.Roles = append(snapshot.Roles, proto.Clone(p.roles[roleName]).(*corev1.Role))
	}
	for _, userID := range sortedMapKeys(p.assignments) {
		snapshot.Assignments = append(snapshot.Assignments, &corev1.RBACAssignmentSnapshot{UserId: userID, RoleNames: sortedMapKeys(p.assignments[userID])})
	}
	keys := make([]rbacDecisionKey, 0, len(p.decisions))
	for key := range p.decisions {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.scope != b.scope {
			return a.scope < b.scope
		}
		if a.scopeID != b.scopeID {
			return a.scopeID < b.scopeID
		}
		if a.subjectKind != b.subjectKind {
			return a.subjectKind < b.subjectKind
		}
		if a.subject != b.subject {
			return a.subject < b.subject
		}
		return a.permission < b.permission
	})
	for _, key := range keys {
		snapshot.Decisions = append(snapshot.Decisions, &corev1.RBACDecisionSnapshot{Scope: string(key.scope), ScopeId: key.scopeID, SubjectKind: key.subjectKind, Subject: key.subject, Permission: string(key.permission), Decision: string(p.decisions[key])})
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *RBACProjection) Restore(data []byte) error {
	snapshot := &corev1.RBACProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal RBAC snapshot: %w", err)
		}
	}
	guard, err := restoreReplayGuard(snapshot.GetReplayGuard())
	if err != nil {
		return fmt.Errorf("RBAC snapshot replay guard: %w", err)
	}
	roles := make(map[string]*corev1.Role, len(snapshot.GetRoles()))
	for _, role := range snapshot.GetRoles() {
		if role.GetName() == "" {
			return fmt.Errorf("RBAC snapshot has empty role name")
		}
		if _, duplicate := roles[role.GetName()]; duplicate {
			return fmt.Errorf("RBAC snapshot repeats role %q", role.GetName())
		}
		roles[role.GetName()] = proto.Clone(role).(*corev1.Role)
	}
	assignments := make(map[string]map[string]struct{}, len(snapshot.GetAssignments()))
	for _, assignment := range snapshot.GetAssignments() {
		if assignment.GetUserId() == "" {
			return fmt.Errorf("RBAC snapshot has empty assignment user ID")
		}
		if _, duplicate := assignments[assignment.GetUserId()]; duplicate {
			return fmt.Errorf("RBAC snapshot repeats assignments for user %q", assignment.GetUserId())
		}
		set := make(map[string]struct{}, len(assignment.GetRoleNames()))
		for _, roleName := range assignment.GetRoleNames() {
			if roleName == "" {
				return fmt.Errorf("RBAC snapshot has empty assigned role")
			}
			if _, duplicate := set[roleName]; duplicate {
				return fmt.Errorf("RBAC snapshot repeats assigned role %q", roleName)
			}
			set[roleName] = struct{}{}
		}
		assignments[assignment.GetUserId()] = set
	}
	decisions := make(map[rbacDecisionKey]DecisionKind, len(snapshot.GetDecisions()))
	for _, row := range snapshot.GetDecisions() {
		scope := PermissionScope(row.GetScope())
		if scope != ScopeServer && scope != ScopeGroup && scope != ScopeRoom {
			return fmt.Errorf("RBAC snapshot has invalid scope %q", scope)
		}
		if scope != ScopeServer && row.GetScopeId() == "" {
			return fmt.Errorf("RBAC snapshot has empty scoped object ID")
		}
		if row.GetSubjectKind() == corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_UNSPECIFIED || row.GetSubject() == "" || row.GetPermission() == "" {
			return fmt.Errorf("RBAC snapshot has invalid decision")
		}
		decision := DecisionKind(row.GetDecision())
		if decision != DecisionAllow && decision != DecisionDeny {
			return fmt.Errorf("RBAC snapshot has invalid decision %q", decision)
		}
		key := rbacDecisionKey{scope: scope, scopeID: row.GetScopeId(), subjectKind: row.GetSubjectKind(), subject: row.GetSubject(), permission: Permission(row.GetPermission())}
		if _, duplicate := decisions[key]; duplicate {
			return fmt.Errorf("RBAC snapshot repeats decision")
		}
		decisions[key] = decision
	}
	p.Lock()
	p.roles, p.assignments, p.decisions, p.replayGuard = roles, assignments, decisions, guard
	p.Unlock()
	return nil
}
