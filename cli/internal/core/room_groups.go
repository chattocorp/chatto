package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const maxMoveRoomToGroupRetries = 5

// room_groups.go is the API surface for channel-room groups (ADR-031).
//
// ADR-035 phase 6: storage is event-sourced. Group lifecycle and
// per-group room-membership events live on `evt.group.{G}`; the
// operator-defined inter-group ordering lives on the singleton
// `evt.layout.default` aggregate. The legacy `room_group.{id}` and
// `room_layout` KV records are no longer written; they remain
// populated from pre-phase-6 history and the boot-time migrations,
// kept around solely to allow a rollback to a pre-phase-6 binary.
//
// Reads compose three read-model indexes:
//   - RoomGroups: per-group metadata + ordered room_ids
//   - RoomLayout: operator-defined ordering of group IDs
//   - RoomCatalog: room metadata, used for the final reconciliation
//
// `ListRoomGroupsOrdered` walks the layout's ordering, drops stale
// entries, and appends any orphan groups (present in RoomGroups but
// missing from the layout) at the end by NanoID order — same
// self-healing reconciliation the KV-era code did, just sourced from
// in-memory projections.
//
// Authorization is enforced at the API boundary; these methods
// assume the caller is authorized.

// Errors specific to room-group operations.
var (
	ErrRoomGroupNotFound      = errors.New("room group not found")
	ErrRoomGroupHasRooms      = errors.New("room group has rooms; move them out before deleting")
	ErrRoomGroupNameEmpty     = errors.New("room group name must not be empty")
	ErrRoomGroupOrderMismatch = errors.New("room group order must be a permutation of existing groups")
)

// CreateRoomGroup publishes a RoomGroupCreatedEvent and appends the
// new group ID to the layout ordering via a RoomGroupsReorderedEvent.
// Name is trimmed; description may be empty.
func (c *ChattoCore) CreateRoomGroup(ctx context.Context, actorID, name, description string) (*corev1.RoomGroup, error) {
	name = strings.TrimSpace(name)
	if err := validateRoomGroupMetadata(name, description); err != nil {
		return nil, err
	}

	group := &corev1.RoomGroup{
		Id:          NewRoomGroupID(),
		Name:        name,
		Description: description,
	}

	createdEvent := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomGroupCreated{
			RoomGroupCreated: &corev1.RoomGroupCreatedEvent{
				GroupId:     group.Id,
				Name:        group.Name,
				Description: group.Description,
			},
		},
	})
	if _, err := c.roomService.appendGroupLayoutEventually(ctx, c.EventPublisher, events.GroupAggregate(group.Id), createdEvent); err != nil {
		return nil, fmt.Errorf("publish RoomGroupCreatedEvent: %w", err)
	}

	// Append the new group to the layout ordering. Best-effort: if
	// this fails the group still exists in the catalog and the
	// reconciler in ListRoomGroupsOrdered will append it as an orphan.
	if err := c.appendGroupToLayout(ctx, actorID, group.Id); err != nil {
		c.logger.Warn("Failed to append new group to layout ordering",
			"group_id", group.Id, "error", err)
	}

	c.logger.Info("Created room group", "group_id", group.Id, "name", name, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "create_group")
	return group, nil
}

// UpdateRoomGroup publishes a RoomGroupUpdatedEvent. Layout ordering
// is untouched; only metadata changes.
func (c *ChattoCore) UpdateRoomGroup(ctx context.Context, actorID, groupID, name, description string) (*corev1.RoomGroup, error) {
	name = strings.TrimSpace(name)
	if err := validateRoomGroupMetadata(name, description); err != nil {
		return nil, err
	}

	if !c.RoomGroups.Exists(groupID) {
		return nil, ErrRoomGroupNotFound
	}

	updatedEvent := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomGroupUpdated{
			RoomGroupUpdated: &corev1.RoomGroupUpdatedEvent{
				GroupId:     groupID,
				Name:        name,
				Description: description,
			},
		},
	})
	if _, err := c.roomService.appendGroupLayout(ctx, c.EventPublisher, events.GroupAggregate(groupID), updatedEvent); err != nil {
		return nil, fmt.Errorf("publish RoomGroupUpdatedEvent: %w", err)
	}

	c.logger.Info("Updated room group", "group_id", groupID, "name", name, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "update_group")

	updated, _ := c.RoomGroups.Get(groupID)
	return updated, nil
}

