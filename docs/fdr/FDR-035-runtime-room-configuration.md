# FDR-035: Runtime Room Configuration

**Status:** Experimental
**Last reviewed:** 2026-08-08

## Overview

Runtime room configuration lets administrators change typed room behaviour
without restarting the server. Sparse configuration layers can be set at the
server, room-group, or room scope. Unset values inherit from the nearest parent
and ultimately from a stable product default.

The first setting is the message author edit window. Slow Mode and threading
preferences are future room settings and are not defined by this record.

## Behavior

- Every room setting has a documented type, product default, valid range,
  applicable scopes, authorization, client behavior, and change-over-time
  semantics.
- Administrators may contribute a value at a scope or remove it from that
  layer to resume inheritance.
- Administrative updates name the fields they change; fields unknown to an
  older client remain untouched.
- Effective values resolve room → current room group → server → product
  default. Direct messages resolve server → product default.
- Parent values provide defaults, not constraints. Each contributed value is
  validated against the setting's global bounds.
- Administrative UI shows the value contributed by the current scope and the
  effective value. An absent local value is shown as inherited without tracing
  the exact ancestor that supplied it.
- Effective room configuration travels with room viewer state so normal
  clients receive configuration resolved for the current viewer and can render
  expected behavior. Enforcement remains server-side. Changes to fields that
  are not part of the public room configuration do not produce ordinary-client
  realtime traffic.
- Role- and user-specific layers are not supported initially; their precedence
  and combination behavior remain deliberately deferred.

## Design Decisions

### 1. Typed room configuration, not a generic property bag

**Decision:** Room behavior is represented by explicit fields in `RoomConfig`
and `RoomConfigLayer`.
**Why:** Values need setting-specific validation, API documentation, frontend
controls, compatibility behavior, and enforcement. Arbitrary names and values
move those contracts into runtime convention and make malformed or abandoned
configuration easy to persist.
**Tradeoff:** Adding a setting requires code generation and deliberate work
across layers. That ceremony is useful because each setting changes observable
product behavior.

### 2. One change event for the room configuration resource

**Decision:** A typed field-mask patch event changes selected fields in one
room-configuration layer.
**Why:** The event envelope and every consumer remain bounded as settings are
added, while typed fields preserve validation and generated contracts. Field
masks distinguish setting a field, removing it, and leaving it untouched.
**Tradeoff:** Replay must interpret mask paths and ignore fields introduced by
newer versions.

### 3. Sparse layers with nearest-scope inheritance

**Decision:** Store only values contributed at a scope and resolve each field
independently from room through room group and server to its product default.
**Why:** Removing a value has an obvious meaning, parent changes propagate, and
moving a room between groups immediately adopts the new group's defaults.
**Tradeoff:** Reads combine configuration and current room topology; a single
stored layer does not contain the complete effective configuration.

### 4. Parent values are defaults

**Decision:** A child layer replaces its parent's value rather than being
bounded by it.
**Why:** The common administrative intent is “use this broadly, except here.”
Constraint inheritance would require separate minimum/maximum concepts and
could make a valid-looking child value ineffective.
**Tradeoff:** A server administrator cannot use a parent value to prohibit a
room manager from choosing another globally valid value. Constraints can be
added as a distinct feature if a concrete need appears.

### 5. Effective values are public; layers are administrative

**Decision:** Normal room resources expose a curated effective configuration
required for client behavior as part of room viewer state. The Admin Room
Configuration API exposes the stored layer and effective values, but not a
parallel per-field provenance structure.
**Why:** Clients should not reproduce inheritance or fetch administrative data,
while administrators need to distinguish a local value from inheritance.
Exact ancestor labels do not affect behavior and would double the resolved
schema as settings grow. Viewer state is the resolved behavioral view for one
user: future role- or user-specific layers may make the value viewer-dependent,
and realtime can replace it without replacing shared room metadata.
**Tradeoff:** The UI says that a value is inherited without identifying the
specific ancestor. Adding a client-relevant setting can extend both public and
admin protobufs; private settings remain outside the public resolved shape and
the explicit realtime field allow-list.

### 6. Changes apply to current behavior

**Decision:** Unless a setting explicitly documents otherwise, a command uses
the effective value at commit time, not the value that existed when the
affected resource was created.
**Why:** Runtime settings should take effect without migrations or copied
historical state. The server rechecks configuration-dependent eligibility
inside its optimistic-concurrency attempt.
**Tradeoff:** Lowering a limit can immediately disallow an action; raising it
can make an action available again. Each setting's UI and FDR must call this
out.

### 7. Subject-specific layers remain additive and deferred

**Decision:** The first storage shape contains only baseline layers. Role and
user layers would use the same hierarchy tiers and typed layer, but a new event
variant so older binaries cannot mistake a subject-specific patch for a
baseline patch.
**Why:** This preserves a feasible path without carrying unused schema or
prematurely choosing how direct-user and multiple-role values combine.
**Tradeoff:** Adding subjects later requires new resolution logic and a clear
compatibility plan.

## Permissions

- `server.manage` — view and change the server room-configuration layer.
- `room.manage` — view and change a room-group or room layer at a scope the
  caller can manage.

## Related

- **ADRs:** ADR-068 (Event-Sourced Hierarchical Room Configuration)
- **FDRs:** FDR-004 (Message Editing & Deletion), FDR-017 (Room Groups & Sidebar
  Layout), FDR-020 (Server Branding & Configuration), FDR-021 (Admin Dashboard
  & System Monitoring)
