package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
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
func (m *RoomLayoutModel) CreateRoomGroup(ctx context.Context, actorID, name, description string) (*readmodel.RoomGroup, error) {
	return m.createRoomGroup(ctx, actorID, name, description, nil)
}

func (m *RoomLayoutModel) createRoomGroup(ctx context.Context, actorID, name, description string, authorize func() error) (*readmodel.RoomGroup, error) {
	name = strings.TrimSpace(name)
	if err := validateRoomGroupMetadata(name, description); err != nil {
		return nil, err
	}

	group := &readmodel.RoomGroup{
		Id:          NewRoomGroupID(),
		Name:        name,
		Description: description,
	}

	for attempt := range maxMoveRoomToGroupRetries {
		groupPosition, layoutPosition, err := m.currentGroupLayoutPositions(ctx)
		if err != nil {
			return nil, fmt.Errorf("prepare room-group creation: %w", err)
		}
		groups, err := m.ListRoomGroupsOrdered(ctx, KindChannel)
		if err != nil {
			return nil, err
		}
		orderedGroupIDs := make([]string, 0, len(groups)+1)
		for _, current := range groups {
			orderedGroupIDs = append(orderedGroupIDs, current.GetId())
		}
		orderedGroupIDs = append(orderedGroupIDs, group.GetId())

		if err := m.policy.authorizeAtStableInputs(ctx, authorize); err != nil {
			return nil, err
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
		orderedEvent := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_RoomGroupsReordered{
				RoomGroupsReordered: &evtv1.RoomGroupsReorderedEvent{GroupIds: orderedGroupIDs},
			},
		})
		entries := []evtstream.BatchEntry{
			{
				Subject: evtstream.GroupAggregate(group.Id).SubjectFor(createdEvent),
				Event:   createdEvent,
				Expect:  events.ExpectFilterSeq(evtstream.GroupSubjectFilter(), groupPosition.Seq),
			},
			{
				Subject: evtstream.LayoutAggregate().SubjectFor(orderedEvent),
				Event:   orderedEvent,
				Expect:  events.ExpectFilterSeq(evtstream.LayoutSubjectFilter(), layoutPosition.Seq),
			},
		}
		seqs, err := m.publisher.AppendBatch(ctx, entries)
		if err == nil {
			if err := m.rooms.waitForGroupLayout(ctx, events.SubjectPosition(entries[1].Subject, seqs[1])); err != nil {
				return nil, fmt.Errorf("wait for created room group and layout: %w", err)
			}
			m.logger.Info("Created room group", "group_id", group.Id, "name", name, "actor_id", actorID)
			created, ok := m.rooms.roomGroup(group.GetId())
			if !ok {
				return nil, ErrRoomGroupNotFound
			}
			return created, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return nil, fmt.Errorf("publish atomic room-group creation: %w", err)
		}
		if err := m.waitForGroupLayoutRetry(ctx, attempt, "wait for room-group layout after creation OCC conflict"); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("create-room-group OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

// UpdateRoomGroup publishes a RoomGroupUpdatedEvent. Layout ordering
// is untouched; only metadata changes.
func (m *RoomLayoutModel) UpdateRoomGroup(ctx context.Context, actorID, groupID, name, description string) (*readmodel.RoomGroup, error) {
	return m.updateRoomGroupFields(ctx, actorID, groupID, &name, &description, nil)
}

// UpdateRoomGroupFields applies a sparse metadata patch. Each OCC retry
// recomposes omitted fields from the latest group projection so a stale client
// cannot overwrite a concurrent update to a field it did not submit.
func (m *RoomLayoutModel) UpdateRoomGroupFields(ctx context.Context, actorID, groupID string, name, description *string) (*readmodel.RoomGroup, error) {
	return m.updateRoomGroupFields(ctx, actorID, groupID, name, description, nil)
}

func (m *RoomLayoutModel) updateRoomGroupFields(ctx context.Context, actorID, groupID string, name, description *string, authorize func() error) (*readmodel.RoomGroup, error) {
	if name == nil && description == nil {
		return nil, fmt.Errorf("%w: provide at least one room group field to update", ErrInvalidArgument)
	}
	for attempt := range maxMoveRoomToGroupRetries {
		snapshot := m.rooms.roomGroupSnapshot(groupID)
		if !snapshot.Exists {
			if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
				return nil, fmt.Errorf("wait for room group layout projection before update: %w", err)
			}
			snapshot = m.rooms.roomGroupSnapshot(groupID)
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
		if _, err := m.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(groupID), updatedEvent, snapshot.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := m.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after update OCC conflict"); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("publish RoomGroupUpdatedEvent: %w", err)
		}

		m.logger.Info("Updated room group", "group_id", groupID, "name", nextName, "actor_id", actorID)
		updated, _ := m.rooms.roomGroup(groupID)
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
func (m *RoomLayoutModel) GetRoomGroup(ctx context.Context, groupID string) (*readmodel.RoomGroup, error) {
	return m.rooms.GetRoomGroup(ctx, groupID)
}

func (m *RoomLayoutModel) sidebarLinkGroup(ctx context.Context, linkID string) (string, error) {
	groupID := m.rooms.roomGroupForSidebarLink(linkID)
	if groupID != "" {
		return groupID, nil
	}
	if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
		return "", fmt.Errorf("wait for room group layout projection before sidebar-link lookup: %w", err)
	}
	groupID = m.rooms.roomGroupForSidebarLink(linkID)
	if groupID == "" {
		return "", ErrSidebarLinkNotFound
	}
	return groupID, nil
}

func sidebarLinkFromGroup(group *readmodel.RoomGroup, linkID string) *readmodel.SidebarLink {
	if group == nil {
		return nil
	}
	for _, link := range group.GetSidebarLinks() {
		if link.GetId() == linkID {
			return link.Clone()
		}
	}
	return nil
}

func (m *RoomLayoutModel) appendGroupLayoutAtFilter(ctx context.Context, agg evtstream.Aggregate, event *evtv1.Event, expectedSeq uint64, check func() error) (events.StreamPosition, error) {
	if err := m.policy.authorizeAtStableInputs(ctx, check); err != nil {
		return events.StreamPosition{}, err
	}
	subject := agg.SubjectFor(event)
	filter := evtstream.GroupSubjectFilter()
	if agg.Type == evtstream.AggregateLayout {
		filter = evtstream.LayoutSubjectFilter()
	}
	entries := []evtstream.BatchEntry{{
		Subject: subject,
		Event:   event,
		Expect:  events.ExpectFilterSeq(filter, expectedSeq),
	}}
	seqs, err := m.publisher.AppendBatch(ctx, entries)
	if err != nil {
		return events.StreamPosition{}, err
	}
	seq := seqs[0]
	pos := events.SubjectPosition(subject, seq)
	if err := m.rooms.waitForGroupLayout(ctx, pos); err != nil {
		return pos, err
	}
	return pos, nil
}

func (m *RoomLayoutModel) appendGroupLayoutMutation(ctx context.Context, agg evtstream.Aggregate, event *evtv1.Event, check func() error) (events.StreamPosition, error) {
	filter := evtstream.GroupSubjectFilter()
	if agg.Type == evtstream.AggregateLayout {
		filter = evtstream.LayoutSubjectFilter()
	}
	for attempt := range maxMoveRoomToGroupRetries {
		position, err := m.publisher.LastSubjectPosition(ctx, filter)
		if err != nil {
			return events.StreamPosition{}, fmt.Errorf("read room-group OCC position: %w", err)
		}
		if err := m.rooms.waitForGroupLayout(ctx, position); err != nil {
			return events.StreamPosition{}, fmt.Errorf("wait for room-group projection: %w", err)
		}
		committed, err := m.appendGroupLayoutAtFilter(ctx, agg, event, position.Seq, check)
		if err == nil {
			return committed, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return events.StreamPosition{}, err
		}
		if err := m.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after authorization OCC conflict"); err != nil {
			return events.StreamPosition{}, err
		}
	}
	return events.StreamPosition{}, fmt.Errorf("room-group authorization OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func (m *RoomLayoutModel) roomGroupAuthorityCheck(ctx context.Context, actorID string, groupIDs ...string) func() error {
	return func() error {
		if actorID == SystemActorID {
			return nil
		}
		for _, groupID := range groupIDs {
			if err := m.policy.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
				return err
			}
		}
		return nil
	}
}

func (m *RoomLayoutModel) anyRoomAuthorityCheck(ctx context.Context, actorID string) func() error {
	return func() error {
		if actorID == SystemActorID {
			return nil
		}
		return m.policy.requireCanManageAnyRoom(ctx, actorID)
	}
}

func (m *RoomLayoutModel) waitForGroupOCCRetry(ctx context.Context, attempt int, message string) error {
	if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		return nil
	}
}

