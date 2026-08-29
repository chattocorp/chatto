package core

import (
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/projection/v1"
	"slices"

	"google.golang.org/protobuf/proto"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

var roomGroupLayoutSnapshotContractID = snapshotContractID("v1", &projectionv1.RoomGroupLayoutProjectionSnapshot{})

func (*RoomGroupLayoutProjection) SnapshotContractID() string {
	return roomGroupLayoutSnapshotContractID
}

func (p *RoomGroupLayoutProjection) Snapshot() ([]byte, error) {
	p.Groups.RLock()
	p.Layout.RLock()
	defer p.Groups.RUnlock()
	defer p.Layout.RUnlock()
	snapshot := &projectionv1.RoomGroupLayoutProjectionSnapshot{GroupIds: slices.Clone(p.Layout.groupIDs), Sequence: p.Groups.seq}
	for _, groupID := range sortedMapKeys(p.Groups.groups) {
		entry := p.Groups.groups[groupID]
		snapshot.Groups = append(snapshot.Groups, &projectionv1.RoomGroupStateSnapshot{
			Group: entryToGroup(groupID, entry),
		})
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(snapshot)
}

func (p *RoomGroupLayoutProjection) Restore(data []byte) error {
	snapshot := &projectionv1.RoomGroupLayoutProjectionSnapshot{}
	if len(data) > 0 {
		if err := proto.Unmarshal(data, snapshot); err != nil {
			return fmt.Errorf("unmarshal room group layout snapshot: %w", err)
		}
	}
	groups := make(map[string]*roomGroupEntry, len(snapshot.GetGroups()))
	for _, state := range snapshot.GetGroups() {
		group := state.GetGroup()
		if group.GetId() == "" {
			return fmt.Errorf("room group layout snapshot has empty group ID")
		}
		if _, duplicate := groups[group.GetId()]; duplicate {
			return fmt.Errorf("room group layout snapshot repeats group %q", group.GetId())
		}
		entry := &roomGroupEntry{name: group.GetName(), description: group.GetDescription(), roomIDs: slices.Clone(group.GetRoomIds()), entries: cloneSidebarEntries(group.GetEntries()), links: make(map[string]*evtv1.SidebarLink)}
		links := group.GetSidebarLinks()
		for _, link := range links {
			if link.GetId() == "" {
				return fmt.Errorf("room group layout snapshot has empty link ID in group %q", group.GetId())
			}
			if _, duplicate := entry.links[link.GetId()]; duplicate {
				return fmt.Errorf("room group layout snapshot repeats link %q", link.GetId())
			}
			entry.links[link.GetId()] = cloneSidebarLink(link)
		}
		groups[group.GetId()] = entry
	}
	seenLayout := make(map[string]struct{}, len(snapshot.GetGroupIds()))
	for _, groupID := range snapshot.GetGroupIds() {
		if groupID == "" {
			return fmt.Errorf("room group layout snapshot has empty layout group ID")
		}
		if _, duplicate := seenLayout[groupID]; duplicate {
			return fmt.Errorf("room group layout snapshot repeats layout group %q", groupID)
		}
		seenLayout[groupID] = struct{}{}
	}
	p.Groups.Lock()
	p.Layout.Lock()
	p.Groups.groups, p.Groups.seq = groups, snapshot.GetSequence()
	p.Layout.groupIDs = slices.Clone(snapshot.GetGroupIds())
	p.Layout.Unlock()
	p.Groups.Unlock()
	return nil
}
