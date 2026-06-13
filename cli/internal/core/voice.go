package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	lkauth "github.com/livekit/protocol/auth"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// VoiceCallToken contains the LiveKit JWT for a client to join a call.
type VoiceCallToken struct {
	Token   string
	E2EEKey string
}

// participantMetadata is serialized as JSON and stored in the LiveKit token's
// metadata field so the frontend can display avatars without extra queries.
// Also used to parse metadata from LiveKit webhook participant info.
type participantMetadata struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatarUrl,omitempty"`
}

// ParseParticipantMetadata parses JSON metadata from a LiveKit participant.
// Returns zero-value struct if metadata is empty or invalid.
func ParseParticipantMetadata(metadata string) participantMetadata {
	if metadata == "" {
		return participantMetadata{}
	}
	var md participantMetadata
	if err := json.Unmarshal([]byte(metadata), &md); err != nil {
		return participantMetadata{}
	}
	return md
}

// LiveKitRoomName constructs a deterministic LiveKit room name from space and room IDs.
// When serverID is non-empty, the room name is prefixed with "{serverID}." so the
// webhook bridge can route events to the correct Chatto server in shared deployments.
// Authorization: Caller must verify room membership before calling.
func LiveKitRoomName(serverID, spaceID, roomID string) string {
	base := spaceID + "_" + roomID
	if serverID != "" {
		return serverID + "." + base
	}
	return base
}

// ParseLiveKitRoomName extracts the space ID and room ID from a LiveKit room name.
// Handles both prefixed ("{serverID}.{spaceID}_{roomID}") and unprefixed
// ("{spaceID}_{roomID}") formats. Returns empty strings if the format is unexpected.
func ParseLiveKitRoomName(lkRoomName string) (spaceID, roomID string) {
	name := lkRoomName

	// Strip server ID prefix if present (dot separator).
	// Safe because server IDs (K8s names, UUIDs, NanoIDs) and space/room NanoIDs
	// never contain dots.
	if idx := strings.IndexByte(name, '.'); idx >= 0 {
		name = name[idx+1:]
	}

	// Split on first underscore: {spaceID}_{roomID}
	idx := strings.IndexByte(name, '_')
	if idx < 0 {
		return "", ""
	}
	return name[:idx], name[idx+1:]
}

// ParseLiveKitRoomServerID extracts just the server ID prefix from a LiveKit room
// name. Returns empty string if no prefix is present (unprefixed format).
func ParseLiveKitRoomServerID(lkRoomName string) string {
	idx := strings.IndexByte(lkRoomName, '.')
	if idx < 0 {
		return ""
	}
	return lkRoomName[:idx]
}

// GenerateVoiceCallToken creates a LiveKit join token for a user.
// The login and avatarURL are embedded as JSON metadata so the frontend can
// render avatars without additional queries.
// Authorization: Caller must verify room membership before calling.
func GenerateVoiceCallToken(apiKey, apiSecret, roomName, userID, displayName, login, avatarURL, e2eeKey string) (*VoiceCallToken, error) {
	at := lkauth.NewAccessToken(apiKey, apiSecret)
	grant := &lkauth.VideoGrant{
		RoomJoin: true,
		Room:     roomName,
	}
	at.SetVideoGrant(grant).
		SetIdentity(userID).
		SetName(displayName).
		SetValidFor(30 * time.Second)

	md, err := json.Marshal(participantMetadata{Login: login, AvatarURL: avatarURL})
	if err != nil {
		return nil, fmt.Errorf("marshal participant metadata: %w", err)
	}
	at.SetMetadata(string(md))

	token, err := at.ToJWT()
	if err != nil {
		return nil, fmt.Errorf("generate LiveKit token: %w", err)
	}
	return &VoiceCallToken{Token: token, E2EEKey: e2eeKey}, nil
}

// HandleCallParticipantJoined appends a durable LiveKit-observed join fact.
// Called by the webhook handler when LiveKit reports a participant joined.
func (c *ChattoCore) HandleCallParticipantJoined(ctx context.Context, spaceID, roomID, userID, displayName, login, avatarURL string) error {
	return c.RecordCallParticipantJoined(ctx, RoomKindFromLegacySpaceID(spaceID), roomID, userID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_LIVEKIT)
}

