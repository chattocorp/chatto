package http_server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireListRoomBans(ctx context.Context, userID, requestID string, body *apiv1.ListRoomBansRequest) (*apiv1.ListRoomBansResponse, *wirev1.WireError) {
	canModerate, err := c.server.core.HasServerPermission(ctx, userID, core.PermRoomMemberBan)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("check room.ban-member: %w", err))
	}
	if !canModerate {
		return nil, c.errorFromRequestErr(requestID, core.ErrPermissionDenied)
	}

	var roomID *string
	if trimmed := strings.TrimSpace(body.GetRoomId()); trimmed != "" {
		roomID = &trimmed
	}
	bans, err := c.server.core.ListActiveRoomBans(ctx, roomID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	out := make([]*apiv1.RoomBanView, 0, len(bans))
	for _, ban := range bans {
		view, err := c.roomBanView(ctx, ban)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		out = append(out, view)
	}
	return &apiv1.ListRoomBansResponse{Bans: out}, nil
}

func (c *wireConn) handleWireUnbanRoomMember(ctx context.Context, userID, requestID string, body *apiv1.UnbanRoomMemberRequest) (*apiv1.UnbanRoomMemberResponse, *wirev1.WireError) {
	if body.GetRoomId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: room_id is required", errWireInvalidArgument))
	}
	if body.GetUserId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: user_id is required", errWireInvalidArgument))
	}
	reason := strings.TrimSpace(body.GetReason())
	if reason == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: unban reason is required", errWireInvalidArgument))
	}
	if len([]rune(reason)) > core.MaxRoomBanReasonLength {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: unban reason exceeds %d characters", errWireInvalidArgument, core.MaxRoomBanReasonLength))
	}

	kind, err := c.server.core.FindRoomKind(ctx, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if kind == core.KindDM {
		return nil, c.errorFromRequestErr(requestID, core.ErrCannotBanDMRoomMember)
	}

	canBan, err := c.server.core.PermResolver().HasRoomPermission(ctx, userID, core.KindChannel, body.GetRoomId(), core.PermRoomMemberBan)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !canBan {
		return nil, c.errorFromRequestErr(requestID, core.ErrPermissionDenied)
	}

	if err := c.server.core.UnbanRoomMember(ctx, userID, core.KindChannel, body.GetRoomId(), body.GetUserId(), reason); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.UnbanRoomMemberResponse{Unbanned: true}, nil
}

func (c *wireConn) roomBanView(ctx context.Context, ban core.RoomBan) (*apiv1.RoomBanView, error) {
	view := &apiv1.RoomBanView{
		Id:          ban.EventID,
		RoomId:      ban.RoomID,
		UserId:      ban.UserID,
		ModeratorId: ban.ModeratorID,
		Reason:      ban.Reason,
	}
	if !ban.CreatedAt.IsZero() {
		view.CreatedAt = timestamppb.New(ban.CreatedAt)
	}
	if ban.ExpiresAt != nil && !ban.ExpiresAt.IsZero() {
		view.ExpiresAt = timestamppb.New(*ban.ExpiresAt)
	}

	room, err := c.roomBanRoomView(ctx, ban.RoomID)
	if err != nil {
		return nil, err
	}
	view.Room = room

	user, err := c.userAvatarView(ctx, ban.UserID)
	if err != nil {
		return nil, err
	}
	view.User = user

	moderator, err := c.userAvatarView(ctx, ban.ModeratorID)
	if err != nil {
		return nil, err
	}
	view.Moderator = moderator

	return view, nil
}

func (c *wireConn) roomBanRoomView(ctx context.Context, roomID string) (*apiv1.AdminRoomInfoView, error) {
	if roomID == "" {
		return nil, nil
	}
	room, err := c.server.core.GetRoom(ctx, core.KindChannel, roomID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, core.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return adminRoomInfoView(room), nil
}

func (c *wireConn) userAvatarView(ctx context.Context, userID string) (*apiv1.UserAvatarView, error) {
	if userID == "" {
		return nil, nil
	}
	user, err := c.server.core.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, core.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	avatarURL, err := c.server.core.GetUserAvatarURL(ctx, userID, nil, nil, "")
	if err != nil {
		return nil, err
	}
	presence, err := c.server.core.GetUserPresence(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &apiv1.UserAvatarView{
		User:           cloneUser(user),
		AvatarUrl:      avatarURL,
		PresenceStatus: currentUserPresenceStatus(presence),
	}, nil
}
