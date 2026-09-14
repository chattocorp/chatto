package core

import (
	"context"
	"errors"
	"fmt"
	"slices"

	lkauth "github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
)

// reconcileParticipantPermissions runs on each elected LiveKit scan. Expiring a
// token does not revoke an established session. This checks the logical owner
// for companion publishers too, and never relies on a cooperative frontend.
func (c *liveKitRoomClient) reconcileParticipantPermissions(ctx context.Context, roomName, roomID, callID string, participant *livekit.ParticipantInfo) (bool, error) {
	snapshot, err := c.core.GetCallSnapshot(roomID)
	if err != nil {
		return false, err
	}
	// Let the existing stale-room cleanup handle a different call generation.
	if snapshot.Call.CallID != callID {
		return true, nil
	}
	identity := participant.GetIdentity()
	actorID := identity
	companion := IsCallMediaPublisher(participant.GetMetadata())
	if companion {
		actorID = ParseParticipantMetadata(participant.GetMetadata()).OwnerIdentity
	}
	permissions, err := c.core.AuthorizeCall(ctx, actorID, roomID, false)
	if err != nil && !errors.Is(err, ErrPermissionDenied) && !errors.Is(err, ErrNotRoomMember) && !errors.Is(err, ErrNotFound) {
		return false, err
	}
	allowed := err == nil && (!companion || permissions.ScreenShare)
	if allowed && companion {
		_, allowed = callParticipantByUser(snapshot.Participants, actorID)
	}
	if !allowed {
		legacySpaceID, _, _ := ParseLiveKitRoomIdentity(roomName)
		if err := c.RemoveCallParticipant(ctx, legacySpaceID, roomID, callID, identity); err != nil {
			return false, err
		}
		return false, nil
	}
	sources := permissions.PublishSources()
	if companion {
		sources = []livekit.TrackSource{livekit.TrackSource_SCREEN_SHARE, livekit.TrackSource_MICROPHONE}
	}
	desired := &livekit.ParticipantPermission{CanSubscribe: !companion, CanPublish: len(sources) > 0, CanPublishSources: sources, CanPublishData: false}
	if sameCallMediaPermission(participant.GetPermission(), desired) {
		return true, nil
	}
	updater, ok := c.service.(interface {
		UpdateParticipant(context.Context, *livekit.UpdateParticipantRequest) (*livekit.ParticipantInfo, error)
	})
	if !ok {
		return false, fmt.Errorf("LiveKit participant permission updates unavailable")
	}
	_, err = updater.UpdateParticipant(c.withVideoGrant(ctx, &lkauth.VideoGrant{RoomAdmin: true, Room: roomName}), &livekit.UpdateParticipantRequest{Room: roomName, Identity: identity, Permission: desired})
	if err != nil && !isLiveKitRoomNotFound(err) {
		return false, err
	}
	return true, nil
}

func sameCallMediaPermission(current, desired *livekit.ParticipantPermission) bool {
	if current == nil || current.CanSubscribe != desired.CanSubscribe || current.CanPublish != desired.CanPublish || current.CanPublishData != desired.CanPublishData || len(current.CanPublishSources) != len(desired.CanPublishSources) {
		return false
	}
	for _, source := range desired.CanPublishSources {
		if !slices.Contains(current.CanPublishSources, source) {
			return false
		}
	}
	return true
}

// Permission synchronization failure is not a LiveKit listing outage. In
// particular it must not advance the counter that ends all projected calls.
type callPermissionReconcileError struct{ err error }

func (e *callPermissionReconcileError) Error() string {
	return "reconcile call permissions: " + e.err.Error()
}
func (e *callPermissionReconcileError) Unwrap() error { return e.err }
