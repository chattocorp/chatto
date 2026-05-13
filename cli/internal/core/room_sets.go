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

// room_sets.go provides core operations on room sets — the named, ordered
// groups of channel rooms that also serve as permission containers
// (see ADR-031). All operations mutate the single `RoomLayout` KV entry
// using the same optimistic-concurrency-control pattern as
// `UpdateRoomLayout`, so admin actions remain atomic.
//
// Authorization is enforced at the API boundary; these methods assume
// the caller is authorized.

// Errors specific to room-set operations.
var (
	// ErrRoomSetNotFound is returned when a room set ID doesn't match any existing set.
	ErrRoomSetNotFound = errors.New("room set not found")
	// ErrRoomSetHasRooms is returned when trying to delete a set that still contains rooms.
	ErrRoomSetHasRooms = errors.New("room set has rooms; move them out before deleting")
	// ErrRoomSetNameEmpty is returned when a set name is empty or whitespace.
	ErrRoomSetNameEmpty = errors.New("room set name must not be empty")
)

// CreateRoomSet appends a new (empty) room set to the layout and returns it.
// Name is trimmed; description may be empty.
func (c *ChattoCore) CreateRoomSet(ctx context.Context, actorID, name, description string) (*corev1.RoomSet, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrRoomSetNameEmpty
	}

	newSet := &corev1.RoomSet{
		Id:          NewRoomSetID(),
		Name:        name,
		Description: description,
	}

	if _, err := c.mutateRoomLayout(ctx, func(layout *corev1.RoomLayout) error {
		layout.Sets = append(layout.Sets, newSet)
		return nil
	}); err != nil {
		return nil, err
	}

	c.logger.Info("Created room set", "set_id", newSet.Id, "name", name, "actor_id", actorID)
	return newSet, nil
}

// UpdateRoomSet changes a set's name and/or description. The set's id,
// position in the layout, and member room list are preserved.
func (c *ChattoCore) UpdateRoomSet(ctx context.Context, actorID, setID, name, description string) (*corev1.RoomSet, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrRoomSetNameEmpty
	}

	var updated *corev1.RoomSet
	if _, err := c.mutateRoomLayout(ctx, func(layout *corev1.RoomLayout) error {
		idx := findSetIndex(layout, setID)
		if idx == -1 {
			return ErrRoomSetNotFound
		}
		layout.Sets[idx].Name = name
		layout.Sets[idx].Description = description
		updated = layout.Sets[idx]
		return nil
	}); err != nil {
		return nil, err
	}

	c.logger.Info("Updated room set", "set_id", setID, "name", name, "actor_id", actorID)
	return updated, nil
}

// GetRoomSet returns the named set, or ErrRoomSetNotFound if no such set exists.
func (c *ChattoCore) GetRoomSet(ctx context.Context, setID string) (*corev1.RoomSet, error) {
	layout, err := c.GetRoomLayout(ctx, KindChannel)
	if err != nil {
		return nil, err
	}
	if layout == nil {
		return nil, ErrRoomSetNotFound
	}
	idx := findSetIndex(layout, setID)
	if idx == -1 {
		return nil, ErrRoomSetNotFound
	}
	return layout.Sets[idx], nil
}

// DeleteRoomSet removes a set from the layout. Fails with ErrRoomSetHasRooms
// if the set still contains any rooms — the operator must move them out
// first. There is no cascade.
func (c *ChattoCore) DeleteRoomSet(ctx context.Context, actorID, setID string) error {
	if _, err := c.mutateRoomLayout(ctx, func(layout *corev1.RoomLayout) error {
		idx := findSetIndex(layout, setID)
		if idx == -1 {
			return ErrRoomSetNotFound
		}
		if len(layout.Sets[idx].RoomIds) > 0 {
			return ErrRoomSetHasRooms
		}
		layout.Sets = slices.Delete(layout.Sets, idx, idx+1)
		return nil
	}); err != nil {
		return err
	}

	c.logger.Info("Deleted room set", "set_id", setID, "actor_id", actorID)
	return nil
}

// MoveRoomToSet moves a room into the target set, removing it from any
// other set it was previously in. The room is appended to the end of
// the target set's room list. If the room was not previously in any
// set (e.g., a freshly created room not yet assigned), it is simply
// added to the target.
//
// Authorization for the *source* and *target* sets must be checked by
// the caller — see ADR-031's two-set rule.
func (c *ChattoCore) MoveRoomToSet(ctx context.Context, actorID, roomID, targetSetID string) error {
	if _, err := c.mutateRoomLayout(ctx, func(layout *corev1.RoomLayout) error {
		targetIdx := findSetIndex(layout, targetSetID)
		if targetIdx == -1 {
			return ErrRoomSetNotFound
		}

		// Remove from any other set
		for _, set := range layout.Sets {
			set.RoomIds = slices.DeleteFunc(set.RoomIds, func(id string) bool {
				return id == roomID
			})
		}

		// Append to target (avoiding duplicates — defensive)
		if !slices.Contains(layout.Sets[targetIdx].RoomIds, roomID) {
			layout.Sets[targetIdx].RoomIds = append(layout.Sets[targetIdx].RoomIds, roomID)
		}
		return nil
	}); err != nil {
		return err
	}

	c.logger.Info("Moved room to set", "room_id", roomID, "set_id", targetSetID, "actor_id", actorID)
	return nil
}

// ReorderRoomSets reorders the layout's room sets to match the given list of
// IDs. Every existing set must appear exactly once; unknown IDs and missing
// IDs both return ErrRoomSetNotFound. Room-list contents are preserved.
func (c *ChattoCore) ReorderRoomSets(ctx context.Context, actorID string, orderedSetIDs []string) error {
	if _, err := c.mutateRoomLayout(ctx, func(layout *corev1.RoomLayout) error {
		if len(orderedSetIDs) != len(layout.Sets) {
			return ErrRoomSetNotFound
		}

		bySetID := make(map[string]*corev1.RoomSet, len(layout.Sets))
		for _, s := range layout.Sets {
			bySetID[s.Id] = s
		}

		reordered := make([]*corev1.RoomSet, 0, len(orderedSetIDs))
		for _, id := range orderedSetIDs {
			set, ok := bySetID[id]
			if !ok {
				return ErrRoomSetNotFound
			}
			reordered = append(reordered, set)
		}
		layout.Sets = reordered
		return nil
	}); err != nil {
		return err
	}

	c.logger.Info("Reordered room sets", "order", orderedSetIDs, "actor_id", actorID)
	return nil
}

// mutateRoomLayout reads the layout, applies the given mutator, and writes
// it back atomically using OCC. If the layout doesn't yet exist, an empty
// one is created and the mutator runs against that. Returns ErrConfigConflict
// if too many retries are exhausted.
//
// The mutator is allowed to return a domain error (e.g. ErrRoomSetNotFound);
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

// findSetIndex returns the index of the named set in the layout, or -1
// if no set with that id exists.
func findSetIndex(layout *corev1.RoomLayout, setID string) int {
	for i, s := range layout.Sets {
		if s.Id == setID {
			return i
		}
	}
	return -1
}
