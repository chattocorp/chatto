# FDR-035: Runtime Policies

**Status:** Experimental
**Last reviewed:** 2026-08-08

## Overview

Runtime policies are typed, administrator-controlled settings that change
Chatto's product behavior without restarting the server. A policy may be set at
server, room-group, or room scope. Unset values inherit from the nearest parent
and ultimately from a stable product default.

The first policy is the message author edit window. Slow Mode and thread-first
room behavior are future policies and are not defined by this record.

## Behavior

- Every policy has a documented type, product default, valid range, applicable
  scopes, authorization, client behavior, and change-over-time semantics.
- Administrators may store a sparse override or clear it to resume inheritance.
- Administrative updates name the fields they change; fields unknown to an
  older client remain untouched.
- Effective baseline values resolve room → current room group → server →
  product default. Server values resolve server → product default.
- Parent values provide defaults, not constraints. Each stored value is
  validated against the policy's global bounds.
- Administrative UI shows whether the current scope inherits or overrides,
  the effective value, and the scope that supplies it.
- Effective room values travel with room viewer state so normal clients can
  render the behavior they should expect. Enforcement remains server-side.
- Direct messages inherit server policy values and do not accept room
  overrides.
- The storage target can represent baseline, role, and user subjects. Only
  baseline policy values are supported initially; role/user behavior is
  deliberately deferred.

## Design Decisions

### 1. Typed policy fields, not a generic property bag

**Decision:** Every runtime policy adds explicit protobuf fields and typed core
accessors.
**Why:** Values need policy-specific validation, API documentation, frontend
controls, compatibility behavior, and enforcement. Arbitrary names and values
move those contracts into runtime convention and make malformed or abandoned
configuration easy to persist.
**Tradeoff:** Adding a policy requires code generation and deliberate work
across layers. That ceremony is useful because each policy changes observable
product behavior.

### 2. Sparse overrides with nearest-scope inheritance

**Decision:** Store only explicit overrides and resolve each field independently
from room through room group and server to its product default.
**Why:** Clearing an override has an obvious meaning, parent changes propagate,
and moving a room between groups immediately adopts the new group's defaults.
**Tradeoff:** Reads must combine configuration and current room topology; a
single stored record does not contain the complete effective configuration.

### 3. Parent values are defaults

**Decision:** A child override replaces its parent's value rather than being
bounded by it.
**Why:** The common administrative intent is “use this broadly, except here.”
Constraint inheritance would require separate minimum/maximum concepts and
could make a valid-looking child setting ineffective.
**Tradeoff:** A server administrator cannot use a parent value to prohibit a
room manager from choosing another globally valid value. Constraints can be
added as a distinct policy feature if a concrete need appears.

### 4. Effective values are public; stored overrides are administrative

**Decision:** Normal room resources expose effective policy values needed for
client behavior. The Admin Policy API exposes stored overrides, effective
values, and their source.
**Why:** Clients should not reproduce inheritance or fetch administrative data,
while administrators need to understand what is stored and why a value wins.
**Tradeoff:** Adding a client-relevant policy can extend both public and admin
protobufs even though its durable event is already typed.

### 5. Changes apply to current behavior

**Decision:** Unless a policy explicitly documents otherwise, a command uses
the effective value at commit time, not the value that existed when the
affected resource was created.
**Why:** Runtime settings should take effect without migrations or copied
historical state. The server rechecks policy-dependent eligibility inside its
optimistic-concurrency attempt.
**Tradeoff:** Lowering a limit can immediately disallow an action; raising it
can make an action available again. Each policy's UI and FDR must call this out.

### 6. Subject-specific values are designed in but deferred

**Decision:** Persistence identifies a baseline, role, or user subject at the
same server/group/room tiers. Initial APIs reject non-baseline subjects.
**Why:** This preserves a feasible additive path without prematurely choosing
how direct-user and multiple-role values combine.
**Tradeoff:** The first implementation carries a small amount of unused schema
surface. It does not promise a release in which role/user policy overrides are
available.

## Permissions

- `server.manage` — view and change server policy configuration.
- `room.manage` — view and change room-group or room policy configuration at a
  scope the caller can manage.

## Related

- **ADR:** ADR-068 (Event-Sourced Hierarchical Runtime Policies)
- **FDRs:** FDR-004 (Message Editing & Deletion), FDR-017 (Room Groups & Sidebar
  Layout), FDR-020 (Server Branding & Configuration), FDR-021 (Admin Dashboard
  & System Monitoring)
