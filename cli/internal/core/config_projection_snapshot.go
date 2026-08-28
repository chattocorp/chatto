package core

import (
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/projection/v1"

	"google.golang.org/protobuf/proto"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

var configSnapshotContractID = snapshotContractID("v1", &projectionv1.ConfigProjectionSnapshot{})

func (*ConfigProjection) SnapshotContractID() string { return configSnapshotContractID }

func (p *ConfigProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	snapshot := &projectionv1.ConfigProjectionSnapshot{ServerName: p.server.serverName, Description: p.server.description, WelcomeMessage: p.server.welcomeMessage, Motd: p.server.motd, Logo: cloneAssetRecord(p.server.logo), Banner: cloneAssetRecord(p.server.banner)}
	if p.server.blockedUsernames != nil {
		value := *p.server.blockedUsernames
		snapshot.BlockedUsernames = &value
	}
	for _, userID := range sortedMapKeys(p.users) {
		user := p.users[userID]
		row := &projectionv1.UserConfigSnapshot{UserId: userID}
		if user.timezone != nil {
			value := *user.timezone
			row.Timezone = &value
		}
		if user.timeFormat != nil {
			value := *user.timeFormat
			row.TimeFormat = &value
		}
		row.ServerNotificationModes = cloneNotificationDeliveryModes(user.serverModes)
		for _, groupID := range sortedMapKeys(user.roomGroupModesByGroup) {
			row.RoomGroupNotificationModes = append(row.RoomGroupNotificationModes, &projectionv1.RoomGroupNotificationModesSnapshot{
				RoomGroupId: groupID,
				Modes:       cloneNotificationDeliveryModes(user.roomGroupModesByGroup[groupID]),
			})
		}
		for _, roomID := range sortedMapKeys(user.roomModesByRoom) {
			row.RoomNotificationModes = append(row.RoomNotificationModes, &projectionv1.RoomNotificationModesSnapshot{
				RoomId: roomID,
				Modes:  cloneNotificationDeliveryModes(user.roomModesByRoom[roomID]),
			})
		}
		snapshot.Users = append(snapshot.Users, row)
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *ConfigProjection) Restore(data []byte) error {
	snapshot := &projectionv1.ConfigProjectionSnapshot{}
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
			roomGroupModesByGroup: make(map[string]*evtv1.NotificationDeliveryModes),
			roomModesByRoom:       make(map[string]*evtv1.NotificationDeliveryModes),
		}
		if row.Timezone != nil {
			value := row.GetTimezone()
			user.timezone = &value
		}
		if row.TimeFormat != nil {
			value := row.GetTimeFormat()
			user.timeFormat = &value
		}
		user.serverModes = cloneNotificationDeliveryModes(row.GetServerNotificationModes())
		for _, group := range row.GetRoomGroupNotificationModes() {
			if group.GetRoomGroupId() == "" {
				return fmt.Errorf("config snapshot has empty notification preference room group ID")
			}
			if _, duplicate := user.roomGroupModesByGroup[group.GetRoomGroupId()]; duplicate {
				return fmt.Errorf("config snapshot repeats room group notification preferences")
			}
			user.roomGroupModesByGroup[group.GetRoomGroupId()] = cloneNotificationDeliveryModes(group.GetModes())
		}
		for _, room := range row.GetRoomNotificationModes() {
			if room.GetRoomId() == "" {
				return fmt.Errorf("config snapshot has empty notification preference room ID")
			}
			if _, duplicate := user.roomModesByRoom[room.GetRoomId()]; duplicate {
				return fmt.Errorf("config snapshot repeats room notification preferences")
			}
			user.roomModesByRoom[room.GetRoomId()] = cloneNotificationDeliveryModes(room.GetModes())
		}
		users[row.GetUserId()] = user
	}
	p.Lock()
	p.server, p.users = server, users
	p.Unlock()
	return nil
}
