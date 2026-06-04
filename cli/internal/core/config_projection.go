package core

import (
	"sort"
	"strings"

	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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
	logo             *corev1.DeprecatedAsset
	banner           *corev1.DeprecatedAsset
}

type userConfigState struct {
	timezone        *string
	timeFormat      *corev1.TimeFormat
	serverLevel     *corev1.NotificationLevel
	roomLevelByRoom map[string]corev1.NotificationLevel
}

// ServerConfigProjection is kept as a compatibility alias while callers and
// tests move from the old singleton server-config projection name.
type ServerConfigProjection = ConfigProjection

func NewConfigProjection() *ConfigProjection {
	return &ConfigProjection{users: make(map[string]*userConfigState)}
}

func NewServerConfigProjection() *ConfigProjection {
	return NewConfigProjection()
}

func (p *ConfigProjection) Subjects() []string {
	return []string{
		events.ConfigEventTypeFilter(events.EventServerConfigChanged),
		events.ConfigEventTypeFilter(events.EventServerNameChanged),
		events.ConfigEventTypeFilter(events.EventServerDescriptionChanged),
		events.ConfigEventTypeFilter(events.EventServerWelcomeMessageChanged),
		events.ConfigEventTypeFilter(events.EventServerMotdChanged),
		events.ConfigEventTypeFilter(events.EventServerBlockedUsernamesChanged),
		events.ConfigEventTypeFilter(events.EventServerLogoSet),
		events.ConfigEventTypeFilter(events.EventServerLogoCleared),
		events.ConfigEventTypeFilter(events.EventServerBannerSet),
		events.ConfigEventTypeFilter(events.EventServerBannerCleared),
		events.ConfigEventTypeFilter(events.EventUserTimezoneChanged),
		events.ConfigEventTypeFilter(events.EventUserTimezoneCleared),
		events.ConfigEventTypeFilter(events.EventUserTimeFormatChanged),
		events.ConfigEventTypeFilter(events.EventUserTimeFormatCleared),
		events.ConfigEventTypeFilter(events.EventUserServerNotificationLevelSet),
		events.ConfigEventTypeFilter(events.EventUserServerNotificationLevelCleared),
		events.ConfigEventTypeFilter(events.EventUserRoomNotificationLevelSet),
		events.ConfigEventTypeFilter(events.EventUserRoomNotificationLevelCleared),
		events.UserEventTypeFilter(events.EventUserServerPreferencesChanged),
		events.UserEventTypeFilter(events.EventUserAccountDeleted),
	}
}