func validateRoomGroupMetadata(name, description string) error {
	if name == "" {
		return ErrRoomGroupNameEmpty
	}
	if err := validateStringMaxLength("room group name", name, MaxRoomGroupNameLength); err != nil {
		return err
	}
	if err := validateStringMaxLength("room group description", description, MaxRoomGroupDescriptionLength); err != nil {
		return err
	}
	return nil
}

// GetRoomGroup reads a single group from the RoomGroups projection.
// Returns ErrRoomGroupNotFound if no RoomGroupCreatedEvent for the
// ID has been observed.
func (c *ChattoCore) GetRoomGroup(_ context.Context, groupID string) (*corev1.RoomGroup, error) {
	g, ok := c.RoomGroups.Get(groupID)
	if !ok {
		return nil, ErrRoomGroupNotFound
	}
	return g, nil
}

// DeleteRoomGroup removes a group via RoomGroupDeletedEvent. Fails
// with ErrRoomGroupHasRooms if the group still contains any rooms —
// operators must move them out first. The layout ordering is updated
// via a follow-up RoomGroupsReorderedEvent.
func (c *ChattoCore) DeleteRoomGroup(ctx context.Context, actorID, groupID string) error {
	g, ok := c.RoomGroups.Get(groupID)
	if !ok {
		return ErrRoomGroupNotFound
	}
	if len(g.RoomIds) > 0 {
		return ErrRoomGroupHasRooms
	}

	deletedEvent := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomGroupDeleted{
			RoomGroupDeleted: &corev1.RoomGroupDeletedEvent{
				GroupId: groupID,
			},
		},
	})
	if _, err := c.roomService.appendGroupLayoutEventually(ctx, c.EventPublisher, events.GroupAggregate(groupID), deletedEvent); err != nil {
		return fmt.Errorf("publish RoomGroupDeletedEvent: %w", err)
	}

	if err := c.removeGroupFromLayout(ctx, actorID, groupID); err != nil {
		c.logger.Warn("Failed to remove deleted group from layout ordering",
			"group_id", groupID, "error", err)
	}

	c.logger.Info("Deleted room group", "group_id", groupID, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "delete_group")
	return nil
}

