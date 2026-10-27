package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// MaxDMParticipants bounds lookup of stored DMs from older servers.
// New DMs allow at most two distinct participants; use channels for groups.
const MaxDMParticipants = 10

// RoomKind is the closed enum of room kinds carried in subjects and KV
// keys (`server.room.{kind}.>`, `room_membership.{kind}.{roomId}.{userId}`,
// etc.). The string form goes on the wire — don't rename the variants.
type RoomKind string

const (
	// KindChannel is a regular (non-DM) chat room.
	KindChannel RoomKind = "channel"
	// KindDM is a direct-message room.
	KindDM RoomKind = "dm"
)

// ProtoKindForRoomKind maps the Go-side RoomKind string to the proto
// enum stored on Room.kind.
func ProtoKindForRoomKind(kind RoomKind) evtv1.RoomKind {
	if kind == KindDM {
		return evtv1.RoomKind_ROOM_KIND_DM
	}
	return evtv1.RoomKind_ROOM_KIND_CHANNEL
}

// KindOfRoom returns the canonical RoomKind for a Room. Reads
// Room.kind directly; legacy `space_id`-based fallback was removed
// when the field was retired from the proto.
func KindOfRoom(room *readmodel.Room) RoomKind {
	switch room.Kind {
	case evtv1.RoomKind_ROOM_KIND_DM:
		return KindDM
	default:
		return KindChannel
	}
}

// DMRoomID generates a deterministic room ID from participant IDs.
// The same set of participants always produces the same room ID,
// regardless of order. This enables find-or-create semantics without
// database queries.
func DMRoomID(participantIDs []string) string {
	if len(participantIDs) < 1 {
		return ""
	}

	// Sort to ensure consistent ordering
	sorted := make([]string, len(participantIDs))
	copy(sorted, participantIDs)
	sort.Strings(sorted)

	// Hash the sorted participant list
	h := sha256.New()
	for _, id := range sorted {
		h.Write([]byte(id))
		h.Write([]byte{0}) // separator to prevent collisions
	}

	// 14 hex chars (matches NanoID length used elsewhere)
	return hex.EncodeToString(h.Sum(nil))[:14]
}

// ============================================================================
// DM Room Management
// ============================================================================

// FindOrCreateDM finds an existing DM conversation or creates a new one.
// The caller (creatorID) is automatically included in the participant list.
// Returns the room and a boolean indicating whether it was newly created.
//
// For existing DMs, the caller must already be a participant.
// For new DMs, all participants are automatically joined to the room.
func (c *ChattoCore) FindOrCreateDM(ctx context.Context, creatorID string, participantIDs []string) (*readmodel.Room, bool, error) {
	return c.roomCommands.findOrCreateDM(ctx, creatorID, participantIDs)
}

// ensureInList ensures the given ID is in the list, adding it if not present.
func ensureInList(list []string, id string) []string {
	if slices.Contains(list, id) {
		return list
	}
	return append(list, id)
}
