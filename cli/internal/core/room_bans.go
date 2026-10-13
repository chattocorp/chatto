package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

const MaxRoomBanReasonLength = 1000

// BanMember records a durable room ban and emits an ordinary leave event
// for public room history. The caller is responsible for permission checks.
// The target must currently be a room member.
func (m *RoomMembershipModel) BanMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID, reason string, expiresAt *time.Time) (*RoomBan, error) {
	return m.banMember(ctx, actorID, kind, roomID, targetUserID, reason, expiresAt, nil)
}

// banMember retains the stored ban fact for suspensions created by RemoveUser.
// Authorization is checked again after each room aggregate catch-up.
func (m *RoomMembershipModel) banMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID, reason string, expiresAt *time.Time, authorize func() error) (*RoomBan, error) {
	if err := m.policy.authorizeAtStableInputs(ctx, authorize); err != nil {
		return nil, err
	}
	if kind == KindDM {
		return nil, ErrCannotRemoveDMRoomMember
	}
	if actorID == targetUserID {
		return nil, ErrPermissionDenied
	}
	if _, err := m.rooms.GetRoom(ctx, kind, roomID); err != nil {
		return nil, err
	}
	if _, err := m.users.GetUser(ctx, targetUserID); err != nil {
		return nil, err
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("removal reason is required")
	}
	if len([]rune(reason)) > MaxRoomBanReasonLength {
		return nil, fmt.Errorf("removal reason exceeds %d characters", MaxRoomBanReasonLength)
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return nil, fmt.Errorf("suspension expiry must be in the future")
	}

	banPayload := &evtv1.RoomMemberBannedEvent{
		RoomId: roomID,
		UserId: targetUserID,
		Reason: reason,
	}
	if expiresAt != nil {
		banPayload.ExpiresAt = timestamppb.New(*expiresAt)
	}
	banEvent := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomMemberBanned{
			RoomMemberBanned: banPayload,
		},
	})

	agg := evtstream.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()
	for attempt := range maxJoinRoomRetries {
		expectedSeq, err := m.publisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("read room ban OCC tail: %w", err)
		}
		if err := m.waitForRoomLeaveTail(ctx, filter, expectedSeq); err != nil {
			return nil, fmt.Errorf("wait for room projections before ban: %w", err)
		}
		if err := m.policy.authorizeAtStableInputs(ctx, authorize); err != nil {
			return nil, err
		}
		isMember, err := m.RoomMembershipExists(ctx, kind, targetUserID, roomID)
		if err != nil {
			return nil, err
		}
		if !isMember {
			return nil, ErrNotRoomMember
		}

		if err := m.appendRoomLeaveBatch(ctx, kind, roomID, targetUserID, expectedSeq, banEvent); err == nil {
			ban, ok := m.rooms.activeRoomBan(roomID, targetUserID, time.Now())
			if !ok {
				return nil, fmt.Errorf("room ban projection did not contain newly published ban")
			}
			return &ban, nil
		} else if !errors.Is(err, events.ErrConflict) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("publish room ban retry exhausted after %d attempts: %w", maxJoinRoomRetries, events.ErrConflict)
}

// removeUserWithoutSuspension records a reasoned moderator removal. It does
// not prevent the target from rejoining under ordinary room permissions.
func (m *RoomMembershipModel) removeUserWithoutSuspension(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID, reason string, authorize func() error) error {
	if err := m.policy.authorizeAtStableInputs(ctx, authorize); err != nil {
		return err
	}
	if kind == KindDM {
		return ErrCannotRemoveDMRoomMember
	}
	if actorID == targetUserID {
		return ErrPermissionDenied
	}
	room, err := m.rooms.GetRoom(ctx, kind, roomID)
	if err != nil {
		return err
	}
	if room.GetUniversal() {
		return invalidArgument("universal room removal requires a suspension")
	}
	if _, err := m.users.GetUser(ctx, targetUserID); err != nil {
		return err
	}

	auditEvent := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomMemberRemoved{
			RoomMemberRemoved: &evtv1.RoomMemberRemovedEvent{
				RoomId: roomID,
				UserId: targetUserID,
				Reason: reason,
			},
		},
	})
	agg := evtstream.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()
	for attempt := range maxJoinRoomRetries {
		expectedSeq, err := m.publisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return fmt.Errorf("read room removal OCC tail: %w", err)
		}
		if err := m.waitForRoomLeaveTail(ctx, filter, expectedSeq); err != nil {
			return fmt.Errorf("wait for room projections before removal: %w", err)
		}
		if err := m.policy.authorizeAtStableInputs(ctx, authorize); err != nil {
			return err
		}
		room, err := m.rooms.GetRoom(ctx, kind, roomID)
		if err != nil {
			return err
		}
		if room.GetUniversal() {
			return invalidArgument("universal room removal requires a suspension")
		}
		if !m.rooms.hasExplicitRoomMembership(roomID, targetUserID) {
			return ErrNotRoomMember
		}
		if err := m.appendRoomLeaveBatch(ctx, kind, roomID, targetUserID, expectedSeq, auditEvent); err == nil {
			return nil
		} else if !errors.Is(err, events.ErrConflict) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return fmt.Errorf("publish room removal retry exhausted after %d attempts: %w", maxJoinRoomRetries, events.ErrConflict)
}

// UnbanMember clears an active room ban. It is idempotent when no active
// ban exists; otherwise a durable moderation event records the moderator action.
func (m *RoomMembershipModel) UnbanMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID, reason string) error {
	if kind == KindDM {
		return ErrCannotRemoveDMRoomMember
	}
	if _, err := m.rooms.GetRoom(ctx, kind, roomID); err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("suspension lift reason is required")
	}
	if len([]rune(reason)) > MaxRoomBanReasonLength {
		return fmt.Errorf("suspension lift reason exceeds %d characters", MaxRoomBanReasonLength)
	}
	if _, ok := m.rooms.activeRoomBan(roomID, targetUserID, time.Now()); !ok {
		return nil
	}

	event := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomMemberUnbanned{
			RoomMemberUnbanned: &evtv1.RoomMemberUnbannedEvent{
				RoomId: roomID,
				UserId: targetUserID,
				Reason: reason,
			},
		},
	})
	pos, err := m.rooms.appendDirectoryEventually(ctx, m.publisher, evtstream.RoomAggregate(roomID), event)
	if err != nil {
		return fmt.Errorf("publish RoomMemberUnbannedEvent: %w", err)
	}
	if err := m.rooms.waitForTimeline(ctx, pos); err != nil {
		return err
	}
	return nil
}

func (m *RoomMembershipModel) ListActiveRoomBans(_ context.Context, roomID *string) ([]RoomBan, error) {
	now := time.Now()
	if roomID != nil && *roomID != "" {
		return m.rooms.activeRoomBans(*roomID, now), nil
	}
	return m.rooms.activeBans(now), nil
}
