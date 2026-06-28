package connectapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type voiceCallService struct {
	api *API
}

func (s *voiceCallService) ListActiveCallRooms(ctx context.Context, _ *connect.Request[apiv1.ListActiveCallRoomsRequest]) (*connect.Response[apiv1.ListActiveCallRoomsResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}
	if !s.api.config.LiveKit.IsConfigured() {
		return connect.NewResponse(&apiv1.ListActiveCallRoomsResponse{}), nil
	}

	roomIDs, err := s.api.core.GetActiveCallRoomIDs(ctx, core.LegacySpaceIDForRoomKind(core.KindChannel))
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ListActiveCallRoomsResponse{RoomIds: roomIDs}), nil
}

func (s *voiceCallService) ListCallParticipants(ctx context.Context, req *connect.Request[apiv1.ListCallParticipantsRequest]) (*connect.Response[apiv1.ListCallParticipantsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	room, _, err := s.api.core.VoiceCallRoomForMember(ctx, caller.UserID, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	if !s.api.config.LiveKit.IsConfigured() {
		return connect.NewResponse(&apiv1.ListCallParticipantsResponse{}), nil
	}

	participants, err := s.api.core.GetCallParticipants(ctx, core.LegacySpaceIDForRoomKind(core.KindOfRoom(room)), room.GetId())
	if err != nil {
		return nil, connectError(err)
	}

	responseParticipants := make([]*apiv1.CallParticipant, 0, len(participants))
	for _, participant := range participants {
		mapped, err := s.callParticipant(ctx, participant)
		if err != nil {
			return nil, err
		}
		if mapped != nil {
			responseParticipants = append(responseParticipants, mapped)
		}
	}
	return connect.NewResponse(&apiv1.ListCallParticipantsResponse{Participants: responseParticipants}), nil
}

func (s *voiceCallService) JoinCall(ctx context.Context, req *connect.Request[apiv1.JoinCallRequest]) (*connect.Response[apiv1.JoinCallResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	_, kind, err := s.api.core.VoiceCallRoomForMember(ctx, caller.UserID, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	if !s.api.config.LiveKit.IsConfigured() {
		return connect.NewResponse(&apiv1.JoinCallResponse{}), nil
	}
	if err := s.api.core.RecordCallParticipantJoined(ctx, kind, req.Msg.GetRoomId(), caller.UserID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.JoinCallResponse{Joined: true}), nil
}

func (s *voiceCallService) GetCallToken(ctx context.Context, req *connect.Request[apiv1.GetCallTokenRequest]) (*connect.Response[apiv1.GetCallTokenResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	_, kind, err := s.api.core.VoiceCallRoomForMember(ctx, caller.UserID, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	if !s.api.config.LiveKit.IsConfigured() {
		return connect.NewResponse(&apiv1.GetCallTokenResponse{}), nil
	}

	user, err := s.api.core.GetUser(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	activeCall, ok := s.api.core.CallState.ActiveCall(req.Msg.GetRoomId())
	if !ok {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no active voice call for room %s", req.Msg.GetRoomId()))
	}
	e2eeKey, err := s.api.core.GetVoiceCallE2EEKey(ctx, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	avatarSize := 96
	avatarURL, _ := s.api.core.GetUserAvatarURL(ctx, caller.UserID, &avatarSize, &avatarSize, "cover")
	roomName := core.LiveKitRoomName(s.api.config.LiveKit.ServerID, core.LegacySpaceIDForRoomKind(kind), req.Msg.GetRoomId(), activeCall.CallID)
	token, err := core.GenerateVoiceCallToken(
		s.api.config.LiveKit.APIKey,
		s.api.config.LiveKit.APISecret,
		roomName,
		user.GetId(),
		user.GetDisplayName(),
		user.GetLogin(),
		s.api.absolutizeAssetURL(ctx, avatarURL),
		e2eeKey,
		activeCall.CallID,
	)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&apiv1.GetCallTokenResponse{
		Token:   token.Token,
		E2EeKey: token.E2EEKey,
		CallId:  token.CallID,
	}), nil
}

func (s *voiceCallService) LeaveCall(ctx context.Context, req *connect.Request[apiv1.LeaveCallRequest]) (*connect.Response[apiv1.LeaveCallResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	_, kind, err := s.api.core.VoiceCallRoomForMember(ctx, caller.UserID, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	if !s.api.config.LiveKit.IsConfigured() {
		return connect.NewResponse(&apiv1.LeaveCallResponse{}), nil
	}
	if err := s.api.core.RecordCallParticipantLeft(ctx, kind, req.Msg.GetRoomId(), caller.UserID, corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.LeaveCallResponse{Left: true}), nil
}

func (s *voiceCallService) callParticipant(ctx context.Context, participant core.CallParticipant) (*apiv1.CallParticipant, error) {
	user, err := s.api.core.GetUser(ctx, participant.UserID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, nil
		}
		return nil, connectError(err)
	}
	presence, err := s.api.core.GetUserPresence(ctx, user.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	apiUser := &apiv1.UserPresenceSummary{
		Id:             user.GetId(),
		Login:          user.GetLogin(),
		DisplayName:    user.GetDisplayName(),
		Deleted:        user.GetDeleted(),
		PresenceStatus: corePresenceStatusToAPI(presence),
		CustomStatus:   coreCustomStatusToAPI(user.GetCustomStatus()),
	}
	avatarSize := 96
	if avatarURL, err := s.api.core.GetUserAvatarURL(ctx, user.GetId(), &avatarSize, &avatarSize, "cover"); err != nil {
		return nil, connectError(err)
	} else if avatarURL != "" {
		apiUser.AvatarUrl = stringPtr(s.api.absolutizeAssetURL(ctx, avatarURL))
	}

	return &apiv1.CallParticipant{
		User:     apiUser,
		JoinedAt: timestamppb.New(time.Unix(participant.JoinedAt, 0)),
		CallId:   participant.CallID,
	}, nil
}
