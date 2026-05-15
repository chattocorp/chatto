package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// room_groups.go provides core operations on room groups — the named, ordered
// groups of channel rooms that also serve as permission containers
// (see ADR-031). All operations mutate the single `RoomLayout` KV entry
// using the same optimistic-concurrency-control pattern as
// `UpdateRoomLayout`, so admin actions remain atomic.
//
// Authorization is enforced at the API boundary; these methods assume
// the caller is authorized.

// Errors specific to room-set operations.
var (
	// ErrRoomGroupNotFound is returned when a room group ID doesn't match any existing set.
	ErrRoomGroupNotFound = errors.New("room group not found")
	// ErrRoomGroupHasRooms is returned when trying to delete a set that still contains rooms.
	ErrRoomGroupHasRooms = errors.New("room group has rooms; move them out before deleting")
	// ErrRoomGroupNameEmpty is returned when a set name is empty or whitespace.
	ErrRoomGroupNameEmpty = errors.New("room group name must not be empty")
)

// CreateRoomGroup appends a new (empty) room group to the layout and returns it.
// Name is trimmed; description may be empty.
func (c *ChattoCore) CreateRoomGroup(ctx context.Context, actorID, name, description string) (*corev1.RoomGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrRoomGroupNameEmpty
	}

	newSet := &corev1.RoomGroup{
		Id:          NewRoomGroupID(),
		Name:        name,
		Description: description,
	}

	if _, err := c.mutateRoomLayout(ctx, func(layout *corev1.RoomLayout) error {
		layout.Groups = append(layout.Groups, newSet)
		return nil
	}); err != nil {
		return nil, err
	}

	// Seed default channel-room permissions. Without this, nobody (not even
	// the owner) can list / post in rooms placed in this set, because
	// channel-room permissions are resolved at set scope (ADR-031).
	if err := c.SeedDefaultRoomGroupPermissions(ctx, newSet.Id); err != nil {
		c.logger.Warn("Failed to seed default permissions for new set",
			"error", err, "group_id", newSet.Id)
	}

	c.logger.Info("Created room group", "group_id", newSet.Id, "name", name, "actor_id", actorID)
	return newSet, nil
}

// UpdateRoomGroup changes a set's name and/or description. The set's id,
// position in the layout, and member room list are preserved.
func (c *ChattoCore) UpdateRoomGroup(ctx context.Context, actorID, groupID, name, description string) (*corev1.RoomGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrRoomGroupNameEmpty
	}

	var updated *corev1.RoomGroup
	if _, err := c.mutateRoomLayout(ctx, func(layout *corev1.RoomLayout) error {
		idx := findGroupIndex(layout, groupID)
		if idx == -1 {
			return ErrRoomGroupNotFound
		}
		layout.Groups[idx].Name = name
		layout.Groups[idx].Description = description
		updated = layout.Groups[idx]
		return nil
	}); err != nil {
		return nil, err
	}

	c.logger.Info("Updated room group", "group_id", groupID, "name", name, "actor_id", actorID)
	return updated, nil
}

// GetRoomGroup returns the named set, or ErrRoomGroupNotFound if no such set exists.
func (c *ChattoCore) GetRoomGroup(ctx context.Context, groupID string) (*corev1.RoomGroup, error) {
	layout, err := c.GetRoomLayout(ctx, KindChannel)
	if err != nil {
		return nil, err
	}
	if layout == nil {
		return nil, ErrRoomGroupNotFound
	}
	idx := findGroupIndex(layout, groupID)
	if idx == -1 {
		return nil, ErrRoomGroupNotFound
	}
	return layout.Groups[idx], nil
}

// DeleteRoomGroup removes a set from the layout. Fails with ErrRoomGroupHasRooms
// if the set still contains any rooms — the operator must move them out
// first. There is no cascade.
func (c *ChattoCore) DeleteRoomGroup(ctx context.Context, actorID, groupID string) error {
	if _, err := c.mutateRoomLayout(ctx, func(layout *corev1.RoomLayout) error {
		idx := findGroupIndex(layout, groupID)
		if idx == -1 {
			return ErrRoomGroupNotFound
		}
		if len(layout.Groups[idx].RoomIds) > 0 {
			return ErrRoomGroupHasRooms
		}
		layout.Groups = slices.Delete(layout.Groups, idx, idx+1)
		return nil
	}); err != nil {
		return err
	}

	c.logger.Info("Deleted room group", "group_id", groupID, "actor_id", actorID)
	return nil
}

