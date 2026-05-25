package core

import (
	"slices"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomLayoutProjection holds the operator-defined inter-group ordering
// for the sidebar — the `group_ids` slice that used to live in the
// `room_layout` KV doc. The set of groups themselves is owned by the
// group aggregate (and projected by RoomGroupProjection); this
// projection only tracks the order operators have explicitly set.
//
// New groups appear at the end of the sidebar implicitly: the
// reconciliation in ListRoomGroupsOrdered walks this ordering first,
// then appends any groups present in RoomGroupProjection that aren't
// already listed (by NanoID order — roughly creation order). That's
// why this projection doesn't need to observe group create/delete
// events; it only cares about explicit reorders.
type RoomLayoutProjection struct {
	events.MemoryProjection
	groupIDs []string
}

// NewRoomLayoutProjection returns an empty projection. Order is empty
// on a fresh server; the boot-time migration seeds it from any
// pre-existing room_layout KV.
func NewRoomLayoutProjection() *RoomLayoutProjection {
	return &RoomLayoutProjection{}
}

// Subjects implements events.Projection. Singleton aggregate with one
// event type — the per-(agg, event-type) subject is exact.
func (p *RoomLayoutProjection) Subjects() []string {
	return []string{events.LayoutAggregate().Subject(events.EventRoomGroupsReordered)}
}

// Apply implements events.Projection. Recognised events:
// RoomGroupsReordered (full ordering replacement). Other variants
// are silently ignored per the framework's forward-compat rule.
func (p *RoomLayoutProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	if e, ok := event.GetEvent().(*corev1.Event_RoomGroupsReordered); ok {
		p.Lock()
		p.groupIDs = slices.Clone(e.RoomGroupsReordered.GetGroupIds())
		p.Unlock()
	}
	return nil
}

// Order returns the current explicit ordering of group IDs. May
// reference IDs of groups that have since been deleted; the
// reconciler in ListRoomGroupsOrdered drops those before returning
// to callers.
func (p *RoomLayoutProjection) Order() []string {
	p.RLock()
	defer p.RUnlock()
	return slices.Clone(p.groupIDs)
}
