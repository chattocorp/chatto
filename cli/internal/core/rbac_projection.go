package core

import (
	"sort"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RBACProjection derives deployment-wide roles, role assignments, and
// explicit permission decisions from durable evt.rbac.> events.
type RBACProjection struct {
	events.MemoryProjection
	roles       map[string]*corev1.Role
	assignments map[string]map[string]struct{} // userID -> roleName set
	decisions   map[rbacDecisionKey]DecisionKind
	eventIDSeen map[string]struct{}
}

type rbacDecisionKey struct {
	scope       PermissionScope
	scopeID     string
	subjectKind corev1.RbacPermissionSubjectKind
	subject     string
	permission  Permission
}

func NewRBACProjection() *RBACProjection {
	return &RBACProjection{
		roles:       make(map[string]*corev1.Role),
		assignments: make(map[string]map[string]struct{}),
		decisions:   make(map[rbacDecisionKey]DecisionKind),
		eventIDSeen: make(map[string]struct{}),
	}
}

func (p *RBACProjection) Subjects() []string {
	return []string{events.RBACSubjectFilter()}
}

func (p *RBACProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	if id := event.GetId(); id != "" {
		p.Lock()
		if _, ok := p.eventIDSeen[id]; ok {
			p.Unlock()
			return nil
		}
		p.eventIDSeen[id] = struct{}{}
		p.Unlock()
	}

	p.Lock()
	defer p.Unlock()

	switch e := event.GetEvent().(type) {
	case *corev1.Event_RbacRoleCreated:
		p.applyRoleUpsert(rbacRoleFromCreated(e.RbacRoleCreated))
	case *corev1.Event_RbacRoleDisplayNameChanged:
		p.applyRoleDisplayNameChanged(e.RbacRoleDisplayNameChanged.GetRoleName(), e.RbacRoleDisplayNameChanged.GetDisplayName())
	case *corev1.Event_RbacRoleDescriptionChanged:
		p.applyRoleDescriptionChanged(e.RbacRoleDescriptionChanged.GetRoleName(), e.RbacRoleDescriptionChanged.GetDescription())
	case *corev1.Event_RbacRolePingableChanged:
		p.applyRolePingableChanged(e.RbacRolePingableChanged.GetRoleName(), e.RbacRolePingableChanged.GetPingable())
	case *corev1.Event_RbacRoleDeleted:
		p.applyRoleDeleted(e.RbacRoleDeleted.GetRoleName())
	case *corev1.Event_RbacRolesReordered:
		p.applyRolesReordered(e.RbacRolesReordered.GetRoleNames())
	case *corev1.Event_RbacRoleAssigned:
		p.applyRoleAssigned(e.RbacRoleAssigned.GetUserId(), e.RbacRoleAssigned.GetRoleName())
	case *corev1.Event_RbacRoleRevoked:
		p.applyRoleRevoked(e.RbacRoleRevoked.GetUserId(), e.RbacRoleRevoked.GetRoleName())
	case *corev1.Event_RbacPermissionGranted:
		p.applyPermissionDecision(
			e.RbacPermissionGranted.GetScope(),
			e.RbacPermissionGranted.GetSubject(),
			e.RbacPermissionGranted.GetPermission(),
			DecisionAllow,
			e.RbacPermissionGranted,
		)
	case *corev1.Event_RbacPermissionDenied:
		p.applyPermissionDecision(
			e.RbacPermissionDenied.GetScope(),
			e.RbacPermissionDenied.GetSubject(),
			e.RbacPermissionDenied.GetPermission(),
			DecisionDeny,
			e.RbacPermissionDenied,
		)
	case *corev1.Event_RbacPermissionCleared:
		p.applyPermissionCleared(
			e.RbacPermissionCleared.GetScope(),
			e.RbacPermissionCleared.GetSubject(),
			e.RbacPermissionCleared.GetPermission(),
			e.RbacPermissionCleared,
		)
	}
	return nil
}

func rbacRoleFromCreated(event *corev1.RbacRoleCreatedEvent) *corev1.Role {
	if event == nil {
		return nil
	}
	return &corev1.Role{
		Name:        event.GetRoleName(),
		DisplayName: event.GetDisplayName(),
		Description: event.GetDescription(),
		Position:    event.GetRank(),
		Pingable:    event.GetPingable(),
	}
}

func (p *RBACProjection) applyRoleUpsert(role *corev1.Role) {
	if role == nil || role.GetName() == "" {
		return
	}
	p.roles[role.GetName()] = proto.Clone(role).(*corev1.Role)
}

func (p *RBACProjection) applyRoleDisplayNameChanged(roleName, displayName string) {
	if roleName == "" {
		return
	}
	role := p.roles[roleName]
	if role == nil {
		return
	}
	updated := proto.Clone(role).(*corev1.Role)
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
	updated := proto.Clone(role).(*corev1.Role)
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
	updated := proto.Clone(role).(*corev1.Role)
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
		updated := proto.Clone(role).(*corev1.Role)
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
		if key.subjectKind == corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE && key.subject == roleName {
			delete(p.decisions, key)
		}
	}
}

