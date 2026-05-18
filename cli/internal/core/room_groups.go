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

// room_groups.go is the storage and API surface for channel-room groups
// (see ADR-031). Each group is its own KV document under
// `room_group.{id}`; a single `room_layout` document holds the
// authoritative *ordering* as a list of group IDs. The split keeps
// per-group property edits (name, description, future visual fields)
// from contending with each other or with reorder operations.
//
// Read path: `ListRoomGroupsOrdered` is the canonical accessor. It runs
// the legacy-shape migrator if needed, then reconciles the layout's
// `group_ids` against the set of actual `room_group.*` documents. Stale
// references (layout entries with no matching doc) are dropped. Orphan
// docs (groups missing from the layout) are appended at the end so a
// corrupted layout self-heals on read.
//
// Authorization is enforced at the API boundary; these methods assume
// the caller is authorized.

// Errors specific to room-group operations.
var (
	// ErrRoomGroupNotFound is returned when a group ID doesn't match any existing group.
	ErrRoomGroupNotFound = errors.New("room group not found")
	// ErrRoomGroupHasRooms is returned when trying to delete a group that still contains rooms.
	ErrRoomGroupHasRooms = errors.New("room group has rooms; move them out before deleting")
	// ErrRoomGroupNameEmpty is returned when a group name is empty or whitespace.
	ErrRoomGroupNameEmpty = errors.New("room group name must not be empty")
	// ErrRoomGroupOrderMismatch is returned when an order list doesn't match the
	// current set of groups (extras, duplicates, or missing IDs).
	ErrRoomGroupOrderMismatch = errors.New("room group order must be a permutation of existing groups")
)

// roomGroupKey returns the KV key for a single group document.
func roomGroupKey(groupID string) string {
	return "room_group." + groupID
}

// roomGroupKeyPrefix is the prefix used by ListKeysFiltered to enumerate
// every group document.
const roomGroupKeyPrefix = "room_group."

// CreateRoomGroup writes a new group document and appends its ID to the
// layout. Name is trimmed; description may be empty.
func (c *ChattoCore) CreateRoomGroup(ctx context.Context, actorID, name, description string) (*corev1.RoomGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrRoomGroupNameEmpty
	}

	group := &corev1.RoomGroup{
		Id:          NewRoomGroupID(),
		Name:        name,
		Description: description,
	}
	if err := c.writeRoomGroup(ctx, group, 0); err != nil {
		return nil, fmt.Errorf("write group doc: %w", err)
	}

	if err := c.mutateRoomLayoutOrder(ctx, func(order []string) ([]string, error) {
		if slices.Contains(order, group.Id) {
			return order, nil // defensive — shouldn't happen with fresh NanoID
		}
		return append(order, group.Id), nil
	}); err != nil {
		// Best-effort cleanup of the orphan doc on layout-write failure.
		_ = c.deleteRoomGroupDoc(ctx, group.Id)
		return nil, fmt.Errorf("append to layout: %w", err)
	}

	// New groups start with no explicit grants. Channel-room permissions
	// resolve via the server-tier cascade until an operator adds an
	// override here. See ADR-031 for the inheritance story; the seeder
	// `SeedDefaultRoomGroupPermissions` is still available to admin tools
	// that want to materialise the defaults explicitly into a group.

	c.logger.Info("Created room group", "group_id", group.Id, "name", name, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "create_group")
	return group, nil
}

// UpdateRoomGroup changes a group's name and/or description. The
// layout's order is untouched; only the per-group doc is rewritten.
func (c *ChattoCore) UpdateRoomGroup(ctx context.Context, actorID, groupID, name, description string) (*corev1.RoomGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrRoomGroupNameEmpty
	}

	var updated *corev1.RoomGroup
	if err := c.mutateRoomGroup(ctx, groupID, func(g *corev1.RoomGroup) error {
		g.Name = name
		g.Description = description
		updated = g
		return nil
	}); err != nil {
		return nil, err
	}

	c.logger.Info("Updated room group", "group_id", groupID, "name", name, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "update_group")
	return updated, nil
}

