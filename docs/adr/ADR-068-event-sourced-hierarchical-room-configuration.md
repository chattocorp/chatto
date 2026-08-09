# ADR-068: Event-Sourced Hierarchical Room Configuration

**Date:** 2026-08-08

## Context

Chatto needs typed settings that administrators can change while a server is
running. The first settings govern room behaviour: the message author edit
window is the proof of concept, while Slow Mode and threading preferences are
expected to follow. Their effective values can vary by server, room group, and
room.

These settings are neither permissions nor deployment configuration.
Permissions answer whether a viewer may perform an action. Deployment
configuration in `chatto.toml` and the environment controls process wiring.
Runtime configuration selects typed product behaviour without a restart.

Adding one durable event type and one stored record shape per setting would
make the event envelope, projection, replay logic, and consumers grow
unnecessarily. A single untyped property bag would avoid that growth but lose
schema validation, generated clients, compatibility analysis, and discoverable
contracts.

The system may eventually need role- and user-specific values at the same
hierarchy tiers. Resolving several applicable subjects requires a separately
designed combination rule, so the first version must remain extensible without
persisting unused subject concepts.

## Decision

Runtime configuration is divided by the type of resource whose behaviour is
configured. Room behaviour uses a room-specific family:

- `RoomConfig` is the fully resolved configuration governing a room.
- `RoomConfigLayer` is the sparse set of values contributed at one inheritance
  scope.
- `RoomConfigChangedEvent` records a typed patch to one layer.

Future configurable resource types receive their own typed families rather
than sharing a universal runtime-configuration property bag.

Chatto stores room-configuration changes as durable facts in `EVT` on the
existing `evt.config.*` namespace. The configuration projection retains sparse
layers and resolves inherited values at read and command boundaries; it does
not persist materialised effective configuration.

Each setting has a compile-time product default and a typed value. Time
intervals use `google.protobuf.Duration` at protobuf boundaries rather than a
unit-bearing scalar field. Room configuration resolves independently per field
in this order:

1. room layer;
2. the room's current room-group layer;
3. server layer; and
4. product default.

The nearest layer containing a field supplies its value. Parent values are
defaults, not constraints: a child may select any value accepted by the
setting's global validation rules. Moving a room between groups immediately
changes inheritance because resolution follows the current room-group
relationship rather than a copied value. Direct messages skip room and group
layers and resolve from the server layer and product defaults.

`RoomConfigChangedEvent` carries a scope, a typed `RoomConfigLayer`, and a
`FieldMask`. For a selected path, a present value sets the field and an absent
value removes it from the layer; unselected fields remain unchanged. Replay
code ignores unknown future paths. This supports atomic multi-field updates
and lets older writers change known fields without erasing newer ones.

Writers use optimistic concurrency on the affected configuration aggregate and
retry from projected state. Configuration changes that affect
authorization-adjacent command checks advance the existing authorization fence
atomically, allowing a command to prove it used configuration and authorization
state no older than its commit.

Administrative APIs expose the stored layer and effective configuration. The
layer's field presence tells an administrator whether a value is set at the
selected scope or inherited; the API does not maintain a parallel per-field
provenance tree. Public room resources expose only the curated effective values
required for ordinary client behaviour as room viewer state. Persisted or
administrative configuration fields do not automatically become public.
Realtime delivery uses an explicit allow-list of public field-mask paths;
private-only changes neither produce public operations nor force client
reconnects.

Room viewer state is resolved for the current user rather than shared room
metadata: future role- or user-specific layers may make it viewer-dependent,
and realtime can replace it independently. Realtime delivery replaces one
room's viewer state for room-scoped changes. Server- and room-group-scoped
changes request a compacted reconnect instead of expanding into an unbounded
set of per-room updates.

Role- and user-specific layers are deferred. They would use the same server,
room-group, and room tiers, with a subject selector orthogonal to the scope.
They must use a new subject-specific event variant rather than adding a selector
to `RoomConfigChangedEvent`: an older reader would otherwise ignore the new
selector while applying the recognized patch globally to the baseline layer.
The new event can share `RoomConfigLayer` and field-mask semantics once
precedence between the baseline, user, and possibly multiple applicable role
layers has been decided.

Deleting a room or room group records removal of the corresponding layer while
retaining its durable history in `EVT`.

## Consequences

- Configuration changes are auditable, replayable, included in normal EVT
  backup, and safe across multiple replicas without a new KV bucket.
- Adding a room setting extends `RoomConfig` and `RoomConfigLayer` and their
  validation, behavior, documentation, and tests, but does not add another EVT
  envelope variant.
- Different configurable resource types remain independently typed and cannot
  turn one universal message into a miscellaneous settings bucket.
- The administrative API reports whether the selected layer contributes a
  field and what value is effective, but not the exact ancestor supplying an
  inherited value. Exact provenance can be added only if a concrete operator
  workflow justifies the parallel metadata.
- Client-visible room configuration is an explicit subset. Internal or
  administrative settings can remain private by omitting them from the public
  resolved message and its realtime field allow-list.
- Effective reads depend on current room-group topology as well as the
  configuration projection. Commands whose correctness depends on room
  configuration include both state boundaries in their OCC retry.
- A parent cannot cap or forbid child values. Organisational constraints would
  require distinct, explicitly modelled semantics.
- Adding subject-specific layers later remains wire-additive through a new
  event variant, but requires an ADR amendment defining combination behavior.
- A downgraded server safely ignores the additive event variant and applies its
  compiled behaviour until upgraded again. Operators should avoid changing
  room configuration during a downgrade because the older process cannot
  enforce or display it.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-034](ADR-034-single-event-stream.md)
- [ADR-040](ADR-040-permission-only-rbac-with-owner-override.md)
- [ADR-045](ADR-045-public-api-stability-tiers.md)
- [ADR-051](ADR-051-server-scoped-resumable-client-projection.md)
- [FDR-035](../fdr/FDR-035-runtime-room-configuration.md)