// HandleCallParticipantLeft appends a durable LiveKit-observed leave fact.
// Called by the webhook handler when LiveKit reports a participant left.
func (c *ChattoCore) HandleCallParticipantLeft(ctx context.Context, spaceID, roomID, userID string) error {
	return c.RecordCallParticipantLeft(ctx, RoomKindFromLegacySpaceID(spaceID), roomID, userID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_LIVEKIT)
}

// HandleCallRoomFinished appends LiveKit-observed leave facts for any remaining
// projected participants in the room.
// Called by the webhook handler when LiveKit reports a room has finished (closed).
func (c *ChattoCore) HandleCallRoomFinished(ctx context.Context, spaceID, roomID string) error {
	for _, p := range c.CallState.Participants(roomID) {
		if err := c.RecordCallParticipantLeft(ctx, RoomKindFromLegacySpaceID(spaceID), roomID, p.UserID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_LIVEKIT); err != nil {
			return err
		}
	}
	return nil
}

func (c *ChattoCore) RecordCallParticipantJoined(ctx context.Context, kind RoomKind, roomID, userID string, source corev1.CallParticipantEventSource) error {
	if c.callService == nil {
		return fmt.Errorf("call service is not initialized")
	}
	return c.callService.AppendJoined(ctx, roomID, userID, source)
}

func (c *ChattoCore) RecordCallParticipantLeft(ctx context.Context, kind RoomKind, roomID, userID string, source corev1.CallParticipantEventSource) error {
	if c.callService == nil {
		return fmt.Errorf("call service is not initialized")
	}
	return c.callService.AppendLeft(ctx, roomID, userID, source)
}

func (c *ChattoCore) GetVoiceCallE2EEKey(ctx context.Context, roomID string) (string, error) {
	if c.callService == nil {
		return "", fmt.Errorf("call service is not initialized")
	}
	return c.callService.GetE2EEKey(ctx, roomID)
}

// GetCallParticipants returns the participants currently in a voice call.
// Returns an empty slice if no call is active.
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) GetCallParticipants(ctx context.Context, spaceID, roomID string) ([]CallParticipant, error) {
	return c.CallState.Participants(roomID), nil
}

// GetActiveCallRoomIDs returns the room IDs in a space that have active voice calls.
// Reads from the call-state projection, not MEMORY_CACHE.
// Authorization: Caller must verify space membership before calling.
func (c *ChattoCore) GetActiveCallRoomIDs(ctx context.Context, spaceID string) ([]string, error) {
	kind := RoomKindFromLegacySpaceID(spaceID)
	roomIDs := c.CallState.ActiveRoomIDs()
	if c.RoomCatalog == nil {
		return roomIDs, nil
	}
	filtered := make([]string, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		room, ok := c.RoomCatalog.Get(roomID)
		if !ok || KindOfRoom(room) == kind {
			filtered = append(filtered, roomID)
		}
	}
	sort.Strings(filtered)
	return filtered, nil
}

func appendCallJoinedEventForTest(ctx context.Context, publisher *events.Publisher, projector *events.Projector, roomID, userID string, source corev1.CallParticipantEventSource) error {
	event := newEvent(userID, &corev1.Event{
		Event: &corev1.Event_VoiceCallParticipantJoined{
			VoiceCallParticipantJoined: &corev1.CallParticipantJoinedEvent{RoomId: roomID, Source: source},
		},
	})
	_, err := projector.AppendEventuallyAndWait(ctx, publisher, events.RoomAggregate(roomID), event)
	return err
}

func appendCallLeftEventForTest(ctx context.Context, publisher *events.Publisher, projector *events.Projector, roomID, userID string, source corev1.CallParticipantEventSource) error {
	event := newEvent(userID, &corev1.Event{
		Event: &corev1.Event_VoiceCallParticipantLeft{
			VoiceCallParticipantLeft: &corev1.CallParticipantLeftEvent{RoomId: roomID, Source: source},
		},
	})
	_, err := projector.AppendEventuallyAndWait(ctx, publisher, events.RoomAggregate(roomID), event)
	return err
}