// GetRoomGroup reads a single group document, or ErrRoomGroupNotFound if
// the doc doesn't exist. Does NOT consult the layout — a group can exist
// as a doc without being in the layout's order (the reconciler will pick
// it up as an orphan on the next list call).
func (c *ChattoCore) GetRoomGroup(ctx context.Context, groupID string) (*corev1.RoomGroup, error) {
	g, err := c.loadRoomGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return g, nil
}

// DeleteRoomGroup removes a group's doc and its entry from the layout.
// Fails with ErrRoomGroupHasRooms if the group still contains any rooms —
// the operator must move them out first. There is no cascade.
func (c *ChattoCore) DeleteRoomGroup(ctx context.Context, actorID, groupID string) error {
	g, err := c.loadRoomGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if len(g.RoomIds) > 0 {
		return ErrRoomGroupHasRooms
	}

	if err := c.mutateRoomLayoutOrder(ctx, func(order []string) ([]string, error) {
		return slices.DeleteFunc(order, func(id string) bool { return id == groupID }), nil
	}); err != nil {
		return fmt.Errorf("remove from layout: %w", err)
	}
	if err := c.deleteRoomGroupDoc(ctx, groupID); err != nil {
		return fmt.Errorf("delete group doc: %w", err)
	}

	c.logger.Info("Deleted room group", "group_id", groupID, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "delete_group")
	return nil
}

// MoveRoomToGroup moves a room into the target group, removing it from
// any other group it was previously in. The room is appended to the end
// of the target group's room list. Touches at most two group documents
// (source and target); the layout's order is not modified.
//
// Authorization for the source and target groups must be checked by
// the caller — see ADR-031's two-group rule.
func (c *ChattoCore) MoveRoomToGroup(ctx context.Context, actorID, roomID, targetGroupID string) error {
	groups, err := c.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		return err
	}

	var sourceGroupID string
	for _, g := range groups {
		if slices.Contains(g.RoomIds, roomID) {
			sourceGroupID = g.Id
			break
		}
	}

	if sourceGroupID == targetGroupID {
		// Already in the target group; idempotent no-op.
		return nil
	}

	if sourceGroupID != "" {
		if err := c.mutateRoomGroup(ctx, sourceGroupID, func(g *corev1.RoomGroup) error {
			g.RoomIds = slices.DeleteFunc(g.RoomIds, func(id string) bool { return id == roomID })
			return nil
		}); err != nil {
			return fmt.Errorf("remove from source group %s: %w", sourceGroupID, err)
		}
	}

	if err := c.mutateRoomGroup(ctx, targetGroupID, func(g *corev1.RoomGroup) error {
		if slices.Contains(g.RoomIds, roomID) {
			return nil // defensive — already present
		}
		g.RoomIds = append(g.RoomIds, roomID)
		return nil
	}); err != nil {
		return fmt.Errorf("append to target group %s: %w", targetGroupID, err)
	}

	c.logger.Info("Moved room to group", "room_id", roomID, "group_id", targetGroupID, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "move_room")
	return nil
}

// ReorderRoomGroups validates that orderedGroupIDs is a permutation of
// the current set of group documents and writes the new ordering to the
// layout. Rejects with ErrRoomGroupOrderMismatch on extras, duplicates,
// or missing IDs. Group documents are untouched — only the layout's
// `group_ids` field changes.
func (c *ChattoCore) ReorderRoomGroups(ctx context.Context, actorID string, orderedGroupIDs []string) error {
	docs, err := c.listAllRoomGroupDocs(ctx)
	if err != nil {
		return err
	}

	if len(orderedGroupIDs) != len(docs) {
		return ErrRoomGroupOrderMismatch
	}
	seen := make(map[string]struct{}, len(orderedGroupIDs))
	for _, id := range orderedGroupIDs {
		if _, dup := seen[id]; dup {
			return ErrRoomGroupOrderMismatch
		}
		if _, ok := docs[id]; !ok {
			return ErrRoomGroupOrderMismatch
		}
		seen[id] = struct{}{}
	}

	if err := c.mutateRoomLayoutOrder(ctx, func(_ []string) ([]string, error) {
		return slices.Clone(orderedGroupIDs), nil
	}); err != nil {
		return err
	}

	c.logger.Info("Reordered room groups", "order", orderedGroupIDs, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "reorder_groups")
	return nil
}

