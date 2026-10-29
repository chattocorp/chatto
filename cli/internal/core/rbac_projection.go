package core

import (
	"slices"
	"sort"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// RBACProjection derives deployment-wide roles, role assignments, and
// explicit permission decisions from durable evt.rbac.> events.
type RBACProjection struct {
	events.MemoryProjection
	roles map[string]*readmodel.Role
	// assignments maps userID -> roleName -> owner. The owner is the identity
	// issuer of the auth provider whose group_roles mapping created the
	// assignment, or empty for an admin or configuration assignment. Only the
	// owning provider revokes an assignment (FDR-050).
	assignments map[string]map[string]string
	decisions   map[rbacDecisionKey]DecisionKind
	replayGuard projectionReplayGuard
}

type rbacDecisionKey struct {
	scope       PermissionScope
	scopeID     string
	subjectKind evtv1.RbacPermissionSubjectKind
	subject     string
	permission  Permission
}

// ScopedRolePermissionDecision is one explicit permission decision carried by
// a role. It is used to keep delegated role assignment within the caller's own
// authority without treating display order as an authorization rank.
type ScopedRolePermissionDecision struct {
	Scope      PermissionScope
	ScopeID    string
	Permission Permission
	Decision   DecisionKind
}

func NewRBACProjection() *RBACProjection {
	return &RBACProjection{
		roles:       make(map[string]*readmodel.Role),
		assignments: make(map[string]map[string]string),
		decisions:   make(map[rbacDecisionKey]DecisionKind),
		replayGuard: newProjectionReplayGuard(),
	}
}

func (p *RBACProjection) Subjects() []string {
	return []string{evtstream.RBACSubjectFilter()}
}

func (p *RBACProjection) Apply(event *evtv1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	if p.replayGuard.seenOrMark(event, seq) {
		return nil
	}

	switch e := event.GetEvent().(type) {
	case *evtv1.Event_RbacRoleCreated:
		p.applyRoleUpsert(rbacRoleFromCreated(e.RbacRoleCreated))
	case *evtv1.Event_RbacRoleDisplayNameChanged:
		p.applyRoleDisplayNameChanged(e.RbacRoleDisplayNameChanged.GetRoleName(), e.RbacRoleDisplayNameChanged.GetDisplayName())
	case *evtv1.Event_RbacRoleDescriptionChanged:
		p.applyRoleDescriptionChanged(e.RbacRoleDescriptionChanged.GetRoleName(), e.RbacRoleDescriptionChanged.GetDescription())
	case *evtv1.Event_RbacRolePingableChanged:
		p.applyRolePingableChanged(e.RbacRolePingableChanged.GetRoleName(), e.RbacRolePingableChanged.GetPingable())
	case *evtv1.Event_RbacRoleDeleted:
		p.applyRoleDeleted(e.RbacRoleDeleted.GetRoleName())
	case *evtv1.Event_RbacRolesReordered:
		p.applyRolesReordered(e.RbacRolesReordered.GetRoleNames())
	case *evtv1.Event_RbacRoleAssigned:
		p.applyRoleAssigned(e.RbacRoleAssigned.GetUserId(), e.RbacRoleAssigned.GetRoleName(), e.RbacRoleAssigned.GetReason().GetProviderGroups().GetIssuer())
	case *evtv1.Event_RbacRoleRevoked:
		p.applyRoleRevoked(e.RbacRoleRevoked.GetUserId(), e.RbacRoleRevoked.GetRoleName())
	case *evtv1.Event_RbacPermissionGranted:
		p.applyPermissionDecision(
			e.RbacPermissionGranted.GetScope(),
			e.RbacPermissionGranted.GetSubject(),
			e.RbacPermissionGranted.GetPermission(),
			DecisionAllow,
			e.RbacPermissionGranted,
		)
	case *evtv1.Event_RbacPermissionDenied:
		p.applyPermissionDecision(
			e.RbacPermissionDenied.GetScope(),
			e.RbacPermissionDenied.GetSubject(),
			e.RbacPermissionDenied.GetPermission(),
			DecisionDeny,
			e.RbacPermissionDenied,
		)
	case *evtv1.Event_RbacPermissionCleared:
		p.applyPermissionCleared(
			e.RbacPermissionCleared.GetScope(),
			e.RbacPermissionCleared.GetSubject(),
			e.RbacPermissionCleared.GetPermission(),
			e.RbacPermissionCleared,
		)
	}
	return nil
}

func (p *RBACProjection) CompleteStartupReplay() {
	p.Lock()
	defer p.Unlock()
	p.replayGuard.completeReplay()
}

func rbacRoleFromCreated(event *evtv1.RbacRoleCreatedEvent) *readmodel.Role {
	if event == nil {
		return nil
	}
	return &readmodel.Role{
		Name:        event.GetRoleName(),
		DisplayName: event.GetDisplayName(),
		Description: event.GetDescription(),
		Position:    event.GetRank(),
		Pingable:    event.GetPingable(),
	}
}

func (p *RBACProjection) applyRoleUpsert(role *readmodel.Role) {
	if role == nil || role.GetName() == "" {
		return
	}
	p.roles[role.GetName()] = role.Clone()
}

func (p *RBACProjection) applyRoleDisplayNameChanged(roleName, displayName string) {
	if roleName == "" {
		return
	}
	role := p.roles[roleName]
	if role == nil {
		return
	}
	updated := role.Clone()
	updated.DisplayName = displayName
	p.roles[roleName] = updated
}

func (p *RBACProjection) applyRoleDescriptionChanged(roleName, description string) {
	if roleName == "" {
		return
	}
	role := p.roles[roleName]
	if role == nil {
		return
	}
	updated := role.Clone()
	updated.Description = description
	p.roles[roleName] = updated
}

func (p *RBACProjection) applyRolePingableChanged(roleName string, pingable bool) {
	if roleName == "" {
		return
	}
	role := p.roles[roleName]
	if role == nil {
		return
	}
	updated := role.Clone()
	updated.Pingable = pingable
	p.roles[roleName] = updated
}

func (p *RBACProjection) applyRolesReordered(roleNames []string) {
	position := PositionCustomFirst
	for _, roleName := range roleNames {
		role := p.roles[roleName]
		if role == nil || IsSystemRole(roleName) {
			continue
		}
		for isSystemPosition(position) {
			position++
		}
		updated := role.Clone()
		updated.Position = position
		p.roles[roleName] = updated
		position++
	}
}

func (p *RBACProjection) applyRoleDeleted(roleName string) {
	if roleName == "" {
		return
	}
	delete(p.roles, roleName)
	for userID, roles := range p.assignments {
		delete(roles, roleName)
		if len(roles) == 0 {
			delete(p.assignments, userID)
		}
	}
	for key := range p.decisions {
		if key.subjectKind == evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE && key.subject == roleName {
			delete(p.decisions, key)
		}
	}
}

// applyRoleAssigned records an assignment and its owner. A repeated
// assignment keeps the owner of the assignment that already exists.
func (p *RBACProjection) applyRoleAssigned(userID, roleName, ownerIssuer string) {
	if userID == "" || roleName == "" {
		return
	}
	if p.assignments[userID] == nil {
		p.assignments[userID] = make(map[string]string)
	}
	if _, assigned := p.assignments[userID][roleName]; assigned {
		return
	}
	p.assignments[userID][roleName] = ownerIssuer
}

func (p *RBACProjection) applyRoleRevoked(userID, roleName string) {
	if userID == "" || roleName == "" {
		return
	}
	delete(p.assignments[userID], roleName)
	if len(p.assignments[userID]) == 0 {
		delete(p.assignments, userID)
	}
}

func (p *RBACProjection) applyPermissionDecision(scope *evtv1.RbacPermissionScope, subject *evtv1.RbacPermissionSubject, permission string, decision DecisionKind, legacy proto.Message) {
	key, ok := rbacDecisionKeyFromFields(scope, subject, permission)
	if !ok {
		key, ok = legacyRBACDecisionKeyFromUnknown(legacy, permission)
	}
	if !ok {
		return
	}
	p.decisions[key] = decision
}

func (p *RBACProjection) applyPermissionCleared(scope *evtv1.RbacPermissionScope, subject *evtv1.RbacPermissionSubject, permission string, legacy proto.Message) {
	key, ok := rbacDecisionKeyFromFields(scope, subject, permission)
	if !ok {
		key, ok = legacyRBACDecisionKeyFromUnknown(legacy, permission)
	}
	if !ok {
		return
	}
	delete(p.decisions, key)
}

func rbacDecisionKeyFromFields(scope *evtv1.RbacPermissionScope, subject *evtv1.RbacPermissionSubject, permission string) (rbacDecisionKey, bool) {
	if scope == nil || subject == nil || subject.GetId() == "" || permission == "" {
		return rbacDecisionKey{}, false
	}
	if subject.GetKind() == evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_UNSPECIFIED {
		return rbacDecisionKey{}, false
	}
	permScope, ok := permissionScopeFromProto(scope)
	if !ok {
		return rbacDecisionKey{}, false
	}
	scopeID := scope.GetId()
	if permScope == ScopeServer {
		scopeID = ""
	}
	return rbacDecisionKey{
		scope:       permScope,
		scopeID:     scopeID,
		subjectKind: subject.GetKind(),
		subject:     subject.GetId(),
		permission:  Permission(permission),
	}, true
}

func legacyRBACDecisionKeyFromUnknown(msg proto.Message, permission string) (rbacDecisionKey, bool) {
	if msg == nil || permission == "" {
		return rbacDecisionKey{}, false
	}
	var location, subject string
	unknown := msg.ProtoReflect().GetUnknown()
	for len(unknown) > 0 {
		num, typ, n := protowire.ConsumeTag(unknown)
		if n < 0 {
			return rbacDecisionKey{}, false
		}
		unknown = unknown[n:]
		if typ == protowire.BytesType && (num == 1 || num == 2) {
			value, m := protowire.ConsumeString(unknown)
			if m < 0 {
				return rbacDecisionKey{}, false
			}
			if num == 1 {
				location = value
			} else {
				subject = value
			}
			unknown = unknown[m:]
			continue
		}
		m := protowire.ConsumeFieldValue(num, typ, unknown)
		if m < 0 {
			return rbacDecisionKey{}, false
		}
		unknown = unknown[m:]
	}
	return rbacDecisionKeyFromLegacyFields(location, subject, permission)
}

func rbacDecisionKeyFromLegacyFields(location, subject, permission string) (rbacDecisionKey, bool) {
	if subject == "" || permission == "" {
		return rbacDecisionKey{}, false
	}
	if location == string(ScopeServer) {
		return rbacDecisionKey{
			scope:       ScopeServer,
			subjectKind: rbacPermissionSubjectKindForID(subject),
			subject:     subject,
			permission:  Permission(permission),
		}, true
	}
	scope, ok := rbacScopeFromLegacyLocation(location)
	if !ok {
		return rbacDecisionKey{}, false
	}
	return rbacDecisionKey{
		scope:       scope,
		scopeID:     location,
		subjectKind: rbacPermissionSubjectKindForID(subject),
		subject:     subject,
		permission:  Permission(permission),
	}, true
}

func rbacScopeFromLegacyLocation(location string) (PermissionScope, bool) {
	if location == "" {
		return "", false
	}
	switch location[0] {
	case 'G':
		return ScopeGroup, true
	case 'R':
		return ScopeRoom, true
	default:
		return "", false
	}
}

func permissionScopeFromProto(scope *evtv1.RbacPermissionScope) (PermissionScope, bool) {
	if scope == nil {
		return "", false
	}
	switch scope.GetKind() {
	case evtv1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_SERVER:
		return ScopeServer, true
	case evtv1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_GROUP:
		return ScopeGroup, scope.GetId() != ""
	case evtv1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_ROOM:
		return ScopeRoom, scope.GetId() != ""
	case evtv1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_DM:
		return ScopeDM, scope.GetId() == ""
	default:
		return "", false
	}
}

func rbacDecisionKeyFor(scope PermissionScope, scopeID, subject string, perm Permission) rbacDecisionKey {
	if scope == ScopeServer || scope == ScopeDM {
		scopeID = ""
	}
	return rbacDecisionKey{
		scope:       scope,
		scopeID:     scopeID,
		subjectKind: rbacPermissionSubjectKindForID(subject),
		subject:     subject,
		permission:  perm,
	}
}

func (p *RBACProjection) GetRole(name string) (*readmodel.Role, bool) {
	p.RLock()
	defer p.RUnlock()
	role := p.roles[name]
	if role == nil {
		return nil, false
	}
	return role.Clone(), true
}

func (p *RBACProjection) RoleExists(name string) bool {
	p.RLock()
	defer p.RUnlock()
	return p.roles[name] != nil
}

func (p *RBACProjection) ListRoles() []*readmodel.Role {
	p.RLock()
	defer p.RUnlock()
	roles := make([]*readmodel.Role, 0, len(p.roles))
	for _, role := range p.roles {
		roles = append(roles, role.Clone())
	}
	sort.SliceStable(roles, func(i, j int) bool {
		if roles[i].GetPosition() != roles[j].GetPosition() {
			return roles[i].GetPosition() < roles[j].GetPosition()
		}
		return roles[i].GetName() < roles[j].GetName()
	})
	return roles
}

func (p *RBACProjection) GetUserRoles(userID string) []string {
	p.RLock()
	defer p.RUnlock()
	roles := make([]string, 0, len(p.assignments[userID]))
	for roleName := range p.assignments[userID] {
		roles = append(roles, roleName)
	}
	sort.Strings(roles)
	return roles
}

func (p *RBACProjection) HasRole(userID, roleName string) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.assignments[userID][roleName]
	return ok
}