// currentGroupLayoutPositions returns projection-aligned OCC positions for the
// group and singleton layout lanes. A caller that derives one batch from both
// read models must guard both positions when it commits.
func (m *RoomLayoutModel) currentGroupLayoutPositions(ctx context.Context) (events.StreamPosition, events.StreamPosition, error) {
	groupPosition, err := m.publisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
	if err != nil {
		return events.StreamPosition{}, events.StreamPosition{}, fmt.Errorf("read room-group OCC position: %w", err)
	}
	layoutPosition, err := m.publisher.LastSubjectPosition(ctx, evtstream.LayoutSubjectFilter())
	if err != nil {
		return events.StreamPosition{}, events.StreamPosition{}, fmt.Errorf("read room-layout OCC position: %w", err)
	}
	if !groupPosition.IsZero() {
		if err := m.rooms.waitForGroupLayout(ctx, groupPosition); err != nil {
			return events.StreamPosition{}, events.StreamPosition{}, fmt.Errorf("wait for room-group projection: %w", err)
		}
	}
	if !layoutPosition.IsZero() {
		if err := m.rooms.waitForGroupLayout(ctx, layoutPosition); err != nil {
			return events.StreamPosition{}, events.StreamPosition{}, fmt.Errorf("wait for room-layout projection: %w", err)
		}
	}
	return groupPosition, layoutPosition, nil
}

