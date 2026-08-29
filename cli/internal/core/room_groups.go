package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const maxMoveRoomToGroupRetries = 5

// room_groups.go is the API surface for channel-room groups (ADR-031).
//
// ADR-035 phase 6: storage is event-sourced. Group lifecycle and
// per-group room-membership events live on `evt.group.{G}`; the
// operator-defined inter-group ordering lives on the singleton
// `evt.layout.default` aggregate. The legacy `room_group.{id}` and
// `room_layout` KV records are no longer written or read by current code.
//
// RoomModel composes three read-model indexes:
//   - RoomGroupProjection: per-group metadata + ordered room_ids
//   - RoomLayoutProjection: operator-defined ordering of group IDs
//   - RoomCatalogProjection: room metadata, used for final reconciliation
//
// `ListRoomGroupsOrdered` walks the layout's ordering, drops stale
// entries, and appends any orphan groups (present in group state but
// missing from the layout) at the end by NanoID order — same
// self-healing reconciliation the KV-era code did, just sourced from
// in-memory projections.
//
// Low-level mutation helpers in this file assume the caller is authorized.
// Public admin room-layout operations should go through
// admin_room_layout_management.go so authorization stays in core.

// Errors specific to room-group operations.
var (
	ErrRoomGroupNotFound        = errors.New("room group not found")
	ErrRoomGroupHasRooms        = errors.New("room group has rooms; move them out before deleting")
	ErrRoomGroupNameEmpty       = errors.New("room group name must not be empty")
	ErrRoomGroupOrderMismatch   = errors.New("room group order must be a permutation of existing groups")
	ErrRoomMoveSourceChanged    = errors.New("room move source group changed")
	ErrSidebarLinkNotFound      = errors.New("sidebar link not found")
	ErrSidebarLinkSourceChanged = errors.New("sidebar link source group changed")
	ErrSidebarLinkLabelEmpty    = errors.New("sidebar link label must not be empty")
	ErrSidebarLinkURLInvalid    = errors.New("sidebar link URL must be an absolute http(s) URL or server-local path")
	ErrSidebarItemNotFound      = errors.New("sidebar item not found")
	ErrSidebarItemPlacement     = errors.New("sidebar item placement target is not in the destination group")
)

// CreateRoomGroup publishes a RoomGroupCreatedEvent and appends the
// new group ID to the layout ordering via a RoomGroupsReorderedEvent.
// Name is trimmed; description may be empty.
func (c *ChattoCore) CreateRoomGroup(ctx context.Context, actorID, name, description string) (*evtv1.RoomGroup, error) {
	return c.createRoomGroup(ctx, actorID, name, description, nil)
}

func (c *ChattoCore) createRoomGroup(ctx context.Context, actorID, name, description string, authorize func() error) (*evtv1.RoomGroup, error) {
	name = strings.TrimSpace(name)
	if err := validateRoomGroupMetadata(name, description); err != nil {
		return nil, err
	}

	group := &evtv1.RoomGroup{
		Id:          NewRoomGroupID(),
		Name:        name,
		Description: description,
	}

	createdEvent := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomGroupCreated{
			RoomGroupCreated: &evtv1.RoomGroupCreatedEvent{
				GroupId:     group.Id,
				Name:        group.Name,
				Description: group.Description,
			},
		},
	})
	if _, err := c.appendGroupLayoutMutation(ctx, evtstream.GroupAggregate(group.Id), createdEvent, authorize); err != nil {
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
func (c *ChattoCore) UpdateRoomGroup(ctx context.Context, actorID, groupID, name, description string) (*evtv1.RoomGroup, error) {
	return c.updateRoomGroupFields(ctx, actorID, groupID, &name, &description, nil)
}

// UpdateRoomGroupFields applies a sparse metadata patch. Each OCC retry
// recomposes omitted fields from the latest group projection so a stale client
// cannot overwrite a concurrent update to a field it did not submit.
func (c *ChattoCore) UpdateRoomGroupFields(ctx context.Context, actorID, groupID string, name, description *string) (*evtv1.RoomGroup, error) {
	return c.updateRoomGroupFields(ctx, actorID, groupID, name, description, nil)
}

func (c *ChattoCore) updateRoomGroupFields(ctx context.Context, actorID, groupID string, name, description *string, authorize func() error) (*evtv1.RoomGroup, error) {
	if name == nil && description == nil {
		return nil, fmt.Errorf("%w: provide at least one room group field to update", ErrInvalidArgument)
	}
	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		snapshot := c.roomModel.roomGroupSnapshot(groupID)
		if !snapshot.Exists {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return nil, fmt.Errorf("wait for room group layout projection before update: %w", err)
			}
			snapshot = c.roomModel.roomGroupSnapshot(groupID)
			if !snapshot.Exists {
				return nil, ErrRoomGroupNotFound
			}
		}

		nextName := snapshot.Group.GetName()
		if name != nil {
			nextName = strings.TrimSpace(*name)
		}
		nextDescription := snapshot.Group.GetDescription()
		if description != nil {
			nextDescription = *description
		}
		if err := validateRoomGroupMetadata(nextName, nextDescription); err != nil {
			return nil, err
		}

		updatedEvent := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_RoomGroupUpdated{
				RoomGroupUpdated: &evtv1.RoomGroupUpdatedEvent{
					GroupId:     groupID,
					Name:        nextName,
					Description: nextDescription,
				},
			},
		})
		if _, err := c.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(groupID), updatedEvent, snapshot.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := c.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after update OCC conflict"); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("publish RoomGroupUpdatedEvent: %w", err)
		}

		c.logger.Info("Updated room group", "group_id", groupID, "name", nextName, "actor_id", actorID)
		c.notifyRoomLayoutChanged(ctx, actorID, "update_group")
		updated, _ := c.roomModel.roomGroup(groupID)
		return updated, nil
	}
	return nil, fmt.Errorf("update-room-group OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
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

func validateSidebarLink(label, rawURL string) (string, string, error) {
	label = strings.TrimSpace(label)
	rawURL = strings.TrimSpace(rawURL)
	if label == "" {
		return "", "", ErrSidebarLinkLabelEmpty
	}
	if err := validateStringMaxLength("sidebar link label", label, MaxSidebarLinkLabelLength); err != nil {
		return "", "", err
	}
	if err := validateStringMaxLength("sidebar link URL", rawURL, MaxSidebarLinkURLLength); err != nil {
		return "", "", err
	}
	if !isValidSidebarLinkURL(rawURL) {
		return "", "", ErrSidebarLinkURLInvalid
	}
	return label, rawURL, nil
}

func isValidSidebarLinkURL(rawURL string) bool {
	if strings.HasPrefix(rawURL, "/") {
		if strings.HasPrefix(rawURL, "//") || strings.Contains(rawURL, "\\") {
			return false
		}
		parsed, err := url.ParseRequestURI(rawURL)
		return err == nil && parsed != nil && parsed.Scheme == "" && parsed.Host == ""
	}

	parsed, err := url.Parse(rawURL)
	return err == nil &&
		parsed != nil &&
		parsed.IsAbs() &&
		parsed.Host != "" &&
		(parsed.Scheme == "http" || parsed.Scheme == "https")
}

// GetRoomGroup reads a single group through RoomModel.
// Returns ErrRoomGroupNotFound if no RoomGroupCreatedEvent for the
// ID has been observed.
func (c *ChattoCore) GetRoomGroup(_ context.Context, groupID string) (*evtv1.RoomGroup, error) {
	g, ok := c.roomModel.roomGroup(groupID)
	if !ok {
		return nil, ErrRoomGroupNotFound
	}
	return g, nil
}

func (c *ChattoCore) sidebarLinkGroup(ctx context.Context, linkID string) (string, error) {
	groupID := c.roomModel.roomGroupForSidebarLink(linkID)
	if groupID != "" {
		return groupID, nil
	}
	if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
		return "", fmt.Errorf("wait for room group layout projection before sidebar-link lookup: %w", err)
	}
	groupID = c.roomModel.roomGroupForSidebarLink(linkID)
	if groupID == "" {
		return "", ErrSidebarLinkNotFound
	}
	return groupID, nil
}

