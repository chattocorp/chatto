package core

import (
	"sort"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// CallParticipant represents a user currently in a voice call.
type CallParticipant struct {
	UserID   string
	JoinedAt int64
	Source   corev1.CallParticipantEventSource
}

// CallStateProjection derives the active-call snapshot from durable room
// facts. It deliberately keeps only process-local projection state; LiveKit
// reconciliation appends more facts instead of mutating the projection directly.
type CallStateProjection struct {
	events.MemoryProjection
	rooms   map[string]map[string]CallParticipant
	roomSeq map[string]uint64
}

type CallRoomSnapshot struct {
	Participants []CallParticipant
	Seq          uint64
}

func NewCallStateProjection() *CallStateProjection {
	return &CallStateProjection{
		rooms:   make(map[string]map[string]CallParticipant),
		roomSeq: make(map[string]uint64),
	}
}

func (p *CallStateProjection) Subjects() []string {
	return []string{events.RoomSubjectFilter()}
}

func (p *CallStateProjection) Apply(event *corev1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	roomID := roomIDOfEvent(event)
	if roomID == "" {
		return nil
	}

	p.Lock()
	defer p.Unlock()

	if seq > p.roomSeq[roomID] {
		p.roomSeq[roomID] = seq
	}
	if event.GetActorId() == "" {
		return nil
	}

	switch e := event.GetEvent().(type) {
	case *corev1.Event_VoiceCallParticipantJoined:
		joinedAt := int64(0)
		if ts := event.GetCreatedAt(); ts != nil {
			joinedAt = ts.AsTime().Unix()
		}
		source := e.VoiceCallParticipantJoined.GetSource()
		if source == corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_UNSPECIFIED {
			source = corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER
		}
		if p.rooms[roomID] == nil {
			p.rooms[roomID] = make(map[string]CallParticipant)
		}
		existing, exists := p.rooms[roomID][event.GetActorId()]
		if exists && joinedAt == 0 {
			joinedAt = existing.JoinedAt
		}
		if exists && joinedAt == existing.JoinedAt && callParticipantSourcePriority(existing.Source) > callParticipantSourcePriority(source) {
			source = existing.Source
		}
		p.rooms[roomID][event.GetActorId()] = CallParticipant{
			UserID:   event.GetActorId(),
			JoinedAt: joinedAt,
			Source:   source,
		}
	case *corev1.Event_VoiceCallParticipantLeft:
		if participants := p.rooms[roomID]; participants != nil {
			delete(participants, event.GetActorId())
			if len(participants) == 0 {
				delete(p.rooms, roomID)
			}
		}
	case *corev1.Event_RoomDeleted:
		delete(p.rooms, roomID)
	}
	return nil
}

func callParticipantSourcePriority(source corev1.CallParticipantEventSource) int {
	switch source {
	case corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_LIVEKIT,
		corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_RECONCILIATION:
		return 2
	case corev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER:
		return 1
	default:
		return 0
	}
}

func (p *CallStateProjection) Participants(roomID string) []CallParticipant {
	p.RLock()
	defer p.RUnlock()
	return p.participantsLocked(roomID)
}

func (p *CallStateProjection) RoomSnapshot(roomID string) CallRoomSnapshot {
	p.RLock()
	defer p.RUnlock()
	return CallRoomSnapshot{
		Participants: p.participantsLocked(roomID),
		Seq:          p.roomSeq[roomID],
	}
}

func (p *CallStateProjection) participantsLocked(roomID string) []CallParticipant {
	participants := p.rooms[roomID]
	if len(participants) == 0 {
		return nil
	}
	out := make([]CallParticipant, 0, len(participants))
	for _, participant := range participants {
		out = append(out, participant)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].JoinedAt == out[j].JoinedAt {
			return out[i].UserID < out[j].UserID
		}
		return out[i].JoinedAt < out[j].JoinedAt
	})
	return out
}

func (p *CallStateProjection) ActiveRoomIDs() []string {
	p.RLock()
	defer p.RUnlock()
	if len(p.rooms) == 0 {
		return nil
	}
	out := make([]string, 0, len(p.rooms))
	for roomID, participants := range p.rooms {
		if len(participants) > 0 {
			out = append(out, roomID)
		}
	}
	sort.Strings(out)
	return out
}

func (p *CallStateProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var participants int64
	var bytes int64
	for roomID, users := range p.rooms {
		bytes += projectionMapEntryOverhead + int64(len(roomID))
		for userID := range users {
			participants++
			bytes += projectionMapEntryOverhead + int64(len(userID)) + 16
		}
	}
	return participants, bytes, []ProjectionAdminMetric{
		{Name: "active_rooms", Value: int64(len(p.rooms)), Bytes: 0},
		{Name: "participants", Value: participants, Bytes: bytes},
	}
}
