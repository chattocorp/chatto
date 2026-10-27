package core

import (
	"slices"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// RoomGroupProjection holds the current set of room groups derived
// from evt.group.{G} events: id, name, description, and the ordered
// list of room IDs in the group. The group aggregate owns both
// "what rooms are in me" and "in what order" — see ADR-034 / the
// design discussion accompanying the room-metadata + room-group
// migration PR.
//
// Move-room operations land as two events (one per affected group),
// matching the per-aggregate cascade rule from ADR-034 Approach A.
type RoomGroupProjection struct {
	events.MemoryProjection
	groups map[string]*roomGroupEntry
	seq    uint64
}

type roomGroupEntry struct {
	name        string
	description string
	roomIDs     []string
	entries     []*evtv1.SidebarGroupEntry
	links       map[string]*readmodel.SidebarLink
}

type RoomGroupMoveSnapshot struct {
	TargetExists  bool
	SourceGroupID string
	Seq           uint64
}

type RoomGroupSnapshot struct {
	Group  *readmodel.RoomGroup
	Exists bool
	Seq    uint64
}

type SidebarLinkMoveSnapshot struct {
	TargetExists  bool
	SourceGroupID string
	Link          *readmodel.SidebarLink
	Seq           uint64
}

// SidebarItemPlacementSnapshot captures the source and destination groups for
// one relative sidebar-item move from a single projection generation.
type SidebarItemPlacementSnapshot struct {
	SourceGroupID string
	SourceGroup   *readmodel.RoomGroup
	TargetGroup   *readmodel.RoomGroup
	Link          *readmodel.SidebarLink
	Seq           uint64
}

// NewRoomGroupProjection returns an empty projection.
func NewRoomGroupProjection() *RoomGroupProjection {
	return &RoomGroupProjection{
		groups: make(map[string]*roomGroupEntry),
	}
}

// Subjects implements evtstream.Projection. Room groups are a group-derived read
// model, so the projection subscribes to the group aggregate namespace and
// ignores group events it does not handle.
func (p *RoomGroupProjection) Subjects() []string {
	return []string{evtstream.GroupSubjectFilter()}
}

// Apply implements evtstream.Projection. Recognised events:
// RoomGroupCreated, RoomGroupUpdated, RoomGroupDeleted,
// RoomAddedToGroup, RoomRemovedFromGroup, RoomsInGroupReordered,
// SidebarLinkAddedToGroup, SidebarLinkUpdated, SidebarLinkRemovedFromGroup,
// SidebarGroupEntriesReordered.
// Unrecognised variants are silently ignored.
func (p *RoomGroupProjection) Apply(event *evtv1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	p.noteSeq(seq)
	switch e := event.GetEvent().(type) {
	case *evtv1.Event_RoomGroupCreated:
		c := e.RoomGroupCreated
		// Idempotent: re-creating an existing group overwrites
		// metadata but preserves room membership. In practice the
		// Append OCC scope prevents re-creation; this is defensive.
		entry := p.groups[c.GetGroupId()]
		if entry == nil {
			entry = &roomGroupEntry{}
			p.groups[c.GetGroupId()] = entry
		}
		entry.ensureMaps()
		entry.name = c.GetName()
		entry.description = c.GetDescription()

	case *evtv1.Event_RoomGroupUpdated:
		u := e.RoomGroupUpdated
		if entry := p.groups[u.GetGroupId()]; entry != nil {
			entry.name = u.GetName()
			entry.description = u.GetDescription()
		}

	case *evtv1.Event_RoomGroupDeleted:
		delete(p.groups, e.RoomGroupDeleted.GetGroupId())

	case *evtv1.Event_RoomAddedToGroup:
		a := e.RoomAddedToGroup
		if entry := p.groups[a.GetGroupId()]; entry != nil {
			if !slices.Contains(entry.roomIDs, a.GetRoomId()) {
				entry.roomIDs = append(entry.roomIDs, a.GetRoomId())
			}
			entry.addEntry(&evtv1.SidebarGroupEntry{
				Kind: evtv1.SidebarGroupEntry_ROOM,
				Id:   a.GetRoomId(),
			})
		}

	case *evtv1.Event_RoomRemovedFromGroup:
		r := e.RoomRemovedFromGroup
		if entry := p.groups[r.GetGroupId()]; entry != nil {
			entry.roomIDs = slices.DeleteFunc(entry.roomIDs, func(id string) bool {
				return id == r.GetRoomId()
			})
			entry.removeEntry(evtv1.SidebarGroupEntry_ROOM, r.GetRoomId())
		}

	case *evtv1.Event_RoomsInGroupReordered:
		r := e.RoomsInGroupReordered
		if entry := p.groups[r.GetGroupId()]; entry != nil {
			entry.roomIDs = slices.Clone(r.GetRoomIds())
			entry.reorderRoomEntries(r.GetRoomIds())
		}

	case *evtv1.Event_SidebarLinkAddedToGroup:
		a := e.SidebarLinkAddedToGroup
		if entry := p.groups[a.GetGroupId()]; entry != nil {
			entry.ensureMaps()
			entry.links[a.GetLinkId()] = &readmodel.SidebarLink{
				Id:    a.GetLinkId(),
				Label: a.GetLabel(),
				Url:   a.GetUrl(),
			}
			entry.addEntry(&evtv1.SidebarGroupEntry{
				Kind: evtv1.SidebarGroupEntry_SIDEBAR_LINK,
				Id:   a.GetLinkId(),
			})
		}

	case *evtv1.Event_SidebarLinkUpdated:
		u := e.SidebarLinkUpdated
		if entry := p.groups[u.GetGroupId()]; entry != nil {
			entry.ensureMaps()
			if _, ok := entry.links[u.GetLinkId()]; ok {
				entry.links[u.GetLinkId()] = &readmodel.SidebarLink{
					Id:    u.GetLinkId(),
					Label: u.GetLabel(),
					Url:   u.GetUrl(),
				}
			}
		}

	case *evtv1.Event_SidebarLinkRemovedFromGroup:
		r := e.SidebarLinkRemovedFromGroup
		if entry := p.groups[r.GetGroupId()]; entry != nil {
			entry.ensureMaps()
			delete(entry.links, r.GetLinkId())
			entry.removeEntry(evtv1.SidebarGroupEntry_SIDEBAR_LINK, r.GetLinkId())
		}

	case *evtv1.Event_SidebarGroupEntriesReordered:
		r := e.SidebarGroupEntriesReordered
		if entry := p.groups[r.GetGroupId()]; entry != nil {
			entry.entries = cloneSidebarEntries(r.GetEntries())
			entry.roomIDs = roomIDsFromEntries(entry.entries)
		}
	}
	return nil
}

func (p *RoomGroupProjection) noteSeq(seq uint64) {
	if seq > p.seq {
		p.seq = seq
	}
}

// Get returns the group's data, or (nil, false) if no such group has
// been projected. The returned model and its nested values are detached,
// so callers can change them without changing the projection.
func (p *RoomGroupProjection) Get(groupID string) (*readmodel.RoomGroup, bool) {
	p.RLock()
	defer p.RUnlock()
	entry, ok := p.groups[groupID]
	if !ok {
		return nil, false
	}
	return entryToGroup(groupID, entry), true
}

func (p *RoomGroupProjection) Snapshot(groupID string) RoomGroupSnapshot {
	p.RLock()
	defer p.RUnlock()
	snapshot := RoomGroupSnapshot{Seq: p.seq}
	if entry, ok := p.groups[groupID]; ok {
		snapshot.Exists = true
		snapshot.Group = entryToGroup(groupID, entry)
	}
	return snapshot
}

// PlacementSnapshot returns a consistent source/destination view for a room or
// sidebar-link placement command. The returned read models are detached
// from projection state.
func (p *RoomGroupProjection) PlacementSnapshot(item *evtv1.SidebarGroupEntry, targetGroupID string) SidebarItemPlacementSnapshot {
	p.RLock()
	defer p.RUnlock()

	snapshot := SidebarItemPlacementSnapshot{Seq: p.seq}
	if target, ok := p.groups[targetGroupID]; ok {
		snapshot.TargetGroup = entryToGroup(targetGroupID, target)
	}
	if item == nil || item.GetId() == "" {
		return snapshot
	}
	for groupID, entry := range p.groups {
		for _, candidate := range entry.entries {
			if candidate.GetKind() != item.GetKind() || candidate.GetId() != item.GetId() {
				continue
			}
			snapshot.SourceGroupID = groupID
			snapshot.SourceGroup = entryToGroup(groupID, entry)
			if item.GetKind() == evtv1.SidebarGroupEntry_SIDEBAR_LINK {
				snapshot.Link = entry.links[item.GetId()].Clone()
			}
			return snapshot
		}
	}
	return snapshot
}

// Exists reports whether the group is in the projection.
func (p *RoomGroupProjection) Exists(groupID string) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.groups[groupID]
	return ok
}