// ProviderAssignments returns the sorted role names of the user's assignments
// that the auth provider with identity issuer owns. An empty issuer matches
// nothing, because admin assignments have no owner.
func (p *RBACProjection) ProviderAssignments(userID, issuer string) []string {
	if issuer == "" {
		return nil
	}
	p.RLock()
	defer p.RUnlock()
	var roleNames []string
	for roleName, owner := range p.assignments[userID] {
		if owner == issuer {
			roleNames = append(roleNames, roleName)
		}
	}
	sort.Strings(roleNames)
	return roleNames
}

// AssignmentOwner returns the identity issuer of the auth provider that owns
// the user's assignment of roleName. It is empty for an admin or
// configuration assignment. assigned is false when the user does not have the
// role.
func (p *RBACProjection) AssignmentOwner(userID, roleName string) (ownerIssuer string, assigned bool) {
	p.RLock()
	defer p.RUnlock()
	ownerIssuer, assigned = p.assignments[userID][roleName]
	return ownerIssuer, assigned
}

func (p *RBACProjection) GetRoleUsers(roleName string) []string {
	p.RLock()
	defer p.RUnlock()
	users := make([]string, 0)
	for userID, roles := range p.assignments {
		if _, ok := roles[roleName]; ok {
			users = append(users, userID)
		}
	}
	sort.Strings(users)
	return users
}