// MoveRoomToGroup moves a room into the target set, removing it from any
// other set it was previously in. The room is appended to the end of
// the target set's room list. If the room was not previously in any
// set (e.g., a freshly created room not yet assigned), it is simply
// added to the target.
//
// Authorization for the *source* and *target* sets must be checked by
// the caller — see ADR-031's two-set rule.
func (c *ChattoCore) MoveRoomToGroup(ctx context.Context, actorID, roomID, targetGroupID string) error {
	if _, err := c.mutateRoomLayout(ctx, func(layout *corev1.RoomLayout) error {
		targetIdx := findGroupIndex(layout, targetGroupID)
		if targetIdx == -1 {
			return ErrRoomGroupNotFound
		}

		// Remove from any other set
		for _, set := range layout.Groups {
			set.RoomIds = slices.DeleteFunc(set.RoomIds, func(id string) bool {
				return id == roomID
			})
		}

		// Append to target (avoiding duplicates — defensive)
		if !slices.Contains(layout.Groups[targetIdx].RoomIds, roomID) {
			layout.Groups[targetIdx].RoomIds = append(layout.Groups[targetIdx].RoomIds, roomID)
		}
		return nil
	}); err != nil {
		return err
	}

	c.logger.Info("Moved room to set", "room_id", roomID, "group_id", targetGroupID, "actor_id", actorID)
	return nil
}

// ReorderRoomGroups reorders the layout's room groups to match the given list of
// IDs. Every existing set must appear exactly once; unknown IDs and missing
// IDs both return ErrRoomGroupNotFound. Room-list contents are preserved.
func (c *ChattoCore) ReorderRoomGroups(ctx context.Context, actorID string, orderedGroupIDs []string) error {
	if _, err := c.mutateRoomLayout(ctx, func(layout *corev1.RoomLayout) error {
		if len(orderedGroupIDs) != len(layout.Groups) {
			return ErrRoomGroupNotFound
		}

		byGroupID := make(map[string]*corev1.RoomGroup, len(layout.Groups))
		for _, s := range layout.Groups {
			byGroupID[s.Id] = s
		}

		reordered := make([]*corev1.RoomGroup, 0, len(orderedGroupIDs))
		for _, id := range orderedGroupIDs {
			set, ok := byGroupID[id]
			if !ok {
				return ErrRoomGroupNotFound
			}
			reordered = append(reordered, set)
		}
		layout.Groups = reordered
		return nil
	}); err != nil {
		return err
	}

	c.logger.Info("Reordered room groups", "order", orderedGroupIDs, "actor_id", actorID)
	return nil
}

// mutateRoomLayout reads the layout, applies the given mutator, and writes
// it back atomically using OCC. If the layout doesn't yet exist, an empty
// one is created and the mutator runs against that. Returns ErrConfigConflict
// if too many retries are exhausted.
//
// The mutator is allowed to return a domain error (e.g. ErrRoomGroupNotFound);
// when it does, the layout is left unchanged and the error is returned
// verbatim. The mutator may be called multiple times under OCC retry, so
// it must be deterministic.
func (c *ChattoCore) mutateRoomLayout(ctx context.Context, mutate func(*corev1.RoomLayout) error) (*corev1.RoomLayout, error) {
	bucket := c.storage.serverConfigKV

	for attempt := 0; attempt < maxLayoutRetries; attempt++ {
		entry, getErr := bucket.Get(ctx, roomLayoutKey)

		var revision uint64
		layout := &corev1.RoomLayout{}
		if getErr != nil {
			if !errors.Is(getErr, jetstream.ErrKeyNotFound) {
				return nil, fmt.Errorf("failed to get room layout: %w", getErr)
			}
			// Layout doesn't exist yet — start with an empty one
			revision = 0
		} else {
			if err := proto.Unmarshal(entry.Value(), layout); err != nil {
				return nil, fmt.Errorf("failed to unmarshal room layout: %w", err)
			}
			revision = entry.Revision()
		}

		if err := mutate(layout); err != nil {
			return nil, err
		}

		data, err := proto.Marshal(layout)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal room layout: %w", err)
		}

		var writeErr error
		if revision == 0 {
			_, writeErr = bucket.Create(ctx, roomLayoutKey, data)
		} else {
			_, writeErr = bucket.Update(ctx, roomLayoutKey, data, revision)
		}

		if writeErr == nil {
			return layout, nil
		}

		if errors.Is(writeErr, jetstream.ErrKeyExists) {
			continue // OCC conflict, retry
		}

		return nil, fmt.Errorf("failed to store room layout: %w", writeErr)
	}

	return nil, ErrConfigConflict
}

