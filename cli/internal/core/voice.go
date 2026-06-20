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
	CallID  string
}

// VoiceCallTokenTTL gives browser clients enough time for E2EE worker setup,
// permission prompts, and a signaling retry without making leaked join tokens
// long-lived.
const VoiceCallTokenTTL = 5 * time.Minute

// participantMetadata is serialized as JSON and stored in the LiveKit token's
// metadata field so the frontend can display avatars without extra queries.
// Also used to parse metadata from LiveKit webhook participant info.
type participantMetadata struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	CallID    string `json:"callId,omitempty"`
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
func LiveKitRoomName(serverID, spaceID, roomID string, callID ...string) string {
	base := spaceID + "_" + roomID
	if len(callID) > 0 && callID[0] != "" {
		base += "@" + callID[0]
	}
	if serverID != "" {
		return serverID + "." + base
	}
	return base
}

// ParseLiveKitRoomName extracts the space ID and room ID from a LiveKit room name.
// Handles both prefixed ("{serverID}.{spaceID}_{roomID}") and unprefixed
// ("{spaceID}_{roomID}") formats. Returns empty strings if the format is unexpected.
func ParseLiveKitRoomName(lkRoomName string) (spaceID, roomID string) {
	spaceID, roomID, _ = ParseLiveKitRoomIdentity(lkRoomName)
	return spaceID, roomID
}

// ParseLiveKitRoomIdentity extracts the space ID, room ID, and optional Chatto
// call ID from a LiveKit room name. New room names append "@{callId}" so LiveKit
// room_finished events can be tied to one Chatto call session; names without
// a suffix are accepted for compatibility with older active LiveKit rooms.
func ParseLiveKitRoomIdentity(lkRoomName string) (spaceID, roomID, callID string) {
	name := lkRoomName

	// Strip server ID prefix if present (dot separator).
	// Safe because server IDs (K8s names, UUIDs, NanoIDs) and space/room NanoIDs
	// never contain dots.
	if idx := strings.IndexByte(name, '.'); idx >= 0 {
		name = name[idx+1:]
	}

	if idx := strings.LastIndexByte(name, '@'); idx >= 0 {
		callID = name[idx+1:]
		name = name[:idx]
	}

	// Split on first underscore: {spaceID}_{roomID}
	idx := strings.IndexByte(name, '_')
	if idx < 0 {
		return "", "", ""
	}
	return name[:idx], name[idx+1:], callID
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
func GenerateVoiceCallToken(apiKey, apiSecret, roomName, userID, displayName, login, avatarURL, e2eeKey string, callID ...string) (*VoiceCallToken, error) {
	at := lkauth.NewAccessToken(apiKey, apiSecret)
	grant := &lkauth.VideoGrant{
		RoomJoin: true,
		Room:     roomName,
	}
	at.SetVideoGrant(grant).
		SetIdentity(userID).
		SetName(displayName).
		SetValidFor(VoiceCallTokenTTL)

	var activeCallID string
	if len(callID) > 0 {
		activeCallID = callID[0]
	}
	md, err := json.Marshal(participantMetadata{Login: login, AvatarURL: avatarURL, CallID: activeCallID})
	if err != nil {
		return nil, fmt.Errorf("marshal participant metadata: %w", err)
	}
	at.SetMetadata(string(md))

	token, err := at.ToJWT()
	if err != nil {
		return nil, fmt.Errorf("generate LiveKit token: %w", err)
	}
	return &VoiceCallToken{Token: token, E2EEKey: e2eeKey, CallID: activeCallID}, nil
}

// HandleCallParticipantJoined appends a durable LiveKit-observed join fact.
// Called by the webhook handler when LiveKit reports a participant joined.
func (c *ChattoCore) HandleCallParticipantJoined(ctx context.Context, spaceID, roomID, userID, displayName, login, avatarURL string, callID ...string) error {
	expectedCallID := optionalCallID(callID)
	if c.callService == nil {
		return fmt.Errorf("call service is not initialized")
	}
	return c.callService.AppendJoinedForCall(ctx, roomID, userID, expectedCallID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_LIVEKIT)
}

// HandleCallParticipantLeft appends a durable LiveKit-observed leave fact.
// Called by the webhook handler when LiveKit reports a participant left.
func (c *ChattoCore) HandleCallParticipantLeft(ctx context.Context, spaceID, roomID, userID string, callID ...string) error {
	if c.callService == nil {
		return fmt.Errorf("call service is not initialized")
	}
	return c.callService.AppendLeftForCall(ctx, roomID, userID, optionalCallID(callID), corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_LIVEKIT)
}

// HandleCallRoomFinished appends LiveKit-observed leave facts for any remaining
// projected participants in the room.
// Called by the webhook handler when LiveKit reports a room has finished (closed).
func (c *ChattoCore) HandleCallRoomFinished(ctx context.Context, spaceID, roomID string, callID ...string) error {
	expectedCallID := optionalCallID(callID)
	if expectedCallID != "" {
		active, ok := c.CallState.ActiveCall(roomID)
		if !ok || active.CallID != expectedCallID {
			return nil
		}
	}
	for _, p := range c.CallState.Participants(roomID) {
		if c.callService == nil {
			return fmt.Errorf("call service is not initialized")
		}
		if err := c.callService.AppendLeftForCall(ctx, roomID, p.UserID, expectedCallID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_LIVEKIT); err != nil {
			return err
		}
	}
	return nil
}

func optionalCallID(callID []string) string {
	if len(callID) == 0 {
		return ""
	}
	return callID[0]
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
