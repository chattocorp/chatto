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
		for _, category := range sortedNotificationPreferenceCategories(user.serverModeByCategory) {
			row.ServerNotificationPreferences = append(row.ServerNotificationPreferences, &corev1.NotificationPreferenceSnapshot{
				Category: category,
				Mode:     user.serverModeByCategory[category],
			})
		}
		for _, roomID := range sortedMapKeys(user.roomModeByRoomAndCategory) {
			room := &corev1.RoomNotificationPreferenceSnapshot{RoomId: roomID}
			for _, category := range sortedNotificationPreferenceCategories(user.roomModeByRoomAndCategory[roomID]) {
				room.Preferences = append(room.Preferences, &corev1.NotificationPreferenceSnapshot{
					Category: category,
					Mode:     user.roomModeByRoomAndCategory[roomID][category],
				})
			}
			row.RoomNotificationPreferences = append(row.RoomNotificationPreferences, room)
		}
		snapshot.Users = append(snapshot.Users, row)
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
		user := &userConfigState{
			serverModeByCategory:      make(map[corev1.NotificationPreferenceCategory]corev1.NotificationDeliveryMode),
			roomModeByRoomAndCategory: make(map[string]map[corev1.NotificationPreferenceCategory]corev1.NotificationDeliveryMode),
		}
		if row.Timezone != nil {
			value := row.GetTimezone()
			user.timezone = &value
		}
		if row.TimeFormat != nil {
			value := row.GetTimeFormat()
			user.timeFormat = &value
		}
		for _, preference := range row.GetServerNotificationPreferences() {
			if _, duplicate := user.serverModeByCategory[preference.GetCategory()]; duplicate {
				return fmt.Errorf("config snapshot repeats server notification preference")
			}
			user.serverModeByCategory[preference.GetCategory()] = preference.GetMode()
		}
		for _, room := range row.GetRoomNotificationPreferences() {
			if room.GetRoomId() == "" {
				return fmt.Errorf("config snapshot has empty notification preference room ID")
			}
			if _, duplicate := user.roomModeByRoomAndCategory[room.GetRoomId()]; duplicate {
				return fmt.Errorf("config snapshot repeats room notification preferences")
			}
			preferences := make(map[corev1.NotificationPreferenceCategory]corev1.NotificationDeliveryMode)
			for _, preference := range room.GetPreferences() {
				if _, duplicate := preferences[preference.GetCategory()]; duplicate {
					return fmt.Errorf("config snapshot repeats room notification policy kind")
				}
				preferences[preference.GetCategory()] = preference.GetMode()
			}
			user.roomModeByRoomAndCategory[room.GetRoomId()] = preferences
		}
		users[row.GetUserId()] = user
	}
	p.Lock()
	p.server, p.users = server, users
	p.Unlock()
	return nil
}

func sortedNotificationPreferenceCategories(values map[corev1.NotificationPreferenceCategory]corev1.NotificationDeliveryMode) []corev1.NotificationPreferenceCategory {
	keys := make([]corev1.NotificationPreferenceCategory, 0, len(values))
	for kind := range values {
		keys = append(keys, kind)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