func (p *RBACProjection) RolePermissionDecisions(roleName string) []ScopedRolePermissionDecision {
	p.RLock()
	defer p.RUnlock()
	decisions := make([]ScopedRolePermissionDecision, 0)
	for key, decision := range p.decisions {
		if key.subjectKind != evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE || key.subject != roleName {
			continue
		}
		decisions = append(decisions, ScopedRolePermissionDecision{
			Scope:      key.scope,
			ScopeID:    key.scopeID,
			Permission: key.permission,
			Decision:   decision,
		})
	}
	sort.Slice(decisions, func(i, j int) bool {
		if decisions[i].Scope != decisions[j].Scope {
			return decisions[i].Scope < decisions[j].Scope
		}
		if decisions[i].ScopeID != decisions[j].ScopeID {
			return decisions[i].ScopeID < decisions[j].ScopeID
		}
		return decisions[i].Permission < decisions[j].Permission
	})
	return decisions
}

func (p *RBACProjection) Assignments() []rbacSeedAssignment {
	p.RLock()
	defer p.RUnlock()
	assignments := make([]rbacSeedAssignment, 0)
	for userID, roles := range p.assignments {
		for roleName := range roles {
			assignments = append(assignments, rbacSeedAssignment{userID: userID, roleName: roleName})
		}
	}
	sort.Slice(assignments, func(i, j int) bool {
		if assignments[i].userID != assignments[j].userID {
			return assignments[i].userID < assignments[j].userID
		}
		return assignments[i].roleName < assignments[j].roleName
	})
	return assignments
}

