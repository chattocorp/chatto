package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RoomInvitationModel owns invitation commands and participant-only reads.
// All invitation transitions share the room aggregate guard with membership and lifecycle.
type RoomInvitationModel struct {
	core       *ChattoCore
	projection events.ProjectionHandle[*RoomInvitationProjection]
}

// RoomInvitations returns the room invitation command and query model.
func (c *ChattoCore) RoomInvitations() *RoomInvitationModel { return c.roomInvitations }

// Get returns an invitation only to its sender or recipient.
func (m *RoomInvitationModel) Get(ctx context.Context, actorID, id string) (RoomInvitationState, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return RoomInvitationState{}, err
	}
	if err := m.projection.Projector().WaitForCurrent(ctx); err != nil {
		return RoomInvitationState{}, err
	}
	v, ok := m.projection.Projection().get(id)
	if !ok || (v.SenderID != actorID && v.RecipientID != actorID) {
		return RoomInvitationState{}, ErrNotFound
	}
	if _, err := m.core.userModel.GetUser(ctx, v.RecipientID); err != nil {
		return RoomInvitationState{}, err
	}
	return v, nil
}

// List returns the actor's sent or received invitations.
func (m *RoomInvitationModel) List(ctx context.Context, actorID string, sent, pendingOnly bool) ([]RoomInvitationState, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	if err := m.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	result := []RoomInvitationState{}
	for _, v := range m.projection.Projection().all() {
		matches := v.RecipientID == actorID
		if sent {
			matches = v.SenderID == actorID
		}
		if matches && (!pendingOnly || v.pending(time.Now())) {
			if _, err := m.core.userModel.GetUser(ctx, v.RecipientID); err == nil {
				result = append(result, v)
			} else if !errors.Is(err, ErrNotFound) {
				return nil, err
			}
		}
	}
	return result, nil
}

// eligible checks present authority without granting message access or overriding suspensions.
func (m *RoomInvitationModel) eligible(ctx context.Context, sender, recipient, roomID string) error {
	c := m.core
	room, err := c.roomModel.GetRoom(ctx, KindChannel, roomID)
	if err != nil {
		return err
	}
	if room.GetArchived() {
		return ErrRoomArchived
	}
	if room.GetUniversal() {
		return invalidArgument("universal rooms cannot receive invitations")
	}
	if sender == recipient {
		return invalidArgument("cannot invite yourself")
	}
	for _, id := range []string{sender, recipient} {
		user, err := c.userModel.GetUser(ctx, id)
		if err != nil {
			return err
		}
		if user.GetIsBot() {
			return invalidArgument("room invitations require human accounts")
		}
		if user.GetDeactivated() {
			return invalidArgument("deactivated accounts cannot receive invitations")
		}
	}
	member, err := c.RoomMembershipExists(ctx, KindChannel, sender, roomID)
	if err != nil {
		return err
	}
	if !member {
		return ErrPermissionDenied
	}
	// An invitation outlives the sending session. Temporary owner elevation
	// must not create an invitation that its recipient cannot accept.
	permissionContext := withPrivilegedModeEvaluation(ctx, sender, false)
	allowed, err := c.permissionResolver.HasRoomPermission(permissionContext, sender, KindChannel, roomID, PermRoomInvite)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPermissionDenied
	}
	if c.roomModel.isRoomBanActive(roomID, recipient, time.Now()) {
		return ErrPermissionDenied
	}
	return nil
}

// CanAccept reports whether the recipient can accept with present authority.
func (m *RoomInvitationModel) CanAccept(ctx context.Context, actorID string, v RoomInvitationState) (bool, error) {
	if actorID != v.RecipientID || !v.pending(time.Now()) {
		return false, nil
	}
	err := m.core.authorization.authorizeAtStableInputs(ctx, func() error { return m.eligible(ctx, v.SenderID, v.RecipientID, v.RoomID) })
	if errors.Is(err, ErrPermissionDenied) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrRoomArchived) || errors.Is(err, ErrInvalidArgument) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
func (m *RoomInvitationModel) catchUp(ctx context.Context, roomID string) (uint64, error) {
	filter := evtstream.RoomAggregate(roomID).AllEventsFilter()
	seq, err := m.core.EventPublisher.LastSubjectSeq(ctx, filter)
	if err != nil {
		return 0, err
	}
	pos := events.SubjectPosition(filter, seq)
	if err = m.projection.Projector().WaitFor(ctx, pos); err != nil {
		return 0, err
	}
	if err = m.core.roomModel.waitForDirectory(ctx, pos); err != nil {
		return 0, err
	}
	return seq, nil
}

