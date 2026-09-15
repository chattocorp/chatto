package core

import (
	"context"

	"github.com/livekit/protocol/livekit"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
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
	var result CallPermissions
	for i, target := range []*bool{&result.Start, &result.Join, &result.Voice, &result.Camera, &result.ScreenShare} {
		allowed, err := c.PermResolver().HasRoomPermission(ctx, actorID, kind, roomID, callPermissionIDs()[i])
		if err != nil {
			return CallPermissions{}, err
		}
		*target = allowed
	}
	return result, nil
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
	return c.callModel.appendParticipantTransitionAuthorized(ctx, roomID, actorID, true, "", evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER,
		func(snapshot CallRoomSnapshot) error {
			_, err := c.AuthorizeCall(ctx, actorID, roomID, snapshot.Call.CallID == "")
			return err
		})
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
