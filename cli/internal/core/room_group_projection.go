package core

import (
	"slices"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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
	entries     []*corev1.SidebarGroupEntry
	links       map[string]*corev1.SidebarLink
}

type RoomGroupMoveSnapshot struct {
	TargetExists  bool
	SourceGroupID string
	Seq           uint64
}

type RoomGroupSnapshot struct {
	Group  *corev1.RoomGroup
	Exists bool
	Seq    uint64
}

type SidebarLinkMoveSnapshot struct {
	TargetExists  bool
	SourceGroupID string
	Link          *corev1.SidebarLink
	Seq           uint64
}

// NewRoomGroupProjection returns an empty projection.
func NewRoomGroupProjection() *RoomGroupProjection {
	return &RoomGroupProjection{
		groups: make(map[string]*roomGroupEntry),
	}
}

// Subjects implements events.Projection. Room groups are a group-derived read
// model, so the projection subscribes to the group aggregate namespace and
// ignores group events it does not handle.
func (p *RoomGroupProjection) Subjects() []string {
	return []string{events.GroupSubjectFilter()}
}

// Apply implements events.Projection. Recognised events:
// RoomGroupCreated, RoomGroupUpdated, RoomGroupDeleted,
// RoomAddedToGroup, RoomRemovedFromGroup, RoomsInGroupReordered,
// SidebarLinkAddedToGroup, SidebarLinkUpdated, SidebarLinkRemovedFromGroup,
// SidebarGroupEntriesReordered.
// Unrecognised variants are silently ignored.
func (p *RoomGroupProjection) Apply(event *corev1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	p.noteSeq(seq)
	switch e := event.GetEvent().(type) {
	case *corev1.Event_RoomGroupCreated:
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

	case *corev1.Event_RoomGroupUpdated:
		u := e.RoomGroupUpdated
		if entry := p.groups[u.GetGroupId()]; entry != nil {
			entry.name = u.GetName()
			entry.description = u.GetDescription()
		}

	case *corev1.Event_RoomGroupDeleted:
		delete(p.groups, e.RoomGroupDeleted.GetGroupId())

	case *corev1.Event_RoomAddedToGroup:
		a := e.RoomAddedToGroup
		if entry := p.groups[a.GetGroupId()]; entry != nil {
			if !slices.Contains(entry.roomIDs, a.GetRoomId()) {
				entry.roomIDs = append(entry.roomIDs, a.GetRoomId())
			}
			entry.addEntry(&corev1.SidebarGroupEntry{
				Kind: corev1.SidebarGroupEntry_ROOM,
				Id:   a.GetRoomId(),
			})
		}

	case *corev1.Event_RoomRemovedFromGroup:
		r := e.RoomRemovedFromGroup
		if entry := p.groups[r.GetGroupId()]; entry != nil {
			entry.roomIDs = slices.DeleteFunc(entry.roomIDs, func(id string) bool {
				return id == r.GetRoomId()
			})
			entry.removeEntry(corev1.SidebarGroupEntry_ROOM, r.GetRoomId())
		}

	case *corev1.Event_RoomsInGroupReordered:
		r := e.RoomsInGroupReordered
		if entry := p.groups[r.GetGroupId()]; entry != nil {
			entry.roomIDs = slices.Clone(r.GetRoomIds())
			entry.reorderRoomEntries(r.GetRoomIds())
		}

	case *corev1.Event_SidebarLinkAddedToGroup:
		a := e.SidebarLinkAddedToGroup
		if entry := p.groups[a.GetGroupId()]; entry != nil {
			entry.ensureMaps()
			entry.links[a.GetLinkId()] = &corev1.SidebarLink{
				Id:    a.GetLinkId(),
				Label: a.GetLabel(),
				Url:   a.GetUrl(),
			}
			entry.addEntry(&corev1.SidebarGroupEntry{
				Kind: corev1.SidebarGroupEntry_SIDEBAR_LINK,
				Id:   a.GetLinkId(),
			})
		}

	case *corev1.Event_SidebarLinkUpdated:
		u := e.SidebarLinkUpdated
		if entry := p.groups[u.GetGroupId()]; entry != nil {
			entry.ensureMaps()
			if _, ok := entry.links[u.GetLinkId()]; ok {
				entry.links[u.GetLinkId()] = &corev1.SidebarLink{
					Id:    u.GetLinkId(),
					Label: u.GetLabel(),
					Url:   u.GetUrl(),
				}
			}
		}

	case *corev1.Event_SidebarLinkRemovedFromGroup:
		r := e.SidebarLinkRemovedFromGroup
		if entry := p.groups[r.GetGroupId()]; entry != nil {
			entry.ensureMaps()
			delete(entry.links, r.GetLinkId())
			entry.removeEntry(corev1.SidebarGroupEntry_SIDEBAR_LINK, r.GetLinkId())
		}

	case *corev1.Event_SidebarGroupEntriesReordered:
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
// been projected. The returned proto is a fresh value — including a
// cloned room_ids slice — so callers may mutate freely.
func (p *RoomGroupProjection) Get(groupID string) (*corev1.RoomGroup, bool) {
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

// Exists reports whether the group is in the projection.
func (p *RoomGroupProjection) Exists(groupID string) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.groups[groupID]
	return ok
}

// All returns every group in the projection. Order is unspecified;
// the layout aggregate (KV-backed for now) provides the operator-
// preferred sort. Returned protos are fresh values.
func (p *RoomGroupProjection) All() []*corev1.RoomGroup {
	p.RLock()
	defer p.RUnlock()
	out := make([]*corev1.RoomGroup, 0, len(p.groups))
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
			snapshot.Link = cloneSidebarLink(link)
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

// entryToGroup builds a public *corev1.RoomGroup from the private
// entry, including fresh slices for all sidebar layout state.
func entryToGroup(id string, entry *roomGroupEntry) *corev1.RoomGroup {
	return &corev1.RoomGroup{
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
		e.links = make(map[string]*corev1.SidebarLink)
	}
}

func (e *roomGroupEntry) addEntry(entry *corev1.SidebarGroupEntry) {
	if entry == nil || entry.GetId() == "" {
		return
	}
	e.removeEntry(entry.GetKind(), entry.GetId())
	e.entries = append(e.entries, cloneSidebarEntry(entry))
}

func (e *roomGroupEntry) removeEntry(kind corev1.SidebarGroupEntry_Kind, id string) {
	e.entries = slices.DeleteFunc(e.entries, func(entry *corev1.SidebarGroupEntry) bool {
		return entry.GetKind() == kind && entry.GetId() == id
	})
}

func (e *roomGroupEntry) reorderRoomEntries(roomIDs []string) {
	nextRooms := make([]*corev1.SidebarGroupEntry, 0, len(roomIDs))
	for _, id := range roomIDs {
		nextRooms = append(nextRooms, &corev1.SidebarGroupEntry{
			Kind: corev1.SidebarGroupEntry_ROOM,
			Id:   id,
		})
	}
	if len(e.entries) == 0 {
		e.entries = nextRooms
		return
	}

	next := make([]*corev1.SidebarGroupEntry, 0, len(e.entries)+len(nextRooms))
	roomIndex := 0
	for _, current := range e.entries {
		if current.GetKind() == corev1.SidebarGroupEntry_ROOM {
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
	return slices.ContainsFunc(e.entries, func(entry *corev1.SidebarGroupEntry) bool {
		return entry.GetKind() == corev1.SidebarGroupEntry_SIDEBAR_LINK && entry.GetId() == linkID
	})
}

func (e *roomGroupEntry) link(linkID string) *corev1.SidebarLink {
	if e.links == nil {
		return nil
	}
	return e.links[linkID]
}

func (e *roomGroupEntry) clonedEntries() []*corev1.SidebarGroupEntry {
	if len(e.entries) > 0 {
		return cloneSidebarEntries(e.entries)
	}
	entries := make([]*corev1.SidebarGroupEntry, 0, len(e.roomIDs))
	for _, roomID := range e.roomIDs {
		entries = append(entries, &corev1.SidebarGroupEntry{
			Kind: corev1.SidebarGroupEntry_ROOM,
			Id:   roomID,
		})
	}
	return entries
}

func (e *roomGroupEntry) clonedLinks() []*corev1.SidebarLink {
	if len(e.links) == 0 {
		return nil
	}
	links := make([]*corev1.SidebarLink, 0, len(e.links))
	used := make(map[string]struct{}, len(e.links))
	for _, entry := range e.entries {
		if entry.GetKind() != corev1.SidebarGroupEntry_SIDEBAR_LINK {
			continue
		}
		link := e.links[entry.GetId()]
		if link == nil {
			continue
		}
		links = append(links, cloneSidebarLink(link))
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
		links = append(links, cloneSidebarLink(e.links[id]))
	}
	return links
}

func cloneSidebarEntries(entries []*corev1.SidebarGroupEntry) []*corev1.SidebarGroupEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]*corev1.SidebarGroupEntry, 0, len(entries))
	for _, entry := range entries {
		if cloned := cloneSidebarEntry(entry); cloned != nil {
			out = append(out, cloned)
		}
	}
	return out
}

func cloneSidebarEntry(entry *corev1.SidebarGroupEntry) *corev1.SidebarGroupEntry {
	if entry == nil {
		return nil
	}
	return &corev1.SidebarGroupEntry{
		Kind: entry.GetKind(),
		Id:   entry.GetId(),
	}
}

func cloneSidebarLink(link *corev1.SidebarLink) *corev1.SidebarLink {
	if link == nil {
		return nil
	}
	return &corev1.SidebarLink{
		Id:    link.GetId(),
		Label: link.GetLabel(),
		Url:   link.GetUrl(),
	}
}

func roomIDsFromEntries(entries []*corev1.SidebarGroupEntry) []string {
	roomIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.GetKind() == corev1.SidebarGroupEntry_ROOM {
			roomIDs = append(roomIDs, entry.GetId())
		}
	}
	return roomIDs
}