func (m *RoomLayoutModel) waitForGroupLayoutRetry(ctx context.Context, attempt int, message string) error {
	if _, _, err := m.currentGroupLayoutPositions(ctx); err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		return nil
	}
}

// DeleteRoomGroup removes a group via RoomGroupDeletedEvent. Fails with
// ErrRoomGroupHasRooms if the group still contains any rooms or sidebar links.
// The group tombstone and resulting RoomGroupsReorderedEvent commit atomically.
func (m *RoomLayoutModel) DeleteRoomGroup(ctx context.Context, actorID, groupID string) error {
	return m.deleteRoomGroup(ctx, actorID, groupID, nil)
}

func (m *RoomLayoutModel) deleteRoomGroup(ctx context.Context, actorID, groupID string, authorize func() error) error {
	for attempt := range maxMoveRoomToGroupRetries {
		groupPosition, layoutPosition, err := m.currentGroupLayoutPositions(ctx)
		if err != nil {
			return fmt.Errorf("prepare room-group deletion: %w", err)
		}
		snapshot := m.rooms.roomGroupSnapshot(groupID)
		if !snapshot.Exists {
			if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before delete: %w", err)
			}
			snapshot = m.rooms.roomGroupSnapshot(groupID)
			if !snapshot.Exists {
				return ErrRoomGroupNotFound
			}
		}
		if len(snapshot.Group.GetRoomIds()) > 0 || len(snapshot.Group.GetSidebarLinks()) > 0 {
			return ErrRoomGroupHasRooms
		}

		groups, err := m.ListRoomGroupsOrdered(ctx, KindChannel)
		if err != nil {
			return err
		}
		orderedGroupIDs := make([]string, 0, len(groups)-1)
		for _, group := range groups {
			if group.GetId() != groupID {
				orderedGroupIDs = append(orderedGroupIDs, group.GetId())
			}
		}
		if err := m.policy.authorizeAtStableInputs(ctx, authorize); err != nil {
			return err
		}
		deletedEvent := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_RoomGroupDeleted{
				RoomGroupDeleted: &evtv1.RoomGroupDeletedEvent{
					GroupId: groupID,
				},
			},
		})
		orderedEvent := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_RoomGroupsReordered{
				RoomGroupsReordered: &evtv1.RoomGroupsReorderedEvent{GroupIds: orderedGroupIDs},
			},
		})
		entries := []evtstream.BatchEntry{
			{
				Subject: evtstream.GroupAggregate(groupID).SubjectFor(deletedEvent),
				Event:   deletedEvent,
				Expect:  events.ExpectFilterSeq(evtstream.GroupSubjectFilter(), groupPosition.Seq),
			},
			{
				Subject: evtstream.LayoutAggregate().SubjectFor(orderedEvent),
				Event:   orderedEvent,
				Expect:  events.ExpectFilterSeq(evtstream.LayoutSubjectFilter(), layoutPosition.Seq),
			},
		}
		seqs, err := m.publisher.AppendBatch(ctx, entries)
		if err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := m.waitForGroupLayoutRetry(ctx, attempt, "wait for room-group layout after deletion OCC conflict"); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("publish atomic room-group deletion: %w", err)
		}
		if err := m.rooms.waitForGroupLayout(ctx, events.SubjectPosition(entries[1].Subject, seqs[1])); err != nil {
			return fmt.Errorf("wait for deleted room group and layout: %w", err)
		}

		m.logger.Info("Deleted room group", "group_id", groupID, "actor_id", actorID)
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
func (m *RoomLayoutModel) MoveRoomToGroup(ctx context.Context, actorID, roomID, targetGroupID string) error {
	return m.moveRoomToGroup(ctx, actorID, roomID, "", targetGroupID, false, nil)
}