func (c *ChattoCore) GetSidebarLinkGroup(ctx context.Context, linkID string) (string, error) {
	return c.sidebarLinkGroup(ctx, linkID)
}

func (c *ChattoCore) sidebarLinkInGroup(groupID, linkID string) (*evtv1.SidebarLink, error) {
	group, ok := c.roomModel.roomGroup(groupID)
	if !ok {
		return nil, ErrRoomGroupNotFound
	}
	for _, link := range group.GetSidebarLinks() {
		if link.GetId() == linkID {
			return link, nil
		}
	}
	return nil, ErrSidebarLinkNotFound
}

func sidebarLinkFromGroup(group *evtv1.RoomGroup, linkID string) *evtv1.SidebarLink {
	if group == nil {
		return nil
	}
	for _, link := range group.GetSidebarLinks() {
		if link.GetId() == linkID {
			return cloneSidebarLink(link)
		}
	}
	return nil
}

func (c *ChattoCore) appendGroupLayoutAtFilter(ctx context.Context, agg evtstream.Aggregate, event *evtv1.Event, expectedSeq uint64, check func() error) (events.StreamPosition, error) {
	authorizationSeq, err := c.authorizationFenceSeq(ctx)
	if err != nil {
		return events.StreamPosition{}, fmt.Errorf("read authorization fence seq: %w", err)
	}
	rbacSeq, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.RBACSubjectFilter())
	if err != nil {
		return events.StreamPosition{}, fmt.Errorf("read RBAC OCC filter seq: %w", err)
	}
	if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(evtstream.RBACSubjectFilter(), rbacSeq)); err != nil {
		return events.StreamPosition{}, fmt.Errorf("wait for RBAC projection: %w", err)
	}
	if check != nil {
		if err := check(); err != nil {
			return events.StreamPosition{}, err
		}
	}
	subject := agg.SubjectFor(event)
	filter := evtstream.GroupSubjectFilter()
	if agg.Type == evtstream.AggregateLayout {
		filter = evtstream.LayoutSubjectFilter()
	}
	entries := []evtstream.BatchEntry{{
		Subject:       subject,
		Event:         event,
		HasOCC:        true,
		ExpectedSeq:   expectedSeq,
		FilterSubject: filter,
	}}
	seqs, err := c.appendAuthorizationFencedBatch(ctx, event.GetActorId(), entries, authorizationSeq)
	if err != nil {
		return events.StreamPosition{}, err
	}
	seq := seqs[0]
	pos := events.SubjectPosition(subject, seq)
	if err := c.roomModel.waitForGroupLayout(ctx, pos); err != nil {
		return pos, err
	}
	return pos, nil
}

func (c *ChattoCore) appendGroupLayoutMutation(ctx context.Context, agg evtstream.Aggregate, event *evtv1.Event, check func() error) (events.StreamPosition, error) {
	filter := evtstream.GroupSubjectFilter()
	if agg.Type == evtstream.AggregateLayout {
		filter = evtstream.LayoutSubjectFilter()
	}
	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		position, err := c.EventPublisher.LastSubjectPosition(ctx, filter)
		if err != nil {
			return events.StreamPosition{}, fmt.Errorf("read room-group OCC position: %w", err)
		}
		if err := c.roomModel.waitForGroupLayout(ctx, position); err != nil {
			return events.StreamPosition{}, fmt.Errorf("wait for room-group projection: %w", err)
		}
		committed, err := c.appendGroupLayoutAtFilter(ctx, agg, event, position.Seq, check)
		if err == nil {
			return committed, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return events.StreamPosition{}, err
		}
		if err := c.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after authorization OCC conflict"); err != nil {
			return events.StreamPosition{}, err
		}
	}
	return events.StreamPosition{}, fmt.Errorf("room-group authorization OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func (c *ChattoCore) roomGroupAuthorityCheck(ctx context.Context, actorID string, groupIDs ...string) func() error {
	return func() error {
		if actorID == SystemActorID {
			return nil
		}
		for _, groupID := range groupIDs {
			if err := c.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
				return err
			}
		}
		return nil
	}
}

