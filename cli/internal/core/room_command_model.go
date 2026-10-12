package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// RoomCommands returns the operation-level model for public room lifecycle,
// membership, and moderation writes.
func (c *ChattoCore) RoomCommands() *RoomCommandModel {
	return c.roomCommands
}

// RoomCommandModel owns user-facing room commands. Lower-level room helpers are
// still available to trusted internal callers, while this model centralizes
// public API authorization and DM/channel preconditions. The publisher and
// projection owner keep update and pin consistency inside this model; explicit
// capabilities supply the remaining policy and low-level mutations.
type RoomCommandModel struct {
	pins          roomPinOperations
	rooms         *RoomModel
	permissions   *PermissionResolver
	publisher     *evtstream.Publisher
	users         *UserModel
	authorization roomCommandAuthorization
	memberships   roomCommandMemberships
	lifecycle     roomCommandLifecycle
	accounts      roomCommandAccounts
}

type RoomCreateInput struct {
	ActorID       string
	GroupID       string
	Name          string
	Description   string
	Universal     bool
	ThreadingMode evtv1.RoomThreadingMode
}

type RoomUpdateInput struct {
	ActorID         string
	RoomID          string
	Name            *string
	Description     *string
	Universal       *bool
	SlowModeSeconds *uint32
	ThreadingMode   *evtv1.RoomThreadingMode
}

type RoomIDInput struct {
	ActorID string
	RoomID  string
}

type RoomUserInput struct {
	ActorID string
	RoomID  string
	UserID  string
}

type RoomStartDMInput struct {
	ActorID        string
	ParticipantIDs []string
}

// RoomRemoveUserInput describes a reasoned moderation removal. Suspension is
// true for both a timed suspension and an indefinite one.
type RoomRemoveUserInput struct {
	ActorID    string
	RoomID     string
	UserID     string
	Reason     string
	Suspension bool
	ExpiresAt  *time.Time
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

func (s *RoomCommandModel) CreateRoom(ctx context.Context, input RoomCreateInput) (*evtv1.Room, error) {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return nil, err
	}
	if err := validateRoomNameAndDescription(input.Name, input.Description); err != nil {
		return nil, err
	}
	can, err := s.authorization.CanCreateRoom(ctx, input.ActorID, KindChannel, input.GroupID)
	if err != nil {
		return nil, err
	}
	if !can {
		return nil, ErrPermissionDenied
	}
	if input.ThreadingMode != evtv1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED && !IsValidRoomThreadingMode(input.ThreadingMode) {
		return nil, invalidArgument("invalid room threading mode")
	}
	return s.lifecycle.CreateRoom(ctx, input.ActorID, KindChannel, input.GroupID, input.Name, input.Description,
		WithUniversalRoom(input.Universal), WithRoomThreadingMode(input.ThreadingMode))
}

// UpdateRoom commits all supplied fields atomically. Omitted fields retain
// their latest values, including after a concurrent update forces a retry.
func (s *RoomCommandModel) UpdateRoom(ctx context.Context, input RoomUpdateInput) (*evtv1.Room, error) {
	return s.updateRoom(ctx, input, nil)
}