// All returns every group in the projection. Order is unspecified;
// RoomLayoutProjection provides the configured order. Returned models
// and their nested values are detached.
func (p *RoomGroupProjection) All() []*readmodel.RoomGroup {
	p.RLock()
	defer p.RUnlock()
	out := make([]*readmodel.RoomGroup, 0, len(p.groups))
	for id, entry := range p.groups {
		out = append(out, entryToGroup(id, entry))
	}
	return out
}

// GroupForRoom returns the group ID that currently contains the
// given room, or "" if the room isn't in any group. Linear scan;
// fine for the small group counts we expect on a server.
func (p *RoomGroupProjection) GroupForRoom(roomID string) string {
	return p.MoveSnapshot(roomID, "").SourceGroupID
}

func (p *RoomGroupProjection) GroupForSidebarLink(linkID string) string {
	p.RLock()
	defer p.RUnlock()
	for groupID, entry := range p.groups {
		if entry.hasLink(linkID) {
			return groupID
		}
	}
	return ""
}

func (p *RoomGroupProjection) SidebarLinkMoveSnapshot(linkID, targetGroupID string) SidebarLinkMoveSnapshot {
	p.RLock()
	defer p.RUnlock()
	snapshot := SidebarLinkMoveSnapshot{
		TargetExists: targetGroupID == "",
		Seq:          p.seq,
	}
	if targetGroupID != "" {
		_, snapshot.TargetExists = p.groups[targetGroupID]
	}
	for groupID, entry := range p.groups {
		if link := entry.link(linkID); link != nil {
			snapshot.SourceGroupID = groupID
			snapshot.Link = link.Clone()
			return snapshot
		}
	}
	return snapshot
}

