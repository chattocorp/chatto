package core

import corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"

type eventIDSet map[string]struct{}

func newEventIDSet() eventIDSet {
	return make(eventIDSet)
}

func (s eventIDSet) has(event *corev1.Event) bool {
	eventID := event.GetId()
	if eventID == "" {
		return false
	}
	_, exists := s[eventID]
	return exists
}

func (s eventIDSet) mark(event *corev1.Event) {
	eventID := event.GetId()
	if eventID == "" {
		return
	}
	s[eventID] = struct{}{}
}

// projectionReplayGuard preserves event-ID idempotency while historical events
// are replayed, then releases that history when stream-sequence ordering is
// sufficient. If replay discovers duplicate event IDs, the guard retains the
// set and its existing first-event-wins behavior for compatibility with that
// history.
//
// Callers must synchronize access with their projection lock.
type projectionReplayGuard struct {
	eventIDs          eventIDSet
	highestSeq        uint64
	replayComplete    bool
	compatibilityMode bool
}

func newProjectionReplayGuard() projectionReplayGuard {
	return projectionReplayGuard{eventIDs: newEventIDSet()}
}

func (g *projectionReplayGuard) seen(event *corev1.Event, seq uint64) bool {
	if g.replayComplete && !g.compatibilityMode {
		return seq <= g.highestSeq
	}
	if g.eventIDs.has(event) {
		g.compatibilityMode = true
		return true
	}
	return false
}

func (g *projectionReplayGuard) mark(event *corev1.Event, seq uint64) {
	if seq > g.highestSeq {
		g.highestSeq = seq
	}
	if !g.replayComplete || g.compatibilityMode {
		g.eventIDs.mark(event)
	}
}

func (g *projectionReplayGuard) seenOrMark(event *corev1.Event, seq uint64) bool {
	if g.seen(event, seq) {
		return true
	}
	g.mark(event, seq)
	return false
}

func (g *projectionReplayGuard) completeReplay() {
	g.replayComplete = true
	if !g.compatibilityMode {
		g.eventIDs = nil
	}
}

func (g *projectionReplayGuard) retainedEventIDs() eventIDSet {
	return g.eventIDs
}

func (g *projectionReplayGuard) compatibilityValue() int64 {
	if g.compatibilityMode {
		return 1
	}
	return 0
}
