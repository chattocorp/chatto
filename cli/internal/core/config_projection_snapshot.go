package core

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var configSnapshotContractID = snapshotContractID("v1", &corev1.ConfigProjectionSnapshot{})

func (*ConfigProjection) SnapshotContractID() string { return configSnapshotContractID }

func (p *ConfigProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	snapshot := &corev1.ConfigProjectionSnapshot{ServerName: p.server.serverName, Description: p.server.description, WelcomeMessage: p.server.welcomeMessage, Motd: p.server.motd, Logo: cloneAssetRecord(p.server.logo), Banner: cloneAssetRecord(p.server.banner)}
	if p.server.blockedUsernames != nil {
		value := *p.server.blockedUsernames
		snapshot.BlockedUsernames = &value
	}
	for _, userID := range sortedMapKeys(p.users) {
		user := p.users[userID]
		row := &corev1.UserConfigSnapshot{UserId: userID}
		if user.timezone != nil {
			value := *user.timezone
			row.Timezone = &value
		}
		if user.timeFormat != nil {
			value := *user.timeFormat
			row.TimeFormat = &value
		}
		if user.serverLevel != nil {
			value := *user.serverLevel
			row.ServerNotificationLevel = &value
		}
		for _, roomID := range sortedMapKeys(user.roomLevelByRoom) {
			row.RoomNotificationLevels = append(row.RoomNotificationLevels, &corev1.RoomNotificationLevelSnapshot{RoomId: roomID, Level: user.roomLevelByRoom[roomID]})
		}
		snapshot.Users = append(snapshot.Users, row)
	}
	policyTargets := make([]runtimePolicyTargetKey, 0, len(p.policies))
	for target := range p.policies {
		policyTargets = append(policyTargets, target)
	}
	sort.Slice(policyTargets, func(i, j int) bool {
		a, b := policyTargets[i], policyTargets[j]
		if a.scopeKind != b.scopeKind {
			return a.scopeKind < b.scopeKind
		}
		if a.scopeID != b.scopeID {
			return a.scopeID < b.scopeID
		}
		if a.subjectKind != b.subjectKind {
			return a.subjectKind < b.subjectKind
		}
		return a.subjectID < b.subjectID
	})
	for _, target := range policyTargets {
		state := p.policies[target]
		row := &corev1.RuntimePolicySnapshot{Target: &corev1.RuntimePolicyTarget{
			ScopeKind: target.scopeKind, ScopeId: target.scopeID,
			SubjectKind: target.subjectKind, SubjectId: target.subjectID,
		}}
		if state.authorEditWindowSeconds != nil {
			value := *state.authorEditWindowSeconds
			row.AuthorEditWindowSeconds = &value
		}
		snapshot.Policies = append(snapshot.Policies, row)
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *ConfigProjection) Restore(data []byte) error {
	snapshot := &corev1.ConfigProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal config snapshot: %w", err)
		}
	}
	server := serverConfigState{serverName: snapshot.GetServerName(), description: snapshot.GetDescription(), welcomeMessage: snapshot.GetWelcomeMessage(), motd: snapshot.GetMotd(), logo: cloneAssetRecord(snapshot.GetLogo()), banner: cloneAssetRecord(snapshot.GetBanner())}
	if snapshot.BlockedUsernames != nil {
		value := snapshot.GetBlockedUsernames()
		server.blockedUsernames = &value
	}
	users := make(map[string]*userConfigState, len(snapshot.GetUsers()))
	for _, row := range snapshot.GetUsers() {
		if row.GetUserId() == "" {
			return fmt.Errorf("config snapshot has empty user ID")
		}
		if _, duplicate := users[row.GetUserId()]; duplicate {
			return fmt.Errorf("config snapshot repeats user %q", row.GetUserId())
		}
		user := &userConfigState{roomLevelByRoom: make(map[string]corev1.NotificationLevel)}
		if row.Timezone != nil {
			value := row.GetTimezone()
			user.timezone = &value
		}
		if row.TimeFormat != nil {
			value := row.GetTimeFormat()
			user.timeFormat = &value
		}
		if row.ServerNotificationLevel != nil {
			value := row.GetServerNotificationLevel()
			user.serverLevel = &value
		}
		for _, level := range row.GetRoomNotificationLevels() {
			if level.GetRoomId() == "" {
				return fmt.Errorf("config snapshot has empty notification room ID")
			}
			if _, duplicate := user.roomLevelByRoom[level.GetRoomId()]; duplicate {
				return fmt.Errorf("config snapshot repeats room notification level")
			}
			user.roomLevelByRoom[level.GetRoomId()] = level.GetLevel()
		}
		users[row.GetUserId()] = user
	}
	policies := make(map[runtimePolicyTargetKey]*runtimePolicyState, len(snapshot.GetPolicies()))
	for _, row := range snapshot.GetPolicies() {
		target := runtimePolicyKey(row.GetTarget())
		if target.scopeKind == corev1.RuntimePolicyScopeKind_RUNTIME_POLICY_SCOPE_KIND_UNSPECIFIED ||
			target.subjectKind == corev1.RuntimePolicySubjectKind_RUNTIME_POLICY_SUBJECT_KIND_UNSPECIFIED {
			return fmt.Errorf("config snapshot has invalid policy target")
		}
		if _, duplicate := policies[target]; duplicate {
			return fmt.Errorf("config snapshot repeats policy target")
		}
		state := &runtimePolicyState{}
		if row.AuthorEditWindowSeconds != nil {
			value := row.GetAuthorEditWindowSeconds()
			state.authorEditWindowSeconds = &value
		}
		policies[target] = state
	}
	p.Lock()
	p.server, p.users, p.policies = server, users, policies
	p.Unlock()
	return nil
}