func (p *ConfigProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()

	switch e := event.GetEvent().(type) {
	case *corev1.Event_ServerConfigChanged:
		p.applyLegacyServerConfigLocked(e.ServerConfigChanged.GetConfig())
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
		p.server.logo = cloneDeprecatedAsset(e.ServerLogoSet.GetAsset())
	case *corev1.Event_ServerLogoCleared:
		p.server.logo = nil
	case *corev1.Event_ServerBannerSet:
		p.server.banner = cloneDeprecatedAsset(e.ServerBannerSet.GetAsset())
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

func (p *ConfigProjection) applyLegacyServerConfigLocked(cfg *configv1.ServerConfig) {
	if cfg == nil {
		p.server.serverName = ""
		p.server.description = ""
		p.server.welcomeMessage = ""
		p.server.motd = ""
		blocked := ""
		p.server.blockedUsernames = &blocked
		return
	}
	blocked := cfg.GetBlockedUsernames()
	p.server.serverName = cfg.GetServerName()
	p.server.description = cfg.GetDescription()
	p.server.welcomeMessage = cfg.GetWelcomeMessage()
	p.server.motd = cfg.GetMotd()
	p.server.blockedUsernames = &blocked
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

func (p *ConfigProjection) Get() *configv1.ServerConfig {
	p.RLock()
	defer p.RUnlock()
	if p.server.serverName == "" &&
		p.server.description == "" &&
		p.server.welcomeMessage == "" &&
		p.server.motd == "" &&
		p.server.blockedUsernames == nil {
		return nil
	}
	cfg := &configv1.ServerConfig{
		ServerName:     p.server.serverName,
		Description:    p.server.description,
		WelcomeMessage: p.server.welcomeMessage,
		Motd:           p.server.motd,
	}
	if p.server.blockedUsernames != nil {
		cfg.BlockedUsernames = *p.server.blockedUsernames
	}
	return cfg
}

func (p *ConfigProjection) ServerLogo() (*corev1.DeprecatedAsset, bool, error) {
	p.RLock()
	defer p.RUnlock()
	if p.server.logo == nil {
		return nil, false, nil
	}
	return cloneDeprecatedAsset(p.server.logo), true, nil
}

func (p *ConfigProjection) ServerBanner() (*corev1.DeprecatedAsset, bool, error) {
	p.RLock()
	defer p.RUnlock()
	if p.server.banner == nil {
		return nil, false, nil
	}
	return cloneDeprecatedAsset(p.server.banner), true, nil
}

func (p *ConfigProjection) UserSettings(userID string) (*corev1.ServerUserPreferences, bool, error) {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || (u.timezone == nil && u.timeFormat == nil) {
		return nil, false, nil
	}
	prefs := &corev1.ServerUserPreferences{}
	if u.timezone != nil {
		tz := *u.timezone
		prefs.Timezone = &tz
	}
	if u.timeFormat != nil {
		prefs.TimeFormat = *u.timeFormat
	}
	return prefs, true, nil
}

func (p *ConfigProjection) EffectiveServerName() string {
	p.RLock()
	defer p.RUnlock()
	if p.server.serverName != "" {
		return p.server.serverName
	}
	return "Chatto"
}

func (p *ConfigProjection) EffectiveWelcomeMessage() string {
	p.RLock()
	defer p.RUnlock()
	return p.server.welcomeMessage
}

func (p *ConfigProjection) EffectiveMOTD() string {
	p.RLock()
	defer p.RUnlock()
	return p.server.motd
}

func (p *ConfigProjection) EffectiveDescription() string {
	p.RLock()
	defer p.RUnlock()
	if p.server.description != "" {
		return p.server.description
	}
	return DefaultDescription
}

func (p *ConfigProjection) EffectiveBlockedUsernames() string {
	p.RLock()
	defer p.RUnlock()
	if p.server.blockedUsernames == nil {
		return DefaultBlockedUsernames
	}
	return *p.server.blockedUsernames
}

func (p *ConfigProjection) BlockedUsernamesList() []string {
	return parseBlockedUsernames(p.EffectiveBlockedUsernames())
}

func (p *ConfigProjection) IsUsernameBlocked(login string) bool {
	loginLower := strings.ToLower(login)
	for _, blocked := range p.BlockedUsernamesList() {
		if blocked == loginLower {
			return true
		}
	}
	return false
}

func (p *ConfigProjection) NotificationServerLevel(userID string) corev1.NotificationLevel {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.serverLevel == nil {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT
	}
	return *u.serverLevel
}

func (p *ConfigProjection) NotificationRoomLevel(userID, roomID string) corev1.NotificationLevel {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || u.roomLevelByRoom == nil {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT
	}
	if level, ok := u.roomLevelByRoom[roomID]; ok {
		return level
	}
	return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT
}

func (p *ConfigProjection) NotificationRoomIDs(userID string) []string {
	p.RLock()
	defer p.RUnlock()
	u := p.users[userID]
	if u == nil || len(u.roomLevelByRoom) == 0 {
		return nil
	}
	ids := make([]string, 0, len(u.roomLevelByRoom))
	for roomID := range u.roomLevelByRoom {
		ids = append(ids, roomID)
	}
	sort.Strings(ids)
	return ids
}