func (c *ChattoCore) anyRoomAuthorityCheck(ctx context.Context, actorID string) func() error {
	return func() error {
		if actorID == SystemActorID {
			return nil
		}
		return c.requireCanManageAnyRoom(ctx, actorID)
	}
}

func (c *ChattoCore) waitForGroupOCCRetry(ctx context.Context, attempt int, message string) error {
	if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		return nil
	}
}

// DeleteRoomGroup removes a group via RoomGroupDeletedEvent. Fails
// with ErrRoomGroupHasRooms if the group still contains any rooms or
// sidebar links. The layout ordering is updated via a follow-up
// RoomGroupsReorderedEvent.
func (c *ChattoCore) DeleteRoomGroup(ctx context.Context, actorID, groupID string) error {
	return c.deleteRoomGroup(ctx, actorID, groupID, nil)
}

func (c *ChattoCore) deleteRoomGroup(ctx context.Context, actorID, groupID string, authorize func() error) error {
	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		snapshot := c.roomModel.roomGroupSnapshot(groupID)
		if !snapshot.Exists {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before delete: %w", err)
			}
			snapshot = c.roomModel.roomGroupSnapshot(groupID)
			if !snapshot.Exists {
				return ErrRoomGroupNotFound
			}
		}
		if len(snapshot.Group.GetRoomIds()) > 0 || len(snapshot.Group.GetSidebarLinks()) > 0 {
			return ErrRoomGroupHasRooms
		}

		deletedEvent := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_RoomGroupDeleted{
				RoomGroupDeleted: &evtv1.RoomGroupDeletedEvent{
					GroupId: groupID,
				},
			},
		})
		if _, err := c.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(groupID), deletedEvent, snapshot.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := c.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after delete-group OCC conflict"); err != nil {
					return err
				}
				continue
			}
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
	return fmt.Errorf("delete-room-group OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
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
	return c.moveRoomToGroup(ctx, actorID, roomID, "", targetGroupID, false, nil)
}

// MoveRoomToGroupFromSource moves a room only if the room's current source
// group still matches sourceGroupID. API callers that authorize the source
// group before calling core should use this variant so a concurrent move
// cannot swap the source group between authorization and append.
func (c *ChattoCore) MoveRoomToGroupFromSource(ctx context.Context, actorID, roomID, sourceGroupID, targetGroupID string) error {
	return c.moveRoomToGroup(ctx, actorID, roomID, sourceGroupID, targetGroupID, true, nil)
}

func (c *ChattoCore) moveRoomToGroup(ctx context.Context, actorID, roomID, authorizedSourceGroupID, targetGroupID string, bindSource bool, authorize func(sourceGroupID, targetGroupID string) error) error {
	occFilter := evtstream.GroupSubjectFilter()
	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return fmt.Errorf("read authorization fence seq: %w", err)
		}
		snapshot := c.roomModel.roomGroupMoveSnapshot(roomID, targetGroupID)
		if !snapshot.TargetExists {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before not-found decision: %w", err)
			}
			snapshot = c.roomModel.roomGroupMoveSnapshot(roomID, targetGroupID)
			if !snapshot.TargetExists {
				return ErrRoomGroupNotFound
			}
		}

		sourceGroupID := snapshot.SourceGroupID
		if bindSource && sourceGroupID != authorizedSourceGroupID {
			return ErrRoomMoveSourceChanged
		}
		if sourceGroupID == targetGroupID {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before no-op decision: %w", err)
			}
			snapshot = c.roomModel.roomGroupMoveSnapshot(roomID, targetGroupID)
			sourceGroupID = snapshot.SourceGroupID
			if !snapshot.TargetExists {
				return ErrRoomGroupNotFound
			}
			if bindSource && sourceGroupID != authorizedSourceGroupID {
				return ErrRoomMoveSourceChanged
			}
			if sourceGroupID != targetGroupID {
				continue
			}
			// Already in the target group; idempotent no-op.
			return nil
		}
		if authorize != nil {
			if err := authorize(sourceGroupID, targetGroupID); err != nil {
				return err
			}
		}

		// Build the move as an atomic batch (ADR-034 Approach A): the
		// RoomRemovedFromGroup on the source and the RoomAddedToGroup on
		// the target land adjacently in stream order. The first entry
		// carries wildcard OCC over evt.group.>, so a concurrent move that
		// changes any group membership forces a retry and a fresh source
		// lookup before we publish.
		added := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_RoomAddedToGroup{
				RoomAddedToGroup: &evtv1.RoomAddedToGroupEvent{
					GroupId: targetGroupID,
					RoomId:  roomID,
				},
			},
		})

		var entries []evtstream.BatchEntry
		if sourceGroupID != "" {
			removed := newEvent(actorID, &evtv1.Event{
				Event: &evtv1.Event_RoomRemovedFromGroup{
					RoomRemovedFromGroup: &evtv1.RoomRemovedFromGroupEvent{
						GroupId: sourceGroupID,
						RoomId:  roomID,
					},
				},
			})
			sourceAgg := evtstream.GroupAggregate(sourceGroupID)
			entries = append(entries, evtstream.BatchEntry{
				Subject:       sourceAgg.SubjectFor(removed),
				Event:         removed,
				HasOCC:        true,
				ExpectedSeq:   snapshot.Seq,
				FilterSubject: occFilter,
			})
		}
		targetAgg := evtstream.GroupAggregate(targetGroupID)
		entries = append(entries, evtstream.BatchEntry{
			Subject: targetAgg.SubjectFor(added),
			Event:   added,
		})
		if !entries[0].HasOCC {
			entries[0].HasOCC = true
			entries[0].ExpectedSeq = snapshot.Seq
			entries[0].FilterSubject = occFilter
		}

		seqs, err := c.appendAuthorizationFencedBatch(ctx, actorID, entries, authorizationSeq)
		if err == nil {
			// Wait on the final seq — the projector applies in stream order
			// so reaching the last batch entry's seq implies every earlier
			// entry's Apply has also landed. Publish the transient invalidation
			// only after the local read model can serve the committed move.
			lastDomainIndex := len(entries) - 1
			lastSubject := entries[lastDomainIndex].Subject
			if err := c.roomModel.waitForGroupLayout(ctx, events.SubjectPosition(lastSubject, seqs[lastDomainIndex])); err != nil {
				return fmt.Errorf("wait for room group layout projection: %w", err)
			}

			c.logger.Info("Moved room to group", "room_id", roomID, "group_id", targetGroupID, "actor_id", actorID)
			c.notifyRoomLayoutChanged(ctx, actorID, "move_room")
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return fmt.Errorf("publish move-room batch: %w", err)
		}

		if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
			return fmt.Errorf("wait for room group layout projection after OCC conflict: %w", err)
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
// the current group set in RoomModel — extras,
// duplicates, or missing IDs return ErrRoomGroupOrderMismatch.
func (c *ChattoCore) ReorderRoomGroups(ctx context.Context, actorID string, orderedGroupIDs []string) error {
	return c.reorderRoomGroups(ctx, actorID, orderedGroupIDs, nil)
}

