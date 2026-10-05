package core

import (
	"time"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// Compact projection records store times as Unix nanoseconds in an int64, so
// the records contain no Go pointers. Zero means "no time". Chatto event
// times fall well inside the int64 nanosecond range; a time at the Unix epoch
// itself reads back as "no time".

// projectionUnixNanos converts a time to its compact form.
func projectionUnixNanos(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.UnixNano()
}

// projectionTime converts a compact time back to UTC. Zero returns the zero
// time.
func projectionTime(nanos int64) time.Time {
	if nanos == 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos).UTC()
}

// eventCreatedAt returns the event's creation time, or the zero time when the
// event has none.
func eventCreatedAt(event *evtv1.Event) time.Time {
	if event == nil || event.GetCreatedAt() == nil {
		return time.Time{}
	}
	return event.GetCreatedAt().AsTime()
}

// eventCreatedNanos returns the event's creation time in compact form, or zero
// when the event has none.
func eventCreatedNanos(event *evtv1.Event) int64 {
	return projectionUnixNanos(eventCreatedAt(event))
}
