package core

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
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
	for scope, state := range p.roomConfigLayers {
		config := roomConfigProto(RoomConfig{AuthorEditWindow: state.authorEditWindow})
		switch scope.kind {
		case RoomConfigScopeServer:
			if scope.id != "" {
				return nil, fmt.Errorf("snapshot room configuration has server scope with ID")
			}
			snapshot.ServerRoomConfigLayer = config
		case RoomConfigScopeRoomGroup:
			if scope.id == "" {
				return nil, fmt.Errorf("snapshot room configuration has empty room-group ID")
			}
			if snapshot.RoomGroupConfigLayers == nil {
				snapshot.RoomGroupConfigLayers = make(map[string]*apiv1.RoomConfig)
			}
			snapshot.RoomGroupConfigLayers[scope.id] = config
		case RoomConfigScopeRoom:
			if scope.id == "" {
				return nil, fmt.Errorf("snapshot room configuration has empty room ID")
			}
			if snapshot.RoomConfigLayers == nil {
				snapshot.RoomConfigLayers = make(map[string]*apiv1.RoomConfig)
			}
			snapshot.RoomConfigLayers[scope.id] = config
		default:
			return nil, fmt.Errorf("snapshot room configuration has invalid scope kind %d", scope.kind)
		}
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
	roomConfigLayers := make(map[roomConfigScopeKey]*roomConfigLayerState, 1+len(snapshot.GetRoomGroupConfigLayers())+len(snapshot.GetRoomConfigLayers()))
	restoreRoomConfigLayer := func(scope roomConfigScopeKey, config *apiv1.RoomConfig) error {
		if config == nil {
			return fmt.Errorf("config snapshot has nil room configuration layer")
		}
		if value := config.GetAuthorEditWindow(); value != nil {
			if err := value.CheckValid(); err != nil {
				return fmt.Errorf("config snapshot has invalid author edit window: %w", err)
			}
		}
		domainConfig := roomConfigFromProto(config)
		state := &roomConfigLayerState{authorEditWindow: domainConfig.AuthorEditWindow}
		if state.authorEditWindow == nil {
			return fmt.Errorf("config snapshot has empty room configuration layer")
		}
		roomConfigLayers[scope] = state
		return nil
	}
	if snapshot.ServerRoomConfigLayer != nil {
		if err := restoreRoomConfigLayer(roomConfigScopeKey{kind: RoomConfigScopeServer}, snapshot.ServerRoomConfigLayer); err != nil {
			return err
		}
	}
	for groupID, config := range snapshot.GetRoomGroupConfigLayers() {
		if groupID == "" {
			return fmt.Errorf("config snapshot has empty room-group ID")
		}
		if err := restoreRoomConfigLayer(roomConfigScopeKey{kind: RoomConfigScopeRoomGroup, id: groupID}, config); err != nil {
			return err
		}
	}
	for roomID, config := range snapshot.GetRoomConfigLayers() {
		if roomID == "" {
			return fmt.Errorf("config snapshot has empty room ID")
		}
		if err := restoreRoomConfigLayer(roomConfigScopeKey{kind: RoomConfigScopeRoom, id: roomID}, config); err != nil {
			return err
		}
	}
	p.Lock()
	p.server, p.users, p.roomConfigLayers = server, users, roomConfigLayers
	p.Unlock()
	return nil
}
