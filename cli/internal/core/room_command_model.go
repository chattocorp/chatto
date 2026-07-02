package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomCommands returns the operation-level model for public room lifecycle,
// membership, and moderation writes.
func (c *ChattoCore) RoomCommands() *RoomCommandModel {
	return c.roomCommands
}

// RoomCommandModel owns user-facing room commands. Lower-level room helpers are
// still available to trusted internal callers, while this model centralizes
// public API authorization and DM/channel preconditions.
type RoomCommandModel struct {
	core *ChattoCore
}

type RoomCreateInput struct {
	ActorID     string
	GroupID     string
	Name        string
	Description string
	Universal   bool
}

type RoomUpdateInput struct {
	ActorID     string
	RoomID      string
	Name        *string
	Description *string
}

type RoomIDInput struct {
	ActorID string
	RoomID  string
}

type RoomStartDMInput struct {
	ActorID        string
	ParticipantIDs []string
}

type RoomUniversalInput struct {
	ActorID   string
	RoomID    string
	Universal bool
}

type RoomBanInput struct {
	ActorID   string
	RoomID    string
	UserID    string
	Reason    string
	ExpiresAt *time.Time
}

type RoomUnbanInput struct {
	ActorID string
	RoomID  string
	UserID  string
	Reason  string
}

type RoomBanListInput struct {
	ActorID string
	RoomID  *string
}

func (s *RoomCommandModel) CreateRoom(ctx context.Context, input RoomCreateInput) (*corev1.Room, error) {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return nil, err
	}
	if err := validateRoomNameAndDescription(input.Name, input.Description); err != nil {
		return nil, err
	}
	can, err := s.core.CanCreateRoom(ctx, input.ActorID, KindChannel, input.GroupID)
	if err != nil {
		return nil, err
	}
	if !can {
		return nil, ErrPermissionDenied
	}
	return s.core.CreateRoom(ctx, input.ActorID, KindChannel, input.GroupID, input.Name, input.Description, WithUniversalRoom(input.Universal))
}