func (p *RBACProjection) Decisions() []rbacSeedDecision {
	p.RLock()
	defer p.RUnlock()
	decisions := make([]rbacSeedDecision, 0, len(p.decisions))
	for key, decision := range p.decisions {
		decisions = append(decisions, rbacSeedDecision{
			scope:       key.scope,
			scopeID:     key.scopeID,
			subjectKind: key.subjectKind,
			subject:     key.subject,
			permission:  key.permission,
			decision:    decision,
		})
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
	return decisions
}

func (p *RBACProjection) GetDecision(scope PermissionScope, scopeID, subject string, perm Permission) DecisionKind {
	p.RLock()
	defer p.RUnlock()
	if decision, ok := p.decisions[rbacDecisionKeyFor(scope, scopeID, subject, perm)]; ok {
		return decision
	}
	return DecisionNone
}

func (p *RBACProjection) DecisionsFor(scope PermissionScope, scopeID, subject string) (grants []Permission, denials []Permission) {
	p.RLock()
	defer p.RUnlock()
	for key, decision := range p.decisions {
		if key.scope != scope || key.scopeID != scopeID || key.subject != subject {
			continue
		}
		switch decision {
		case DecisionAllow:
			grants = append(grants, key.permission)
		case DecisionDeny:
			denials = append(denials, key.permission)
		}
	}
	sortPermissions(grants)
	sortPermissions(denials)
	return grants, denials
}

func (p *RBACProjection) DecisionsForRoleServer(roleName string) (grants []Permission, denials []Permission) {
	return p.DecisionsFor(ScopeServer, "", roleName)
}

// HasAnyPrivilegedModeAllow reports whether the user, everyone, or one of the
// user's assigned roles has an explicit allow for an elevation-required
// permission at any scope. Effective authorization still resolves denies and
// scope precedence when the user performs an action.
func (p *RBACProjection) HasAnyPrivilegedModeAllow(userID string) bool {
	p.RLock()
	defer p.RUnlock()
	roles := p.assignments[userID]
	for key, decision := range p.decisions {
		if decision != DecisionAllow {
			continue
		}
		metadata, known := GetPermissionMetadata(key.permission)
		if !known || !metadata.RequiresPrivilegedMode {
			continue
		}
		if key.subjectKind == evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER && key.subject == userID {
			return true
		}
		if key.subjectKind == evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE && (key.subject == RoleEveryone) {
			return true
		}
		if key.subjectKind == evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE {
			if _, assigned := roles[key.subject]; assigned {
				return true
			}
		}
	}
	return false
}

func (p *RBACProjection) NextAvailablePosition() int32 {
	p.RLock()
	defer p.RUnlock()
	maxCustom := PositionEveryone
	for _, role := range p.roles {
		if role == nil || IsSystemRole(role.GetName()) {
			continue
		}
		if role.GetPosition() > maxCustom {
			maxCustom = role.GetPosition()
		}
	}
	next := maxCustom + 1
	for isSystemPosition(next) {
		next++
	}
	return next
}

func sortPermissions(perms []Permission) {
	slices.Sort(perms)
}