func (p *RoomGroupProjection) MoveSnapshot(roomID, targetGroupID string) RoomGroupMoveSnapshot {
	p.RLock()
	defer p.RUnlock()
	snapshot := RoomGroupMoveSnapshot{
		TargetExists: targetGroupID == "",
		Seq:          p.seq,
	}
	if targetGroupID != "" {
		_, snapshot.TargetExists = p.groups[targetGroupID]
	}
	for groupID, entry := range p.groups {
		if slices.Contains(entry.roomIDs, roomID) {
			snapshot.SourceGroupID = groupID
			return snapshot
		}
	}
	return snapshot
}

// Count returns the number of groups projected. Useful for
// admin/diagnostic surfaces.
func (p *RoomGroupProjection) Count() int {
	p.RLock()
	defer p.RUnlock()
	return len(p.groups)
}

// entryToGroup builds a detached read model, including fresh slices and
// nested values for all sidebar layout state.
func entryToGroup(id string, entry *roomGroupEntry) *readmodel.RoomGroup {
	return &readmodel.RoomGroup{
		Id:           id,
		Name:         entry.name,
		Description:  entry.description,
		RoomIds:      slices.Clone(entry.roomIDs),
		Entries:      entry.clonedEntries(),
		SidebarLinks: entry.clonedLinks(),
	}
}

func (e *roomGroupEntry) ensureMaps() {
	if e.links == nil {
		e.links = make(map[string]*readmodel.SidebarLink)
	}
}

func (e *roomGroupEntry) addEntry(entry *evtv1.SidebarGroupEntry) {
	if entry == nil || entry.GetId() == "" {
		return
	}
	e.removeEntry(entry.GetKind(), entry.GetId())
	e.entries = append(e.entries, cloneSidebarEntry(entry))
}