func (c *ChattoCore) reorderRoomGroups(ctx context.Context, actorID string, orderedGroupIDs []string, authorize func() error) error {
	current := c.roomModel.roomGroups()
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

	if err := c.publishLayoutOrdering(ctx, actorID, orderedGroupIDs, authorize); err != nil {
		return err
	}

	c.logger.Info("Reordered room groups", "order", orderedGroupIDs, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "reorder_groups")
	return nil
}

// placeRoomGroup applies relative group-placement intent against the current
// authoritative order on every OCC retry. An empty beforeGroupID places the
// group last.
func (c *ChattoCore) placeRoomGroup(ctx context.Context, actorID, groupID, beforeGroupID string, authorize func() error) error {
	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		position, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.LayoutSubjectFilter())
		if err != nil {
			return fmt.Errorf("read room-layout OCC position: %w", err)
		}
		if err := c.roomModel.waitForGroupLayout(ctx, position); err != nil {
			return fmt.Errorf("wait for room-layout projection: %w", err)
		}
		groups, err := c.ListRoomGroupsOrdered(ctx, KindChannel)
		if err != nil {
			return err
		}
		current := make([]string, 0, len(groups))
		for _, group := range groups {
			current = append(current, group.GetId())
		}
		next, changed, err := placeStringBefore(current, groupID, beforeGroupID)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		event := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_RoomGroupsReordered{
				RoomGroupsReordered: &evtv1.RoomGroupsReorderedEvent{GroupIds: next},
			},
		})
		if _, err := c.appendGroupLayoutAtFilter(ctx, evtstream.LayoutAggregate(), event, position.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := c.waitForGroupOCCRetry(ctx, attempt, "wait for room layout after relative placement conflict"); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("publish relative room-group placement: %w", err)
		}
		c.logger.Info("Moved room group", "group_id", groupID, "actor_id", actorID)
		c.notifyRoomLayoutChanged(ctx, actorID, "move_group")
		return nil
	}
	return fmt.Errorf("move-room-group OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func placeStringBefore(current []string, movingID, beforeID string) ([]string, bool, error) {
	if !slices.Contains(current, movingID) {
		return nil, false, ErrRoomGroupNotFound
	}
	if beforeID == movingID {
		return nil, false, ErrRoomGroupOrderMismatch
	}
	next := slices.DeleteFunc(slices.Clone(current), func(id string) bool { return id == movingID })
	insertAt := len(next)
	if beforeID != "" {
		insertAt = slices.Index(next, beforeID)
		if insertAt < 0 {
			return nil, false, ErrRoomGroupNotFound
		}
	}
	next = slices.Insert(next, insertAt, movingID)
	return next, !slices.Equal(current, next), nil
}

// ReorderRoomsInGroup publishes a RoomsInGroupReorderedEvent with a
// new intra-group room ordering. orderedRoomIDs must be a permutation
// of the group's current room_ids — extras, duplicates, or missing
// IDs return ErrRoomGroupOrderMismatch.
//
// Cross-group moves go through MoveRoomToGroup; this method is for
// intra-group drag-reorder where the membership set doesn't change.
func (c *ChattoCore) ReorderRoomsInGroup(ctx context.Context, actorID, groupID string, orderedRoomIDs []string) error {
	g, ok := c.roomModel.roomGroup(groupID)
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

	reorderedEvent := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomsInGroupReordered{
			RoomsInGroupReordered: &evtv1.RoomsInGroupReorderedEvent{
				GroupId: groupID,
				RoomIds: slices.Clone(orderedRoomIDs),
			},
		},
	})
	if _, err := c.appendGroupLayoutMutation(ctx, evtstream.GroupAggregate(groupID), reorderedEvent, nil); err != nil {
		return fmt.Errorf("publish RoomsInGroupReorderedEvent: %w", err)
	}

	c.logger.Info("Reordered rooms in group", "group_id", groupID, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "reorder_rooms_in_group")
	return nil
}

func (c *ChattoCore) CreateSidebarLink(ctx context.Context, actorID, groupID, label, rawURL string) (*evtv1.SidebarLink, error) {
	return c.createSidebarLink(ctx, actorID, groupID, label, rawURL, nil)
}