// updateRoom rebuilds the complete sparse patch after each OCC conflict. A
// name patch guards the room catalog for uniqueness; other patches guard only
// the target room. attemptPrepared is a test seam immediately before commit.
func (s *RoomCommandModel) updateRoom(ctx context.Context, input RoomUpdateInput, attemptPrepared func(context.Context) error) (*evtv1.Room, error) {
	kind, err := s.authorizeRoomManage(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	if input.Name == nil && input.Description == nil && input.Universal == nil && input.SlowModeSeconds == nil && input.ThreadingMode == nil {
		return nil, fmt.Errorf("%w: provide at least one room field to update", ErrInvalidArgument)
	}
	if input.SlowModeSeconds != nil && *input.SlowModeSeconds > MaxRoomSlowModeSeconds {
		return nil, invalidArgument("slow mode cannot exceed 21600 seconds")
	}
	if input.ThreadingMode != nil && !IsValidRoomThreadingMode(*input.ThreadingMode) {
		return nil, invalidArgument("invalid room threading mode")
	}
	agg := evtstream.RoomAggregate(input.RoomID)
	filter := agg.AllEventsFilter()
	if input.Name != nil {
		filter = evtstream.RoomSubjectFilter()
	}
	for range maxRoomNameClaimRetries {
		position, err := s.publisher.LastSubjectPosition(ctx, filter)
		if err != nil {
			return nil, err
		}
		if err := s.rooms.waitForDirectory(ctx, position); err != nil {
			return nil, err
		}
		if err := s.authorization.authorizeAtStableInputs(ctx, func() error {
			_, err := s.authorizeRoomManage(ctx, input.ActorID, input.RoomID)
			return err
		}); err != nil {
			return nil, err
		}
		room, err := s.rooms.GetRoom(ctx, kind, input.RoomID)
		if err != nil {
			return nil, err
		}
		name, description := room.GetName(), room.GetDescription()
		if input.Name != nil {
			name = normalizeRoomName(*input.Name)
		}
		if input.Description != nil {
			description = *input.Description
		}
		if err := validateRoomNameAndDescription(name, description); err != nil {
			return nil, err
		}
		if input.Name != nil && s.rooms.nameClaimSnapshot(name, input.RoomID).ConflictingRoomID != "" {
			return nil, ErrRoomNameExists
		}
		var entries []evtstream.BatchEntry
		add := func(event *evtv1.Event) {
			event = newEvent(input.ActorID, event)
			entries = append(entries, evtstream.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
		}
		if name != room.GetName() || description != room.GetDescription() {
			add(&evtv1.Event{Event: &evtv1.Event_RoomUpdated{RoomUpdated: &evtv1.RoomUpdatedEvent{RoomId: input.RoomID, Name: name, Description: description}}})
		}
		if input.Universal != nil && *input.Universal != room.GetUniversal() {
			add(&evtv1.Event{Event: &evtv1.Event_RoomUniversalChanged{RoomUniversalChanged: &evtv1.RoomUniversalChangedEvent{RoomId: input.RoomID, Universal: *input.Universal}}})
		}
		if input.SlowModeSeconds != nil && *input.SlowModeSeconds != room.GetSlowModeSeconds() {
			add(&evtv1.Event{Event: &evtv1.Event_RoomSlowModeChanged{RoomSlowModeChanged: &evtv1.RoomSlowModeChangedEvent{RoomId: input.RoomID, SlowModeSeconds: *input.SlowModeSeconds}}})
		}
		if input.ThreadingMode != nil && *input.ThreadingMode != EffectiveRoomThreadingMode(room) {
			add(&evtv1.Event{Event: &evtv1.Event_RoomThreadingModeChanged{RoomThreadingModeChanged: &evtv1.RoomThreadingModeChangedEvent{RoomId: input.RoomID, ThreadingMode: *input.ThreadingMode}}})
		}
		if len(entries) == 0 {
			return room, nil
		}
		entries[0].Expect = events.ExpectFilterSeq(filter, position.Seq)
		if attemptPrepared != nil {
			if err := attemptPrepared(ctx); err != nil {
				return nil, err
			}
		}
		seqs, err := s.publisher.AppendBatch(ctx, entries)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		last := len(entries) - 1
		if err := s.rooms.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(entries[last].Subject, seqs[last])); err != nil {
			return nil, err
		}
		return s.rooms.GetRoom(ctx, kind, input.RoomID)
	}
	return nil, fmt.Errorf("room update retries exhausted: %w", events.ErrConflict)
}

func (s *RoomCommandModel) ArchiveRoom(ctx context.Context, input RoomIDInput) (*evtv1.Room, error) {
	kind, err := s.authorizeRoomManage(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	return s.lifecycle.ArchiveRoom(ctx, input.ActorID, kind, input.RoomID)
}

func (s *RoomCommandModel) UnarchiveRoom(ctx context.Context, input RoomIDInput) (*evtv1.Room, error) {
	kind, err := s.authorizeRoomManage(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	return s.lifecycle.UnarchiveRoom(ctx, input.ActorID, kind, input.RoomID)
}

func (s *RoomCommandModel) JoinRoom(ctx context.Context, input RoomIDInput) (*evtv1.Room, error) {
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
	can, err := s.authorization.CanJoinRoomAt(ctx, input.ActorID, kind, input.RoomID)
	if err != nil {
		return nil, err
	}
	if !can {
		return nil, ErrPermissionDenied
	}
	if _, err := s.memberships.JoinRoom(ctx, input.ActorID, kind, input.ActorID, input.RoomID); err != nil {
		return nil, err
	}
	return s.rooms.GetRoom(ctx, kind, input.RoomID)
}

func (s *RoomCommandModel) LeaveRoom(ctx context.Context, input RoomIDInput) error {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return err
	}
	kind, err := s.resolveRoomKind(ctx, input.RoomID)
	if err != nil {
		return err
	}
	return s.memberships.LeaveRoom(ctx, input.ActorID, kind, input.ActorID, input.RoomID)
}

// AddMember permits account and room managers to add members without room.join.
// Bot managers without either override need the bot's effective room.join,
// including its owner's ceiling. Bans and room lifecycle restrictions still apply.
func (s *RoomCommandModel) AddMember(ctx context.Context, input RoomUserInput) (*evtv1.RoomMembership, error) {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return nil, err
	}
	kind, err := s.resolveRoomKind(ctx, input.RoomID)
	if err != nil {
		return nil, err
	}
	return s.memberships.addMember(ctx, input.ActorID, kind, input.RoomID, input.UserID, func() error {
		return s.authorizeMembershipChange(ctx, input, true)
	})
}

// RemoveMember permits account, room, and target-bot managers to remove members,
// even after join authority is lost. Membership changes preserve grants.
func (s *RoomCommandModel) RemoveMember(ctx context.Context, input RoomUserInput) (bool, error) {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return false, err
	}
	kind, err := s.resolveRoomKind(ctx, input.RoomID)
	if err != nil {
		return false, err
	}
	return s.memberships.removeMember(ctx, input.ActorID, kind, input.RoomID, input.UserID, func() error {
		return s.authorizeMembershipChange(ctx, input, false)
	})
}

