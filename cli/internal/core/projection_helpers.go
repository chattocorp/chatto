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

func (s eventIDSet) seenOrMark(event *corev1.Event) bool {
	if s.has(event) {
		return true
	}
	s.mark(event)
	return false
}