// MoveRoomToGroupFromSource moves a room only if the room's current source
// group still matches sourceGroupID. API callers that authorize the source
// group before calling core should use this variant so a concurrent move
// cannot swap the source group between authorization and append.
func (m *RoomLayoutModel) MoveRoomToGroupFromSource(ctx context.Context, actorID, roomID, sourceGroupID, targetGroupID string) error {
	return m.moveRoomToGroup(ctx, actorID, roomID, sourceGroupID, targetGroupID, true, nil)
}

func (m *RoomLayoutModel) moveRoomToGroup(ctx context.Context, actorID, roomID, authorizedSourceGroupID, targetGroupID string, bindSource bool, authorize func(sourceGroupID, targetGroupID string) error) error {
	occFilter := evtstream.GroupSubjectFilter()
	for attempt := range maxMoveRoomToGroupRetries {
		roomDeletedSubject := evtstream.RoomAggregate(roomID).Subject(evtstream.EventRoomDeleted)
		roomDeletedPosition, err := m.publisher.LastSubjectPosition(ctx, roomDeletedSubject)
		if err != nil {
			return fmt.Errorf("read room deletion boundary before move: %w", err)
		}
		if !roomDeletedPosition.IsZero() {
			return fmt.Errorf("room %s: %w", roomID, ErrNotFound)
		}

		snapshot := m.rooms.roomGroupMoveSnapshot(roomID, targetGroupID)
		if !snapshot.TargetExists {
			if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before not-found decision: %w", err)
			}
			snapshot = m.rooms.roomGroupMoveSnapshot(roomID, targetGroupID)
			if !snapshot.TargetExists {
				return ErrRoomGroupNotFound
			}
		}

		sourceGroupID := snapshot.SourceGroupID
		if bindSource && sourceGroupID != authorizedSourceGroupID {
			return ErrRoomMoveSourceChanged
		}
		if sourceGroupID == targetGroupID {
			if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before no-op decision: %w", err)
			}
			snapshot = m.rooms.roomGroupMoveSnapshot(roomID, targetGroupID)
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
		var authorizationCheck func() error
		if authorize != nil {
			authorizationCheck = func() error { return authorize(sourceGroupID, targetGroupID) }
		}
		if err := m.policy.authorizeAtStableInputs(ctx, authorizationCheck); err != nil {
			return err
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
				Subject: sourceAgg.SubjectFor(removed),
				Event:   removed,
				Expect:  events.ExpectFilterSeq(occFilter, snapshot.Seq),
			})
		}
		targetAgg := evtstream.GroupAggregate(targetGroupID)
		entries = append(entries, evtstream.BatchEntry{
			Subject: targetAgg.SubjectFor(added),
			Event:   added,
			Expect:  events.ExpectFilterSeq(roomDeletedSubject, roomDeletedPosition.Seq),
		})
		if sourceGroupID == "" {
			// A legacy unassigned room has no removal event that can carry the
			// group OCC guard. Guard the full stream on the only group event so
			// both room deletion and group placement stay stable until commit.
			streamSeq, err := m.publisher.LastStreamSeq(ctx)
			if err != nil {
				return fmt.Errorf("read stream OCC tail for unassigned room move: %w", err)
			}
			currentDeletedPosition, err := m.publisher.LastSubjectPosition(ctx, roomDeletedSubject)
			if err != nil {
				return fmt.Errorf("re-read room deletion boundary for unassigned room move: %w", err)
			}
			if !currentDeletedPosition.IsZero() {
				return fmt.Errorf("room %s: %w", roomID, ErrNotFound)
			}
			entries[0].Expect = events.ExpectFilterSeq(occFilter, snapshot.Seq).AndStreamSeq(streamSeq)
		}

		seqs, err := m.publisher.AppendBatch(ctx, entries)
		if err == nil {
			// Wait on the final sequence. The projector applies in stream order,
			// so reaching the last batch entry means that it applied every earlier
			// entry. The command returns only after the local read model is current.
			lastDomainIndex := len(entries) - 1
			lastSubject := entries[lastDomainIndex].Subject
			if err := m.rooms.waitForGroupLayout(ctx, events.SubjectPosition(lastSubject, seqs[lastDomainIndex])); err != nil {
				return fmt.Errorf("wait for room group layout projection: %w", err)
			}

			m.logger.Info("Moved room to group", "room_id", roomID, "group_id", targetGroupID, "actor_id", actorID)
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return fmt.Errorf("publish move-room batch: %w", err)
		}

		if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
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
func (m *RoomLayoutModel) ReorderRoomGroups(ctx context.Context, actorID string, orderedGroupIDs []string) error {
	return m.reorderRoomGroups(ctx, actorID, orderedGroupIDs, nil)
}

