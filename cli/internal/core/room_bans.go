package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const MaxRoomBanReasonLength = 1000

// BanRoomMember records a durable room ban and emits an ordinary leave event
// for public room history. The caller is responsible for permission checks.
// The target must currently be a room member.
func (c *ChattoCore) BanRoomMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID, reason string, expiresAt *time.Time) (*RoomBan, error) {
	if kind == KindDM {
		return nil, ErrCannotBanDMRoomMember
	}
	if actorID == targetUserID {
		return nil, ErrPermissionDenied
	}
	if _, err := c.GetRoom(ctx, kind, roomID); err != nil {
		return nil, err
	}
	if _, err := c.GetUser(ctx, targetUserID); err != nil {
		return nil, err
	}
	isMember, err := c.RoomMembershipExists(ctx, kind, targetUserID, roomID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, ErrNotRoomMember
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("ban reason is required")
	}
	if len([]rune(reason)) > MaxRoomBanReasonLength {
		return nil, fmt.Errorf("ban reason exceeds %d characters", MaxRoomBanReasonLength)
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return nil, fmt.Errorf("ban expiry must be in the future")
	}

	banPayload := &corev1.RoomMemberBannedEvent{
		RoomId: roomID,
		UserId: targetUserID,
		Reason: reason,
	}
	if expiresAt != nil {
		banPayload.ExpiresAt = timestamppb.New(*expiresAt)
	}
	banEvent := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomMemberBanned{
			RoomMemberBanned: banPayload,
		},
	})

	banPos, err := c.rooms().appendDirectoryEventually(ctx, c.EventPublisher, events.RoomAggregate(roomID), banEvent)
	if err != nil {
		return nil, fmt.Errorf("publish RoomMemberBannedEvent: %w", err)
	}
	if err := c.rooms().waitForTimeline(ctx, banPos); err != nil {
		return nil, err
	}

	leaveEvent := newEvent(targetUserID, &corev1.Event{
		Event: &corev1.Event_UserLeftRoom{
			UserLeftRoom: &corev1.UserLeftRoomEvent{
				RoomId: roomID,
			},
		},
	})
	leavePos, err := c.rooms().appendDirectoryEventually(ctx, c.EventPublisher, events.RoomAggregate(roomID), leaveEvent)
	if err != nil {
		return nil, fmt.Errorf("publish UserLeftRoomEvent for room ban: %w", err)
	}
	if err := c.rooms().waitForTimeline(ctx, leavePos); err != nil {
		return nil, err
	}

	ban, ok := c.rooms().activeRoomBan(roomID, targetUserID, time.Now())
	if !ok {
		return nil, fmt.Errorf("room ban projection did not contain newly published ban")
	}
	return &ban, nil
}

// UnbanRoomMember clears an active room ban. It is idempotent when no active
// ban exists; otherwise a durable moderation event records the moderator action.
func (c *ChattoCore) UnbanRoomMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID, reason string) error {
	if kind == KindDM {
		return ErrCannotBanDMRoomMember
	}
	if _, err := c.GetRoom(ctx, kind, roomID); err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("unban reason is required")
	}
	if len([]rune(reason)) > MaxRoomBanReasonLength {
		return fmt.Errorf("unban reason exceeds %d characters", MaxRoomBanReasonLength)
	}
	if _, ok := c.rooms().activeRoomBan(roomID, targetUserID, time.Now()); !ok {
		return nil
	}

	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomMemberUnbanned{
			RoomMemberUnbanned: &corev1.RoomMemberUnbannedEvent{
				RoomId: roomID,
				UserId: targetUserID,
				Reason: reason,
			},
		},
	})
	pos, err := c.rooms().appendDirectoryEventually(ctx, c.EventPublisher, events.RoomAggregate(roomID), event)
	if err != nil {
		return fmt.Errorf("publish RoomMemberUnbannedEvent: %w", err)
	}
	if err := c.rooms().waitForTimeline(ctx, pos); err != nil {
		return err
	}
	return nil
}

func (c *ChattoCore) ListActiveRoomBans(_ context.Context, roomID *string) ([]RoomBan, error) {
	now := time.Now()
	if roomID != nil && *roomID != "" {
		return c.rooms().activeRoomBans(*roomID, now), nil
	}
	return c.rooms().activeBans(now), nil
}
