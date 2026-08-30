# ADR-086: Commit Room-Layout Structural Mutations Atomically

**Status:** Accepted
**Date:** 2026-08-30

## Context

Room groups, room membership in groups, and sidebar ordering are durable EVT
facts. However, some commands still commit related facts in separate steps.
Room creation commits `RoomCreatedEvent` before it adds the room to a group.
Room deletion commits `RoomDeletedEvent` before it removes the room from its
group. Room-group creation and deletion also update the singleton layout in a
follow-up step.

These follow-up writes are best-effort. A process failure or a permanent write
error between the steps can leave an incomplete structural state. Read-model
reconciliation hides some of these states, but reconciliation does not make the
missing durable fact part of EVT history. This conflicts with the invariant
that each channel room belongs to exactly one room group.

The existing event vocabulary already records the required facts. A new event
type or aggregate subject is not necessary to close this consistency gap.

## Decision

Commit each structural command as one atomic EVT batch:

- Channel-room creation commits `RoomCreatedEvent` and
  `RoomAddedToGroupEvent` together. Creation-time permission facts stay in the
  same batch.
- Channel-room deletion commits `RoomDeletedEvent` and
  `RoomRemovedFromGroupEvent` together.
- Room-group creation commits `RoomGroupCreatedEvent` and the resulting
  `RoomGroupsReorderedEvent` together.
- Room-group deletion commits `RoomGroupDeletedEvent` and the resulting
  `RoomGroupsReorderedEvent` together.

Each batch uses OCC for every projected state boundary used to build it. Room
name claims use the `evt.room.>` tail. Group membership and lifecycle use the
`evt.group.>` tail. Global group order uses the `evt.layout.>` tail. Commands
through user-authorized group and layout entry points also guard the
authorization fence and repeat their decision after an OCC conflict.

Room moves guard the exact room-deletion subject in addition to the group
membership tail. This prevents a move that loses a race with deletion from
adding the deleted room to another group. Legacy unassigned-room repair uses a
whole-stream guard when one group event cannot carry both narrow boundaries.

The writer recomputes the complete batch from the original command intent on
each retry. It does not retry a precomputed ordering or membership change.
After a successful commit, it waits for each projection used by the response or
the next read.

Historical events and subjects remain valid. Projections continue to replay
the existing event families without a migration. This decision does not choose
a new aggregate for sidebar placement. A later change can evaluate a canonical
placement fact separately, with its own mixed-version and rollback plan.

## Consequences

- A successful channel-room creation cannot leave the room without its initial
  room-group membership.
- A successful room or room-group deletion cannot leave a stale structural
  reference that depends on reconciliation for correctness.
- Group creation and deletion always have a matching authoritative group order.
- Structural commands have more OCC inputs and can retry when an unrelated
  mutation advances one of those inputs.
- The change adds no durable protobuf type, subject, stream, or data migration.
- The current group and layout aggregate split remains in place. This ADR fixes
  its atomicity but does not claim that the split is the final domain model.

## Related

- [ADR-031](ADR-031-room-group-centric-acl.md)
- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-034](ADR-034-single-event-stream.md)
- [ADR-068](ADR-068-selectable-event-mutation-consistency-boundaries.md)