func (c *ChattoCore) createSidebarLink(ctx context.Context, actorID, groupID, label, rawURL string, authorize func() error) (*evtv1.SidebarLink, error) {
	label, rawURL, err := validateSidebarLink(label, rawURL)
	if err != nil {
		return nil, err
	}
	link := &evtv1.SidebarLink{
		Id:    NewSidebarLinkID(),
		Label: label,
		Url:   rawURL,
	}
	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		snapshot := c.roomModel.roomGroupSnapshot(groupID)
		if !snapshot.Exists {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return nil, fmt.Errorf("wait for room group layout projection before sidebar-link create: %w", err)
			}
			snapshot = c.roomModel.roomGroupSnapshot(groupID)
			if !snapshot.Exists {
				return nil, ErrRoomGroupNotFound
			}
		}

		event := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_SidebarLinkAddedToGroup{
				SidebarLinkAddedToGroup: &evtv1.SidebarLinkAddedToGroupEvent{
					GroupId: groupID,
					LinkId:  link.Id,
					Label:   link.Label,
					Url:     link.Url,
				},
			},
		})
		if _, err := c.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(groupID), event, snapshot.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := c.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after sidebar-link create OCC conflict"); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("publish SidebarLinkAddedToGroupEvent: %w", err)
		}

		c.logger.Info("Created sidebar link", "group_id", groupID, "link_id", link.Id, "actor_id", actorID)
		c.notifyRoomLayoutChanged(ctx, actorID, "create_sidebar_link")
		return link, nil
	}
	return nil, fmt.Errorf("create-sidebar-link OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func (c *ChattoCore) UpdateSidebarLink(ctx context.Context, actorID, linkID, label, rawURL string) (*evtv1.SidebarLink, error) {
	groupID, err := c.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return nil, err
	}
	return c.UpdateSidebarLinkInGroup(ctx, actorID, groupID, linkID, label, rawURL)
}

func (c *ChattoCore) UpdateSidebarLinkInGroup(ctx context.Context, actorID, groupID, linkID, label, rawURL string) (*evtv1.SidebarLink, error) {
	return c.updateSidebarLinkInGroup(ctx, actorID, groupID, linkID, label, rawURL, nil)
}

func (c *ChattoCore) updateSidebarLinkInGroup(ctx context.Context, actorID, groupID, linkID, label, rawURL string, authorize func() error) (*evtv1.SidebarLink, error) {
	label, rawURL, err := validateSidebarLink(label, rawURL)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		snapshot := c.roomModel.roomGroupSnapshot(groupID)
		if !snapshot.Exists {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return nil, fmt.Errorf("wait for room group layout projection before sidebar-link update: %w", err)
			}
			snapshot = c.roomModel.roomGroupSnapshot(groupID)
			if !snapshot.Exists {
				return nil, ErrRoomGroupNotFound
			}
		}
		if sidebarLinkFromGroup(snapshot.Group, linkID) == nil {
			return nil, ErrSidebarLinkNotFound
		}

		event := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_SidebarLinkUpdated{
				SidebarLinkUpdated: &evtv1.SidebarLinkUpdatedEvent{
					GroupId: groupID,
					LinkId:  linkID,
					Label:   label,
					Url:     rawURL,
				},
			},
		})
		if _, err := c.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(groupID), event, snapshot.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := c.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after sidebar-link update OCC conflict"); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("publish SidebarLinkUpdatedEvent: %w", err)
		}

		c.logger.Info("Updated sidebar link", "group_id", groupID, "link_id", linkID, "actor_id", actorID)
		c.notifyRoomLayoutChanged(ctx, actorID, "update_sidebar_link")
		return &evtv1.SidebarLink{Id: linkID, Label: label, Url: rawURL}, nil
	}
	return nil, fmt.Errorf("update-sidebar-link OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func (c *ChattoCore) DeleteSidebarLink(ctx context.Context, actorID, linkID string) error {
	groupID, err := c.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return err
	}
	return c.DeleteSidebarLinkInGroup(ctx, actorID, groupID, linkID)
}

func (c *ChattoCore) DeleteSidebarLinkInGroup(ctx context.Context, actorID, groupID, linkID string) error {
	return c.deleteSidebarLinkInGroup(ctx, actorID, groupID, linkID, nil)
}

func (c *ChattoCore) deleteSidebarLinkInGroup(ctx context.Context, actorID, groupID, linkID string, authorize func() error) error {
	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		snapshot := c.roomModel.roomGroupSnapshot(groupID)
		if !snapshot.Exists {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before sidebar-link delete: %w", err)
			}
			snapshot = c.roomModel.roomGroupSnapshot(groupID)
			if !snapshot.Exists {
				return ErrRoomGroupNotFound
			}
		}
		if sidebarLinkFromGroup(snapshot.Group, linkID) == nil {
			return ErrSidebarLinkNotFound
		}

		event := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_SidebarLinkRemovedFromGroup{
				SidebarLinkRemovedFromGroup: &evtv1.SidebarLinkRemovedFromGroupEvent{
					GroupId: groupID,
					LinkId:  linkID,
				},
			},
		})
		if _, err := c.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(groupID), event, snapshot.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := c.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after sidebar-link delete OCC conflict"); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("publish SidebarLinkRemovedFromGroupEvent: %w", err)
		}

		c.logger.Info("Deleted sidebar link", "group_id", groupID, "link_id", linkID, "actor_id", actorID)
		c.notifyRoomLayoutChanged(ctx, actorID, "delete_sidebar_link")
		return nil
	}
	return fmt.Errorf("delete-sidebar-link OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func (c *ChattoCore) MoveSidebarLinkToGroup(ctx context.Context, actorID, linkID, targetGroupID string) error {
	sourceGroupID, err := c.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return err
	}
	return c.MoveSidebarLinkBetweenGroups(ctx, actorID, linkID, sourceGroupID, targetGroupID)
}

