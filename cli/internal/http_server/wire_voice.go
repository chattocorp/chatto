package http_server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireListActiveCalls(ctx context.Context, userID, requestID string) (*apiv1.ListActiveCallsResponse, *wirev1.WireError) {
	if !c.server.config.LiveKit.IsConfigured() {
		return &apiv1.ListActiveCallsResponse{}, nil
	}

	roomIDs, err := c.server.core.GetActiveCallRoomIDs(ctx, core.LegacySpaceIDForRoomKind(core.KindChannel))
	if err != nil {
		c.server.logger.Warn("Wire failed to get active call rooms", "error", err)
		return &apiv1.ListActiveCallsResponse{}, nil
	}

	resp := &apiv1.ListActiveCallsResponse{Calls: make([]*apiv1.ActiveCallView, 0, len(roomIDs))}
	for _, roomID := range roomIDs {
		if roomID == "" {
			continue
		}
		member, err := c.server.core.RoomMembershipExists(ctx, core.KindChannel, userID, roomID)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if !member {
			continue
		}

		participants, err := c.callParticipantViews(ctx, core.KindChannel, roomID)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		callID := ""
		if call, ok := c.server.core.CallState.ActiveCall(roomID); ok {
			callID = call.CallID
		}
		resp.Calls = append(resp.Calls, &apiv1.ActiveCallView{
			RoomId:       roomID,
			CallId:       callID,
			Participants: participants,
		})
	}
	return resp, nil
}

func (c *wireConn) handleWireGetCallParticipants(ctx context.Context, userID, requestID string, body *apiv1.GetCallParticipantsRequest) (*apiv1.GetCallParticipantsResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !c.server.config.LiveKit.IsConfigured() {
		return &apiv1.GetCallParticipantsResponse{}, nil
	}

	participants, err := c.callParticipantViews(ctx, kind, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.GetCallParticipantsResponse{Participants: participants}, nil
}

func (c *wireConn) handleWireJoinVoiceCall(ctx context.Context, userID, requestID string, body *apiv1.JoinVoiceCallRequest) (*apiv1.JoinVoiceCallResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !c.server.config.LiveKit.IsConfigured() {
		return &apiv1.JoinVoiceCallResponse{Joined: false}, nil
	}

	if err := c.server.core.RecordCallParticipantJoined(ctx, kind, body.GetRoomId(), userID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.JoinVoiceCallResponse{Joined: true}, nil
}

func (c *wireConn) handleWireLeaveVoiceCall(ctx context.Context, userID, requestID string, body *apiv1.LeaveVoiceCallRequest) (*apiv1.LeaveVoiceCallResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !c.server.config.LiveKit.IsConfigured() {
		return &apiv1.LeaveVoiceCallResponse{Left: false}, nil
	}

	if err := c.server.core.RecordCallParticipantLeft(ctx, kind, body.GetRoomId(), userID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.LeaveVoiceCallResponse{Left: true}, nil
}

func (c *wireConn) handleWireGetVoiceCallToken(ctx context.Context, userID, requestID string, body *apiv1.GetVoiceCallTokenRequest) (*apiv1.GetVoiceCallTokenResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !c.server.config.LiveKit.IsConfigured() {
		return &apiv1.GetVoiceCallTokenResponse{}, nil
	}

	user, err := c.server.core.GetUser(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	activeCall, ok := c.server.core.CallState.ActiveCall(body.GetRoomId())
	if !ok {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: no active voice call", core.ErrNotFound))
	}
	e2eeKey, err := c.server.core.GetVoiceCallE2EEKey(ctx, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	avatarSize := 96
	avatarURL, _ := c.server.core.GetUserAvatarURL(ctx, userID, &avatarSize, &avatarSize, "cover")
	livekit := c.server.config.LiveKit
	roomName := core.LiveKitRoomName(livekit.ServerID, core.LegacySpaceIDForRoomKind(kind), body.GetRoomId())
	token, err := core.GenerateVoiceCallToken(
		livekit.APIKey,
		livekit.APISecret,
		roomName,
		user.GetId(),
		user.GetDisplayName(),
		user.GetLogin(),
		avatarURL,
		e2eeKey,
	)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	return &apiv1.GetVoiceCallTokenResponse{Token: &apiv1.VoiceCallTokenView{
		Token:   token.Token,
		E2EeKey: token.E2EEKey,
		CallId:  activeCall.CallID,
	}}, nil
}

func (c *wireConn) callParticipantViews(ctx context.Context, kind core.RoomKind, roomID string) ([]*apiv1.CallParticipantView, error) {
	participants, err := c.server.core.GetCallParticipants(ctx, core.LegacySpaceIDForRoomKind(kind), roomID)
	if err != nil {
		return nil, err
	}
	views := make([]*apiv1.CallParticipantView, 0, len(participants))
	for _, participant := range participants {
		view, err := c.callParticipantView(ctx, participant)
		if err != nil {
			return nil, err
		}
		if view != nil {
			views = append(views, view)
		}
	}
	return views, nil
}

func (c *wireConn) callParticipantView(ctx context.Context, participant core.CallParticipant) (*apiv1.CallParticipantView, error) {
	user, err := c.server.core.GetUser(ctx, participant.UserID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	view := &apiv1.CallParticipantView{
		User:   cloneUser(user),
		CallId: participant.CallID,
	}
	if participant.JoinedAt > 0 {
		view.JoinedAt = timestamppb.New(time.Unix(participant.JoinedAt, 0))
	}
	return view, nil
}
