package core

import (
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const ConfigSubjectServer = "server"

// ConfigProjection consumes first-party configuration events from EVT and
// keeps the current server and notification-policy state in memory.
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
	serverModes           *corev1.NotificationDeliveryModes
	roomGroupModesByGroup map[string]*corev1.NotificationDeliveryModes
	roomModesByRoom       map[string]*corev1.NotificationDeliveryModes
}

func NewConfigProjection() *ConfigProjection {
	return &ConfigProjection{users: make(map[string]*userConfigState)}
}

func (p *ConfigProjection) Subjects() []string {
	return []string{
		evtstream.ConfigSubjectFilter(),
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
	case *corev1.Event_UserNotificationPolicyChanged:
		policy := e.UserNotificationPolicyChanged
		u := p.ensureUserLocked(policy.GetUserId())
		modes := cloneNotificationDeliveryModes(policy.GetOverrides())
		if policy.RoomId == nil {
			u.serverModes = modes
			break
		}
		if u.roomModesByRoom == nil {
			u.roomModesByRoom = make(map[string]*corev1.NotificationDeliveryModes)
		}
		roomID := policy.GetRoomId()
		if notificationDeliveryModesEmpty(modes) {
			delete(u.roomModesByRoom, roomID)
		} else {
			u.roomModesByRoom[roomID] = modes
		}
	case *corev1.Event_UserRoomGroupNotificationPolicyChanged:
		policy := e.UserRoomGroupNotificationPolicyChanged
		u := p.ensureUserLocked(policy.GetUserId())
		if u.roomGroupModesByGroup == nil {
			u.roomGroupModesByGroup = make(map[string]*corev1.NotificationDeliveryModes)
		}
		modes := cloneNotificationDeliveryModes(policy.GetOverrides())
		groupID := policy.GetRoomGroupId()
		if notificationDeliveryModesEmpty(modes) {
			delete(u.roomGroupModesByGroup, groupID)
		} else {
			u.roomGroupModesByGroup[groupID] = modes
		}
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
