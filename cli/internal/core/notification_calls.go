package core

import (
	"context"
	"errors"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"google.golang.org/protobuf/proto"
)

// A separate ordered consumer lets upgraded replicas handle calls without
// changing the filter or pending work of the existing notification consumers.
const callNotificationConsumerName = "chatto-call-notifications-v1"

func callNotificationSubjects() []string {
	return []string{
		evtstream.RoomEventTypeFilter(evtstream.EventCallStarted),
		evtstream.RoomEventTypeFilter(evtstream.EventCallParticipantJoined),
		evtstream.RoomEventTypeFilter(evtstream.EventCallEnded),
	}
}

func incomingCallSignal(roomID, callID string) *notificationv1.NotificationSignal {
	return &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_IncomingCall{
		IncomingCall: &notificationv1.IncomingCall{RoomId: roomID, CallId: callID},
	}}
}

// callAudienceMember checks the user's current membership, its start sequence,
// and call admission. The caller owns the decision projection read lock.
func (s *notificationDecisionSnapshot) callAudienceMember(userID, roomID string, sequence uint64) bool {
	if _, active := s.activeUsers[userID]; !active || !s.membershipExists(userID, roomID) {
		return false
	}
	user, _ := s.badges.ids.Lookup(userID)
	room, _ := s.badges.ids.Lookup(roomID)
	return sequence > s.badgeMembershipStart(user, room, userID, roomID) &&
		s.roomPermissionAllowed(userID, roomID, s.groups.Groups.GroupForRoom(roomID), PermCallJoin)
}

func (m *NotificationMaterializer) processCallDelivery(ctx context.Context, delivery events.DurableDelivery) error {
	var source evtv1.Event
	if err := proto.Unmarshal(delivery.Data, &source); err != nil {
		return events.TerminateDelivery("invalid call notification source", err)
	}
	if err := m.decisions.Projector().WaitForCurrent(ctx); err != nil {
		return err
	}
	if err := m.core.callModel.callState.Projector().WaitForCurrent(ctx); err != nil {
		return err
	}
	roomID := roomIDOfEvent(&source)
	if started := source.GetVoiceCallStarted(); started != nil {
		call, ok := m.core.callModel.activeCall(roomID)
		if !ok || call.CallID != started.GetCallId() || call.Source != evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER {
			return nil
		}
		return m.materializeCall(ctx, &source, roomID, call)
	}
	recipientID := ""
	callID := source.GetVoiceCallEnded().GetCallId()
	if joined := source.GetVoiceCallParticipantJoined(); joined != nil {
		recipientID = source.GetActorId()
		callID = joined.GetCallId()
	}
	if callID == "" {
		return nil
	}
	occurrences := m.core.notificationOccurrences
	if err := occurrences.projection.Projector().WaitForCurrent(ctx); err != nil {
		return err
	}
	rows := occurrences.projection.Projection().callOccurrences(roomID, callID, recipientID, occurrences.now())
	_, err := occurrences.deleteOccurrences(ctx, rows)
	return err
}

// callOccurrences reads only the active generation. An empty recipient selects
// all remaining alerts when the call ends.
func (p *NotificationProjection) callOccurrences(roomID, callID, recipientID string, now time.Time) []*notificationv1.NotificationOccurrence {
	p.RLock()
	defer p.RUnlock()
	var matches []*notificationv1.NotificationOccurrence
	for _, row := range p.rows.byCall[callID] {
		if row.expiresAt <= saturatedUnixNano(now) || (recipientID != "" && p.rows.ids.Resolve(row.recipient) != recipientID) {
			continue
		}
		if row.full.GetSignal().GetIncomingCall().GetRoomId() == roomID {
			matches = append(matches, p.rows.occurrence(row))
		}
	}
	return matches
}

func (m *NotificationMaterializer) materializeCall(ctx context.Context, source *evtv1.Event, roomID string, call CallSession) error {
	var decisions []notificationRecipientDecision
	err := m.decisions.Projection().withCurrent(time.Now(), func(s *notificationDecisionSnapshot) error {
		kind, ok := s.roomKind(roomID)
		if !ok {
			return nil
		}
		signal := incomingCallSignal(roomID, call.CallID)
		for _, id := range s.roomMemberIDs(roomID) {
			mode := s.effectiveNotificationMode(id, roomID, kind, signal)
			if id == source.GetActorId() || !notificationModeProducesOccurrence(mode) || !s.callAudienceMember(id, roomID, call.StartSequence) {
				continue
			}
			if !m.core.callModel.callState.Projection().awaitingJoin(roomID, call.CallID, id) {
				continue
			}
			decisions = append(decisions, notificationRecipientDecision{recipientID: id, signal: signal, mode: mode})
		}
		return nil
	})
	if err != nil {
		return err
	}
	inputs := newNotificationOccurrenceInputs(source, decisions)
	for i := range inputs {
		inputs[i].SourceStreamSequence = call.StartSequence
	}
	return m.core.notificationOccurrences.CreateMany(ctx, inputs)
}

// callVisible rechecks call generation, membership and admission on reads and
// immediately before push. The caller holds the content-view read boundary.
func (m *NotificationOccurrenceModel) callVisible(ctx context.Context, recipientID string, call *notificationv1.IncomingCall) (bool, error) {
	if !m.core.callModel.callState.Projection().awaitingJoin(call.GetRoomId(), call.GetCallId(), recipientID) {
		return false, nil
	}
	_, kind, err := m.core.authorization.requireRoomMemberInView(ctx, recipientID, call.GetRoomId())
	if errors.Is(err, ErrNotRoomMember) || errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return m.core.authorization.resolver.HasRoomPermission(ctx, recipientID, kind, call.GetRoomId(), PermCallJoin)
}

// awaitingJoin checks a single participant without copying the room roster.
func (p *CallStateProjection) awaitingJoin(roomID, callID, recipientID string) bool {
	p.RLock()
	defer p.RUnlock()
	active, exists := p.activeCalls[roomID]
	_, joined := p.rooms[roomID][recipientID]
	return exists && active.CallID == callID && !joined && !p.departed[roomID][recipientID]
}