func (s *RoomCommandModel) UpdateRoom(ctx context.Context, input RoomUpdateInput) (*corev1.Room, error) {
	kind, err := s.authorizeRoomManage(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	if input.Name == nil && input.Description == nil {
		return nil, fmt.Errorf("%w: provide at least one room field to update", ErrInvalidArgument)
	}
	room, err := s.core.GetRoom(ctx, kind, input.RoomID)
	if err != nil {
		return nil, err
	}
	name := room.GetName()
	if input.Name != nil {
		name = *input.Name
	}
	description := room.GetDescription()
	if input.Description != nil {
		description = *input.Description
	}
	if err := validateRoomNameAndDescription(name, description); err != nil {
		return nil, err
	}
	return s.core.UpdateRoom(ctx, input.ActorID, kind, input.RoomID, name, description)
}

func (s *RoomCommandModel) ArchiveRoom(ctx context.Context, input RoomIDInput) (*corev1.Room, error) {
	kind, err := s.authorizeRoomManage(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	return s.core.ArchiveRoom(ctx, input.ActorID, kind, input.RoomID)
}

func (s *RoomCommandModel) UnarchiveRoom(ctx context.Context, input RoomIDInput) (*corev1.Room, error) {
	kind, err := s.authorizeRoomManage(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	return s.core.UnarchiveRoom(ctx, input.ActorID, kind, input.RoomID)
}

func (s *RoomCommandModel) SetRoomUniversal(ctx context.Context, input RoomUniversalInput) (*corev1.Room, error) {
	kind, err := s.authorizeRoomManage(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	return s.core.SetRoomUniversal(ctx, input.ActorID, kind, input.RoomID, input.Universal)
}

func (s *RoomCommandModel) JoinRoom(ctx context.Context, input RoomIDInput) (*corev1.Room, error) {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return nil, err
	}
	kind, err := s.resolveRoomKind(ctx, input.RoomID)
	if err != nil {
		return nil, err
	}
	if kind == KindDM {
		return nil, invalidArgument("DM rooms cannot be joined through RoomService")
	}
	can, err := s.core.CanJoinRoomAt(ctx, input.ActorID, kind, input.RoomID)
	if err != nil {
		return nil, err
	}
	if !can {
		return nil, ErrPermissionDenied
	}
	if _, err := s.core.JoinRoom(ctx, input.ActorID, kind, input.ActorID, input.RoomID); err != nil {
		return nil, err
	}
	return s.core.GetRoom(ctx, kind, input.RoomID)
}

func (s *RoomCommandModel) LeaveRoom(ctx context.Context, input RoomIDInput) error {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return err
	}
	kind, err := s.resolveRoomKind(ctx, input.RoomID)
	if err != nil {
		return err
	}
	return s.core.LeaveRoom(ctx, input.ActorID, kind, input.ActorID, input.RoomID)
}

func (s *RoomCommandModel) StartDM(ctx context.Context, input RoomStartDMInput) (*corev1.Room, bool, error) {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return nil, false, err
	}
	if len(input.ParticipantIDs) > MaxDMParticipants-1 {
		return nil, false, invalidArgument("DM conversations are limited to 10 participants")
	}
	can, err := s.core.CanStartDM(ctx, input.ActorID)
	if err != nil {
		return nil, false, err
	}
	if !can {
		return nil, false, ErrPermissionDenied
	}
	return s.core.FindOrCreateDM(ctx, input.ActorID, input.ParticipantIDs)
}

func (s *RoomCommandModel) BanRoomMember(ctx context.Context, input RoomBanInput) (*RoomBan, error) {
	kind, err := s.authorizeRoomBan(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	if err := validateRoomBanInput(input.Reason, input.ExpiresAt); err != nil {
		return nil, err
	}
	return s.core.BanRoomMember(ctx, input.ActorID, kind, input.RoomID, input.UserID, input.Reason, input.ExpiresAt)
}

func (s *RoomCommandModel) UnbanRoomMember(ctx context.Context, input RoomUnbanInput) error {
	kind, err := s.authorizeRoomBan(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return err
	}
	if err := validateRoomBanReason(input.Reason); err != nil {
		return err
	}
	return s.core.UnbanRoomMember(ctx, input.ActorID, kind, input.RoomID, input.UserID, input.Reason)
}

func (s *RoomCommandModel) ListActiveRoomBans(ctx context.Context, input RoomBanListInput) ([]RoomBan, error) {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return nil, err
	}
	canModerate, err := s.core.HasServerPermission(ctx, input.ActorID, PermRoomMemberBan)
	if err != nil {
		return nil, err
	}
	if !canModerate {
		return nil, ErrPermissionDenied
	}
	return s.core.ListActiveRoomBans(ctx, input.RoomID)
}

func (s *RoomCommandModel) authorizeRoomManage(ctx context.Context, actorID, roomID string) (RoomKind, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return KindChannel, err
	}
	kind, err := s.resolveRoomKind(ctx, roomID)
	if err != nil {
		return KindChannel, err
	}
	if kind == KindDM {
		return KindChannel, invalidArgument("DM rooms cannot be managed through RoomService")
	}
	can, err := s.core.PermResolver().HasRoomPermission(ctx, actorID, kind, roomID, PermRoomManage)
	if err != nil {
		return KindChannel, err
	}
	if !can {
		return KindChannel, ErrPermissionDenied
	}
	return kind, nil
}

func (s *RoomCommandModel) authorizeRoomBan(ctx context.Context, actorID, roomID string) (RoomKind, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return KindChannel, err
	}
	kind, err := s.resolveRoomKind(ctx, roomID)
	if err != nil {
		return KindChannel, err
	}
	if kind == KindDM {
		return KindChannel, ErrCannotBanDMRoomMember
	}
	can, err := s.core.PermResolver().HasRoomPermission(ctx, actorID, kind, roomID, PermRoomMemberBan)
	if err != nil {
		return KindChannel, err
	}
	if !can {
		return KindChannel, ErrPermissionDenied
	}
	return kind, nil
}

func (s *RoomCommandModel) resolveRoomKind(ctx context.Context, roomID string) (RoomKind, error) {
	if strings.TrimSpace(roomID) == "" {
		return KindChannel, invalidArgument("room_id is required")
	}
	return s.core.FindRoomKind(ctx, roomID)
}

func validateRoomBanInput(reason string, expiresAt *time.Time) error {
	if err := validateRoomBanReason(reason); err != nil {
		return err
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return invalidArgument("ban expiry must be in the future")
	}
	return nil
}

func validateRoomNameAndDescription(name, description string) error {
	if err := ValidateRoomName(name); err != nil {
		return invalidArgument(err.Error())
	}
	if err := ValidateRoomDescription(description); err != nil {
		return invalidArgument(err.Error())
	}
	return nil
}

func validateRoomBanReason(reason string) error {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return invalidArgument("ban reason is required")
	}
	if len([]rune(trimmed)) > MaxRoomBanReasonLength {
		return invalidArgument(fmt.Sprintf("ban reason exceeds %d characters", MaxRoomBanReasonLength))
	}
	return nil
}