// findGroupIndex returns the index of the named set in the layout, or -1
// if no set with that id exists.
func findGroupIndex(layout *corev1.RoomLayout, groupID string) int {
	for i, s := range layout.Groups {
		if s.Id == groupID {
			return i
		}
	}
	return -1
}

// SeedDefaultRoomGroupName is the operator-facing name given to the
// auto-created seed room group on first boot. The set is not
// system-protected — operators can rename, reorder, or delete it like
// any other.
const SeedDefaultRoomGroupName = "Rooms"

// ensureChannelRoomsAreInAGroup is the boot-time migration hook that
// satisfies ADR-031's "every channel room belongs to exactly one set"
// invariant. Idempotent — safe to call on every boot.
//
// Behavior:
//   - If no sets exist and there are channel rooms (or none — same path),
//     a seed "Rooms" set is created.
//   - Every channel room not currently referenced by any set is appended
//     to the first set in the layout. The room's GroupId proto field is
//     stamped to match the assigned set so the resolver and admin tooling
//     can rely on it.
//   - Rooms already in a set whose GroupId proto field is stale or empty
//     get re-stamped to match.
//
// Authorization: internal-only — runs as SystemActorID for layout
// mutations.
func (c *ChattoCore) ensureChannelRoomsAreInAGroup(ctx context.Context) error {
	rooms, err := c.ListRooms(ctx, KindChannel)
	if err != nil {
		return fmt.Errorf("list channel rooms: %w", err)
	}

	layout, err := c.GetRoomLayout(ctx, KindChannel)
	if err != nil {
		return fmt.Errorf("get room layout: %w", err)
	}

	// Build "room → set" map from current layout.
	roomToSet := make(map[string]string, len(rooms))
	if layout != nil {
		for _, set := range layout.Groups {
			for _, rid := range set.RoomIds {
				roomToSet[rid] = set.Id
			}
		}
	}

	// Identify rooms that aren't in any set.
	var unassigned []string
	for _, r := range rooms {
		if _, ok := roomToSet[r.Id]; !ok {
			unassigned = append(unassigned, r.Id)
		}
	}

	// If there are unassigned rooms (or no layout at all), ensure a target
	// set exists and put them in it.
	if len(unassigned) > 0 || layout == nil || len(layout.Groups) == 0 {
		var targetGroupID string
		if layout != nil && len(layout.Groups) > 0 {
			targetGroupID = layout.Groups[0].Id
		} else {
			set, err := c.CreateRoomGroup(ctx, SystemActorID, SeedDefaultRoomGroupName, "")
			if err != nil {
				return fmt.Errorf("seed default room group: %w", err)
			}
			targetGroupID = set.Id
			c.logger.Info("Seeded default room group", "group_id", set.Id, "name", SeedDefaultRoomGroupName)

			// Seed default channel-room permissions onto the new set so
			// rooms in it are operable out of the box. Idempotent — only
			// writes if neither allow nor deny is already configured.
			if err := c.SeedDefaultRoomGroupPermissions(ctx, set.Id); err != nil {
				return fmt.Errorf("seed default permissions on seed set: %w", err)
			}
		}

		for _, rid := range unassigned {
			if err := c.MoveRoomToGroup(ctx, SystemActorID, rid, targetGroupID); err != nil {
				return fmt.Errorf("move room %s to default set: %w", rid, err)
			}
			roomToSet[rid] = targetGroupID
		}
	}

	// Stamp Room.GroupId for any room whose proto field doesn't match its
	// layout membership. New rooms created post-#454 already have GroupId
	// set correctly; this loop catches rooms that pre-date the field.
	for _, r := range rooms {
		want := roomToSet[r.Id]
		if r.GroupId == want {
			continue
		}
		r.GroupId = want
		data, err := proto.Marshal(r)
		if err != nil {
			return fmt.Errorf("marshal room %s: %w", r.Id, err)
		}
		if _, err := c.storage.serverConfigKV.Put(ctx, roomKey(KindChannel, r.Id), data); err != nil {
			return fmt.Errorf("stamp group_id on room %s: %w", r.Id, err)
		}
		c.logger.Debug("Stamped room.group_id", "room_id", r.Id, "group_id", want)
	}

	return nil
}