func (m *RoomLayoutModel) reorderRoomGroups(ctx context.Context, actorID string, orderedGroupIDs []string, authorize func() error) error {
	current := m.rooms.roomGroups()
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

	if err := m.publishLayoutOrdering(ctx, actorID, orderedGroupIDs, authorize); err != nil {
		return err
	}

	m.logger.Info("Reordered room groups", "order", orderedGroupIDs, "actor_id", actorID)
	return nil
}

// placeRoomGroup applies relative group-placement intent against the current
// authoritative order on every OCC retry. An empty beforeGroupID places the
// group last.
func (m *RoomLayoutModel) placeRoomGroup(ctx context.Context, actorID, groupID, beforeGroupID string, authorize func() error) error {
	for attempt := range maxMoveRoomToGroupRetries {
		position, err := m.publisher.LastSubjectPosition(ctx, evtstream.LayoutSubjectFilter())
		if err != nil {
			return fmt.Errorf("read room-layout OCC position: %w", err)
		}
		if err := m.rooms.waitForGroupLayout(ctx, position); err != nil {
			return fmt.Errorf("wait for room-layout projection: %w", err)
		}
		groups, err := m.ListRoomGroupsOrdered(ctx, KindChannel)
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
		if _, err := m.appendGroupLayoutAtFilter(ctx, evtstream.LayoutAggregate(), event, position.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := m.waitForGroupOCCRetry(ctx, attempt, "wait for room layout after relative placement conflict"); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("publish relative room-group placement: %w", err)
		}
		m.logger.Info("Moved room group", "group_id", groupID, "actor_id", actorID)
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
func (m *RoomLayoutModel) ReorderRoomsInGroup(ctx context.Context, actorID, groupID string, orderedRoomIDs []string) error {
	g, ok := m.rooms.roomGroup(groupID)
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
	if _, err := m.appendGroupLayoutMutation(ctx, evtstream.GroupAggregate(groupID), reorderedEvent, nil); err != nil {
		return fmt.Errorf("publish RoomsInGroupReorderedEvent: %w", err)
	}

	m.logger.Info("Reordered rooms in group", "group_id", groupID, "actor_id", actorID)
	return nil
}

func (m *RoomLayoutModel) CreateSidebarLink(ctx context.Context, actorID, groupID, label, rawURL string) (*readmodel.SidebarLink, error) {
	return m.createSidebarLink(ctx, actorID, groupID, label, rawURL, nil)
}

func (m *RoomLayoutModel) createSidebarLink(ctx context.Context, actorID, groupID, label, rawURL string, authorize func() error) (*readmodel.SidebarLink, error) {
	label, rawURL, err := validateSidebarLink(label, rawURL)
	if err != nil {
		return nil, err
	}
	link := &readmodel.SidebarLink{
		Id:    NewSidebarLinkID(),
		Label: label,
		Url:   rawURL,
	}
	for attempt := range maxMoveRoomToGroupRetries {
		snapshot := m.rooms.roomGroupSnapshot(groupID)
		if !snapshot.Exists {
			if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
				return nil, fmt.Errorf("wait for room group layout projection before sidebar-link create: %w", err)
			}
			snapshot = m.rooms.roomGroupSnapshot(groupID)
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
		if _, err := m.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(groupID), event, snapshot.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := m.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after sidebar-link create OCC conflict"); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("publish SidebarLinkAddedToGroupEvent: %w", err)
		}

		m.logger.Info("Created sidebar link", "group_id", groupID, "link_id", link.Id, "actor_id", actorID)
		return link, nil
	}
	return nil, fmt.Errorf("create-sidebar-link OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func (m *RoomLayoutModel) UpdateSidebarLink(ctx context.Context, actorID, linkID, label, rawURL string) (*readmodel.SidebarLink, error) {
	groupID, err := m.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return nil, err
	}
	return m.UpdateSidebarLinkInGroup(ctx, actorID, groupID, linkID, label, rawURL)
}

func (m *RoomLayoutModel) UpdateSidebarLinkInGroup(ctx context.Context, actorID, groupID, linkID, label, rawURL string) (*readmodel.SidebarLink, error) {
	return m.updateSidebarLinkInGroup(ctx, actorID, groupID, linkID, label, rawURL, nil)
}

func (m *RoomLayoutModel) updateSidebarLinkInGroup(ctx context.Context, actorID, groupID, linkID, label, rawURL string, authorize func() error) (*readmodel.SidebarLink, error) {
	label, rawURL, err := validateSidebarLink(label, rawURL)
	if err != nil {
		return nil, err
	}
	for attempt := range maxMoveRoomToGroupRetries {
		snapshot := m.rooms.roomGroupSnapshot(groupID)
		if !snapshot.Exists {
			if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
				return nil, fmt.Errorf("wait for room group layout projection before sidebar-link update: %w", err)
			}
			snapshot = m.rooms.roomGroupSnapshot(groupID)
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
		if _, err := m.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(groupID), event, snapshot.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := m.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after sidebar-link update OCC conflict"); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("publish SidebarLinkUpdatedEvent: %w", err)
		}

		m.logger.Info("Updated sidebar link", "group_id", groupID, "link_id", linkID, "actor_id", actorID)
		return &readmodel.SidebarLink{Id: linkID, Label: label, Url: rawURL}, nil
	}
	return nil, fmt.Errorf("update-sidebar-link OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func (m *RoomLayoutModel) DeleteSidebarLink(ctx context.Context, actorID, linkID string) error {
	groupID, err := m.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return err
	}
	return m.DeleteSidebarLinkInGroup(ctx, actorID, groupID, linkID)
}

