package core

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const roomDirectorySnapshotCompatibilityID = "v1"

func (*RoomDirectoryProjection) SnapshotCompatibilityID() string {
	return roomDirectorySnapshotCompatibilityID
}

func (p *RoomDirectoryProjection) Snapshot() ([]byte, error) {
	p.Catalog.RLock()
	p.Membership.RLock()
	p.Bans.RLock()
	defer p.Catalog.RUnlock()
	defer p.Membership.RUnlock()
	defer p.Bans.RUnlock()

	snapshot := &corev1.RoomDirectoryProjectionSnapshot{CatalogSequence: p.Catalog.seq}
	for _, roomID := range sortedMapKeys(p.Catalog.rooms) {
		snapshot.Rooms = append(snapshot.Rooms, entryToRoom(roomID, p.Catalog.rooms[roomID]))
	}
	for _, roomID := range sortedMapKeys(p.Membership.byRoom) {
		snapshot.Memberships = append(snapshot.Memberships, &corev1.RoomMembershipSnapshot{
			RoomId:  roomID,
			UserIds: sortedMapKeys(p.Membership.byRoom[roomID]),
		})
	}
	for _, roomID := range sortedMapKeys(p.Bans.byRoom) {
		for _, userID := range sortedMapKeys(p.Bans.byRoom[roomID]) {
			ban := p.Bans.byRoom[roomID][userID]
			row := &corev1.RoomBanSnapshot{
				EventId: ban.EventID, RoomId: ban.RoomID, UserId: ban.UserID,
				ModeratorId: ban.ModeratorID, Reason: ban.Reason,
			}
			if !ban.CreatedAt.IsZero() {
				row.CreatedAt = timestamppb.New(ban.CreatedAt)
			}
			if ban.ExpiresAt != nil {
				row.ExpiresAt = timestamppb.New(*ban.ExpiresAt)
			}
			snapshot.Bans = append(snapshot.Bans, row)
		}
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *RoomDirectoryProjection) Restore(data []byte) error {
	snapshot := &corev1.RoomDirectoryProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal room directory snapshot: %w", err)
		}
	}
	rooms := make(map[string]*roomCatalogEntry, len(snapshot.GetRooms()))
	for _, room := range snapshot.GetRooms() {
		if room.GetId() == "" {
			return fmt.Errorf("room directory snapshot has empty room ID")
		}
		if _, duplicate := rooms[room.GetId()]; duplicate {
			return fmt.Errorf("room directory snapshot repeats room %q", room.GetId())
		}
		rooms[room.GetId()] = &roomCatalogEntry{name: room.GetName(), description: room.GetDescription(), kind: room.GetKind(), archived: room.GetArchived(), universal: room.GetUniversal()}
	}
	byRoom := make(map[string]map[string]struct{}, len(snapshot.GetMemberships()))
	byUser := make(map[string]map[string]struct{})
	for _, membership := range snapshot.GetMemberships() {
		roomID := membership.GetRoomId()
		if roomID == "" {
			return fmt.Errorf("room directory snapshot has empty membership room ID")
		}
		if _, duplicate := byRoom[roomID]; duplicate {
			return fmt.Errorf("room directory snapshot repeats membership room %q", roomID)
		}
		users := make(map[string]struct{}, len(membership.GetUserIds()))
		for _, userID := range membership.GetUserIds() {
			if userID == "" {
				return fmt.Errorf("room directory snapshot has empty member in room %q", roomID)
			}
			if _, duplicate := users[userID]; duplicate {
				return fmt.Errorf("room directory snapshot repeats member %q in room %q", userID, roomID)
			}
			users[userID] = struct{}{}
			if byUser[userID] == nil {
				byUser[userID] = make(map[string]struct{})
			}
			byUser[userID][roomID] = struct{}{}
		}
		byRoom[roomID] = users
	}
	bans := make(map[string]map[string]RoomBan)
	for _, row := range snapshot.GetBans() {
		if row.GetRoomId() == "" || row.GetUserId() == "" || row.GetReason() == "" {
			return fmt.Errorf("room directory snapshot has invalid ban")
		}
		createdAt, err := snapshotTime(row.GetCreatedAt())
		if err != nil {
			return fmt.Errorf("room ban created_at: %w", err)
		}
		var expiresAt *time.Time
		if row.GetExpiresAt() != nil {
			value, err := snapshotTime(row.GetExpiresAt())
			if err != nil {
				return fmt.Errorf("room ban expires_at: %w", err)
			}
			expiresAt = &value
		}
		if bans[row.GetRoomId()] == nil {
			bans[row.GetRoomId()] = make(map[string]RoomBan)
		}
		if _, duplicate := bans[row.GetRoomId()][row.GetUserId()]; duplicate {
			return fmt.Errorf("room directory snapshot repeats ban for room %q user %q", row.GetRoomId(), row.GetUserId())
		}
		bans[row.GetRoomId()][row.GetUserId()] = RoomBan{EventID: row.GetEventId(), RoomID: row.GetRoomId(), UserID: row.GetUserId(), ModeratorID: row.GetModeratorId(), Reason: row.GetReason(), CreatedAt: createdAt, ExpiresAt: expiresAt}
	}
	p.Catalog.Lock()
	p.Membership.Lock()
	p.Bans.Lock()
	p.Catalog.rooms, p.Catalog.seq = rooms, snapshot.GetCatalogSequence()
	p.Membership.byRoom, p.Membership.byUser = byRoom, byUser
	p.Bans.byRoom = bans
	p.Bans.Unlock()
	p.Membership.Unlock()
	p.Catalog.Unlock()
	return nil
}

func snapshotTime(value *timestamppb.Timestamp) (time.Time, error) {
	if value == nil {
		return time.Time{}, nil
	}
	if err := value.CheckValid(); err != nil {
		return time.Time{}, err
	}
	return value.AsTime(), nil
}