func (e *roomGroupEntry) removeEntry(kind evtv1.SidebarGroupEntry_Kind, id string) {
	e.entries = slices.DeleteFunc(e.entries, func(entry *evtv1.SidebarGroupEntry) bool {
		return entry.GetKind() == kind && entry.GetId() == id
	})
}

func (e *roomGroupEntry) reorderRoomEntries(roomIDs []string) {
	nextRooms := make([]*evtv1.SidebarGroupEntry, 0, len(roomIDs))
	for _, id := range roomIDs {
		nextRooms = append(nextRooms, &evtv1.SidebarGroupEntry{
			Kind: evtv1.SidebarGroupEntry_ROOM,
			Id:   id,
		})
	}
	if len(e.entries) == 0 {
		e.entries = nextRooms
		return
	}

	next := make([]*evtv1.SidebarGroupEntry, 0, len(e.entries)+len(nextRooms))
	roomIndex := 0
	for _, current := range e.entries {
		if current.GetKind() == evtv1.SidebarGroupEntry_ROOM {
			if roomIndex < len(nextRooms) {
				next = append(next, cloneSidebarEntry(nextRooms[roomIndex]))
				roomIndex++
			}
			continue
		}
		next = append(next, cloneSidebarEntry(current))
	}
	for roomIndex < len(nextRooms) {
		next = append(next, cloneSidebarEntry(nextRooms[roomIndex]))
		roomIndex++
	}
	e.entries = next
}

func (e *roomGroupEntry) hasLink(linkID string) bool {
	if e.links != nil {
		if _, ok := e.links[linkID]; ok {
			return true
		}
	}
	return slices.ContainsFunc(e.entries, func(entry *evtv1.SidebarGroupEntry) bool {
		return entry.GetKind() == evtv1.SidebarGroupEntry_SIDEBAR_LINK && entry.GetId() == linkID
	})
}

func (e *roomGroupEntry) link(linkID string) *readmodel.SidebarLink {
	if e.links == nil {
		return nil
	}
	return e.links[linkID]
}

func (e *roomGroupEntry) clonedEntries() []*evtv1.SidebarGroupEntry {
	if len(e.entries) > 0 {
		return cloneSidebarEntries(e.entries)
	}
	entries := make([]*evtv1.SidebarGroupEntry, 0, len(e.roomIDs))
	for _, roomID := range e.roomIDs {
		entries = append(entries, &evtv1.SidebarGroupEntry{
			Kind: evtv1.SidebarGroupEntry_ROOM,
			Id:   roomID,
		})
	}
	return entries
}

func (e *roomGroupEntry) clonedLinks() []*readmodel.SidebarLink {
	if len(e.links) == 0 {
		return nil
	}
	links := make([]*readmodel.SidebarLink, 0, len(e.links))
	used := make(map[string]struct{}, len(e.links))
	for _, entry := range e.entries {
		if entry.GetKind() != evtv1.SidebarGroupEntry_SIDEBAR_LINK {
			continue
		}
		link := e.links[entry.GetId()]
		if link == nil {
			continue
		}
		links = append(links, link.Clone())
		used[link.GetId()] = struct{}{}
	}
	var orphans []string
	for id := range e.links {
		if _, ok := used[id]; !ok {
			orphans = append(orphans, id)
		}
	}
	slices.Sort(orphans)
	for _, id := range orphans {
		links = append(links, e.links[id].Clone())
	}
	return links
}

func cloneSidebarEntries(entries []*evtv1.SidebarGroupEntry) []*evtv1.SidebarGroupEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]*evtv1.SidebarGroupEntry, 0, len(entries))
	for _, entry := range entries {
		if cloned := cloneSidebarEntry(entry); cloned != nil {
			out = append(out, cloned)
		}
	}
	return out
}

func cloneSidebarEntry(entry *evtv1.SidebarGroupEntry) *evtv1.SidebarGroupEntry {
	if entry == nil {
		return nil
	}
	return &evtv1.SidebarGroupEntry{
		Kind: entry.GetKind(),
		Id:   entry.GetId(),
	}
}

func roomIDsFromEntries(entries []*evtv1.SidebarGroupEntry) []string {
	roomIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.GetKind() == evtv1.SidebarGroupEntry_ROOM {
			roomIDs = append(roomIDs, entry.GetId())
		}
	}
	return roomIDs
}
