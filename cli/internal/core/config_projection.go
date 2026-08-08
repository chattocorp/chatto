package core

import (
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const ConfigSubjectServer = "server"

// ConfigProjection consumes first-party configuration/preference events from
// EVT and keeps the current server/user settings in memory. It also understands
// legacy UserServerPreferencesChangedEvent events so older EVT streams keep
// projecting correctly.
type ConfigProjection struct {
	events.MemoryProjection
	server   serverConfigState
	users    map[string]*userConfigState
	policies map[runtimePolicyTargetKey]*runtimePolicyState
}

type runtimePolicyTargetKey struct {
	scopeKind   corev1.RuntimePolicyScopeKind
	scopeID     string
	subjectKind corev1.RuntimePolicySubjectKind
	subjectID   string
}

type runtimePolicyState struct {
	authorEditWindowSeconds *int32
}

type serverConfigState struct {
	serverName       string
	description      string
	welcomeMessage   string
	motd             string
	blockedUsernames *string
	logo             *corev1.AssetRecord
	banner           *corev1.AssetRecord
}

type userConfigState struct {
	timezone        *string
	timeFormat      *corev1.TimeFormat
	serverLevel     *corev1.NotificationLevel
	roomLevelByRoom map[string]corev1.NotificationLevel
}

func NewConfigProjection() *ConfigProjection {
	return &ConfigProjection{
		users:    make(map[string]*userConfigState),
		policies: make(map[runtimePolicyTargetKey]*runtimePolicyState),
	}
}

func (p *ConfigProjection) Subjects() []string {
	return []string{
		evtstream.ConfigSubjectFilter(),
		evtstream.UserEventTypeFilter(evtstream.EventUserServerPreferencesChanged),
		evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted),
		evtstream.RoomEventTypeFilter(evtstream.EventRoomDeleted),
		evtstream.GroupEventTypeFilter(evtstream.EventRoomGroupDeleted),
	}
}

func (p *ConfigProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	switch e := event.GetEvent().(type) {
	case *corev1.Event_ServerNameChanged:
		p.server.serverName = e.ServerNameChanged.GetName()
	case *corev1.Event_ServerDescriptionChanged:
		p.server.description = e.ServerDescriptionChanged.GetDescription()
	case *corev1.Event_ServerWelcomeMessageChanged:
		p.server.welcomeMessage = e.ServerWelcomeMessageChanged.GetWelcomeMessage()
	case *corev1.Event_ServerMotdChanged:
		p.server.motd = e.ServerMotdChanged.GetMotd()
	case *corev1.Event_ServerBlockedUsernamesChanged:
		blocked := e.ServerBlockedUsernamesChanged.GetBlockedUsernames()
		p.server.blockedUsernames = &blocked
	case *corev1.Event_ServerLogoSet:
		p.server.logo = cloneAssetRecord(e.ServerLogoSet.GetAsset())
	case *corev1.Event_ServerLogoCleared:
		p.server.logo = nil
	case *corev1.Event_ServerBannerSet:
		p.server.banner = cloneAssetRecord(e.ServerBannerSet.GetAsset())
	case *corev1.Event_ServerBannerCleared:
		p.server.banner = nil
	case *corev1.Event_UserTimezoneChanged:
		u := p.ensureUserLocked(e.UserTimezoneChanged.GetUserId())
		tz := e.UserTimezoneChanged.GetTimezone()
		u.timezone = &tz
	case *corev1.Event_UserTimezoneCleared:
		p.ensureUserLocked(e.UserTimezoneCleared.GetUserId()).timezone = nil
	case *corev1.Event_UserTimeFormatChanged:
		u := p.ensureUserLocked(e.UserTimeFormatChanged.GetUserId())
		tf := e.UserTimeFormatChanged.GetTimeFormat()
		u.timeFormat = &tf
	case *corev1.Event_UserTimeFormatCleared:
		p.ensureUserLocked(e.UserTimeFormatCleared.GetUserId()).timeFormat = nil
	case *corev1.Event_UserServerNotificationLevelSet:
		u := p.ensureUserLocked(e.UserServerNotificationLevelSet.GetUserId())
		level := e.UserServerNotificationLevelSet.GetLevel()
		u.serverLevel = &level
	case *corev1.Event_UserServerNotificationLevelCleared:
		p.ensureUserLocked(e.UserServerNotificationLevelCleared.GetUserId()).serverLevel = nil
	case *corev1.Event_UserRoomNotificationLevelSet:
		u := p.ensureUserLocked(e.UserRoomNotificationLevelSet.GetUserId())
		if u.roomLevelByRoom == nil {
			u.roomLevelByRoom = make(map[string]corev1.NotificationLevel)
		}
		u.roomLevelByRoom[e.UserRoomNotificationLevelSet.GetRoomId()] = e.UserRoomNotificationLevelSet.GetLevel()
	case *corev1.Event_UserRoomNotificationLevelCleared:
		if u := p.users[e.UserRoomNotificationLevelCleared.GetUserId()]; u != nil {
			delete(u.roomLevelByRoom, e.UserRoomNotificationLevelCleared.GetRoomId())
		}
	case *corev1.Event_UserServerPreferencesChanged:
		p.applyLegacyUserPreferencesLocked(e.UserServerPreferencesChanged)
	case *corev1.Event_UserAccountDeleted:
		delete(p.users, e.UserAccountDeleted.GetUserId())
	case *corev1.Event_AuthorEditWindowSet:
		target := runtimePolicyKey(e.AuthorEditWindowSet.GetTarget())
		state := p.ensurePolicyLocked(target)
		seconds := e.AuthorEditWindowSet.GetSeconds()
		state.authorEditWindowSeconds = &seconds
	case *corev1.Event_AuthorEditWindowCleared:
		target := runtimePolicyKey(e.AuthorEditWindowCleared.GetTarget())
		if state := p.policies[target]; state != nil {
			state.authorEditWindowSeconds = nil
			p.removeEmptyPolicyLocked(target, state)
		}
	case *corev1.Event_RoomDeleted:
		p.removePolicyScopeLocked(corev1.RuntimePolicyScopeKind_RUNTIME_POLICY_SCOPE_KIND_ROOM, e.RoomDeleted.GetRoomId())
	case *corev1.Event_RoomGroupDeleted:
		p.removePolicyScopeLocked(corev1.RuntimePolicyScopeKind_RUNTIME_POLICY_SCOPE_KIND_ROOM_GROUP, e.RoomGroupDeleted.GetGroupId())
	}
	return nil
}

