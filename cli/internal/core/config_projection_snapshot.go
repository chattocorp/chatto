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
		for _, reason := range sortedNotificationReasons(user.serverIntensityByReason) {
			row.ServerNotificationPreferences = append(row.ServerNotificationPreferences, &corev1.NotificationPreferenceSnapshot{
				Reason:    reason,
				Intensity: user.serverIntensityByReason[reason],
			})
		}
		for _, roomID := range sortedMapKeys(user.roomIntensityByRoomAndCause) {
			room := &corev1.RoomNotificationPreferenceSnapshot{RoomId: roomID}
			for _, reason := range sortedNotificationReasons(user.roomIntensityByRoomAndCause[roomID]) {
				room.Preferences = append(room.Preferences, &corev1.NotificationPreferenceSnapshot{
					Reason:    reason,
					Intensity: user.roomIntensityByRoomAndCause[roomID][reason],
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
			roomLevelByRoom:             make(map[string]corev1.NotificationLevel),
			serverIntensityByReason:     make(map[corev1.NotificationReason]corev1.NotificationDeliveryIntensity),
			roomIntensityByRoomAndCause: make(map[string]map[corev1.NotificationReason]corev1.NotificationDeliveryIntensity),
		}
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
		for _, preference := range row.GetServerNotificationPreferences() {
			if _, duplicate := user.serverIntensityByReason[preference.GetReason()]; duplicate {
				return fmt.Errorf("config snapshot repeats server notification preference")
			}
			user.serverIntensityByReason[preference.GetReason()] = preference.GetIntensity()
		}
		for _, room := range row.GetRoomNotificationPreferences() {
			if room.GetRoomId() == "" {
				return fmt.Errorf("config snapshot has empty notification preference room ID")
			}
			if _, duplicate := user.roomIntensityByRoomAndCause[room.GetRoomId()]; duplicate {
				return fmt.Errorf("config snapshot repeats room notification preferences")
			}
			preferences := make(map[corev1.NotificationReason]corev1.NotificationDeliveryIntensity)
			for _, preference := range room.GetPreferences() {
				if _, duplicate := preferences[preference.GetReason()]; duplicate {
					return fmt.Errorf("config snapshot repeats room notification cause")
				}
				preferences[preference.GetReason()] = preference.GetIntensity()
			}
			user.roomIntensityByRoomAndCause[room.GetRoomId()] = preferences
		}
		users[row.GetUserId()] = user
	}
	p.Lock()
	p.server, p.users = server, users
	p.Unlock()
	return nil
}

func sortedNotificationReasons(values map[corev1.NotificationReason]corev1.NotificationDeliveryIntensity) []corev1.NotificationReason {
	keys := make([]corev1.NotificationReason, 0, len(values))
	for reason := range values {
		keys = append(keys, reason)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