// authorizeMembershipChange runs inside the room OCC retry with stable
// ownership and permission inputs. Bot management does not imply room.manage.
func (s *RoomCommandModel) authorizeMembershipChange(ctx context.Context, input RoomUserInput, joining bool) error {
	if kind, err := s.resolveRoomKind(ctx, input.RoomID); err != nil {
		return err
	} else if kind == KindDM {
		return invalidArgument("DM room participants cannot be managed through RoomService")
	}
	user, err := s.accounts.GetUser(ctx, input.UserID)
	if err != nil {
		// Preserve the room-manager gate before disclosing a missing target.
		if _, gateErr := s.authorizeRoomManage(ctx, input.ActorID, input.RoomID); gateErr != nil {
			return gateErr
		}
		return err
	}
	accountManager, err := s.authorization.CanManageUserAccounts(ctx, input.ActorID)
	if err != nil {
		return err
	}
	roomManager, err := s.permissions.HasRoomPermission(ctx, input.ActorID, KindChannel, input.RoomID, PermRoomManage)
	if err != nil {
		return err
	}
	if !accountManager && !roomManager {
		if !user.GetIsBot() {
			return ErrPermissionDenied
		}
		if _, err := s.authorization.requireBotManager(ctx, input.ActorID, input.UserID); err != nil {
			return err
		}
	}
	room, err := s.rooms.FindRoomByID(ctx, input.RoomID)
	if err != nil {
		return err
	}
	if KindOfRoom(room) != KindChannel || room.GetUniversal() {
		return invalidArgument("direct-message and universal room membership cannot be managed explicitly")
	}
	if room.GetArchived() && joining {
		return ErrRoomArchived
	}
	if joining && s.rooms.isRoomBanActive(input.RoomID, input.UserID, time.Now()) {
		return ErrPermissionDenied
	}
	if joining && !accountManager && !roomManager {
		allowed, err := s.authorization.CanJoinRoomAt(ctx, input.UserID, KindChannel, input.RoomID)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrPermissionDenied
		}
	}
	return nil
}