func (p *RBACProjection) applyRoleAssigned(userID, roleName string) {
	if userID == "" || roleName == "" {
		return
	}
	if p.assignments[userID] == nil {
		p.assignments[userID] = make(map[string]struct{})
	}
	p.assignments[userID][roleName] = struct{}{}
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

func (p *RBACProjection) applyPermissionDecision(scope *corev1.RbacPermissionScope, subject *corev1.RbacPermissionSubject, permission string, decision DecisionKind, legacy proto.Message) {
	key, ok := rbacDecisionKeyFromFields(scope, subject, permission)
	if !ok {
		key, ok = legacyRBACDecisionKeyFromUnknown(legacy, permission)
	}
	if !ok {
		return
	}
	p.decisions[key] = decision
}

func (p *RBACProjection) applyPermissionCleared(scope *corev1.RbacPermissionScope, subject *corev1.RbacPermissionSubject, permission string, legacy proto.Message) {
	key, ok := rbacDecisionKeyFromFields(scope, subject, permission)
	if !ok {
		key, ok = legacyRBACDecisionKeyFromUnknown(legacy, permission)
	}
	if !ok {
		return
	}
	delete(p.decisions, key)
}

func rbacDecisionKeyFromFields(scope *corev1.RbacPermissionScope, subject *corev1.RbacPermissionSubject, permission string) (rbacDecisionKey, bool) {
	if scope == nil || subject == nil || subject.GetId() == "" || permission == "" {
		return rbacDecisionKey{}, false
	}
	if subject.GetKind() == corev1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_UNSPECIFIED {
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

func permissionScopeFromProto(scope *corev1.RbacPermissionScope) (PermissionScope, bool) {
	if scope == nil {
		return "", false
	}
	switch scope.GetKind() {
	case corev1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_SERVER:
		return ScopeServer, true
	case corev1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_GROUP:
		return ScopeGroup, scope.GetId() != ""
	case corev1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_ROOM:
		return ScopeRoom, scope.GetId() != ""
	default:
		return "", false
	}
}

func rbacDecisionKeyFor(scope PermissionScope, scopeID, subject string, perm Permission) rbacDecisionKey {
	if scope == ScopeServer {
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

func (p *RBACProjection) GetRole(name string) (*corev1.Role, bool) {
	p.RLock()
	defer p.RUnlock()
	role := p.roles[name]
	if role == nil {
		return nil, false
	}
	return proto.Clone(role).(*corev1.Role), true
}

func (p *RBACProjection) RoleExists(name string) bool {
	p.RLock()
	defer p.RUnlock()
	return p.roles[name] != nil
}

func (p *RBACProjection) ListRoles() []*corev1.Role {
	p.RLock()
	defer p.RUnlock()
	roles := make([]*corev1.Role, 0, len(p.roles))
	for _, role := range p.roles {
		roles = append(roles, proto.Clone(role).(*corev1.Role))
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

func (p *RBACProjection) RolePosition(roleName string) (int32, bool) {
	p.RLock()
	defer p.RUnlock()
	role := p.roles[roleName]
	if role == nil {
		return PositionEveryone, false
	}
	return role.GetPosition(), true
}

func (p *RBACProjection) GetUserHighestPosition(userID string) int32 {
	p.RLock()
	defer p.RUnlock()
	maxPos := PositionEveryone
	for roleName := range p.assignments[userID] {
		if role := p.roles[roleName]; role != nil && role.GetPosition() > maxPos {
			maxPos = role.GetPosition()
		}
	}
	return maxPos
}

func (p *RBACProjection) RolesWithPositionsForUser(userID string) []roleWithPosition {
	p.RLock()
	defer p.RUnlock()
	result := make([]roleWithPosition, 0, len(p.assignments[userID])+1)
	for roleName := range p.assignments[userID] {
		pos := PositionEveryone
		if role := p.roles[roleName]; role != nil {
			pos = role.GetPosition()
		}
		result = append(result, roleWithPosition{name: roleName, position: pos})
	}
	if _, ok := p.assignments[userID][RoleEveryone]; !ok {
		result = append(result, roleWithPosition{name: RoleEveryone, position: PositionEveryone})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].position != result[j].position {
			return result[i].position > result[j].position
		}
		return result[i].name < result[j].name
	})
	return result
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

func (p *RBACProjection) CountStats() (roles int, assignments int, decisions int) {
	p.RLock()
	defer p.RUnlock()
	for name := range p.roles {
		if strings.TrimSpace(name) != "" {
			roles++
		}
	}
	for _, roleSet := range p.assignments {
		assignments += len(roleSet)
	}
	return roles, assignments, len(p.decisions)
}

func sortPermissions(perms []Permission) {
	sort.Slice(perms, func(i, j int) bool { return perms[i] < perms[j] })
}
