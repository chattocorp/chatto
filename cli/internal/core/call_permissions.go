package core

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/livekit/protocol/livekit"
)

// CallPermissions separates call admission from each media source. Membership
// is checked separately. Start never implies Join or a publishing permission.
type CallPermissions struct {
	Start       bool
	Join        bool
	Voice       bool
	Camera      bool
	ScreenShare bool
}

func callPermissionIDs() []Permission {
	return []Permission{PermCallStart, PermCallJoin, PermCallVoice, PermCallCamera, PermCallScreenShare}
}

// resolveCallPermissions runs inside the caller's content-view read boundary.
func (c *ChattoCore) resolveCallPermissions(ctx context.Context, actorID string, kind RoomKind, roomID string) (CallPermissions, error) {
	return c.authorization.resolveCallPermissions(ctx, actorID, kind, roomID)
}

// AuthorizeCall checks current membership and permissions at stable authority
// inputs. A start additionally needs call.start. Credential issuance must use
// the returned permissions, and must separately validate the call generation.
func (c *ChattoCore) AuthorizeCall(ctx context.Context, actorID, roomID string, starting bool) (CallPermissions, error) {
	var permissions CallPermissions
	err := c.authorizeAtStableInputs(ctx, func() error {
		// Exact-room membership/layout changes must also be stable for token reads,
		// which do not have the join command's room OCC guard.
		before, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
		if err != nil {
			return err
		}
		if err := c.roomModel.waitForDirectory(ctx, before); err != nil {
			return err
		}
		err = c.ReadServerContentView(ctx, func(readCtx context.Context, _ uint64) error {
			_, kind, err := c.requireRoomMember(readCtx, actorID, roomID)
			if err != nil {
				return err
			}
			permissions, err = c.resolveCallPermissions(readCtx, actorID, kind, roomID)
			if err != nil {
				return err
			}
			if !permissions.Join || (starting && !permissions.Start) {
				return ErrPermissionDenied
			}
			return nil
		})
		if err != nil {
			return err
		}
		after, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
		if err != nil {
			return err
		}
		if before.Seq != after.Seq {
			return events.ErrConflict
		}
		return nil
	})
	return permissions, err
}

// JoinVoiceCall records authorized user intent. The authorization callback runs
// on every OCC attempt, including a retry that changes a join into a start.
func (c *ChattoCore) JoinVoiceCall(ctx context.Context, actorID, roomID string) error {
	if _, _, err := c.requireRoomMember(ctx, actorID, roomID); err != nil {
		return err
	}
	_, err := c.admitCall(ctx, actorID, roomID, true)
	return err
}

// admitCall checks permissions on each append attempt. Only an explicit join
// can start a new call. Direct token requests can reserve an existing place.
func (c *ChattoCore) admitCall(ctx context.Context, actorID, roomID string, allowStart bool) (CallPermissions, error) {
	var permissions CallPermissions
	err := c.callModel.appendParticipantTransitionAuthorized(ctx, roomID, actorID, true, "", evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER, c.callModel.maxGlobalParticipants,
		func(snapshot CallRoomSnapshot) error {
			starting := snapshot.Call.CallID == ""
			var err error
			permissions, err = c.AuthorizeCall(ctx, actorID, roomID, allowStart && starting)
			if err == nil && starting && !allowStart {
				return ErrNotFound
			}
			return err
		})
	return permissions, err
}

// CallTokenAccess authorizes primary media access and resolves the key for
// the active generation. A capped server first records the participant's place.
func (c *ChattoCore) CallTokenAccess(ctx context.Context, actorID, roomID string) (CallPermissions, CallAccessMaterial, error) {
	var permissions CallPermissions
	var access CallAccessMaterial
	var err error
	if c.callModel.maxGlobalParticipants >= 0 {
		permissions, err = c.admitCall(ctx, actorID, roomID, false)
		if err == nil {
			access, err = c.callModel.GetParticipantAccessMaterial(ctx, roomID, actorID)
		}
	} else {
		permissions, err = c.AuthorizeCall(ctx, actorID, roomID, false)
		if err == nil {
			access, err = c.GetVoiceCallAccessMaterial(ctx, roomID)
		}
	}
	return permissions, access, err
}

// PublishSources maps logical permissions to the browser participant's sources.
// Native publishers use a separate grant: their microphone source carries
// captured application audio and requires ScreenShare instead of Voice.
func (p CallPermissions) PublishSources() []livekit.TrackSource {
	var sources []livekit.TrackSource
	if p.Voice {
		sources = append(sources, livekit.TrackSource_MICROPHONE)
	}
	if p.Camera {
		sources = append(sources, livekit.TrackSource_CAMERA)
	}
	if p.ScreenShare {
		sources = append(sources, livekit.TrackSource_SCREEN_SHARE, livekit.TrackSource_SCREEN_SHARE_AUDIO)
	}
	return sources
}
