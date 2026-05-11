package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	lkauth "github.com/livekit/protocol/auth"
	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// VoiceCallToken contains the LiveKit JWT for a client to join a call.
type VoiceCallToken struct {
	Token string
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

// CallParticipant represents a user currently in a voice call.
type CallParticipant struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	Login       string `json:"login"`
	AvatarURL   string `json:"avatarUrl,omitempty"`
	JoinedAt    int64  `json:"joinedAt"`
}

// callState is the KV value for an active call in a room.
type callState struct {
	Participants []CallParticipant `json:"participants"`
}

// callStateKey builds the CALL_STATE KV key for a room's call.
func callStateKey(spaceID, roomID string) string {
	return spaceID + "." + roomID
}

// LiveKitRoomName constructs a deterministic LiveKit room name from space and room IDs.
// When serverID is non-empty, the room name is prefixed with "{serverID}." so the
// webhook bridge can route events to the correct Chatto instance in shared deployments.
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

	// Strip instance ID prefix if present (dot separator).
	// Safe because instance IDs (K8s names, UUIDs, NanoIDs) and space/room NanoIDs
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

// ParseLiveKitRoomServerID extracts just the instance ID prefix from a LiveKit room
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
func GenerateVoiceCallToken(apiKey, apiSecret, roomName, userID, displayName, login, avatarURL string) (*VoiceCallToken, error) {
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
	return &VoiceCallToken{Token: token}, nil
}

// HandleCallParticipantJoined updates call state in KV and publishes a join event.
// Called by the webhook handler when LiveKit reports a participant joined.
// Idempotent: duplicate calls for the same user are ignored.
// Uses optimistic locking with retry to prevent lost updates from concurrent webhooks.
func (c *ChattoCore) HandleCallParticipantJoined(ctx context.Context, spaceID, roomID, userID, displayName, login, avatarURL string) error {
	key := callStateKey(spaceID, roomID)

	for attempt := 0; attempt < maxCallStateRetries; attempt++ {
		entry := c.readCallState(ctx, key)

		// Idempotent: skip if already present
		for _, p := range entry.state.Participants {
			if p.UserID == userID {
				return nil
			}
		}

		entry.state.Participants = append(entry.state.Participants, CallParticipant{
			UserID:      userID,
			DisplayName: displayName,
			Login:       login,
			AvatarURL:   avatarURL,
			JoinedAt:    time.Now().Unix(),
		})

		err := c.writeCallState(ctx, key, &entry.state, entry.revision)
		if err == nil {
			return c.PublishCallParticipantJoined(ctx, userID, spaceID, roomID)
		}
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("write call state: %w", err)
		}
		// Conflict — retry with fresh state
	}

	return fmt.Errorf("call state update failed after %d retries", maxCallStateRetries)
}

// HandleCallParticipantLeft updates call state in KV and publishes a leave event.
// Called by the webhook handler when LiveKit reports a participant left.
// Uses optimistic locking with retry to prevent lost updates from concurrent webhooks.
func (c *ChattoCore) HandleCallParticipantLeft(ctx context.Context, spaceID, roomID, userID string) error {
	key := callStateKey(spaceID, roomID)

	for attempt := 0; attempt < maxCallStateRetries; attempt++ {
		entry := c.readCallState(ctx, key)

		// Remove participant
		filtered := make([]CallParticipant, 0, len(entry.state.Participants))
		found := false
		for _, p := range entry.state.Participants {
			if p.UserID == userID {
				found = true
				continue
			}
			filtered = append(filtered, p)
		}

		if !found {
			return nil // Not in call, nothing to do
		}

		if len(filtered) == 0 {
			// Call is now empty — delete the key
			_ = c.storage.callStateKV.Delete(ctx, key)
			return c.PublishCallParticipantLeft(ctx, userID, spaceID, roomID)
		}

		entry.state.Participants = filtered
		err := c.writeCallState(ctx, key, &entry.state, entry.revision)
		if err == nil {
			return c.PublishCallParticipantLeft(ctx, userID, spaceID, roomID)
		}
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("write call state: %w", err)
		}
		// Conflict — retry with fresh state
	}

	return fmt.Errorf("call state update failed after %d retries", maxCallStateRetries)
}