// ReorderRoomsInGroup writes a new room ordering inside a single group.
// `orderedRoomIDs` must be a permutation of the group's current
// `room_ids` — extras, duplicates, or missing IDs return
// ErrRoomGroupOrderMismatch. The layout's group order is untouched.
//
// Cross-group moves go through MoveRoomToGroup; this method exists for
// intra-group drag-reorder where the membership set doesn't change.
func (c *ChattoCore) ReorderRoomsInGroup(ctx context.Context, actorID, groupID string, orderedRoomIDs []string) error {
	if err := c.mutateRoomGroup(ctx, groupID, func(g *corev1.RoomGroup) error {
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
		g.RoomIds = slices.Clone(orderedRoomIDs)
		return nil
	}); err != nil {
		return err
	}

	c.logger.Info("Reordered rooms in group", "group_id", groupID, "actor_id", actorID)
	c.notifyRoomLayoutChanged(ctx, actorID, "reorder_rooms_in_group")
	return nil
}

// ListRoomGroupsOrdered is the canonical reconciled accessor. It returns
// the layout-ordered list of groups, with stale layout entries dropped
// and orphan docs appended at the end (by NanoID, which is roughly
// creation order). Runs the legacy-shape migrator on first call after
// an upgrade.
//
// `kind` is preserved for symmetry with other room APIs; only KindChannel
// participates in the layout today.
func (c *ChattoCore) ListRoomGroupsOrdered(ctx context.Context, kind RoomKind) ([]*corev1.RoomGroup, error) {
	if kind != KindChannel {
		return nil, nil
	}

	// Trigger migration before reading docs — the migrator writes
	// per-key group docs from legacy_sections, so we need it to run
	// first or the docs map will miss freshly-migrated groups.
	order, err := c.GetRoomLayoutOrder(ctx)
	if err != nil {
		return nil, err
	}

	docs, err := c.listAllRoomGroupDocs(ctx)
	if err != nil {
		return nil, err
	}

	// Reconcile: walk the layout's order, picking up docs that exist;
	// then append any docs missing from the order (orphan recovery).
	out := make([]*corev1.RoomGroup, 0, len(docs))
	used := make(map[string]struct{}, len(order))
	for _, id := range order {
		if _, dup := used[id]; dup {
			continue
		}
		g, ok := docs[id]
		if !ok {
			continue // stale reference — drop
		}
		out = append(out, g)
		used[id] = struct{}{}
	}

	// Orphans, sorted by ID for determinism.
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

// GetRoomLayoutOrder returns the raw `group_ids` ordering from the
// layout doc, running the legacy-shape migrator if needed. May contain
// stale references (IDs without matching docs); use
// ListRoomGroupsOrdered for the reconciled view.
func (c *ChattoCore) GetRoomLayoutOrder(ctx context.Context) ([]string, error) {
	layout, err := c.readAndMaybeMigrateLayout(ctx)
	if err != nil {
		return nil, err
	}
	if layout == nil {
		return nil, nil
	}
	return slices.Clone(layout.GroupIds), nil
}

// ----------------------------------------------------------------------
// Per-document KV ops
// ----------------------------------------------------------------------

func (c *ChattoCore) loadRoomGroup(ctx context.Context, groupID string) (*corev1.RoomGroup, error) {
	bucket := c.storage.serverConfigKV
	entry, err := bucket.Get(ctx, roomGroupKey(groupID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrRoomGroupNotFound
		}
		return nil, fmt.Errorf("get group %s: %w", groupID, err)
	}
	g := &corev1.RoomGroup{}
	if err := proto.Unmarshal(entry.Value(), g); err != nil {
		return nil, fmt.Errorf("unmarshal group %s: %w", groupID, err)
	}
	return g, nil
}

// writeRoomGroup writes a group document. Pass revision=0 to create
// (fails with ErrKeyExists if already present); pass a non-zero
// revision to update (fails with ErrKeyExists / ErrWrongLastSequence
// on conflict).
func (c *ChattoCore) writeRoomGroup(ctx context.Context, g *corev1.RoomGroup, revision uint64) error {
	bucket := c.storage.serverConfigKV
	data, err := proto.Marshal(g)
	if err != nil {
		return fmt.Errorf("marshal group %s: %w", g.Id, err)
	}
	if revision == 0 {
		_, err = bucket.Create(ctx, roomGroupKey(g.Id), data)
	} else {
		_, err = bucket.Update(ctx, roomGroupKey(g.Id), data, revision)
	}
	return err
}

func (c *ChattoCore) deleteRoomGroupDoc(ctx context.Context, groupID string) error {
	return c.storage.serverConfigKV.Delete(ctx, roomGroupKey(groupID))
}

// mutateRoomGroup reads, mutates, and rewrites one group document under
// OCC. Returns ErrRoomGroupNotFound if the doc doesn't exist.
func (c *ChattoCore) mutateRoomGroup(ctx context.Context, groupID string, mutate func(*corev1.RoomGroup) error) error {
	bucket := c.storage.serverConfigKV
	for attempt := 0; attempt < maxLayoutRetries; attempt++ {
		entry, err := bucket.Get(ctx, roomGroupKey(groupID))
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				return ErrRoomGroupNotFound
			}
			return fmt.Errorf("get group %s: %w", groupID, err)
		}
		g := &corev1.RoomGroup{}
		if err := proto.Unmarshal(entry.Value(), g); err != nil {
			return fmt.Errorf("unmarshal group %s: %w", groupID, err)
		}
		if err := mutate(g); err != nil {
			return err
		}
		if err := c.writeRoomGroup(ctx, g, entry.Revision()); err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				continue // OCC conflict, retry
			}
			return fmt.Errorf("write group %s: %w", groupID, err)
		}
		return nil
	}
	return ErrConfigConflict
}

