package core

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const callStateSnapshotContractID = "v1"

func (*CallStateProjection) SnapshotContractID() string { return callStateSnapshotContractID }

func (p *CallStateProjection) Snapshot() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()
	roomSet := make(map[string]struct{}, len(p.roomSeq)+len(p.rooms)+len(p.activeCalls))
	for roomID := range p.roomSeq {
		roomSet[roomID] = struct{}{}
	}
	for roomID := range p.rooms {
		roomSet[roomID] = struct{}{}
	}
	for roomID := range p.activeCalls {
		roomSet[roomID] = struct{}{}
	}
	snapshot := &corev1.CallStateProjectionSnapshot{}
	for _, roomID := range sortedMapKeys(roomSet) {
		row := &corev1.CallRoomStateSnapshot{RoomId: roomID, Sequence: p.roomSeq[roomID]}
		if call, ok := p.activeCalls[roomID]; ok {
			row.Call = &corev1.CallSessionSnapshot{CallId: call.CallID, E2EeKeyRef: call.E2EEKeyRef, StartedAt: call.StartedAt, Source: call.Source}
		}
		for _, userID := range sortedMapKeys(p.rooms[roomID]) {
			participant := p.rooms[roomID][userID]
			row.Participants = append(row.Participants, &corev1.CallParticipantSnapshot{UserId: participant.UserID, CallId: participant.CallID, JoinedAt: participant.JoinedAt, Source: participant.Source})
		}
		snapshot.Rooms = append(snapshot.Rooms, row)
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *CallStateProjection) Restore(data []byte) error {
	snapshot := &corev1.CallStateProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal call state snapshot: %w", err)
		}
	}
	rooms := make(map[string]map[string]CallParticipant)
	calls := make(map[string]CallSession)
	sequences := make(map[string]uint64)
	for _, row := range snapshot.GetRooms() {
		roomID := row.GetRoomId()
		if roomID == "" {
			return fmt.Errorf("call state snapshot has empty room ID")
		}
		if _, duplicate := sequences[roomID]; duplicate {
			return fmt.Errorf("call state snapshot repeats room %q", roomID)
		}
		sequences[roomID] = row.GetSequence()
		if call := row.GetCall(); call != nil {
			calls[roomID] = CallSession{CallID: call.GetCallId(), E2EEKeyRef: call.GetE2EeKeyRef(), StartedAt: call.GetStartedAt(), Source: call.GetSource()}
		}
		participants := make(map[string]CallParticipant)
		for _, participant := range row.GetParticipants() {
			if participant.GetUserId() == "" {
				return fmt.Errorf("call state snapshot has empty participant in room %q", roomID)
			}
			if _, duplicate := participants[participant.GetUserId()]; duplicate {
				return fmt.Errorf("call state snapshot repeats participant %q", participant.GetUserId())
			}
			participants[participant.GetUserId()] = CallParticipant{UserID: participant.GetUserId(), CallID: participant.GetCallId(), JoinedAt: participant.GetJoinedAt(), Source: participant.GetSource()}
		}
		if len(participants) > 0 {
			rooms[roomID] = participants
		}
	}
	p.Lock()
	p.rooms, p.activeCalls, p.roomSeq = rooms, calls, sequences
	p.Unlock()
	return nil
}