func (s *RoomCommandModel) StartDM(ctx context.Context, input RoomStartDMInput) (*evtv1.Room, bool, error) {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return nil, false, err
	}
	if len(input.ParticipantIDs) > MaxDMParticipants-1 {
		return nil, false, invalidArgument("DM conversations are limited to 10 participants")
	}
	isBot, _, accountExists := s.users.isBotAndOwner(input.ActorID)
	if !accountExists {
		return nil, false, ErrNotFound
	}
	// Bots cannot use StartDM as a lookup operation. This prevents an existing
	// membership from becoming a bot-discoverable DM directory.
	if isBot {
		return nil, false, ErrPermissionDenied
	}
	room, found, err := s.lifecycle.FindDM(ctx, input.ActorID, input.ParticipantIDs)
	if err != nil {
		return nil, false, err
	}
	if found {
		return room, false, nil
	}
	can, err := s.authorization.CanStartDM(ctx, input.ActorID)
	if err != nil {
		return nil, false, err
	}
	if !can {
		return nil, false, ErrPermissionDenied
	}
	return s.lifecycle.FindOrCreateDM(ctx, input.ActorID, input.ParticipantIDs)
}

// RemoveUser removes a current member and optionally prevents rejoining.
// It uses the room.remove-member permission at the target room.
func (s *RoomCommandModel) RemoveUser(ctx context.Context, input RoomRemoveUserInput) error {
	kind, err := s.authorizeRoomRemoval(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return err
	}
	if err := validateRoomRemovalInput(input.Reason, input.Suspension, input.ExpiresAt); err != nil {
		return err
	}
	authorize := func() error {
		_, err := s.authorizeRoomRemoval(ctx, input.ActorID, input.RoomID)
		return err
	}
	if input.Suspension {
		_, err := s.memberships.banMember(ctx, input.ActorID, kind, input.RoomID, input.UserID, input.Reason, input.ExpiresAt, authorize)
		return err
	}
	return s.memberships.removeUserWithoutSuspension(ctx, input.ActorID, kind, input.RoomID, input.UserID, input.Reason, authorize)
}

func (s *RoomCommandModel) LiftSuspension(ctx context.Context, input RoomUnbanInput) error {
	kind, err := s.authorizeRoomRemoval(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return err
	}
	if err := validateRoomRemovalReason(input.Reason); err != nil {
		return err
	}
	return s.memberships.UnbanMember(ctx, input.ActorID, kind, input.RoomID, input.UserID, input.Reason)
}

func (s *RoomCommandModel) ListActiveRoomSuspensions(ctx context.Context, input RoomBanListInput) ([]RoomBan, error) {
	if err := requireAuthenticatedActor(input.ActorID); err != nil {
		return nil, err
	}
	canModerate, err := s.authorization.HasServerPermission(ctx, input.ActorID, PermRoomMemberRemove)
	if err != nil {
		return nil, err
	}
	if !canModerate {
		return nil, ErrPermissionDenied
	}
	return s.memberships.ListActiveRoomBans(ctx, input.RoomID)
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
	can, err := s.permissions.HasRoomPermission(ctx, actorID, kind, roomID, PermRoomManage)
	if err != nil {
		return KindChannel, err
	}
	if !can {
		return KindChannel, ErrPermissionDenied
	}
	return kind, nil
}

func (s *RoomCommandModel) authorizeRoomRemoval(ctx context.Context, actorID, roomID string) (RoomKind, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return KindChannel, err
	}
	kind, err := s.resolveRoomKind(ctx, roomID)
	if err != nil {
		return KindChannel, err
	}
	if kind == KindDM {
		return KindChannel, ErrCannotRemoveDMRoomMember
	}
	can, err := s.permissions.HasRoomPermission(ctx, actorID, kind, roomID, PermRoomMemberRemove)
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
	return s.rooms.FindRoomKind(ctx, roomID)
}