func (c *ChattoCore) listAllRoomGroupDocs(ctx context.Context) (map[string]*corev1.RoomGroup, error) {
	bucket := c.storage.serverConfigKV
	keyLister, err := bucket.ListKeysFiltered(ctx, roomGroupKeyPrefix+"*")
	if err != nil {
		return nil, fmt.Errorf("list room_group keys: %w", err)
	}
	out := make(map[string]*corev1.RoomGroup)
	for key := range keyLister.Keys() {
		entry, err := bucket.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, fmt.Errorf("get %s: %w", key, err)
		}
		g := &corev1.RoomGroup{}
		if err := proto.Unmarshal(entry.Value(), g); err != nil {
			c.logger.Warn("skipping unparseable group doc", "key", key, "error", err)
			continue
		}
		if g.Id == "" {
			c.logger.Warn("skipping group doc with empty ID", "key", key)
			continue
		}
		out[g.Id] = g
	}
	return out, nil
}

// ----------------------------------------------------------------------
// Layout doc (ordering only) + legacy-shape migration
// ----------------------------------------------------------------------

// mutateRoomLayoutOrder runs the mutator over the layout's `group_ids`
// list with OCC retries. The mutator should treat the input as
// immutable and return the new order.
func (c *ChattoCore) mutateRoomLayoutOrder(ctx context.Context, mutate func([]string) ([]string, error)) error {
	bucket := c.storage.serverConfigKV
	for attempt := 0; attempt < maxLayoutRetries; attempt++ {
		layout, revision, err := c.readLayoutWithRevision(ctx)
		if err != nil {
			return err
		}

		newOrder, err := mutate(layout.GroupIds)
		if err != nil {
			return err
		}
		layout.GroupIds = newOrder

		data, err := proto.Marshal(layout)
		if err != nil {
			return fmt.Errorf("marshal layout: %w", err)
		}
		var writeErr error
		if revision == 0 {
			_, writeErr = bucket.Create(ctx, roomLayoutKey, data)
		} else {
			_, writeErr = bucket.Update(ctx, roomLayoutKey, data, revision)
		}
		if writeErr == nil {
			return nil
		}
		if errors.Is(writeErr, jetstream.ErrKeyExists) {
			continue // OCC conflict, retry
		}
		return fmt.Errorf("write layout: %w", writeErr)
	}
	return ErrConfigConflict
}

