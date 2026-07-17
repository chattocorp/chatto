package core

import (
	"fmt"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func snapshotReplayGuard(guard projectionReplayGuard) *corev1.ProjectionReplayGuardSnapshot {
	snapshot := &corev1.ProjectionReplayGuardSnapshot{
		HighestSequence:   guard.highestSeq,
		CompatibilityMode: guard.compatibilityMode,
		ReplayComplete:    guard.replayComplete,
	}
	if guard.compatibilityMode {
		snapshot.EventIds = sortedMapKeys(guard.eventIDs)
	}
	return snapshot
}

func restoreReplayGuard(snapshot *corev1.ProjectionReplayGuardSnapshot) (projectionReplayGuard, error) {
	guard := newProjectionReplayGuard()
	if snapshot == nil {
		return guard, nil
	}
	guard.highestSeq = snapshot.GetHighestSequence()
	guard.compatibilityMode = snapshot.GetCompatibilityMode()
	guard.replayComplete = snapshot.GetReplayComplete()
	if len(snapshot.GetEventIds()) > 0 && !guard.compatibilityMode {
		return projectionReplayGuard{}, fmt.Errorf("snapshot replay guard has event IDs outside compatibility mode")
	}
	for _, eventID := range snapshot.GetEventIds() {
		if eventID == "" {
			return projectionReplayGuard{}, fmt.Errorf("snapshot replay guard has an empty event ID")
		}
		if _, duplicate := guard.eventIDs[eventID]; duplicate {
			return projectionReplayGuard{}, fmt.Errorf("snapshot replay guard repeats event ID %q", eventID)
		}
		guard.eventIDs[eventID] = struct{}{}
	}
	if guard.replayComplete && !guard.compatibilityMode {
		guard.eventIDs = nil
	}
	return guard, nil
}
