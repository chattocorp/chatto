package core

// LegacyDMRoomSpaceID is the wire-frozen space_id value that older event
// payloads and a few compatibility APIs still use to mean "DM room".
const LegacyDMRoomSpaceID = "DM"

// LegacyServerSpaceID is the wire-frozen space_id value used by channel-room
// payloads and compatibility APIs after the Space tier was retired.
const LegacyServerSpaceID = "server"

// RoomKindFromLegacySpaceID maps a wire-format-frozen `space_id` value to the
// room kind it now represents. Use this only at compatibility boundaries where
// persisted payloads or public APIs still carry `space_id`; code working with
// Room records should read Room.kind via KindOfRoom.
func RoomKindFromLegacySpaceID(spaceID string) RoomKind {
	if spaceID == LegacyDMRoomSpaceID {
		return KindDM
	}
	return KindChannel
}

// LegacySpaceIDForRoomKind returns the wire-format-frozen `space_id` value for
// a room kind. Use this only when filling legacy payload fields or calling APIs
// whose storage keys still include `space_id`.
func LegacySpaceIDForRoomKind(kind RoomKind) string {
	if kind == KindDM {
		return LegacyDMRoomSpaceID
	}
	return LegacyServerSpaceID
}