// readAndMaybeMigrateLayout reads the layout doc and, if it still
// carries legacy fields (`legacy_sections` or `legacy_unsorted_room_ids`),
// drains them into per-group documents + a fresh `group_ids` list. The
// migrator is idempotent; subsequent calls find no legacy fields and
// short-circuit.
func (c *ChattoCore) readAndMaybeMigrateLayout(ctx context.Context) (*corev1.RoomLayout, error) {
	layout, _, err := c.readLayoutWithRevision(ctx)
	if err != nil {
		return nil, err
	}
	if len(layout.LegacySections) == 0 && len(layout.LegacyUnsortedRoomIds) == 0 {
		return layout, nil
	}
	// Run migration; the helper rewrites the layout in place via OCC.
	if err := c.migrateLegacyRoomLayout(ctx); err != nil {
		return nil, fmt.Errorf("migrate legacy layout: %w", err)
	}
	// Re-read so the caller sees the post-migration state.
	layout, _, err = c.readLayoutWithRevision(ctx)
	if err != nil {
		return nil, err
	}
	return layout, nil
}

// readLayoutWithRevision reads the layout key and returns its current
// state plus its revision (0 if absent). Never returns nil — an absent
// key yields an empty RoomLayout, so callers can always populate fields.
func (c *ChattoCore) readLayoutWithRevision(ctx context.Context) (*corev1.RoomLayout, uint64, error) {
	entry, err := c.storage.serverConfigKV.Get(ctx, roomLayoutKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return &corev1.RoomLayout{}, 0, nil
		}
		return nil, 0, fmt.Errorf("get layout: %w", err)
	}
	layout := &corev1.RoomLayout{}
	if err := proto.Unmarshal(entry.Value(), layout); err != nil {
		return nil, 0, fmt.Errorf("unmarshal layout: %w", err)
	}
	return layout, entry.Revision(), nil
}

// migrateLegacyRoomLayout drains `legacy_sections` and
// `legacy_unsorted_room_ids` into per-group documents + a `group_ids`
// list. Idempotent: short-circuits if the legacy fields are empty.
//
// Behaviour:
//   - Each legacy section becomes a `room_group.{id}` document (unless
//     a doc with that ID already exists from a prior migration attempt).
//   - The new layout's `group_ids` mirrors the legacy section order.
//   - `legacy_unsorted_room_ids` are absorbed into the first group's
//     room_ids. If there are no legacy sections, a seed "Lobby" group is
//     created to hold them.
//   - Legacy fields are cleared on the final layout write.
func (c *ChattoCore) migrateLegacyRoomLayout(ctx context.Context) error {
	bucket := c.storage.serverConfigKV
	for attempt := 0; attempt < maxLayoutRetries; attempt++ {
		layout, revision, err := c.readLayoutWithRevision(ctx)
		if err != nil {
			return err
		}
		if len(layout.LegacySections) == 0 && len(layout.LegacyUnsortedRoomIds) == 0 {
			return nil // already migrated, or never had legacy fields
		}

		// Build the new state.
		newOrder := make([]string, 0, len(layout.LegacySections))
		for _, sec := range layout.LegacySections {
			if sec.Id == "" {
				c.logger.Warn("skipping legacy section with empty ID")
				continue
			}
			// Skip if a doc already exists (e.g., partial prior migration).
			existing, loadErr := c.loadRoomGroup(ctx, sec.Id)
			if loadErr != nil && !errors.Is(loadErr, ErrRoomGroupNotFound) {
				return fmt.Errorf("check existing group %s: %w", sec.Id, loadErr)
			}
			if existing == nil {
				g := &corev1.RoomGroup{
					Id:      sec.Id,
					Name:    sec.Name,
					RoomIds: slices.Clone(sec.RoomIds),
				}
				if err := c.writeRoomGroup(ctx, g, 0); err != nil && !errors.Is(err, jetstream.ErrKeyExists) {
					return fmt.Errorf("write migrated group %s: %w", sec.Id, err)
				}
			}
			newOrder = append(newOrder, sec.Id)
		}

		// Absorb unsorted rooms into the first group (creating one if needed).
		if len(layout.LegacyUnsortedRoomIds) > 0 {
			var targetGroupID string
			if len(newOrder) > 0 {
				targetGroupID = newOrder[0]
			} else {
				seed := &corev1.RoomGroup{
					Id:   NewRoomGroupID(),
					Name: SeedDefaultRoomGroupName,
				}
				if err := c.writeRoomGroup(ctx, seed, 0); err != nil {
					return fmt.Errorf("seed group for unsorted rooms: %w", err)
				}
				targetGroupID = seed.Id
				newOrder = append(newOrder, seed.Id)
			}
			if err := c.mutateRoomGroup(ctx, targetGroupID, func(g *corev1.RoomGroup) error {
				for _, rid := range layout.LegacyUnsortedRoomIds {
					if !slices.Contains(g.RoomIds, rid) {
						g.RoomIds = append(g.RoomIds, rid)
					}
				}
				return nil
			}); err != nil {
				return fmt.Errorf("absorb unsorted rooms into %s: %w", targetGroupID, err)
			}
		}

		// Write the layout with the new order and cleared legacy fields.
		migrated := &corev1.RoomLayout{GroupIds: newOrder}
		data, err := proto.Marshal(migrated)
		if err != nil {
			return fmt.Errorf("marshal migrated layout: %w", err)
		}
		var writeErr error
		if revision == 0 {
			_, writeErr = bucket.Create(ctx, roomLayoutKey, data)
		} else {
			_, writeErr = bucket.Update(ctx, roomLayoutKey, data, revision)
		}
		if writeErr == nil {
			c.logger.Info("Migrated legacy room layout",
				"sections", len(layout.LegacySections),
				"unsorted", len(layout.LegacyUnsortedRoomIds),
				"groups", len(newOrder),
			)
			return nil
		}
		if errors.Is(writeErr, jetstream.ErrKeyExists) {
			continue // OCC conflict; retry the whole read-mutate-write cycle
		}
		return fmt.Errorf("write migrated layout: %w", writeErr)
	}
	return ErrConfigConflict
}