func (m *RoomLayoutModel) DeleteSidebarLinkInGroup(ctx context.Context, actorID, groupID, linkID string) error {
	return m.deleteSidebarLinkInGroup(ctx, actorID, groupID, linkID, nil)
}

func (m *RoomLayoutModel) deleteSidebarLinkInGroup(ctx context.Context, actorID, groupID, linkID string, authorize func() error) error {
	for attempt := range maxMoveRoomToGroupRetries {
		snapshot := m.rooms.roomGroupSnapshot(groupID)
		if !snapshot.Exists {
			if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before sidebar-link delete: %w", err)
			}
			snapshot = m.rooms.roomGroupSnapshot(groupID)
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
		if _, err := m.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(groupID), event, snapshot.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := m.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after sidebar-link delete OCC conflict"); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("publish SidebarLinkRemovedFromGroupEvent: %w", err)
		}

		m.logger.Info("Deleted sidebar link", "group_id", groupID, "link_id", linkID, "actor_id", actorID)
		return nil
	}
	return fmt.Errorf("delete-sidebar-link OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func (m *RoomLayoutModel) MoveSidebarLinkToGroup(ctx context.Context, actorID, linkID, targetGroupID string) error {
	sourceGroupID, err := m.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return err
	}
	return m.MoveSidebarLinkBetweenGroups(ctx, actorID, linkID, sourceGroupID, targetGroupID)
}

