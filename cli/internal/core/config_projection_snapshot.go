package core

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

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
	roomConfigScopes := make([]roomConfigScopeKey, 0, len(p.roomConfigLayers))
	for scope := range p.roomConfigLayers {
		roomConfigScopes = append(roomConfigScopes, scope)
	}
	sort.Slice(roomConfigScopes, func(i, j int) bool {
		a, b := roomConfigScopes[i], roomConfigScopes[j]
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return a.id < b.id
	})
	for _, scope := range roomConfigScopes {
		state := p.roomConfigLayers[scope]
		layer := &corev1.RoomConfigLayer{}
		if state.authorEditWindow != nil {
			layer.AuthorEditWindow = durationpb.New(*state.authorEditWindow)
		}
		snapshot.RoomConfigLayers = append(snapshot.RoomConfigLayers, &corev1.RoomConfigLayerSnapshot{
			Scope: roomConfigScopeProto(RoomConfigScope{Kind: scope.kind, ID: scope.id}),
			Layer: layer,
		})
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
	roomConfigLayers := make(map[roomConfigScopeKey]*roomConfigLayerState, len(snapshot.GetRoomConfigLayers()))
	for _, row := range snapshot.GetRoomConfigLayers() {
		scope, ok := roomConfigScopeKeyFromProto(row.GetScope())
		if !ok {
			return fmt.Errorf("config snapshot has invalid room configuration scope")
		}
		if _, duplicate := roomConfigLayers[scope]; duplicate {
			return fmt.Errorf("config snapshot repeats room configuration scope")
		}
		state := &roomConfigLayerState{}
		if layer := row.GetLayer(); layer != nil && layer.AuthorEditWindow != nil {
			value := layer.GetAuthorEditWindow()
			if err := value.CheckValid(); err != nil {
				return fmt.Errorf("config snapshot has invalid author edit window: %w", err)
			}
			window := value.AsDuration()
			state.authorEditWindow = &window
		}
		roomConfigLayers[scope] = state
	}
	p.Lock()
	p.server, p.users, p.roomConfigLayers = server, users, roomConfigLayers
	p.Unlock()
	return nil
}