func validateRoomRemovalInput(reason string, suspension bool, expiresAt *time.Time) error {
	if err := validateRoomRemovalReason(reason); err != nil {
		return err
	}
	if !suspension && expiresAt != nil {
		return invalidArgument("suspension expiry requires a suspension")
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return invalidArgument("suspension expiry must be in the future")
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

func validateRoomRemovalReason(reason string) error {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return invalidArgument("removal reason is required")
	}
	if len([]rune(trimmed)) > MaxRoomBanReasonLength {
		return invalidArgument(fmt.Sprintf("removal reason exceeds %d characters", MaxRoomBanReasonLength))
	}
	return nil
}

// roomCommandAuthorization supplies authorization operations used by the room model.
type roomCommandAuthorization interface {
	CanCreateRoom(ctx context.Context, userID string, kind RoomKind, groupID string) (bool, error)
	authorizeAtStableInputs(ctx context.Context, check func() error) error
	CanJoinRoomAt(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error)
	CanManageUserAccounts(ctx context.Context, userID string) (bool, error)
	requireBotManager(ctx context.Context, actorID, botID string) (*evtv1.User, error)
	CanStartDM(ctx context.Context, userID string) (bool, error)
	HasServerPermission(ctx context.Context, userID string, perm Permission) (bool, error)
}

// roomCommandMemberships supplies membership operations used by the room model.
type roomCommandMemberships interface {
	RoomMembershipExists(context.Context, RoomKind, string, string) (bool, error)
	addMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID string, authorize func() error) (*evtv1.RoomMembership, error)
	removeMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID string, authorize func() error) (bool, error)
	banMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID, reason string, expiresAt *time.Time, authorize func() error) (*RoomBan, error)
	removeUserWithoutSuspension(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID, reason string, authorize func() error) error
	UnbanMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID, reason string) error
	ListActiveRoomBans(_ context.Context, roomID *string) ([]RoomBan, error)
	JoinRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) (*evtv1.RoomMembership, error)
	LeaveRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) error
}

// roomCommandLifecycle supplies lifecycle operations used by the room model.
type roomCommandLifecycle interface {
	CreateRoom(ctx context.Context, actorID string, kind RoomKind, groupID, name, description string, opts ...CreateRoomOption) (*evtv1.Room, error)
	ArchiveRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (*evtv1.Room, error)
	UnarchiveRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (*evtv1.Room, error)
	FindDM(ctx context.Context, creatorID string, participantIDs []string) (*evtv1.Room, bool, error)
	FindOrCreateDM(ctx context.Context, creatorID string, participantIDs []string) (*evtv1.Room, bool, error)
}

// roomCommandAccounts supplies account reads used by the room model.
type roomCommandAccounts interface {
	GetUser(ctx context.Context, userID string) (*evtv1.User, error)
}

// roomPinOperations supplies message identity and consistent authorization for
// pin writes. The command owns the event batch and projection wait.
type roomPinOperations interface {
	prepareMessageAppendAttempt(context.Context, evtstream.Aggregate, func(context.Context) error) (messageAppendAttempt, error)
	requireRoomMember(context.Context, string, string) (*evtv1.Room, RoomKind, error)
	CanReadMessage(context.Context, string, RoomKind, string, string) (bool, error)
	canonicalReactionMessageEventID(string, string) (string, error)
}

// JoinGroup joins each eligible active room in a group. Existing memberships
// are skipped. Each join keeps its own OCC and projection barrier.
func (s *RoomCommandModel) JoinGroup(ctx context.Context, actorID, groupID string) ([]string, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	group, err := s.rooms.GetRoomGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}

	joined := make([]string, 0, len(group.GetRoomIds()))
	for _, roomID := range group.GetRoomIds() {
		room, err := s.rooms.GetRoom(ctx, KindChannel, roomID)
		if err != nil {
			return nil, err
		}
		if room.GetArchived() {
			continue
		}
		alreadyMember, err := s.memberships.RoomMembershipExists(ctx, KindChannel, actorID, roomID)
		if err != nil {
			return nil, err
		}
		if alreadyMember {
			continue
		}
		canJoin, err := s.authorization.CanJoinRoomAt(ctx, actorID, KindChannel, roomID)
		if err != nil {
			return nil, err
		}
		if !canJoin {
			continue
		}
		if _, err := s.memberships.JoinRoom(ctx, actorID, KindChannel, actorID, roomID); err != nil {
			return nil, fmt.Errorf("join %s: %w", roomID, err)
		}
		joined = append(joined, roomID)
	}
	return joined, nil
}
