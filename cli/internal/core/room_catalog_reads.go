package core

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/chattocorp/chatto/cli/internal/readmodel"
)

// Catalog reads do not authorize callers. Shared-view callers retain the
// existing content-view read transaction around combined reads.

// getRoomLastRootEntry returns the latest non-historical root message.
func (m *RoomModel) getRoomLastRootEntry(roomID string) *TimelineEntry {
	entry, ok := m.lastVisibleRoomEntry(roomID, func(entry *TimelineEntry) bool {
		return entry != nil && entry.IsMessagePost() && entry.InThreadEventID == "" && !entry.HistoricalImport
	})
	if !ok {
		return nil
	}
	return entry
}

// getRoomLastMessageEntry returns the latest non-historical message, including thread replies.
func (m *RoomModel) getRoomLastMessageEntry(roomID string) *TimelineEntry {
	entry, ok := m.lastRoomMessageEntry(roomID)
	if !ok {
		return nil
	}
	return entry
}

// GetRoomLastMessageAt returns the latest ordinary message time, or zero for an empty room.
func (m *RoomModel) GetRoomLastMessageAt(ctx context.Context, kind RoomKind, roomID string) (time.Time, error) {
	entry := m.getRoomLastMessageEntry(roomID)
	if entry == nil {
		return time.Time{}, nil
	}
	return entry.CreatedAt, nil
}

// lastPostedMessageTimes reads both posting markers from one timeline state.
// The caller must authorize broad message access before exposing these values.
func (m *RoomModel) lastPostedMessageTimes(roomID string) (time.Time, time.Time) {
	return m.timeline.Projection().LastPostedMessageTimes(roomID)
}

// GetRoom returns a detached room with its current group. A missing room or kind mismatch returns ErrNotFound.
func (m *RoomModel) GetRoom(ctx context.Context, kind RoomKind, roomID string) (*readmodel.Room, error) {
	room, ok := m.room(roomID)
	if !ok || room.Kind != ProtoKindForRoomKind(kind) {
		return nil, fmt.Errorf("room %s: %w", roomID, ErrNotFound)
	}
	if gid := m.roomGroupForRoom(roomID); gid != "" {
		room.GroupId = gid
	}
	return room, nil
}

// FindRoomByID returns a detached room with its current group, without a kind filter.
func (m *RoomModel) FindRoomByID(ctx context.Context, roomID string) (*readmodel.Room, error) {
	room, ok := m.room(roomID)
	if !ok {
		return nil, ErrNotFound
	}
	if gid := m.roomGroupForRoom(roomID); gid != "" {
		room.GroupId = gid
	}
	return room, nil
}

// FindRoomKind resolves the kind of an existing room.
func (m *RoomModel) FindRoomKind(ctx context.Context, roomID string) (RoomKind, error) {
	room, err := m.FindRoomByID(ctx, roomID)
	if err != nil {
		return "", err
	}
	return KindOfRoom(room), nil
}

// ListRooms returns detached rooms of the requested kind with their current groups.
func (m *RoomModel) ListRooms(ctx context.Context, kind RoomKind) ([]*readmodel.Room, error) {
	rooms := m.roomsByKind(ProtoKindForRoomKind(kind))
	for _, r := range rooms {
		if gid := m.roomGroupForRoom(r.Id); gid != "" {
			r.GroupId = gid
		}
	}
	return rooms, nil
}

// GetRoomGroup returns a detached group, or ErrRoomGroupNotFound.
func (m *RoomModel) GetRoomGroup(_ context.Context, groupID string) (*readmodel.RoomGroup, error) {
	g, ok := m.roomGroup(groupID)
	if !ok {
		return nil, ErrRoomGroupNotFound
	}
	return g, nil
}

// ListRoomGroupsOrdered returns channel groups in layout order, followed by orphan groups in ID order.
func (m *RoomModel) ListRoomGroupsOrdered(_ context.Context, kind RoomKind) ([]*readmodel.RoomGroup, error) {
	if kind != KindChannel {
		return nil, nil
	}

	order := m.roomLayoutOrder()
	all := m.roomGroups()
	docs := make(map[string]*readmodel.RoomGroup, len(all))
	for _, g := range all {
		docs[g.Id] = g
	}

	out := make([]*readmodel.RoomGroup, 0, len(docs))
	used := make(map[string]struct{}, len(order))
	for _, id := range order {
		if _, dup := used[id]; dup {
			continue
		}
		g, ok := docs[id]
		if !ok {
			continue
		}
		out = append(out, g)
		used[id] = struct{}{}
	}

	var orphans []string
	for id := range docs {
		if _, ok := used[id]; !ok {
			orphans = append(orphans, id)
		}
	}
	slices.Sort(orphans)
	for _, id := range orphans {
		out = append(out, docs[id])
	}
	return out, nil
}