func runtimePolicyKey(target *corev1.RuntimePolicyTarget) runtimePolicyTargetKey {
	if target == nil {
		return runtimePolicyTargetKey{}
	}
	return runtimePolicyTargetKey{
		scopeKind: target.GetScopeKind(), scopeID: target.GetScopeId(),
		subjectKind: target.GetSubjectKind(), subjectID: target.GetSubjectId(),
	}
}

func (p *ConfigProjection) ensurePolicyLocked(target runtimePolicyTargetKey) *runtimePolicyState {
	if p.policies == nil {
		p.policies = make(map[runtimePolicyTargetKey]*runtimePolicyState)
	}
	state := p.policies[target]
	if state == nil {
		state = &runtimePolicyState{}
		p.policies[target] = state
	}
	return state
}

func (p *ConfigProjection) removeEmptyPolicyLocked(target runtimePolicyTargetKey, state *runtimePolicyState) {
	if state != nil && state.authorEditWindowSeconds == nil {
		delete(p.policies, target)
	}
}

func (p *ConfigProjection) removePolicyScopeLocked(scopeKind corev1.RuntimePolicyScopeKind, scopeID string) {
	for target := range p.policies {
		if target.scopeKind == scopeKind && target.scopeID == scopeID {
			delete(p.policies, target)
		}
	}
}

func (p *ConfigProjection) policyState(target runtimePolicyTargetKey) runtimePolicyState {
	p.RLock()
	defer p.RUnlock()
	state := p.policies[target]
	if state == nil {
		return runtimePolicyState{}
	}
	result := runtimePolicyState{}
	if state.authorEditWindowSeconds != nil {
		value := *state.authorEditWindowSeconds
		result.authorEditWindowSeconds = &value
	}
	return result
}

func (p *ConfigProjection) ensureUserLocked(userID string) *userConfigState {
	if p.users == nil {
		p.users = make(map[string]*userConfigState)
	}
	u := p.users[userID]
	if u == nil {
		u = &userConfigState{}
		p.users[userID] = u
	}
	return u
}

func (p *ConfigProjection) applyLegacyUserPreferencesLocked(e *corev1.UserServerPreferencesChangedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	prefs := e.GetPreferences()
	if prefs == nil {
		u.timezone = nil
		u.timeFormat = nil
		return
	}
	if prefs.GetTimezone() != "" {
		tz := prefs.GetTimezone()
		u.timezone = &tz
	} else {
		u.timezone = nil
	}
	tf := prefs.GetTimeFormat()
	u.timeFormat = &tf
}