// notifyRoomLayoutChanged is the central place every room-layout mutator
// (create/update/delete/reorder a group, move a room between groups,
// create/delete a room) calls to nudge connected clients. It wraps
// PublishRoomGroupsUpdated with best-effort error logging so a callsite
// failure to deliver doesn't roll back the storage mutation that
// preceded it. `reason` is purely for log forensics.
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
// Behaviour:
//   - Runs the legacy-shape migrator if the layout still has legacy
//     fields populated (transparent on subsequent boots).
//   - Creates the seed "Lobby" group if no groups exist.
//   - Every channel room not currently in any group is appended to the
//     first group in the layout. The room's GroupId proto field is
//     stamped to match so resolvers can rely on it.
//
// Authorization: internal-only — runs as SystemActorID for mutations.
func (c *ChattoCore) ensureChannelRoomsAreInAGroup(ctx context.Context) error {
	// Trigger migration up front so the rest of the flow operates on the
	// reconciled view.
	if _, err := c.readAndMaybeMigrateLayout(ctx); err != nil {
		return fmt.Errorf("ensure migrated layout: %w", err)
	}

	rooms, err := c.ListRooms(ctx, KindChannel)
	if err != nil {
		return fmt.Errorf("list channel rooms: %w", err)
	}
	groups, err := c.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		return fmt.Errorf("list room groups: %w", err)
	}

	// Build "room → group" map from current group docs.
	roomToGroup := make(map[string]string, len(rooms))
	for _, g := range groups {
		for _, rid := range g.RoomIds {
			roomToGroup[rid] = g.Id
		}
	}

	// Identify rooms that aren't in any group.
	var unassigned []string
	for _, r := range rooms {
		if _, ok := roomToGroup[r.Id]; !ok {
			unassigned = append(unassigned, r.Id)
		}
	}

	if len(unassigned) > 0 || len(groups) == 0 {
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
			roomToGroup[rid] = targetGroupID
		}
	}

	// Stamp Room.GroupId for rooms whose proto field doesn't match.
	for _, r := range rooms {
		want := roomToGroup[r.Id]
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