func (c *ChattoCore) MoveSidebarLinkBetweenGroups(ctx context.Context, actorID, linkID, sourceGroupID, targetGroupID string) error {
	return c.moveSidebarLinkBetweenGroups(ctx, actorID, linkID, sourceGroupID, targetGroupID, nil)
}

func (c *ChattoCore) moveSidebarLinkBetweenGroups(ctx context.Context, actorID, linkID, sourceGroupID, targetGroupID string, authorize func(sourceGroupID, targetGroupID string) error) error {
	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return fmt.Errorf("read authorization fence seq: %w", err)
		}
		snapshot := c.roomModel.sidebarLinkMoveSnapshot(linkID, targetGroupID)
		if !snapshot.TargetExists {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before sidebar-link move target decision: %w", err)
			}
			snapshot = c.roomModel.sidebarLinkMoveSnapshot(linkID, targetGroupID)
			if !snapshot.TargetExists {
				return ErrRoomGroupNotFound
			}
		}
		if snapshot.SourceGroupID == "" || snapshot.Link == nil {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before sidebar-link move source decision: %w", err)
			}
			snapshot = c.roomModel.sidebarLinkMoveSnapshot(linkID, targetGroupID)
			if snapshot.SourceGroupID == "" || snapshot.Link == nil {
				return ErrSidebarLinkNotFound
			}
		}
		if snapshot.SourceGroupID != sourceGroupID {
			return ErrSidebarLinkSourceChanged
		}
		if snapshot.SourceGroupID == targetGroupID {
			return nil
		}
		if authorize != nil {
			if err := authorize(snapshot.SourceGroupID, targetGroupID); err != nil {
				return err
			}
		}

		removed := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_SidebarLinkRemovedFromGroup{
				SidebarLinkRemovedFromGroup: &evtv1.SidebarLinkRemovedFromGroupEvent{
					GroupId: snapshot.SourceGroupID,
					LinkId:  linkID,
				},
			},
		})
		added := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_SidebarLinkAddedToGroup{
				SidebarLinkAddedToGroup: &evtv1.SidebarLinkAddedToGroupEvent{
					GroupId: targetGroupID,
					LinkId:  linkID,
					Label:   snapshot.Link.Label,
					Url:     snapshot.Link.Url,
				},
			},
		})
		entries := []evtstream.BatchEntry{
			{
				Subject:       evtstream.GroupAggregate(snapshot.SourceGroupID).SubjectFor(removed),
				Event:         removed,
				HasOCC:        true,
				ExpectedSeq:   snapshot.Seq,
				FilterSubject: evtstream.GroupSubjectFilter(),
			},
			{
				Subject: evtstream.GroupAggregate(targetGroupID).SubjectFor(added),
				Event:   added,
			},
		}
		seqs, err := c.appendAuthorizationFencedBatch(ctx, actorID, entries, authorizationSeq)
		if err == nil {
			lastDomainIndex := len(entries) - 1
			lastSubject := entries[lastDomainIndex].Subject
			if err := c.roomModel.waitForGroupLayout(ctx, events.SubjectPosition(lastSubject, seqs[lastDomainIndex])); err != nil {
				return fmt.Errorf("wait for room group layout projection: %w", err)
			}
			c.logger.Info("Moved sidebar link", "link_id", linkID, "source_group_id", snapshot.SourceGroupID, "target_group_id", targetGroupID, "actor_id", actorID)
			c.notifyRoomLayoutChanged(ctx, actorID, "move_sidebar_link")
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return fmt.Errorf("publish sidebar-link move batch: %w", err)
		}
		if err := c.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after sidebar-link move OCC conflict"); err != nil {
			return err
		}
	}
	return fmt.Errorf("move-sidebar-link OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func (c *ChattoCore) ReorderSidebarItemsInGroup(ctx context.Context, actorID, groupID string, orderedEntries []*evtv1.SidebarGroupEntry) error {
	return c.reorderSidebarItemsInGroup(ctx, actorID, groupID, orderedEntries, nil)
}

