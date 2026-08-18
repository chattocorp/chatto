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
	server serverConfigState
	users  map[string]*userConfigState
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
	timezone                  *string
	timeFormat                *corev1.TimeFormat
	serverModeByCategory      map[corev1.NotificationPreferenceCategory]corev1.NotificationDeliveryMode
	roomModeByRoomAndCategory map[string]map[corev1.NotificationPreferenceCategory]corev1.NotificationDeliveryMode
}

func NewConfigProjection() *ConfigProjection {
	return &ConfigProjection{users: make(map[string]*userConfigState)}
}

func (p *ConfigProjection) Subjects() []string {
	return []string{
		evtstream.ConfigSubjectFilter(),
		evtstream.UserEventTypeFilter(evtstream.EventUserServerPreferencesChanged),
		evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted),
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
	case *corev1.Event_UserNotificationPreferenceChanged:
		preference := e.UserNotificationPreferenceChanged
		u := p.ensureUserLocked(preference.GetUserId())
		if preference.RoomId == nil {
			if preference.Override == nil {
				delete(u.serverModeByCategory, preference.GetCategory())
				break
			}
			if u.serverModeByCategory == nil {
				u.serverModeByCategory = make(map[corev1.NotificationPreferenceCategory]corev1.NotificationDeliveryMode)
			}
			u.serverModeByCategory[preference.GetCategory()] = preference.GetOverride()
			break
		}
		roomID := preference.GetRoomId()
		if preference.Override == nil {
			delete(u.roomModeByRoomAndCategory[roomID], preference.GetCategory())
			if len(u.roomModeByRoomAndCategory[roomID]) == 0 {
				delete(u.roomModeByRoomAndCategory, roomID)
			}
			break
		}
		if u.roomModeByRoomAndCategory == nil {
			u.roomModeByRoomAndCategory = make(map[string]map[corev1.NotificationPreferenceCategory]corev1.NotificationDeliveryMode)
		}
		if u.roomModeByRoomAndCategory[roomID] == nil {
			u.roomModeByRoomAndCategory[roomID] = make(map[corev1.NotificationPreferenceCategory]corev1.NotificationDeliveryMode)
		}
		u.roomModeByRoomAndCategory[roomID][preference.GetCategory()] = preference.GetOverride()
	case *corev1.Event_UserServerPreferencesChanged:
		p.applyLegacyUserPreferencesLocked(e.UserServerPreferencesChanged)
	case *corev1.Event_UserAccountDeleted:
		delete(p.users, e.UserAccountDeleted.GetUserId())
	}
	return nil
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
