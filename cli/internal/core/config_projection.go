package core

import (
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
	logo             *evtv1.AssetRecord
	banner           *evtv1.AssetRecord
	neighbors        map[string]Neighbor
}

// Neighbor is one server advertised through the public Neighbor directory.
// Revision is the envelope ID of the latest durable fact for this resource.
type Neighbor struct {
	ID       string
	Origin   string
	Revision string
}

type userConfigState struct {
	timezone              *string
	timeFormat            *evtv1.TimeFormat
	shareTimezone         bool
	serverModes           *evtv1.NotificationDeliveryModes
	roomGroupModesByGroup map[string]*evtv1.NotificationDeliveryModes
	roomModesByRoom       map[string]*evtv1.NotificationDeliveryModes
}

func NewConfigProjection() *ConfigProjection {
	return &ConfigProjection{server: serverConfigState{neighbors: make(map[string]Neighbor)}, users: make(map[string]*userConfigState)}
}

func (p *ConfigProjection) Subjects() []string {
	return []string{
		evtstream.ConfigSubjectFilter(),
		evtstream.UserEventTypeFilter(evtstream.EventUserServerPreferencesChanged),
		evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted),
	}
}

func (p *ConfigProjection) Apply(event *evtv1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	switch e := event.GetEvent().(type) {
	case *evtv1.Event_ServerNameChanged:
		p.server.serverName = e.ServerNameChanged.GetName()
	case *evtv1.Event_ServerDescriptionChanged:
		p.server.description = e.ServerDescriptionChanged.GetDescription()
	case *evtv1.Event_ServerWelcomeMessageChanged:
		p.server.welcomeMessage = e.ServerWelcomeMessageChanged.GetWelcomeMessage()
	case *evtv1.Event_ServerMotdChanged:
		p.server.motd = e.ServerMotdChanged.GetMotd()
	case *evtv1.Event_ServerBlockedUsernamesChanged:
		blocked := e.ServerBlockedUsernamesChanged.GetBlockedUsernames()
		p.server.blockedUsernames = &blocked
	case *evtv1.Event_ServerLogoSet:
		p.server.logo = cloneAssetRecord(e.ServerLogoSet.GetAsset())
	case *evtv1.Event_ServerLogoCleared:
		p.server.logo = nil
	case *evtv1.Event_ServerBannerSet:
		p.server.banner = cloneAssetRecord(e.ServerBannerSet.GetAsset())
	case *evtv1.Event_ServerBannerCleared:
		p.server.banner = nil
	case *evtv1.Event_ServerNeighborCreated:
		neighbor := e.ServerNeighborCreated
		if neighbor == nil || neighbor.GetNeighborId() == "" {
			break
		}
		if p.server.neighbors == nil {
			p.server.neighbors = make(map[string]Neighbor)
		}
		p.server.neighbors[neighbor.GetNeighborId()] = Neighbor{ID: neighbor.GetNeighborId(), Origin: neighbor.GetOrigin(), Revision: event.GetId()}
	case *evtv1.Event_ServerNeighborOriginChanged:
		neighbor := e.ServerNeighborOriginChanged
		if neighbor == nil {
			break
		}
		if current, exists := p.server.neighbors[neighbor.GetNeighborId()]; exists {
			current.Origin = neighbor.GetOrigin()
			current.Revision = event.GetId()
			p.server.neighbors[neighbor.GetNeighborId()] = current
		}
	case *evtv1.Event_ServerNeighborTestimonialChanged:
		// Testimonial changes are retained only to keep revisions correct while
		// replaying EVT histories written by testimonial-capable releases.
		neighbor := e.ServerNeighborTestimonialChanged
		if neighbor == nil {
			break
		}
		if current, exists := p.server.neighbors[neighbor.GetNeighborId()]; exists {
			current.Revision = event.GetId()
			p.server.neighbors[neighbor.GetNeighborId()] = current
		}
	case *evtv1.Event_ServerNeighborDeleted:
		neighborID := e.ServerNeighborDeleted.GetNeighborId()
		delete(p.server.neighbors, neighborID)
	case *evtv1.Event_UserTimezoneChanged:
		u := p.ensureUserLocked(e.UserTimezoneChanged.GetUserId())
		tz := e.UserTimezoneChanged.GetTimezone()
		u.timezone = &tz
	case *evtv1.Event_UserTimezoneCleared:
		p.ensureUserLocked(e.UserTimezoneCleared.GetUserId()).timezone = nil
	case *evtv1.Event_UserTimezoneSharingChanged:
		p.ensureUserLocked(e.UserTimezoneSharingChanged.GetUserId()).shareTimezone = e.UserTimezoneSharingChanged.GetShareTimezone()
	case *evtv1.Event_UserTimeFormatChanged:
		u := p.ensureUserLocked(e.UserTimeFormatChanged.GetUserId())
		tf := e.UserTimeFormatChanged.GetTimeFormat()
		u.timeFormat = &tf
	case *evtv1.Event_UserTimeFormatCleared:
		p.ensureUserLocked(e.UserTimeFormatCleared.GetUserId()).timeFormat = nil
	case *evtv1.Event_UserNotificationPolicyChanged:
		policy := e.UserNotificationPolicyChanged
		u := p.ensureUserLocked(policy.GetUserId())
		modes := cloneNotificationDeliveryModes(policy.GetOverrides())
		if policy.RoomId == nil {
			u.serverModes = modes
			break
		}
		if u.roomModesByRoom == nil {
			u.roomModesByRoom = make(map[string]*evtv1.NotificationDeliveryModes)
		}
		roomID := policy.GetRoomId()
		if notificationDeliveryModesEmpty(modes) {
			delete(u.roomModesByRoom, roomID)
		} else {
			u.roomModesByRoom[roomID] = modes
		}
	case *evtv1.Event_UserRoomGroupNotificationPolicyChanged:
		policy := e.UserRoomGroupNotificationPolicyChanged
		u := p.ensureUserLocked(policy.GetUserId())
		if u.roomGroupModesByGroup == nil {
			u.roomGroupModesByGroup = make(map[string]*evtv1.NotificationDeliveryModes)
		}
		modes := cloneNotificationDeliveryModes(policy.GetOverrides())
		groupID := policy.GetRoomGroupId()
		if notificationDeliveryModesEmpty(modes) {
			delete(u.roomGroupModesByGroup, groupID)
		} else {
			u.roomGroupModesByGroup[groupID] = modes
		}
	case *evtv1.Event_UserServerPreferencesChanged:
		p.applyLegacyUserPreferencesLocked(e.UserServerPreferencesChanged)
	case *evtv1.Event_UserAccountDeleted:
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

func (p *ConfigProjection) applyLegacyUserPreferencesLocked(e *evtv1.UserServerPreferencesChangedEvent) {
	if e == nil || e.GetUserId() == "" {
		return
	}
	u := p.ensureUserLocked(e.GetUserId())
	prefs := e.GetPreferences()
	if prefs == nil {
		u.timezone = nil
		u.timeFormat = nil
		u.shareTimezone = false
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
	u.shareTimezone = prefs.GetShareTimezone()
}