// placeSidebarItem applies one relative room or link placement against a
// consistent room-group projection snapshot. An absent before entry appends to
// the destination. Cross-group moves and the target reorder commit atomically.
func (c *ChattoCore) placeSidebarItem(
	ctx context.Context,
	actorID string,
	item *evtv1.SidebarGroupEntry,
	targetGroupID string,
	before *evtv1.SidebarGroupEntry,
	authorize func(sourceGroupID, targetGroupID string) error,
) error {
	if sidebarEntryKey(item) == "" {
		return fmt.Errorf("%w: invalid item reference", ErrInvalidArgument)
	}
	if before != nil && sidebarEntryKey(before) == "" {
		return fmt.Errorf("%w: invalid before reference", ErrInvalidArgument)
	}
	if before != nil && sidebarEntryKey(before) == sidebarEntryKey(item) {
		return ErrSidebarItemPlacement
	}

	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		snapshot := c.roomModel.sidebarItemPlacementSnapshot(item, targetGroupID)
		if snapshot.TargetGroup == nil || snapshot.SourceGroup == nil {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return fmt.Errorf("wait for room-group projection before sidebar placement: %w", err)
			}
			snapshot = c.roomModel.sidebarItemPlacementSnapshot(item, targetGroupID)
		}
		if snapshot.TargetGroup == nil {
			return ErrRoomGroupNotFound
		}
		if snapshot.SourceGroup == nil {
			return ErrSidebarItemNotFound
		}
		if err := authorize(snapshot.SourceGroupID, targetGroupID); err != nil {
			return err
		}

		targetEntries, changed, err := placeSidebarEntryBefore(
			snapshot.TargetGroup.GetEntries(),
			item,
			before,
			snapshot.SourceGroupID == targetGroupID,
		)
		if err != nil {
			return err
		}
		if !changed && snapshot.SourceGroupID == targetGroupID {
			return nil
		}

		if snapshot.SourceGroupID == targetGroupID {
			event := sidebarEntriesReorderedEvent(actorID, targetGroupID, targetEntries)
			check := func() error { return authorize(snapshot.SourceGroupID, targetGroupID) }
			if _, err := c.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(targetGroupID), event, snapshot.Seq, check); err != nil {
				if errors.Is(err, events.ErrConflict) {
					if err := c.waitForGroupOCCRetry(ctx, attempt, "wait for room-group projection after sidebar placement conflict"); err != nil {
						return err
					}
					continue
				}
				return fmt.Errorf("publish sidebar item placement: %w", err)
			}
			c.notifyRoomLayoutChanged(ctx, actorID, "move_sidebar_item")
			return nil
		}

		authorizationSeq, err := c.authorizationFenceSeq(ctx)
		if err != nil {
			return fmt.Errorf("read authorization fence seq: %w", err)
		}
		entries, err := sidebarItemMoveEvents(actorID, snapshot, item, targetGroupID, targetEntries)
		if err != nil {
			return err
		}
		entries[0].HasOCC = true
		entries[0].ExpectedSeq = snapshot.Seq
		entries[0].FilterSubject = evtstream.GroupSubjectFilter()
		seqs, err := c.appendAuthorizationFencedBatch(ctx, actorID, entries, authorizationSeq)
		if err == nil {
			last := len(entries) - 1
			if err := c.roomModel.waitForGroupLayout(ctx, events.SubjectPosition(entries[last].Subject, seqs[last])); err != nil {
				return fmt.Errorf("wait for room-group projection after sidebar placement: %w", err)
			}
			c.logger.Info("Moved sidebar item", "item_id", item.GetId(), "source_group_id", snapshot.SourceGroupID, "target_group_id", targetGroupID, "actor_id", actorID)
			c.notifyRoomLayoutChanged(ctx, actorID, "move_sidebar_item")
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return fmt.Errorf("publish cross-group sidebar item placement: %w", err)
		}
		if err := c.waitForGroupOCCRetry(ctx, attempt, "wait for room-group projection after cross-group sidebar placement conflict"); err != nil {
			return err
		}
	}
	return fmt.Errorf("move-sidebar-item OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func placeSidebarEntryBefore(current []*evtv1.SidebarGroupEntry, item, before *evtv1.SidebarGroupEntry, removeItem bool) ([]*evtv1.SidebarGroupEntry, bool, error) {
	itemKey := sidebarEntryKey(item)
	next := cloneSidebarEntries(current)
	if removeItem {
		next = slices.DeleteFunc(next, func(entry *evtv1.SidebarGroupEntry) bool {
			return sidebarEntryKey(entry) == itemKey
		})
	}
	insertAt := len(next)
	if before != nil {
		beforeKey := sidebarEntryKey(before)
		insertAt = slices.IndexFunc(next, func(entry *evtv1.SidebarGroupEntry) bool {
			return sidebarEntryKey(entry) == beforeKey
		})
		if insertAt < 0 {
			return nil, false, ErrSidebarItemPlacement
		}
	}
	next = slices.Insert(next, insertAt, &evtv1.SidebarGroupEntry{Kind: item.GetKind(), Id: item.GetId()})
	return next, !sameSidebarEntryOrder(current, next), nil
}

func sameSidebarEntryOrder(left, right []*evtv1.SidebarGroupEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if sidebarEntryKey(left[i]) != sidebarEntryKey(right[i]) {
			return false
		}
	}
	return true
}

func sidebarEntriesReorderedEvent(actorID, groupID string, entries []*evtv1.SidebarGroupEntry) *evtv1.Event {
	return newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_SidebarGroupEntriesReordered{
		SidebarGroupEntriesReordered: &evtv1.SidebarGroupEntriesReorderedEvent{
			GroupId: groupID,
			Entries: cloneSidebarEntries(entries),
		},
	}})
}

func sidebarItemMoveEvents(actorID string, snapshot SidebarItemPlacementSnapshot, item *evtv1.SidebarGroupEntry, targetGroupID string, targetEntries []*evtv1.SidebarGroupEntry) ([]evtstream.BatchEntry, error) {
	sourceAggregate := evtstream.GroupAggregate(snapshot.SourceGroupID)
	targetAggregate := evtstream.GroupAggregate(targetGroupID)
	var removed, added *evtv1.Event
	switch item.GetKind() {
	case evtv1.SidebarGroupEntry_ROOM:
		removed = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RoomRemovedFromGroup{
			RoomRemovedFromGroup: &evtv1.RoomRemovedFromGroupEvent{GroupId: snapshot.SourceGroupID, RoomId: item.GetId()},
		}})
		added = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RoomAddedToGroup{
			RoomAddedToGroup: &evtv1.RoomAddedToGroupEvent{GroupId: targetGroupID, RoomId: item.GetId()},
		}})
	case evtv1.SidebarGroupEntry_SIDEBAR_LINK:
		if snapshot.Link == nil {
			return nil, ErrSidebarItemNotFound
		}
		removed = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_SidebarLinkRemovedFromGroup{
			SidebarLinkRemovedFromGroup: &evtv1.SidebarLinkRemovedFromGroupEvent{GroupId: snapshot.SourceGroupID, LinkId: item.GetId()},
		}})
		added = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_SidebarLinkAddedToGroup{
			SidebarLinkAddedToGroup: &evtv1.SidebarLinkAddedToGroupEvent{
				GroupId: targetGroupID,
				LinkId:  snapshot.Link.GetId(),
				Label:   snapshot.Link.GetLabel(),
				Url:     snapshot.Link.GetUrl(),
			},
		}})
	default:
		return nil, fmt.Errorf("%w: unsupported item kind", ErrInvalidArgument)
	}
	reordered := sidebarEntriesReorderedEvent(actorID, targetGroupID, targetEntries)
	return []evtstream.BatchEntry{
		{Subject: sourceAggregate.SubjectFor(removed), Event: removed},
		{Subject: targetAggregate.SubjectFor(added), Event: added},
		{Subject: targetAggregate.SubjectFor(reordered), Event: reordered},
	}, nil
}