func (m *RoomLayoutModel) MoveSidebarLinkBetweenGroups(ctx context.Context, actorID, linkID, sourceGroupID, targetGroupID string) error {
	return m.moveSidebarLinkBetweenGroups(ctx, actorID, linkID, sourceGroupID, targetGroupID, nil)
}

func (m *RoomLayoutModel) moveSidebarLinkBetweenGroups(ctx context.Context, actorID, linkID, sourceGroupID, targetGroupID string, authorize func(sourceGroupID, targetGroupID string) error) error {
	for attempt := range maxMoveRoomToGroupRetries {
		snapshot := m.rooms.sidebarLinkMoveSnapshot(linkID, targetGroupID)
		if !snapshot.TargetExists {
			if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before sidebar-link move target decision: %w", err)
			}
			snapshot = m.rooms.sidebarLinkMoveSnapshot(linkID, targetGroupID)
			if !snapshot.TargetExists {
				return ErrRoomGroupNotFound
			}
		}
		if snapshot.SourceGroupID == "" || snapshot.Link == nil {
			if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before sidebar-link move source decision: %w", err)
			}
			snapshot = m.rooms.sidebarLinkMoveSnapshot(linkID, targetGroupID)
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
			if err := m.policy.authorizeAtStableInputs(ctx, func() error {
				return authorize(snapshot.SourceGroupID, targetGroupID)
			}); err != nil {
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
				Subject: evtstream.GroupAggregate(snapshot.SourceGroupID).SubjectFor(removed),
				Event:   removed,
				Expect:  events.ExpectFilterSeq(evtstream.GroupSubjectFilter(), snapshot.Seq),
			},
			{
				Subject: evtstream.GroupAggregate(targetGroupID).SubjectFor(added),
				Event:   added,
			},
		}
		seqs, err := m.publisher.AppendBatch(ctx, entries)
		if err == nil {
			lastDomainIndex := len(entries) - 1
			lastSubject := entries[lastDomainIndex].Subject
			if err := m.rooms.waitForGroupLayout(ctx, events.SubjectPosition(lastSubject, seqs[lastDomainIndex])); err != nil {
				return fmt.Errorf("wait for room group layout projection: %w", err)
			}
			m.logger.Info("Moved sidebar link", "link_id", linkID, "source_group_id", snapshot.SourceGroupID, "target_group_id", targetGroupID, "actor_id", actorID)
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return fmt.Errorf("publish sidebar-link move batch: %w", err)
		}
		if err := m.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after sidebar-link move OCC conflict"); err != nil {
			return err
		}
	}
	return fmt.Errorf("move-sidebar-link OCC retry exhausted after %d attempts: %w", maxMoveRoomToGroupRetries, events.ErrConflict)
}

func (m *RoomLayoutModel) ReorderSidebarItemsInGroup(ctx context.Context, actorID, groupID string, orderedEntries []*evtv1.SidebarGroupEntry) error {
	return m.reorderSidebarItemsInGroup(ctx, actorID, groupID, orderedEntries, nil)
}