// Create keeps duplicate sends idempotent without extending the existing deadline.
func (m *RoomInvitationModel) Create(ctx context.Context, actorID, roomID, recipientID string) (RoomInvitationState, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return RoomInvitationState{}, err
	}
	for range maxJoinRoomRetries {
		seq, err := m.catchUp(ctx, roomID)
		if err != nil {
			return RoomInvitationState{}, err
		}
		err = m.core.authorization.authorizeAtStableInputs(ctx, func() error { return m.eligible(ctx, actorID, recipientID, roomID) })
		if err != nil {
			return RoomInvitationState{}, err
		}
		if m.core.roomModel.hasExplicitRoomMembership(roomID, recipientID) {
			return RoomInvitationState{}, invalidArgument("account is already a room member")
		}
		if v, ok := m.projection.Projection().pending(roomID, recipientID, time.Now()); ok {
			if v.SenderID != actorID {
				return RoomInvitationState{}, invalidArgument("account already has a pending invitation")
			}
			return v, nil
		}
		event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RoomInvitationCreated{RoomInvitationCreated: &evtv1.RoomInvitationCreatedEvent{RoomId: roomID, RecipientId: recipientID, ExpiresAt: timestamppb.New(time.Now().Add(7 * 24 * time.Hour))}}})
		event.GetRoomInvitationCreated().InvitationId = event.Id
		if err = m.append(ctx, roomID, seq, event); err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
				continue
			}
			return RoomInvitationState{}, err
		}
		return m.Get(ctx, actorID, event.Id)
	}
	return RoomInvitationState{}, fmt.Errorf("room invitation retry exhausted: %w", events.ErrConflict)
}

// Respond commits the terminal fact and any membership change in a single atomic batch.
// Retrying an accepted invitation never recreates membership after a later leave.
func (m *RoomInvitationModel) Respond(ctx context.Context, actorID, id string, accept bool) (RoomInvitationState, error) {
	initial, err := m.Get(ctx, actorID, id)
	if err != nil {
		return RoomInvitationState{}, err
	}
	if initial.RecipientID != actorID {
		return RoomInvitationState{}, ErrPermissionDenied
	}
	for range maxJoinRoomRetries {
		seq, err := m.catchUp(ctx, initial.RoomID)
		if err != nil {
			return RoomInvitationState{}, err
		}
		v, ok := m.projection.Projection().get(id)
		if !ok {
			return RoomInvitationState{}, ErrNotFound
		}
		desired := "declined"
		if accept {
			desired = "accepted"
		}
		if v.Status == desired {
			return v, nil
		}
		if !v.pending(time.Now()) {
			return RoomInvitationState{}, invalidArgument("invitation is no longer pending")
		}
		var event *evtv1.Event
		if accept {
			err = m.core.authorization.authorizeAtStableInputs(ctx, func() error { return m.eligible(ctx, v.SenderID, v.RecipientID, v.RoomID) })
			if err != nil {
				return RoomInvitationState{}, err
			}
			event = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RoomInvitationAccepted{RoomInvitationAccepted: &evtv1.RoomInvitationAcceptedEvent{RoomId: v.RoomID, InvitationId: id, RecipientId: actorID, SenderId: v.SenderID}}})
		} else {
			event = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RoomInvitationDeclined{RoomInvitationDeclined: &evtv1.RoomInvitationDeclinedEvent{RoomId: v.RoomID, InvitationId: id, RecipientId: actorID, SenderId: v.SenderID}}})
		}
		facts := []*evtv1.Event{event}
		joining := accept && !m.core.roomModel.hasExplicitRoomMembership(v.RoomID, actorID)
		if joining {
			facts = append(facts, newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: v.RoomID}}}))
		}
		if err = m.append(ctx, v.RoomID, seq, facts...); err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
				continue
			}
			return RoomInvitationState{}, err
		}
		if joining {
			m.core.roomMemberships.initializeRoomReadMarker(ctx, KindChannel, actorID, v.RoomID)
		}
		return m.Get(ctx, actorID, id)
	}
	return RoomInvitationState{}, fmt.Errorf("room invitation response retry exhausted: %w", events.ErrConflict)
}
func (m *RoomInvitationModel) append(ctx context.Context, roomID string, seq uint64, facts ...*evtv1.Event) error {
	agg := evtstream.RoomAggregate(roomID)
	entries := make([]evtstream.BatchEntry, len(facts))
	for i, fact := range facts {
		entries[i] = evtstream.BatchEntry{Subject: agg.SubjectFor(fact), Event: fact}
	}
	entries[0].Expect = events.ExpectFilterSeq(agg.AllEventsFilter(), seq)
	seqs, err := m.core.EventPublisher.AppendBatch(ctx, entries)
	if err != nil {
		return err
	}
	pos := events.SubjectPosition(entries[len(entries)-1].Subject, seqs[len(seqs)-1])
	if err = m.projection.Projector().WaitFor(ctx, pos); err != nil {
		return err
	}
	return m.core.roomModel.waitForDirectoryAndTimeline(ctx, pos)
}