func (c *ChattoCore) reorderSidebarItemsInGroup(ctx context.Context, actorID, groupID string, orderedEntries []*evtv1.SidebarGroupEntry, authorize func() error) error {
	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		snapshot := c.roomModel.roomGroupSnapshot(groupID)
		if !snapshot.Exists {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before sidebar-item reorder: %w", err)
			}
			snapshot = c.roomModel.roomGroupSnapshot(groupID)
			if !snapshot.Exists {
				return ErrRoomGroupNotFound
			}
		}
		if !sameSidebarEntrySet(snapshot.Group.GetEntries(), orderedEntries) {
			return ErrRoomGroupOrderMismatch
		}
		event := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_SidebarGroupEntriesReordered{
				SidebarGroupEntriesReordered: &evtv1.SidebarGroupEntriesReorderedEvent{
					GroupId: groupID,
					Entries: cloneSidebarEntries(orderedEntries),
				},
			},
		})
		if _, err := c.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(groupID), event, snapshot.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := c.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after sidebar-item reorder OCC conflict"); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("publish SidebarGroupEntriesReorderedEvent: %w", err)
		}

		c.logger.Info("Reordered sidebar items in group", "group_id", groupID, "actor_id", actorID)
		c.notifyRoomLayoutChanged(ctx, actorID, "reorder_sidebar_items_in_group")
		return nil
	}
	return fmt.Errorf("reorder-sidebar-items OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func sameSidebarEntrySet(current, next []*evtv1.SidebarGroupEntry) bool {
	if len(current) != len(next) {
		return false
	}
	counts := make(map[string]int, len(current))
	for _, entry := range current {
		key := sidebarEntryKey(entry)
		if key == "" {
			return false
		}
		counts[key]++
	}
	for _, entry := range next {
		key := sidebarEntryKey(entry)
		if key == "" {
			return false
		}
		counts[key]--
		if counts[key] < 0 {
			return false
		}
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func sidebarEntryKey(entry *evtv1.SidebarGroupEntry) string {
	if entry == nil || entry.GetId() == "" {
		return ""
	}
	switch entry.GetKind() {
	case evtv1.SidebarGroupEntry_ROOM:
		return "room:" + entry.GetId()
	case evtv1.SidebarGroupEntry_SIDEBAR_LINK:
		return "link:" + entry.GetId()
	default:
		return ""
	}
}

// ListRoomGroupsOrdered returns the layout-ordered list of channel
// groups, dropping stale references and appending orphans (groups
// present in the catalog but missing from the layout) at the end by
// NanoID order so the sidebar self-heals on read.
//
// `kind` is preserved on the signature for symmetry with other room
// APIs; only KindChannel participates in the layout today.
func (c *ChattoCore) ListRoomGroupsOrdered(_ context.Context, kind RoomKind) ([]*evtv1.RoomGroup, error) {
	if kind != KindChannel {
		return nil, nil
	}

	order := c.roomModel.roomLayoutOrder()
	all := c.roomModel.roomGroups()
	docs := make(map[string]*evtv1.RoomGroup, len(all))
	for _, g := range all {
		docs[g.Id] = g
	}

	out := make([]*evtv1.RoomGroup, 0, len(docs))
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
// RoomModel's layout state. May include IDs of groups that have since
// been deleted; use ListRoomGroupsOrdered for the reconciled view.
func (c *ChattoCore) GetRoomLayoutOrder(_ context.Context) ([]string, error) {
	return c.roomModel.roomLayoutOrder(), nil
}

// ----------------------------------------------------------------------
// Layout-ordering helpers
// ----------------------------------------------------------------------

// publishLayoutOrdering writes a RoomGroupsReorderedEvent on the
// singleton layout aggregate and waits for the group/layout projection.
func (c *ChattoCore) publishLayoutOrdering(ctx context.Context, actorID string, groupIDs []string, authorize func() error) error {
	event := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomGroupsReordered{
			RoomGroupsReordered: &evtv1.RoomGroupsReorderedEvent{
				GroupIds: slices.Clone(groupIDs),
			},
		},
	})
	if _, err := c.appendGroupLayoutMutation(ctx, evtstream.LayoutAggregate(), event, authorize); err != nil {
		return fmt.Errorf("publish RoomGroupsReorderedEvent: %w", err)
	}
	return nil
}

// appendGroupToLayout appends groupID to the current layout ordering
// if not already present, then publishes the new ordering.
func (c *ChattoCore) appendGroupToLayout(ctx context.Context, actorID, groupID string) error {
	current := c.roomModel.roomLayoutOrder()
	if slices.Contains(current, groupID) {
		return nil
	}
	return c.publishLayoutOrdering(ctx, actorID, append(current, groupID), nil)
}

// removeGroupFromLayout removes groupID from the current layout
// ordering and republishes if it was present.
func (c *ChattoCore) removeGroupFromLayout(ctx context.Context, actorID, groupID string) error {
	current := c.roomModel.roomLayoutOrder()
	if !slices.Contains(current, groupID) {
		return nil
	}
	next := slices.DeleteFunc(current, func(id string) bool { return id == groupID })
	return c.publishLayoutOrdering(ctx, actorID, next, nil)
}

// notifyRoomLayoutChanged is the central place every room-layout mutator calls
// to nudge connected clients. It waits for the group-layout projection before
// it publishes because realtime handlers build their replacement payload from
// that projection. Without this read-your-writes fence, a fast live event can
// replace a client's current sidebar with stale data.
//
// This notification remains best-effort: a wait or publish failure does not
// roll back the durable mutation that preceded it. reason is only for log
// forensics.
func (c *ChattoCore) notifyRoomLayoutChanged(ctx context.Context, actorID, reason string) {
	if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
		c.logger.Warn("Failed to wait for room layout before publishing update event",
			"error", err, "actor_id", actorID, "reason", reason)
		return
	}
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