// HandleCallRoomFinished clears all call state for a room and publishes leave events.
// Called by the webhook handler when LiveKit reports a room has finished (closed).
func (c *ChattoCore) HandleCallRoomFinished(ctx context.Context, spaceID, roomID string) error {
	key := callStateKey(spaceID, roomID)
	entry := c.readCallState(ctx, key)

	// Publish leave events for any remaining participants
	for _, p := range entry.state.Participants {
		_ = c.PublishCallParticipantLeft(ctx, p.UserID, spaceID, roomID)
	}

	// Delete the key
	_ = c.storage.callStateKV.Delete(ctx, key)
	return nil
}

// GetCallParticipants returns the participants currently in a voice call.
// Returns an empty slice if no call is active.
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) GetCallParticipants(ctx context.Context, spaceID, roomID string) ([]CallParticipant, error) {
	entry := c.readCallState(ctx, callStateKey(spaceID, roomID))
	return entry.state.Participants, nil
}

// GetActiveCallRoomIDs returns the room IDs in a space that have active voice calls.
// Reads from the in-memory CALL_STATE KV bucket (no external API calls).
// Authorization: Caller must verify space membership before calling.
func (c *ChattoCore) GetActiveCallRoomIDs(ctx context.Context, spaceID string) ([]string, error) {
	prefix := spaceID + "."
	keys, err := c.storage.callStateKV.Keys(ctx)
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list call state keys: %w", err)
	}

	var roomIDs []string
	for _, key := range keys {
		if strings.HasPrefix(key, prefix) {
			roomIDs = append(roomIDs, key[len(prefix):])
		}
	}
	return roomIDs, nil
}

// callStateEntry holds call state along with the KV revision for optimistic locking.
type callStateEntry struct {
	state    callState
	revision uint64 // 0 means key did not exist
}

// maxCallStateRetries is the maximum number of CAS retries for call state updates.
const maxCallStateRetries = 5

// readCallState reads and unmarshals call state from KV. Returns empty state on miss.
func (c *ChattoCore) readCallState(ctx context.Context, key string) callStateEntry {
	entry, err := c.storage.callStateKV.Get(ctx, key)
	if err != nil {
		return callStateEntry{}
	}
	var state callState
	if err := json.Unmarshal(entry.Value(), &state); err != nil {
		return callStateEntry{}
	}
	return callStateEntry{state: state, revision: entry.Revision()}
}

// writeCallState marshals and writes call state to KV using optimistic locking.
// Uses Create for new keys and Update with revision for existing keys.
func (c *ChattoCore) writeCallState(ctx context.Context, key string, state *callState, revision uint64) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal call state: %w", err)
	}
	if revision == 0 {
		_, err = c.storage.callStateKV.Create(ctx, key, data)
	} else {
		_, err = c.storage.callStateKV.Update(ctx, key, data, revision)
	}
	return err
}

// PublishCallParticipantJoined publishes a live event notifying room members
// that a user joined a voice call.
func (c *ChattoCore) PublishCallParticipantJoined(ctx context.Context, actorID, spaceID, roomID string) error {
	event := newServerEvent(actorID, &corev1.ServerEvent{
		Event: &corev1.ServerEvent_CallParticipantJoined{
			CallParticipantJoined: &corev1.CallParticipantJoinedEvent{
				SpaceId: spaceID,
				RoomId:  roomID,
			},
		},
	})
	subject := subjects.LiveRoomEvent(kindForSpace(spaceID), roomID, "call_joined")
	return c.publishLiveServerEvent(ctx, subject, event)
}

// PublishCallParticipantLeft publishes a live event notifying room members
// that a user left a voice call.
func (c *ChattoCore) PublishCallParticipantLeft(ctx context.Context, actorID, spaceID, roomID string) error {
	event := newServerEvent(actorID, &corev1.ServerEvent{
		Event: &corev1.ServerEvent_CallParticipantLeft{
			CallParticipantLeft: &corev1.CallParticipantLeftEvent{
				SpaceId: spaceID,
				RoomId:  roomID,
			},
		},
	})
	subject := subjects.LiveRoomEvent(kindForSpace(spaceID), roomID, "call_left")
	return c.publishLiveServerEvent(ctx, subject, event)
}