// MoveRoomToGroup moves a room into the target group, removing it
// from any other group it was previously in (ADR-031's "every room
// belongs to exactly one group" invariant). Two events per ADR-034
// Approach A: RoomRemovedFromGroup on source, RoomAddedToGroup on
// target — projection sees them in stream order and the invariant
// holds at every intermediate sequence.
//
// Authorization for the source and target groups must be checked by
// the caller — see ADR-031's two-group rule.
func (c *ChattoCore) MoveRoomToGroup(ctx context.Context, actorID, roomID, targetGroupID string) error {
	occFilter := events.GroupSubjectFilter()
	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, occFilter)
		if err != nil {
			return fmt.Errorf("read room-group OCC seq: %w", err)
		}
		if filterSeq > 0 {
			if err := c.roomService.waitForGroupLayout(ctx, events.SubjectPosition(occFilter, filterSeq)); err != nil {
				return fmt.Errorf("wait for room group layout projection: %w", err)
			}
		}

		if !c.RoomGroups.Exists(targetGroupID) {
			return ErrRoomGroupNotFound
		}

		sourceGroupID := c.RoomGroups.GroupForRoom(roomID)
		if sourceGroupID == targetGroupID {
			// Already in the target group; idempotent no-op.
			return nil
		}

		// Build the move as an atomic batch (ADR-034 Approach A): the
		// RoomRemovedFromGroup on the source and the RoomAddedToGroup on
		// the target land adjacently in stream order. The first entry
		// carries wildcard OCC over evt.group.>, so a concurrent move that
		// changes any group membership forces a retry and a fresh source
		// lookup before we publish.
		added := newEvent(actorID, &corev1.Event{
			Event: &corev1.Event_RoomAddedToGroup{
				RoomAddedToGroup: &corev1.RoomAddedToGroupEvent{
					GroupId: targetGroupID,
					RoomId:  roomID,
				},
			},
		})

		var entries []events.BatchEntry
		if sourceGroupID != "" {
			removed := newEvent(actorID, &corev1.Event{
				Event: &corev1.Event_RoomRemovedFromGroup{
					RoomRemovedFromGroup: &corev1.RoomRemovedFromGroupEvent{
						GroupId: sourceGroupID,
						RoomId:  roomID,
					},
				},
			})
			sourceAgg := events.GroupAggregate(sourceGroupID)
			entries = append(entries, events.BatchEntry{
				Subject:       sourceAgg.SubjectFor(removed),
				Event:         removed,
				HasOCC:        true,
				ExpectedSeq:   filterSeq,
				FilterSubject: occFilter,
			})
		}
		targetAgg := events.GroupAggregate(targetGroupID)
		entries = append(entries, events.BatchEntry{
			Subject: targetAgg.SubjectFor(added),
			Event:   added,
		})
		if !entries[0].HasOCC {
			entries[0].HasOCC = true
			entries[0].ExpectedSeq = filterSeq
			entries[0].FilterSubject = occFilter
		}

		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			c.logger.Info("Moved room to group", "room_id", roomID, "group_id", targetGroupID, "actor_id", actorID)
			c.notifyRoomLayoutChanged(ctx, actorID, "move_room")

			// Wait on the final seq — the projector applies in stream order
			// so reaching the last batch entry's seq implies every earlier
			// entry's Apply has also landed.
			lastSubject := entries[len(entries)-1].Subject
			if err := c.roomService.waitForGroupLayout(ctx, events.SubjectPosition(lastSubject, seqs[len(seqs)-1])); err != nil {
				return fmt.Errorf("wait for room group layout projection: %w", err)
			}
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return fmt.Errorf("publish move-room batch: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return fmt.Errorf("move-room OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

// ReorderRoomGroups publishes a RoomGroupsReorderedEvent with the
// new inter-group ordering. orderedGroupIDs must be a permutation of
// the current set of groups in the RoomGroups projection — extras,
// duplicates, or missing IDs return ErrRoomGroupOrderMismatch.
func (c *ChattoCore) ReorderRoomGroups(ctx context.Context, actorID string, orderedGroupIDs []string) error {
	current := c.RoomGroups.All()
	currentIDs := make(map[string]struct{}, len(current))
	for _, g := range current {
		currentIDs[g.Id] = struct{}{}
	}

	if len(orderedGroupIDs) != len(currentIDs) {
		return ErrRoomGroupOrderMismatch
	}
	seen := make(map[string]struct{}, len(orderedGroupIDs))
	for _, id := range orderedGroupIDs {
		if _, dup := seen[id]; dup {
			return ErrRoomGroupOrderMismatch
		}
		if _, ok := currentIDs[id]; !ok {
			return ErrRoomGroupOrderMismatch
		}
		seen[id] = struct{}{}
	}

	if err := c.publishLayoutOrdering(ctx, actorID, orderedGroupIDs); err != nil {
		return err
	}

	c.logger.Info("Reordered room groups", "order", orderedGroupIDs, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "reorder_groups")
	return nil
}

// ReorderRoomsInGroup publishes a RoomsInGroupReorderedEvent with a
// new intra-group room ordering. orderedRoomIDs must be a permutation
// of the group's current room_ids — extras, duplicates, or missing
// IDs return ErrRoomGroupOrderMismatch.
//
// Cross-group moves go through MoveRoomToGroup; this method is for
// intra-group drag-reorder where the membership set doesn't change.
func (c *ChattoCore) ReorderRoomsInGroup(ctx context.Context, actorID, groupID string, orderedRoomIDs []string) error {
	g, ok := c.RoomGroups.Get(groupID)
	if !ok {
		return ErrRoomGroupNotFound
	}

	if len(orderedRoomIDs) != len(g.RoomIds) {
		return ErrRoomGroupOrderMismatch
	}
	current := make(map[string]struct{}, len(g.RoomIds))
	for _, id := range g.RoomIds {
		current[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(orderedRoomIDs))
	for _, id := range orderedRoomIDs {
		if _, dup := seen[id]; dup {
			return ErrRoomGroupOrderMismatch
		}
		if _, ok := current[id]; !ok {
			return ErrRoomGroupOrderMismatch
		}
		seen[id] = struct{}{}
	}

	reorderedEvent := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomsInGroupReordered{
			RoomsInGroupReordered: &corev1.RoomsInGroupReorderedEvent{
				GroupId: groupID,
				RoomIds: slices.Clone(orderedRoomIDs),
			},
		},
	})
	if _, err := c.roomService.appendGroupLayout(ctx, c.EventPublisher, events.GroupAggregate(groupID), reorderedEvent); err != nil {
		return fmt.Errorf("publish RoomsInGroupReorderedEvent: %w", err)
	}

	c.logger.Info("Reordered rooms in group", "group_id", groupID, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "reorder_rooms_in_group")
	return nil
}