// placeSidebarItem applies one relative room or link placement against a
// consistent room-group projection snapshot. An absent before entry appends to
// the destination. Cross-group moves and the target reorder commit atomically.
func (m *RoomLayoutModel) placeSidebarItem(
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

	for attempt := range maxMoveRoomToGroupRetries {
		snapshot := m.rooms.sidebarItemPlacementSnapshot(item, targetGroupID)
		if snapshot.TargetGroup == nil || snapshot.SourceGroup == nil {
			if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
				return fmt.Errorf("wait for room-group projection before sidebar placement: %w", err)
			}
			snapshot = m.rooms.sidebarItemPlacementSnapshot(item, targetGroupID)
		}
		if snapshot.TargetGroup == nil {
			return ErrRoomGroupNotFound
		}
		if snapshot.SourceGroup == nil {
			return ErrSidebarItemNotFound
		}
		if err := m.policy.authorizeAtStableInputs(ctx, func() error {
			return authorize(snapshot.SourceGroupID, targetGroupID)
		}); err != nil {
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
			if _, err := m.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(targetGroupID), event, snapshot.Seq, check); err != nil {
				if errors.Is(err, events.ErrConflict) {
					if err := m.waitForGroupOCCRetry(ctx, attempt, "wait for room-group projection after sidebar placement conflict"); err != nil {
						return err
					}
					continue
				}
				return fmt.Errorf("publish sidebar item placement: %w", err)
			}
			return nil
		}

		entries, err := sidebarItemMoveEvents(actorID, snapshot, item, targetGroupID, targetEntries)
		if err != nil {
			return err
		}
		entries[0].Expect = events.ExpectFilterSeq(evtstream.GroupSubjectFilter(), snapshot.Seq)
		seqs, err := m.publisher.AppendBatch(ctx, entries)
		if err == nil {
			last := len(entries) - 1
			if err := m.rooms.waitForGroupLayout(ctx, events.SubjectPosition(entries[last].Subject, seqs[last])); err != nil {
				return fmt.Errorf("wait for room-group projection after sidebar placement: %w", err)
			}
			m.logger.Info("Moved sidebar item", "item_id", item.GetId(), "source_group_id", snapshot.SourceGroupID, "target_group_id", targetGroupID, "actor_id", actorID)
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return fmt.Errorf("publish cross-group sidebar item placement: %w", err)
		}
		if err := m.waitForGroupOCCRetry(ctx, attempt, "wait for room-group projection after cross-group sidebar placement conflict"); err != nil {
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

func (m *RoomLayoutModel) reorderSidebarItemsInGroup(ctx context.Context, actorID, groupID string, orderedEntries []*evtv1.SidebarGroupEntry, authorize func() error) error {
	for attempt := range maxMoveRoomToGroupRetries {
		snapshot := m.rooms.roomGroupSnapshot(groupID)
		if !snapshot.Exists {
			if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
				return fmt.Errorf("wait for room group layout projection before sidebar-item reorder: %w", err)
			}
			snapshot = m.rooms.roomGroupSnapshot(groupID)
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
		if _, err := m.appendGroupLayoutAtFilter(ctx, evtstream.GroupAggregate(groupID), event, snapshot.Seq, authorize); err != nil {
			if errors.Is(err, events.ErrConflict) {
				if err := m.waitForGroupOCCRetry(ctx, attempt, "wait for room group layout projection after sidebar-item reorder OCC conflict"); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("publish SidebarGroupEntriesReorderedEvent: %w", err)
		}

		m.logger.Info("Reordered sidebar items in group", "group_id", groupID, "actor_id", actorID)
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
func (m *RoomLayoutModel) ListRoomGroupsOrdered(ctx context.Context, kind RoomKind) ([]*readmodel.RoomGroup, error) {
	return m.rooms.ListRoomGroupsOrdered(ctx, kind)
}

// ----------------------------------------------------------------------
// Layout-ordering helpers
// ----------------------------------------------------------------------

// publishLayoutOrdering writes a RoomGroupsReorderedEvent on the
// singleton layout aggregate and waits for the group/layout projection.
func (m *RoomLayoutModel) publishLayoutOrdering(ctx context.Context, actorID string, groupIDs []string, authorize func() error) error {
	event := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomGroupsReordered{
			RoomGroupsReordered: &evtv1.RoomGroupsReorderedEvent{
				GroupIds: slices.Clone(groupIDs),
			},
		},
	})
	if _, err := m.appendGroupLayoutMutation(ctx, evtstream.LayoutAggregate(), event, authorize); err != nil {
		return fmt.Errorf("publish RoomGroupsReorderedEvent: %w", err)
	}
	return nil
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
func (m *RoomLayoutModel) ensureChannelRoomsAreInAGroup(ctx context.Context) error {
	rooms, err := m.rooms.ListRooms(ctx, KindChannel)
	if err != nil {
		return fmt.Errorf("list channel rooms: %w", err)
	}
	groups, err := m.ListRoomGroupsOrdered(ctx, KindChannel)
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
		seed, err := m.CreateRoomGroup(ctx, SystemActorID, SeedDefaultRoomGroupName, "")
		if err != nil {
			return fmt.Errorf("seed default room group: %w", err)
		}
		targetGroupID = seed.Id
		m.logger.Info("Seeded default room group", "group_id", seed.Id, "name", SeedDefaultRoomGroupName)
	}

	for _, rid := range unassigned {
		if err := m.MoveRoomToGroup(ctx, SystemActorID, rid, targetGroupID); err != nil {
			return fmt.Errorf("move room %s to default group: %w", rid, err)
		}
	}
	return nil
}