// ListRoomGroupsOrdered returns the layout-ordered list of channel
// groups, dropping stale references and appending orphans (groups
// present in the catalog but missing from the layout) at the end by
// NanoID order so the sidebar self-heals on read.
//
// `kind` is preserved on the signature for symmetry with other room
// APIs; only KindChannel participates in the layout today.
func (c *ChattoCore) ListRoomGroupsOrdered(_ context.Context, kind RoomKind) ([]*corev1.RoomGroup, error) {
	if kind != KindChannel {
		return nil, nil
	}

	order := c.RoomLayout.Order()
	all := c.RoomGroups.All()
	docs := make(map[string]*corev1.RoomGroup, len(all))
	for _, g := range all {
		docs[g.Id] = g
	}

	out := make([]*corev1.RoomGroup, 0, len(docs))
	used := make(map[string]struct{}, len(order))
	for _, id := range order {
		if _, dup := used[id]; dup {
			continue
		}
		g, ok := docs[id]
		if !ok {
			continue
		}
		out = append(out, g)
		used[id] = struct{}{}
	}

	var orphans []string
	for id := range docs {
		if _, ok := used[id]; !ok {
			orphans = append(orphans, id)
		}
	}
	slices.Sort(orphans)
	for _, id := range orphans {
		out = append(out, docs[id])
	}
	return out, nil
}

// GetRoomLayoutOrder returns the operator-defined ordering from the
// RoomLayout projection. May include IDs of groups that have since
// been deleted; use ListRoomGroupsOrdered for the reconciled view.
func (c *ChattoCore) GetRoomLayoutOrder(_ context.Context) ([]string, error) {
	return c.RoomLayout.Order(), nil
}

// ----------------------------------------------------------------------
// Layout-ordering helpers
// ----------------------------------------------------------------------

// publishLayoutOrdering writes a RoomGroupsReorderedEvent on the
// singleton layout aggregate and waits for the group/layout projection.
func (c *ChattoCore) publishLayoutOrdering(ctx context.Context, actorID string, groupIDs []string) error {
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomGroupsReordered{
			RoomGroupsReordered: &corev1.RoomGroupsReorderedEvent{
				GroupIds: slices.Clone(groupIDs),
			},
		},
	})
	if _, err := c.roomService.appendGroupLayout(ctx, c.EventPublisher, events.LayoutAggregate(), event); err != nil {
		return fmt.Errorf("publish RoomGroupsReorderedEvent: %w", err)
	}
	return nil
}

// appendGroupToLayout appends groupID to the current layout ordering
// if not already present, then publishes the new ordering.
func (c *ChattoCore) appendGroupToLayout(ctx context.Context, actorID, groupID string) error {
	current := c.RoomLayout.Order()
	if slices.Contains(current, groupID) {
		return nil
	}
	return c.publishLayoutOrdering(ctx, actorID, append(current, groupID))
}

// removeGroupFromLayout removes groupID from the current layout
// ordering and republishes if it was present.
func (c *ChattoCore) removeGroupFromLayout(ctx context.Context, actorID, groupID string) error {
	current := c.RoomLayout.Order()
	if !slices.Contains(current, groupID) {
		return nil
	}
	next := slices.DeleteFunc(current, func(id string) bool { return id == groupID })
	return c.publishLayoutOrdering(ctx, actorID, next)
}

// notifyRoomLayoutChanged is the central place every room-layout
// mutator calls to nudge connected clients. Best-effort: a publish
// failure here doesn't roll back the storage mutation that preceded
// it. `reason` is purely for log forensics.
func (c *ChattoCore) notifyRoomLayoutChanged(ctx context.Context, actorID, reason string) {
	if err := c.PublishRoomGroupsUpdated(ctx, actorID, KindChannel); err != nil {
		c.logger.Warn("Failed to publish room layout update event",
			"error", err, "actor_id", actorID, "reason", reason)
	}
}

// ----------------------------------------------------------------------
// Seed flow (boot-time)
// ----------------------------------------------------------------------

// SeedDefaultRoomGroupName is the operator-facing name given to the
// auto-created seed room group on first boot. Not system-protected —
// operators can rename, reorder, or delete it like any other.
const SeedDefaultRoomGroupName = "Lobby"

// ensureChannelRoomsAreInAGroup is the boot-time hook that satisfies
// ADR-031's "every channel room belongs to exactly one group"
// invariant. Idempotent — safe to call on every boot.
//
//   - Creates the seed "Lobby" group if no groups exist.
//   - Every channel room not currently in any group is moved into the
//     first group in the layout via MoveRoomToGroup (which emits the
//     appropriate group events).
//
// Authorization: internal-only — runs as SystemActorID for mutations.
func (c *ChattoCore) ensureChannelRoomsAreInAGroup(ctx context.Context) error {
	rooms, err := c.ListRooms(ctx, KindChannel)
	if err != nil {
		return fmt.Errorf("list channel rooms: %w", err)
	}
	groups, err := c.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		return fmt.Errorf("list room groups: %w", err)
	}

	roomToGroup := make(map[string]string, len(rooms))
	for _, g := range groups {
		for _, rid := range g.RoomIds {
			roomToGroup[rid] = g.Id
		}
	}

	var unassigned []string
	for _, r := range rooms {
		if _, ok := roomToGroup[r.Id]; !ok {
			unassigned = append(unassigned, r.Id)
		}
	}

	if len(unassigned) == 0 && len(groups) > 0 {
		return nil
	}

	var targetGroupID string
	if len(groups) > 0 {
		targetGroupID = groups[0].Id
	} else {
		seed, err := c.CreateRoomGroup(ctx, SystemActorID, SeedDefaultRoomGroupName, "")
		if err != nil {
			return fmt.Errorf("seed default room group: %w", err)
		}
		targetGroupID = seed.Id
		c.logger.Info("Seeded default room group", "group_id", seed.Id, "name", SeedDefaultRoomGroupName)
	}

	for _, rid := range unassigned {
		if err := c.MoveRoomToGroup(ctx, SystemActorID, rid, targetGroupID); err != nil {
			return fmt.Errorf("move room %s to default group: %w", rid, err)
		}
	}
	return nil
}
